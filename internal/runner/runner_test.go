package runner

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestPackUnpackRoundTrip(t *testing.T) {
	input := map[string][]byte{
		"hello.txt":      []byte("hello world"),
		"dir/nested.txt": []byte("nested content"),
		"a/b/c/deep.txt": []byte("deep file"),
	}

	r, err := packTar(input)
	if err != nil {
		t.Fatalf("packTar: %v", err)
	}
	output, err := unpackTar(r, 1<<20)
	if err != nil {
		t.Fatalf("unpackTar: %v", err)
	}

	for name, want := range input {
		got, ok := output[name]
		if !ok {
			t.Errorf("missing file %q", name)
			continue
		}
		if !bytes.Equal(got, want) {
			t.Errorf("file %q: got %q, want %q", name, got, want)
		}
	}
	if len(output) != len(input) {
		t.Errorf("file count: got %d, want %d", len(output), len(input))
	}
}

func TestPackTarNestedDirs(t *testing.T) {
	input := map[string][]byte{
		"a/b/c.txt": []byte("data"),
	}
	r, err := packTar(input)
	if err != nil {
		t.Fatalf("packTar: %v", err)
	}
	output, err := unpackTar(r, 1<<20)
	if err != nil {
		t.Fatalf("unpackTar: %v", err)
	}
	if _, ok := output["a/b/c.txt"]; !ok {
		t.Error("missing nested file after roundtrip")
	}
}

func TestPackTarEmptyMap(t *testing.T) {
	r, err := packTar(map[string][]byte{})
	if err != nil {
		t.Fatalf("packTar empty: %v", err)
	}
	output, err := unpackTar(r, 1<<20)
	if err != nil {
		t.Fatalf("unpackTar empty: %v", err)
	}
	if len(output) != 0 {
		t.Errorf("expected empty map, got %d entries", len(output))
	}
}

func TestUnpackTarExceedsLimit(t *testing.T) {
	bigData := bytes.Repeat([]byte("x"), 1000)
	input := map[string][]byte{"big.txt": bigData}

	r, err := packTar(input)
	if err != nil {
		t.Fatalf("packTar: %v", err)
	}
	_, err = unpackTar(r, 500)
	if err == nil {
		t.Fatal("expected error for exceeding limit, got nil")
	}
}

func TestPathNormalization(t *testing.T) {
	cases := []struct {
		name  string
		input map[string][]byte
		want  string
	}{
		{"leading slash", map[string][]byte{"/foo.txt": []byte("a")}, "foo.txt"},
		{"leading dot slash", map[string][]byte{"./bar.txt": []byte("b")}, "bar.txt"},
		{"clean path", map[string][]byte{"baz.txt": []byte("c")}, "baz.txt"},
		{"double dot-slash", map[string][]byte{"././d.txt": []byte("d")}, "d.txt"},
		{"slash-dot-slash", map[string][]byte{"/./e.txt": []byte("e")}, "e.txt"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, err := packTar(tc.input)
			if err != nil {
				t.Fatalf("packTar: %v", err)
			}
			output, err := unpackTar(r, 1<<20)
			if err != nil {
				t.Fatalf("unpackTar: %v", err)
			}
			if _, ok := output[tc.want]; !ok {
				t.Errorf("expected key %q, got keys %v", tc.want, mapKeys(output))
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := defaultConfig()
	if cfg.MemoryLimit != 512*1024*1024 {
		t.Errorf("MemoryLimit: got %d, want %d", cfg.MemoryLimit, 512*1024*1024)
	}
	if cfg.NanoCPUs != 500_000_000 {
		t.Errorf("NanoCPUs: got %d, want %d", cfg.NanoCPUs, 500_000_000)
	}
	if cfg.PidsLimit != 512 {
		t.Errorf("PidsLimit: got %d, want %d", cfg.PidsLimit, 512)
	}
	if cfg.TaskTimeout != 60*time.Second {
		t.Errorf("TaskTimeout: got %v, want %v", cfg.TaskTimeout, 60*time.Second)
	}
	if cfg.ImagePullPolicy != PullIfNotPresent {
		t.Errorf("ImagePullPolicy: got %q, want %q", cfg.ImagePullPolicy, PullIfNotPresent)
	}
	if cfg.WorkDir != "/workspace" {
		t.Errorf("WorkDir: got %q, want %q", cfg.WorkDir, "/workspace")
	}
	if cfg.NetworkMode != "" {
		t.Errorf("NetworkMode: got %q, want empty", cfg.NetworkMode)
	}
	if cfg.MaxOutputBytes != 16<<20 {
		t.Errorf("MaxOutputBytes: got %d, want %d", cfg.MaxOutputBytes, 16<<20)
	}
	if cfg.CapDrop != nil {
		t.Errorf("CapDrop: got %v, want nil", cfg.CapDrop)
	}
	if cfg.SecurityOpt != nil {
		t.Errorf("SecurityOpt: got %v, want nil", cfg.SecurityOpt)
	}
}

func TestWithOptions(t *testing.T) {
	cfg := defaultConfig()
	opts := []Option{
		WithMemoryLimit(1024),
		WithNanoCPUs(1_000_000_000),
		WithPidsLimit(256),
		WithTaskTimeout(30 * time.Second),
		WithImagePullPolicy(PullAlways),
		WithWorkDir("/app"),
		WithNetworkMode("none"),
		WithMaxOutputBytes(1024),
		WithCapDrop([]string{"NET_RAW"}),
		WithSecurityOpt([]string{"no-new-privileges"}),
	}
	for _, o := range opts {
		o(cfg)
	}

	if cfg.MemoryLimit != 1024 {
		t.Errorf("MemoryLimit: got %d, want 1024", cfg.MemoryLimit)
	}
	if cfg.NanoCPUs != 1_000_000_000 {
		t.Errorf("NanoCPUs: got %d, want 1000000000", cfg.NanoCPUs)
	}
	if cfg.PidsLimit != 256 {
		t.Errorf("PidsLimit: got %d, want 256", cfg.PidsLimit)
	}
	if cfg.TaskTimeout != 30*time.Second {
		t.Errorf("TaskTimeout: got %v, want 30s", cfg.TaskTimeout)
	}
	if cfg.ImagePullPolicy != PullAlways {
		t.Errorf("ImagePullPolicy: got %q, want %q", cfg.ImagePullPolicy, PullAlways)
	}
	if cfg.WorkDir != "/app" {
		t.Errorf("WorkDir: got %q, want \"/app\"", cfg.WorkDir)
	}
	if cfg.NetworkMode != "none" {
		t.Errorf("NetworkMode: got %q, want \"none\"", cfg.NetworkMode)
	}
	if cfg.MaxOutputBytes != 1024 {
		t.Errorf("MaxOutputBytes: got %d, want 1024", cfg.MaxOutputBytes)
	}
	if len(cfg.CapDrop) != 1 || cfg.CapDrop[0] != "NET_RAW" {
		t.Errorf("CapDrop: got %v, want [NET_RAW]", cfg.CapDrop)
	}
	if len(cfg.SecurityOpt) != 1 || cfg.SecurityOpt[0] != "no-new-privileges" {
		t.Errorf("SecurityOpt: got %v, want [no-new-privileges]", cfg.SecurityOpt)
	}
}

func TestExecErrorFormat(t *testing.T) {
	err := &ExecError{ExitCode: 1, Stderr: "something failed"}
	want := "command exited with code 1: something failed"
	if got := err.Error(); got != want {
		t.Errorf("Error(): got %q, want %q", got, want)
	}
}

func TestExecErrorAs(t *testing.T) {
	orig := &ExecError{ExitCode: 2, Stderr: "bad input"}
	wrapped := fmt.Errorf("task failed: %w", orig)

	var target *ExecError
	if !errors.As(wrapped, &target) {
		t.Fatal("errors.As failed to unwrap ExecError")
	}
	if target.ExitCode != 2 {
		t.Errorf("ExitCode: got %d, want 2", target.ExitCode)
	}
	if target.Stderr != "bad input" {
		t.Errorf("Stderr: got %q, want %q", target.Stderr, "bad input")
	}
}

func TestUnpackTarSkipsTraversal(t *testing.T) {
	// Manually build a tar with a path-traversal entry.
	var buf bytes.Buffer
	tw := tarWriter(&buf)
	writeEntry(t, tw, "good.txt", []byte("ok"))
	writeEntry(t, tw, "../../etc/passwd", []byte("evil"))
	writeEntry(t, tw, "../sneaky.txt", []byte("nope"))
	tw.Close()

	files, err := unpackTar(&buf, 1<<20)
	if err != nil {
		t.Fatalf("unpackTar: %v", err)
	}
	if _, ok := files["good.txt"]; !ok {
		t.Error("expected good.txt to be extracted")
	}
	for k := range files {
		if k == "../../etc/passwd" || k == "../sneaky.txt" || k == "../etc/passwd" {
			t.Errorf("traversal path %q should have been skipped", k)
		}
	}
}

func TestUnpackTarFileCountLimit(t *testing.T) {
	var buf bytes.Buffer
	tw := tarWriter(&buf)
	// Write maxUnpackFiles+1 entries to exceed the limit.
	for i := 0; i <= maxUnpackFiles; i++ {
		name := fmt.Sprintf("f/%d.txt", i)
		writeEntry(t, tw, name, []byte{0})
	}
	tw.Close()

	_, err := unpackTar(&buf, 1<<30)
	if err == nil {
		t.Fatal("expected file count error, got nil")
	}
}

func TestLimitedWriter(t *testing.T) {
	w := &limitedWriter{max: 10}
	w.Write([]byte("hello"))  // 5 bytes
	w.Write([]byte("world!")) // 6 bytes, only 5 fit
	if got := w.String(); got != "helloworld" {
		t.Errorf("got %q, want %q", got, "helloworld")
	}
	// Further writes are silently discarded.
	w.Write([]byte("more"))
	if w.buf.Len() != 10 {
		t.Errorf("buf len: got %d, want 10", w.buf.Len())
	}
}

// --- test helpers ---

func tarWriter(buf *bytes.Buffer) *tar.Writer { return tar.NewWriter(buf) }

func writeEntry(t *testing.T, tw *tar.Writer, name string, data []byte) {
	t.Helper()
	if err := tw.WriteHeader(&tar.Header{
		Name: name, Size: int64(len(data)), Mode: 0o644, Typeflag: tar.TypeReg,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(data); err != nil {
		t.Fatal(err)
	}
}

func mapKeys(m map[string][]byte) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
