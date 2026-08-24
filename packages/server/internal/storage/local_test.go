package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestManifestRoundtrip(t *testing.T) {
	s := NewLocal(t.TempDir())
	ctx := context.Background()

	want := []byte(`{"name":"@scope/foo","versions":{}}`)
	if err := s.PutManifest(ctx, "@scope/foo", want); err != nil {
		t.Fatalf("put scoped: %v", err)
	}
	got, err := s.GetManifest(ctx, "@scope/foo")
	if err != nil {
		t.Fatalf("get scoped: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("scoped roundtrip = %q, want %q", got, want)
	}

	if err := s.PutManifest(ctx, "bar", []byte(`{"name":"bar"}`)); err != nil {
		t.Fatalf("put unscoped: %v", err)
	}
	if got, err = s.GetManifest(ctx, "bar"); err != nil {
		t.Fatalf("get unscoped: %v", err)
	}
	if !bytes.Equal(got, []byte(`{"name":"bar"}`)) {
		t.Fatalf("unscoped roundtrip = %q", got)
	}

	updated := []byte(`{"name":"bar","versions":{"1.0.0":{}}}`)
	if err := s.PutManifest(ctx, "bar", updated); err != nil {
		t.Fatalf("overwrite: %v", err)
	}
	got, err = s.GetManifest(ctx, "bar")
	if err != nil {
		t.Fatalf("get after overwrite: %v", err)
	}
	if !bytes.Equal(got, updated) {
		t.Fatalf("after overwrite = %q, want %q", got, updated)
	}
}

func TestGetManifestNotExist(t *testing.T) {
	s := NewLocal(t.TempDir())

	if _, err := s.GetManifest(context.Background(), "missing"); !errors.Is(err, ErrNotExist) {
		t.Fatalf("err = %v, want ErrNotExist", err)
	}
	if _, err := s.GetManifest(context.Background(), "@scope/missing"); !errors.Is(err, ErrNotExist) {
		t.Fatalf("scoped err = %v, want ErrNotExist", err)
	}
}

func TestTarballRoundtrip(t *testing.T) {
	s := NewLocal(t.TempDir())
	ctx := context.Background()

	content := "fake tarball bytes"
	n, err := s.PutTarball(ctx, "@scope/foo", "foo-1.0.0.tgz", strings.NewReader(content))
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if n != int64(len(content)) {
		t.Fatalf("written = %d, want %d", n, len(content))
	}

	rc, size, err := s.GetTarball(ctx, "@scope/foo", "foo-1.0.0.tgz")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer rc.Close()

	if size != int64(len(content)) {
		t.Fatalf("size = %d, want %d", size, len(content))
	}
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != content {
		t.Fatalf("content = %q, want %q", got, content)
	}
}

func TestGetTarballNotExist(t *testing.T) {
	s := NewLocal(t.TempDir())
	ctx := context.Background()

	if _, _, err := s.GetTarball(ctx, "pkg", "p-1.0.0.tgz"); !errors.Is(err, ErrNotExist) {
		t.Fatalf("missing file err = %v, want ErrNotExist", err)
	}
	if _, _, err := s.GetTarball(ctx, "@no/such", "p-1.0.0.tgz"); !errors.Is(err, ErrNotExist) {
		t.Fatalf("missing pkg err = %v, want ErrNotExist", err)
	}
}

func TestInvalidInputRejected(t *testing.T) {
	s := NewLocal(t.TempDir())
	ctx := context.Background()

	tests := []struct {
		name string
		call func() error
	}{
		{"put manifest traversal", func() error { return s.PutManifest(ctx, "../escape", []byte(`{}`)) }},
		{"put manifest dot segment", func() error { return s.PutManifest(ctx, "a/./b", []byte(`{}`)) }},
		{"put tarball traversal", func() error {
			_, err := s.PutTarball(ctx, "pkg", "../evil.tgz", strings.NewReader("x"))
			return err
		}},
		{"put tarball nested path", func() error {
			_, err := s.PutTarball(ctx, "pkg", "sub/dir.tgz", strings.NewReader("x"))
			return err
		}},
		{"put tarball empty name", func() error {
			_, err := s.PutTarball(ctx, "pkg", "", strings.NewReader("x"))
			return err
		}},
		{"get manifest traversal", func() error {
			_, err := s.GetManifest(ctx, "../escape")
			return err
		}},
		{"get tarball traversal", func() error {
			_, _, err := s.GetTarball(ctx, "pkg", "../../etc/passwd")
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestListPackages(t *testing.T) {
	root := t.TempDir()
	s := NewLocal(root)

	seed := map[string]string{
		"@scope/foo/package.json":  `{}`,
		"bar/package.json":         `{}`,
		"bar/bar-1.0.0.tgz":        "tarball",
		"@scope/foo/foo-1.0.0.tgz": "tarball",
	}
	for path, data := range seed {
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.ListPackages(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	slices.Sort(got)

	want := []string{"@scope/foo", "bar"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestListPackagesMissingRoot(t *testing.T) {
	s := NewLocal(filepath.Join(t.TempDir(), "absent"))

	got, err := s.ListPackages(context.Background())
	if err != nil {
		t.Fatalf("list on missing root: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %v, want empty", got)
	}
}

func TestLock(t *testing.T) {
	s := NewLocal(t.TempDir())

	unlockA := s.Lock("a")
	unlockB := s.Lock("b")
	unlockB()
	if unlockB == nil {
		t.Fatal("unlock is nil")
	}

	acquired := make(chan struct{})
	go func() {
		s.Lock("a")()
		close(acquired)
	}()

	select {
	case <-acquired:
		t.Fatal("second Lock acquired while first held")
	case <-time.After(50 * time.Millisecond):
	}

	unlockA()

	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatal("second Lock never acquired after unlock")
	}
}

func TestFromEnv(t *testing.T) {
	tests := []struct {
		name        string
		backend     string
		path        string
		wantErr     bool
		errContains string
		checkRoot   bool
		wantRoot    string
	}{
		{name: "default local", backend: "", checkRoot: true, wantRoot: "./storage"},
		{name: "explicit local", backend: "local", path: "/tmp/store", checkRoot: true, wantRoot: "/tmp/store"},
		{name: "s3 not implemented", backend: "s3", wantErr: true, errContains: "not implemented"},
		{name: "unknown backend", backend: "gcs", wantErr: true, errContains: "unknown backend"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("STORAGE_BACKEND", tt.backend)
			t.Setenv("STORAGE_PATH", tt.path)

			st, err := FromEnv(slog.New(slog.NewTextHandler(io.Discard, nil)))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("FromEnv() = %v, want error", st)
				}
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("err = %v, want contains %q", err, tt.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("FromEnv(): %v", err)
			}
			l, ok := st.(*Local)
			if !ok {
				t.Fatalf("FromEnv() returned %T, want *Local", st)
			}
			if tt.checkRoot && l.root != tt.wantRoot {
				t.Fatalf("root = %q, want %q", l.root, tt.wantRoot)
			}
		})
	}
}
