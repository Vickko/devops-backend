//go:build integration

package runner

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"
)

const testImage = "alpine:3.20"

func newTestRunner(t *testing.T, opts ...Option) Runner {
	t.Helper()
	r, err := New(slog.New(slog.NewTextHandler(os.Stderr, nil)), opts...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { r.Close() })
	return r
}

// TestIntegration_RunTask_HelloWorld verifies the basic happy path:
// inject a file, run a command that reads it, check stdout.
func TestIntegration_RunTask_HelloWorld(t *testing.T) {
	r := newTestRunner(t)
	files := map[string][]byte{
		"input.txt": []byte("hello from host"),
	}

	cid, stdout, stderr, err := r.RunTask(context.Background(), testImage, files, []string{"cat", "/workspace/input.txt"})
	if err != nil {
		t.Fatalf("RunTask: %v\nstderr: %s", err, stderr)
	}
	defer r.RemoveContainer(context.Background(), cid)

	if stdout != "hello from host" {
		t.Errorf("stdout: got %q, want %q", stdout, "hello from host")
	}
}

// TestIntegration_RunTask_NestedFiles verifies files in subdirectories are injected correctly.
func TestIntegration_RunTask_NestedFiles(t *testing.T) {
	r := newTestRunner(t)
	files := map[string][]byte{
		"src/main.sh": []byte("#!/bin/sh\necho nested-ok"),
	}

	cid, stdout, _, err := r.RunTask(context.Background(), testImage, files, []string{"sh", "/workspace/src/main.sh"})
	if err != nil {
		t.Fatalf("RunTask: %v", err)
	}
	defer r.RemoveContainer(context.Background(), cid)

	if stdout != "nested-ok\n" {
		t.Errorf("stdout: got %q, want %q", stdout, "nested-ok\n")
	}
}

// TestIntegration_RunTask_NonZeroExit verifies ExecError is returned on failure.
func TestIntegration_RunTask_NonZeroExit(t *testing.T) {
	r := newTestRunner(t)

	cid, _, _, err := r.RunTask(context.Background(), testImage, nil, []string{"sh", "-c", "echo fail >&2; exit 42"})
	if cid != "" {
		defer r.RemoveContainer(context.Background(), cid)
	}

	var execErr *ExecError
	if !errors.As(err, &execErr) {
		t.Fatalf("expected ExecError, got %T: %v", err, err)
	}
	if execErr.ExitCode != 42 {
		t.Errorf("ExitCode: got %d, want 42", execErr.ExitCode)
	}
	if execErr.Stderr != "fail\n" {
		t.Errorf("Stderr: got %q, want %q", execErr.Stderr, "fail\n")
	}
}

// TestIntegration_RunTask_StdoutStderrSeparation verifies demux works correctly.
func TestIntegration_RunTask_StdoutStderrSeparation(t *testing.T) {
	r := newTestRunner(t)

	cid, stdout, stderr, err := r.RunTask(context.Background(), testImage, nil,
		[]string{"sh", "-c", "echo out-line; echo err-line >&2"})
	if err != nil {
		t.Fatalf("RunTask: %v", err)
	}
	defer r.RemoveContainer(context.Background(), cid)

	if stdout != "out-line\n" {
		t.Errorf("stdout: got %q, want %q", stdout, "out-line\n")
	}
	if stderr != "err-line\n" {
		t.Errorf("stderr: got %q, want %q", stderr, "err-line\n")
	}
}

// TestIntegration_CopyFromContainer verifies artifact extraction.
func TestIntegration_CopyFromContainer(t *testing.T) {
	r := newTestRunner(t)

	// Run a command that creates an output file.
	cid, _, _, err := r.RunTask(context.Background(), testImage, nil,
		[]string{"sh", "-c", "mkdir -p /workspace/out && echo artifact > /workspace/out/result.txt"})
	if err != nil {
		t.Fatalf("RunTask: %v", err)
	}
	defer r.RemoveContainer(context.Background(), cid)

	files, err := r.CopyFromContainer(context.Background(), cid, "/workspace/out/")
	if err != nil {
		t.Fatalf("CopyFromContainer: %v", err)
	}

	found := false
	for k, v := range files {
		t.Logf("extracted: %s (%d bytes)", k, len(v))
		if string(v) == "artifact\n" {
			found = true
		}
	}
	if !found {
		t.Error("expected to find result.txt with content \"artifact\\n\"")
	}
}

// TestIntegration_RunTask_Timeout verifies TaskTimeout cancels long-running commands.
func TestIntegration_RunTask_Timeout(t *testing.T) {
	r := newTestRunner(t, WithTaskTimeout(2*time.Second))

	cid, _, _, err := r.RunTask(context.Background(), testImage, nil, []string{"sleep", "60"})
	if cid != "" {
		defer r.RemoveContainer(context.Background(), cid)
	}
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	t.Logf("got expected error: %v", err)
}

// TestIntegration_RemoveContainer verifies cleanup works.
func TestIntegration_RemoveContainer(t *testing.T) {
	r := newTestRunner(t)

	cid, _, _, err := r.RunTask(context.Background(), testImage, nil, []string{"echo", "done"})
	if err != nil {
		t.Fatalf("RunTask: %v", err)
	}

	if err := r.RemoveContainer(context.Background(), cid); err != nil {
		t.Fatalf("RemoveContainer: %v", err)
	}

	// Second remove should fail — container is already gone.
	if err := r.RemoveContainer(context.Background(), cid); err == nil {
		t.Error("expected error on double remove, got nil")
	}
}
