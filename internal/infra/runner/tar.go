package runner

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
)

// packTar creates a tar archive from the given file map.
// Keys are relative paths; values are file contents.
// Intermediate directory entries are generated automatically.
func packTar(files map[string][]byte) (io.Reader, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	// Sort keys for deterministic output.
	keys := make([]string, 0, len(files))
	for k := range files {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Collect intermediate directories.
	dirs := make(map[string]struct{})
	for _, name := range keys {
		dir := path.Dir(normalizePath(name))
		for dir != "." && dir != "" {
			dirs[dir] = struct{}{}
			dir = path.Dir(dir)
		}
	}

	// Write directory entries in sorted order.
	dirList := make([]string, 0, len(dirs))
	for d := range dirs {
		dirList = append(dirList, d)
	}
	sort.Strings(dirList)

	for _, d := range dirList {
		if err := tw.WriteHeader(&tar.Header{
			Name:     d + "/",
			Typeflag: tar.TypeDir,
			Mode:     0o755,
		}); err != nil {
			return nil, fmt.Errorf("write dir header %q: %w", d, err)
		}
	}

	// Write file entries.
	for _, name := range keys {
		normalized := normalizePath(name)
		data := files[name]
		if err := tw.WriteHeader(&tar.Header{
			Name:     normalized,
			Size:     int64(len(data)),
			Mode:     0o644,
			Typeflag: tar.TypeReg,
		}); err != nil {
			return nil, fmt.Errorf("write file header %q: %w", normalized, err)
		}
		if _, err := tw.Write(data); err != nil {
			return nil, fmt.Errorf("write file data %q: %w", normalized, err)
		}
	}

	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("close tar writer: %w", err)
	}
	return &buf, nil
}

const maxUnpackFiles = 10000

// unpackTar extracts regular files from a tar stream into a map.
// maxTotalBytes limits total extracted bytes to prevent tar bombs.
// Symlinks, directories, and path-traversal entries are skipped for security.
// At most maxUnpackFiles files are extracted.
func unpackTar(r io.Reader, maxTotalBytes int64) (map[string][]byte, error) {
	tr := tar.NewReader(r)
	files := make(map[string][]byte)
	var totalRead int64

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read tar header: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		normalized := normalizePath(hdr.Name)
		if !isSafePath(normalized) {
			continue
		}
		if len(files) >= maxUnpackFiles {
			return nil, fmt.Errorf("tar contains more than %d files", maxUnpackFiles)
		}
		if totalRead+hdr.Size > maxTotalBytes {
			return nil, fmt.Errorf("tar extraction exceeds limit of %d bytes", maxTotalBytes)
		}

		data, err := io.ReadAll(io.LimitReader(tr, hdr.Size))
		if err != nil {
			return nil, fmt.Errorf("read file %q: %w", hdr.Name, err)
		}
		totalRead += int64(len(data))
		files[normalized] = data
	}
	return files, nil
}

// normalizePath cleans a path and strips the leading "/".
func normalizePath(p string) string {
	p = path.Clean(p)
	p = strings.TrimPrefix(p, "/")
	return p
}

// isSafePath returns false for paths that escape the root (contain ".." components).
func isSafePath(p string) bool {
	return p != ".." && !strings.HasPrefix(p, "../")
}
