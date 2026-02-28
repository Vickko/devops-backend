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

// createContainer is a test helper that creates a container and registers cleanup.
func createContainer(t *testing.T, r Runner) string {
	t.Helper()
	cid, err := r.Create(context.Background(), testImage)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { r.Remove(context.Background(), cid) })
	return cid
}

func TestIntegration_Create_Exec(t *testing.T) {
	r := newTestRunner(t)
	cid := createContainer(t, r)

	stdout, _, err := r.Exec(context.Background(), cid, []string{"echo", "hello"})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if stdout != "hello\n" {
		t.Errorf("stdout: got %q, want %q", stdout, "hello\n")
	}
}

func TestIntegration_CopyTo_Exec(t *testing.T) {
	r := newTestRunner(t)
	cid := createContainer(t, r)

	if err := r.CopyTo(context.Background(), cid, map[string][]byte{
		"input.txt": []byte("hello from host"),
	}); err != nil {
		t.Fatalf("CopyTo: %v", err)
	}

	stdout, _, err := r.Exec(context.Background(), cid, []string{"cat", "/workspace/input.txt"})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if stdout != "hello from host" {
		t.Errorf("stdout: got %q, want %q", stdout, "hello from host")
	}
}

func TestIntegration_NestedFiles(t *testing.T) {
	r := newTestRunner(t)
	cid := createContainer(t, r)

	if err := r.CopyTo(context.Background(), cid, map[string][]byte{
		"src/main.sh": []byte("#!/bin/sh\necho nested-ok"),
	}); err != nil {
		t.Fatalf("CopyTo: %v", err)
	}

	stdout, _, err := r.Exec(context.Background(), cid, []string{"sh", "/workspace/src/main.sh"})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if stdout != "nested-ok\n" {
		t.Errorf("stdout: got %q, want %q", stdout, "nested-ok\n")
	}
}

func TestIntegration_MultipleExec(t *testing.T) {
	r := newTestRunner(t)
	cid := createContainer(t, r)

	// First exec: create a file.
	_, _, err := r.Exec(context.Background(), cid, []string{"sh", "-c", "echo step1 > /workspace/state.txt"})
	if err != nil {
		t.Fatalf("Exec 1: %v", err)
	}

	// Second exec: read the file created by the first.
	stdout, _, err := r.Exec(context.Background(), cid, []string{"cat", "/workspace/state.txt"})
	if err != nil {
		t.Fatalf("Exec 2: %v", err)
	}
	if stdout != "step1\n" {
		t.Errorf("stdout: got %q, want %q", stdout, "step1\n")
	}
}

func TestIntegration_NonZeroExit(t *testing.T) {
	r := newTestRunner(t)
	cid := createContainer(t, r)

	_, _, err := r.Exec(context.Background(), cid, []string{"sh", "-c", "echo fail >&2; exit 42"})
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

func TestIntegration_StdoutStderrSeparation(t *testing.T) {
	r := newTestRunner(t)
	cid := createContainer(t, r)

	stdout, stderr, err := r.Exec(context.Background(), cid,
		[]string{"sh", "-c", "echo out-line; echo err-line >&2"})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if stdout != "out-line\n" {
		t.Errorf("stdout: got %q, want %q", stdout, "out-line\n")
	}
	if stderr != "err-line\n" {
		t.Errorf("stderr: got %q, want %q", stderr, "err-line\n")
	}
}

func TestIntegration_CopyFrom(t *testing.T) {
	r := newTestRunner(t)
	cid := createContainer(t, r)

	_, _, err := r.Exec(context.Background(), cid,
		[]string{"sh", "-c", "mkdir -p /workspace/out && echo artifact > /workspace/out/result.txt"})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}

	files, err := r.CopyFrom(context.Background(), cid, "/workspace/out/")
	if err != nil {
		t.Fatalf("CopyFrom: %v", err)
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

func TestIntegration_ExecTimeout(t *testing.T) {
	r := newTestRunner(t, WithTaskTimeout(2*time.Second))
	cid := createContainer(t, r)

	_, _, err := r.Exec(context.Background(), cid, []string{"sleep", "60"})
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	t.Logf("got expected error: %v", err)
}

func TestIntegration_Remove(t *testing.T) {
	r := newTestRunner(t)
	cid, err := r.Create(context.Background(), testImage)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := r.Remove(context.Background(), cid); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	// Second remove should fail.
	if err := r.Remove(context.Background(), cid); err == nil {
		t.Error("expected error on double remove, got nil")
	}
}
