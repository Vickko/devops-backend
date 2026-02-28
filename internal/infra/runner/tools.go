package runner

import (
	"context"
	"encoding/base64"
	"errors"
	"unicode/utf8"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// --- Input / Output structs for LLM tool calling ---

type CreateInput struct {
	Image string `json:"image" jsonschema:"required,description=Docker image name (e.g. python:3.12-slim)"`
}

type CreateOutput struct {
	ContainerID string `json:"container_id"`
}

type CopyToInput struct {
	ContainerID string            `json:"container_id" jsonschema:"required,description=target container ID"`
	Files       map[string]string `json:"files"        jsonschema:"required,description=map of relative path to file content"`
}

type CopyToOutput struct {
	Success bool `json:"success"`
}

type ExecInput struct {
	ContainerID string   `json:"container_id" jsonschema:"required,description=target container ID"`
	Cmd         []string `json:"cmd"          jsonschema:"required,description=command and arguments to execute"`
}

type ExecOutput struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
}

type CopyFromInput struct {
	ContainerID string `json:"container_id" jsonschema:"required,description=target container ID"`
	Path        string `json:"path"         jsonschema:"required,description=path inside container to copy from"`
}

type CopyFromOutput struct {
	Files map[string]string `json:"files"`
}

type RemoveInput struct {
	ContainerID string `json:"container_id" jsonschema:"required,description=container ID to remove"`
}

type RemoveOutput struct {
	Success bool `json:"success"`
}

// NewTools wraps the five Runner operations as Eino InvokableTools for LLM function calling.
func NewTools(r Runner) ([]tool.InvokableTool, error) {
	create, err := utils.InferTool("create_container",
		"Create and start a new Docker container from the given image",
		func(ctx context.Context, in CreateInput) (CreateOutput, error) {
			id, err := r.Create(ctx, in.Image)
			if err != nil {
				return CreateOutput{}, err
			}
			return CreateOutput{ContainerID: id}, nil
		})
	if err != nil {
		return nil, err
	}

	copyTo, err := utils.InferTool("copy_to_container",
		"Copy files into a running container",
		func(ctx context.Context, in CopyToInput) (CopyToOutput, error) {
			files := make(map[string][]byte, len(in.Files))
			for k, v := range in.Files {
				files[k] = []byte(v)
			}
			if err := r.CopyTo(ctx, in.ContainerID, files); err != nil {
				return CopyToOutput{}, err
			}
			return CopyToOutput{Success: true}, nil
		})
	if err != nil {
		return nil, err
	}

	exec, err := utils.InferTool("exec_in_container",
		"Execute a command inside a running container and return stdout, stderr, and exit code",
		func(ctx context.Context, in ExecInput) (ExecOutput, error) {
			stdout, stderr, err := r.Exec(ctx, in.ContainerID, in.Cmd)
			if err != nil {
				var execErr *ExecError
				if errors.As(err, &execErr) {
					// Non-zero exit: surface output to LLM, not an error.
					return ExecOutput{
						ExitCode: execErr.ExitCode,
						Stdout:   stdout,
						Stderr:   stderr,
					}, nil
				}
				// Infrastructure error (container gone, daemon unreachable, etc.)
				return ExecOutput{}, err
			}
			return ExecOutput{ExitCode: 0, Stdout: stdout, Stderr: stderr}, nil
		})
	if err != nil {
		return nil, err
	}

	copyFrom, err := utils.InferTool("copy_from_container",
		"Copy files from a container path and return their contents",
		func(ctx context.Context, in CopyFromInput) (CopyFromOutput, error) {
			raw, err := r.CopyFrom(ctx, in.ContainerID, in.Path)
			if err != nil {
				return CopyFromOutput{}, err
			}
			files := make(map[string]string, len(raw))
			for k, v := range raw {
				if utf8.Valid(v) {
					files[k] = string(v)
				} else {
					files[k] = "base64:" + base64.StdEncoding.EncodeToString(v)
				}
			}
			return CopyFromOutput{Files: files}, nil
		})
	if err != nil {
		return nil, err
	}

	remove, err := utils.InferTool("remove_container",
		"Force-remove a container and its volumes",
		func(ctx context.Context, in RemoveInput) (RemoveOutput, error) {
			if err := r.Remove(ctx, in.ContainerID); err != nil {
				return RemoveOutput{}, err
			}
			return RemoveOutput{Success: true}, nil
		})
	if err != nil {
		return nil, err
	}

	return []tool.InvokableTool{create, copyTo, exec, copyFrom, remove}, nil
}
