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
type Runner interface {
	// RunTask creates a container, injects files, executes cmd, and returns
	// the container ID with stdout/stderr. The container is kept alive so the
	// caller can extract artifacts via CopyFromContainer; call RemoveContainer
	// when done.
	RunTask(ctx context.Context, img string, files map[string][]byte, cmd []string) (containerID, stdout, stderr string, err error)
	CopyFromContainer(ctx context.Context, containerID, srcPath string) (map[string][]byte, error)
	RemoveContainer(ctx context.Context, containerID string) error
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

func (r *dockerRunner) RunTask(ctx context.Context, img string, files map[string][]byte, cmd []string) (string, string, string, error) {
	if r.cfg.TaskTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.cfg.TaskTimeout)
		defer cancel()
	}

	if err := r.pullImage(ctx, img); err != nil {
		return "", "", "", fmt.Errorf("pull image: %w", err)
	}

	// Create container with sleep infinity to keep it alive.
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
		return "", "", "", fmt.Errorf("create container: %w", err)
	}
	cid := resp.ID

	if err := r.cli.ContainerStart(ctx, cid, container.StartOptions{}); err != nil {
		_ = r.cli.ContainerRemove(ctx, cid, container.RemoveOptions{Force: true})
		return "", "", "", fmt.Errorf("start container: %w", err)
	}

	// Inject files into the working directory.
	if len(files) > 0 {
		tarReader, err := packTar(files)
		if err != nil {
			return cid, "", "", fmt.Errorf("pack tar: %w", err)
		}
		if err := r.cli.CopyToContainer(ctx, cid, r.cfg.WorkDir, tarReader, container.CopyToContainerOptions{}); err != nil {
			return cid, "", "", fmt.Errorf("copy to container: %w", err)
		}
	}

	// Execute the command inside the container.
	execResp, err := r.cli.ContainerExecCreate(ctx, cid, container.ExecOptions{
		Cmd:          cmd,
		WorkingDir:   r.cfg.WorkDir,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return cid, "", "", fmt.Errorf("exec create: %w", err)
	}

	attachResp, err := r.cli.ContainerExecAttach(ctx, execResp.ID, container.ExecAttachOptions{})
	if err != nil {
		return cid, "", "", fmt.Errorf("exec attach: %w", err)
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
		return cid, outW.String(), errW.String(), fmt.Errorf("exec timed out: %w", ctx.Err())
	}
	if copyErr != nil {
		return cid, "", "", fmt.Errorf("read exec output: %w", copyErr)
	}

	inspect, err := r.cli.ContainerExecInspect(ctx, execResp.ID)
	if err != nil {
		return cid, outW.String(), errW.String(), fmt.Errorf("exec inspect: %w", err)
	}
	if inspect.ExitCode != 0 {
		return cid, outW.String(), errW.String(), &ExecError{
			ExitCode: inspect.ExitCode,
			Stderr:   errW.String(),
		}
	}

	return cid, outW.String(), errW.String(), nil
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

func (r *dockerRunner) CopyFromContainer(ctx context.Context, containerID, srcPath string) (map[string][]byte, error) {
	rc, _, err := r.cli.CopyFromContainer(ctx, containerID, srcPath)
	if err != nil {
		return nil, fmt.Errorf("copy from container: %w", err)
	}
	defer rc.Close()
	return unpackTar(rc, r.cfg.MemoryLimit)
}

func (r *dockerRunner) RemoveContainer(ctx context.Context, containerID string) error {
	return r.cli.ContainerRemove(ctx, containerID, container.RemoveOptions{
		Force:         true,
		RemoveVolumes: true,
	})
}

func (r *dockerRunner) Close() error {
	return r.cli.Close()
}
