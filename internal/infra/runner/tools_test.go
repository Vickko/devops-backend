package runner

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/tool"
)

// mockRunner implements Runner for testing tool wiring.
type mockRunner struct {
	createFn   func(ctx context.Context, image string) (string, error)
	copyToFn   func(ctx context.Context, id string, files map[string][]byte) error
	execFn     func(ctx context.Context, id string, cmd []string) (string, string, error)
	copyFromFn func(ctx context.Context, id, path string) (map[string][]byte, error)
	removeFn   func(ctx context.Context, id string) error
}

func (m *mockRunner) Create(ctx context.Context, image string) (string, error) {
	return m.createFn(ctx, image)
}
func (m *mockRunner) CopyTo(ctx context.Context, id string, files map[string][]byte) error {
	return m.copyToFn(ctx, id, files)
}
func (m *mockRunner) Exec(ctx context.Context, id string, cmd []string) (string, string, error) {
	return m.execFn(ctx, id, cmd)
}
func (m *mockRunner) CopyFrom(ctx context.Context, id, path string) (map[string][]byte, error) {
	return m.copyFromFn(ctx, id, path)
}
func (m *mockRunner) Remove(ctx context.Context, id string) error {
	return m.removeFn(ctx, id)
}
func (m *mockRunner) Close() error { return nil }

func newNoopMock() *mockRunner {
	return &mockRunner{
		createFn:   func(_ context.Context, _ string) (string, error) { return "c-123", nil },
		copyToFn:   func(_ context.Context, _ string, _ map[string][]byte) error { return nil },
		execFn:     func(_ context.Context, _ string, _ []string) (string, string, error) { return "", "", nil },
		copyFromFn: func(_ context.Context, _, _ string) (map[string][]byte, error) { return nil, nil },
		removeFn:   func(_ context.Context, _ string) error { return nil },
	}
}

// toolByName builds a name→tool map for index-independent lookup.
func toolByName(t *testing.T, tools []tool.InvokableTool) map[string]tool.InvokableTool {
	t.Helper()
	m := make(map[string]tool.InvokableTool, len(tools))
	for _, tl := range tools {
		info, err := tl.Info(context.Background())
		if err != nil {
			t.Fatalf("Info(): %v", err)
		}
		m[info.Name] = tl
	}
	return m
}

func TestNewToolsCount(t *testing.T) {
	tools, err := NewTools(newNoopMock())
	if err != nil {
		t.Fatalf("NewTools: %v", err)
	}
	if got := len(tools); got != 5 {
		t.Fatalf("tool count: got %d, want 5", got)
	}
}

func TestNewToolsNames(t *testing.T) {
	tools, err := NewTools(newNoopMock())
	if err != nil {
		t.Fatalf("NewTools: %v", err)
	}

	wantNames := []string{
		"create_container",
		"copy_to_container",
		"exec_in_container",
		"copy_from_container",
		"remove_container",
	}

	tm := toolByName(t, tools)
	for _, want := range wantNames {
		tl, ok := tm[want]
		if !ok {
			t.Errorf("missing tool %q", want)
			continue
		}
		info, _ := tl.Info(context.Background())
		if info.Desc == "" {
			t.Errorf("tool %s: description is empty", want)
		}
		if info.ParamsOneOf == nil {
			t.Errorf("tool %s: ParamsOneOf is nil", want)
		}
	}
}

func TestExecToolHandlesExecError(t *testing.T) {
	mock := newNoopMock()
	mock.execFn = func(_ context.Context, _ string, _ []string) (string, string, error) {
		return "some output", "compile error", &ExecError{ExitCode: 1, Stderr: "compile error"}
	}

	tools, err := NewTools(mock)
	if err != nil {
		t.Fatalf("NewTools: %v", err)
	}

	execTool := toolByName(t, tools)["exec_in_container"]
	result, err := execTool.InvokableRun(context.Background(),
		`{"container_id":"c-1","cmd":["go","build","."]}`)
	if err != nil {
		t.Fatalf("InvokableRun should not return error for ExecError, got: %v", err)
	}
	if result == "" {
		t.Fatal("expected non-empty result")
	}
	if !strings.Contains(result, `"exit_code":1`) {
		t.Errorf("result missing exit_code: %s", result)
	}
	if !strings.Contains(result, `"compile error"`) {
		t.Errorf("result missing stderr content: %s", result)
	}
}

func TestExecToolPropagatesInfraError(t *testing.T) {
	mock := newNoopMock()
	mock.execFn = func(_ context.Context, _ string, _ []string) (string, string, error) {
		return "", "", errors.New("container not found")
	}

	tools, err := NewTools(mock)
	if err != nil {
		t.Fatalf("NewTools: %v", err)
	}

	execTool := toolByName(t, tools)["exec_in_container"]
	_, err = execTool.InvokableRun(context.Background(),
		`{"container_id":"c-gone","cmd":["ls"]}`)
	if err == nil {
		t.Fatal("expected error for infrastructure failure, got nil")
	}
}

func TestCreateToolCallsRunner(t *testing.T) {
	var gotImage string
	mock := newNoopMock()
	mock.createFn = func(_ context.Context, image string) (string, error) {
		gotImage = image
		return "c-new", nil
	}

	tools, err := NewTools(mock)
	if err != nil {
		t.Fatalf("NewTools: %v", err)
	}

	result, err := toolByName(t, tools)["create_container"].InvokableRun(
		context.Background(), `{"image":"python:3.12-slim"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if gotImage != "python:3.12-slim" {
		t.Errorf("image passed to runner: got %q, want %q", gotImage, "python:3.12-slim")
	}
	if !strings.Contains(result, `"c-new"`) {
		t.Errorf("result missing container ID: %s", result)
	}
}

func TestCreateToolPropagatesError(t *testing.T) {
	mock := newNoopMock()
	mock.createFn = func(_ context.Context, _ string) (string, error) {
		return "", errors.New("pull failed")
	}

	tools, err := NewTools(mock)
	if err != nil {
		t.Fatalf("NewTools: %v", err)
	}

	_, err = toolByName(t, tools)["create_container"].InvokableRun(
		context.Background(), `{"image":"bad:image"}`)
	if err == nil {
		t.Fatal("expected error from create, got nil")
	}
}

func TestCopyToToolConvertsStringToBytes(t *testing.T) {
	var gotFiles map[string][]byte
	mock := newNoopMock()
	mock.copyToFn = func(_ context.Context, _ string, files map[string][]byte) error {
		gotFiles = files
		return nil
	}

	tools, err := NewTools(mock)
	if err != nil {
		t.Fatalf("NewTools: %v", err)
	}

	_, err = toolByName(t, tools)["copy_to_container"].InvokableRun(
		context.Background(), `{"container_id":"c-1","files":{"main.py":"print('hi')"}}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if string(gotFiles["main.py"]) != "print('hi')" {
		t.Errorf("file content: got %q, want %q", gotFiles["main.py"], "print('hi')")
	}
}

func TestCopyToToolPropagatesError(t *testing.T) {
	mock := newNoopMock()
	mock.copyToFn = func(_ context.Context, _ string, _ map[string][]byte) error {
		return errors.New("no such container")
	}

	tools, err := NewTools(mock)
	if err != nil {
		t.Fatalf("NewTools: %v", err)
	}

	_, err = toolByName(t, tools)["copy_to_container"].InvokableRun(
		context.Background(), `{"container_id":"c-gone","files":{"a.txt":"x"}}`)
	if err == nil {
		t.Fatal("expected error from copy_to, got nil")
	}
}

func TestCopyFromToolConvertsBytesToString(t *testing.T) {
	mock := newNoopMock()
	mock.copyFromFn = func(_ context.Context, _, _ string) (map[string][]byte, error) {
		return map[string][]byte{"out.txt": []byte("result data")}, nil
	}

	tools, err := NewTools(mock)
	if err != nil {
		t.Fatalf("NewTools: %v", err)
	}

	result, err := toolByName(t, tools)["copy_from_container"].InvokableRun(
		context.Background(), `{"container_id":"c-1","path":"/workspace/out.txt"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if !strings.Contains(result, `"result data"`) {
		t.Errorf("result missing file content: %s", result)
	}
}

func TestCopyFromToolBase64ForBinary(t *testing.T) {
	mock := newNoopMock()
	mock.copyFromFn = func(_ context.Context, _, _ string) (map[string][]byte, error) {
		return map[string][]byte{"bin": {0xff, 0xfe, 0x00, 0x01}}, nil
	}

	tools, err := NewTools(mock)
	if err != nil {
		t.Fatalf("NewTools: %v", err)
	}

	result, err := toolByName(t, tools)["copy_from_container"].InvokableRun(
		context.Background(), `{"container_id":"c-1","path":"/workspace/bin"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if !strings.Contains(result, "base64:") {
		t.Errorf("expected base64 prefix for binary content: %s", result)
	}
}

func TestCopyFromToolPropagatesError(t *testing.T) {
	mock := newNoopMock()
	mock.copyFromFn = func(_ context.Context, _, _ string) (map[string][]byte, error) {
		return nil, errors.New("path not found")
	}

	tools, err := NewTools(mock)
	if err != nil {
		t.Fatalf("NewTools: %v", err)
	}

	_, err = toolByName(t, tools)["copy_from_container"].InvokableRun(
		context.Background(), `{"container_id":"c-1","path":"/nope"}`)
	if err == nil {
		t.Fatal("expected error from copy_from, got nil")
	}
}

func TestRemoveToolCallsRunner(t *testing.T) {
	var removedID string
	mock := newNoopMock()
	mock.removeFn = func(_ context.Context, id string) error {
		removedID = id
		return nil
	}

	tools, err := NewTools(mock)
	if err != nil {
		t.Fatalf("NewTools: %v", err)
	}

	result, err := toolByName(t, tools)["remove_container"].InvokableRun(
		context.Background(), `{"container_id":"c-rm"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if removedID != "c-rm" {
		t.Errorf("removed ID: got %q, want %q", removedID, "c-rm")
	}
	if !strings.Contains(result, `"success":true`) {
		t.Errorf("result missing success: %s", result)
	}
}

func TestRemoveToolPropagatesError(t *testing.T) {
	mock := newNoopMock()
	mock.removeFn = func(_ context.Context, _ string) error {
		return errors.New("remove failed")
	}

	tools, err := NewTools(mock)
	if err != nil {
		t.Fatalf("NewTools: %v", err)
	}

	_, err = toolByName(t, tools)["remove_container"].InvokableRun(
		context.Background(), `{"container_id":"c-x"}`)
	if err == nil {
		t.Fatal("expected error from remove, got nil")
	}
}
