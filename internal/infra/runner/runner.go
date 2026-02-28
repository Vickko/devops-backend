package runner

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

// Runner manages Docker containers for sandboxed code execution.
// Operations are atomic: Create a container, optionally CopyTo files,
// Exec commands, CopyFrom artifacts, and Remove when done.
type Runner interface {
	Create(ctx context.Context, image string) (containerID string, err error)
	CopyTo(ctx context.Context, containerID string, files map[string][]byte) error
	Exec(ctx context.Context, containerID string, cmd []string) (stdout, stderr string, err error)
	CopyFrom(ctx context.Context, containerID, srcPath string) (map[string][]byte, error)
	Remove(ctx context.Context, containerID string) error
	Close() error
}

// ExecError indicates the executed command exited with a non-zero status.
type ExecError struct {
	ExitCode int
	Stderr   string
}

func (e *ExecError) Error() string {
	return fmt.Sprintf("command exited with code %d: %s", e.ExitCode, e.Stderr)
}

// limitedWriter caps the amount of data written to an internal buffer.
// Excess bytes are silently discarded.
type limitedWriter struct {
	buf bytes.Buffer
	max int
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if remaining := w.max - w.buf.Len(); remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		w.buf.Write(p)
	}
	return len(p), nil // always report full consumption to avoid writer errors
}

func (w *limitedWriter) String() string { return w.buf.String() }

type dockerRunner struct {
	cli    client.APIClient
	cfg    *Config
	logger *slog.Logger
}

// New creates a Runner backed by the local Docker daemon.
// It pings the daemon immediately to fail fast if unreachable.
func New(logger *slog.Logger, opts ...Option) (Runner, error) {
	cfg := defaultConfig()
	for _, o := range opts {
		o(cfg)
	}

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("create docker client: %w", err)
	}

	if _, err := cli.Ping(context.Background()); err != nil {
		cli.Close()
		return nil, fmt.Errorf("ping docker daemon: %w", err)
	}

	return &dockerRunner{cli: cli, cfg: cfg, logger: logger}, nil
}

func (r *dockerRunner) Create(ctx context.Context, img string) (string, error) {
	if err := r.pullImage(ctx, img); err != nil {
		return "", fmt.Errorf("pull image: %w", err)
	}

	pidsLimit := r.cfg.PidsLimit
	resp, err := r.cli.ContainerCreate(ctx, &container.Config{
		Image:      img,
		Cmd:        []string{"sleep", "infinity"},
		WorkingDir: r.cfg.WorkDir,
		Tty:        false,
	}, &container.HostConfig{
		Resources: container.Resources{
			Memory:    r.cfg.MemoryLimit,
			NanoCPUs:  r.cfg.NanoCPUs,
			PidsLimit: &pidsLimit,
		},
		NetworkMode: container.NetworkMode(r.cfg.NetworkMode),
		CapDrop:     r.cfg.CapDrop,
		SecurityOpt: r.cfg.SecurityOpt,
	}, nil, nil, "")
	if err != nil {
		return "", fmt.Errorf("create container: %w", err)
	}

	if err := r.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		_ = r.cli.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})
		return "", fmt.Errorf("start container: %w", err)
	}

	return resp.ID, nil
}

func (r *dockerRunner) CopyTo(ctx context.Context, containerID string, files map[string][]byte) error {
	if len(files) == 0 {
		return nil
	}
	tarReader, err := packTar(files)
	if err != nil {
		return fmt.Errorf("pack tar: %w", err)
	}
	return r.cli.CopyToContainer(ctx, containerID, r.cfg.WorkDir, tarReader, container.CopyToContainerOptions{})
}

func (r *dockerRunner) Exec(ctx context.Context, containerID string, cmd []string) (string, string, error) {
	if r.cfg.TaskTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.cfg.TaskTimeout)
		defer cancel()
	}

	execResp, err := r.cli.ContainerExecCreate(ctx, containerID, container.ExecOptions{
		Cmd:          cmd,
		WorkingDir:   r.cfg.WorkDir,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return "", "", fmt.Errorf("exec create: %w", err)
	}

	attachResp, err := r.cli.ContainerExecAttach(ctx, execResp.ID, container.ExecAttachOptions{})
	if err != nil {
		return "", "", fmt.Errorf("exec attach: %w", err)
	}
	defer attachResp.Close()

	// StdCopy blocks on the hijacked connection and does not respect context
	// cancellation. Close the connection when the context fires so StdCopy
	// unblocks.
	execDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			attachResp.Close()
		case <-execDone:
		}
	}()

	outW := &limitedWriter{max: r.cfg.MaxOutputBytes}
	errW := &limitedWriter{max: r.cfg.MaxOutputBytes}
	_, copyErr := stdcopy.StdCopy(outW, errW, attachResp.Reader)
	close(execDone)

	if copyErr != nil && ctx.Err() != nil {
		return outW.String(), errW.String(), fmt.Errorf("exec timed out: %w", ctx.Err())
	}
	if copyErr != nil {
		return "", "", fmt.Errorf("read exec output: %w", copyErr)
	}

	inspect, err := r.cli.ContainerExecInspect(ctx, execResp.ID)
	if err != nil {
		return outW.String(), errW.String(), fmt.Errorf("exec inspect: %w", err)
	}
	if inspect.ExitCode != 0 {
		return outW.String(), errW.String(), &ExecError{
			ExitCode: inspect.ExitCode,
			Stderr:   errW.String(),
		}
	}

	return outW.String(), errW.String(), nil
}

func (r *dockerRunner) pullImage(ctx context.Context, img string) error {
	if r.cfg.ImagePullPolicy == PullIfNotPresent {
		_, _, err := r.cli.ImageInspectWithRaw(ctx, img) //nolint:staticcheck // TODO: migrate to ImageInspect when SDK stabilizes
		if err == nil {
			return nil
		}
	} else if r.cfg.ImagePullPolicy == PullNever {
		return nil
	}

	r.logger.Info("pulling image", "image", img)
	rc, err := r.cli.ImagePull(ctx, img, image.PullOptions{})
	if err != nil {
		return err
	}
	defer rc.Close()
	// Drain the JSON progress stream; required for pull to complete.
	_, _ = io.Copy(io.Discard, rc)
	return nil
}

func (r *dockerRunner) CopyFrom(ctx context.Context, containerID, srcPath string) (map[string][]byte, error) {
	rc, _, err := r.cli.CopyFromContainer(ctx, containerID, srcPath)
	if err != nil {
		return nil, fmt.Errorf("copy from container: %w", err)
	}
	defer rc.Close()
	return unpackTar(rc, r.cfg.MemoryLimit)
}

func (r *dockerRunner) Remove(ctx context.Context, containerID string) error {
	return r.cli.ContainerRemove(ctx, containerID, container.RemoveOptions{
		Force:         true,
		RemoveVolumes: true,
	})
}

func (r *dockerRunner) Close() error {
	return r.cli.Close()
}
