package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"server/internal/blob"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	return New(blob.NewLocal(t.TempDir()))
}

func TestManifestRoundtrip(t *testing.T) {
	s := newTestStore(t)
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
	s := newTestStore(t)

	if _, err := s.GetManifest(context.Background(), "missing"); !errors.Is(err, ErrNotExist) {
		t.Fatalf("err = %v, want ErrNotExist", err)
	}
	if _, err := s.GetManifest(context.Background(), "@scope/missing"); !errors.Is(err, ErrNotExist) {
		t.Fatalf("scoped err = %v, want ErrNotExist", err)
	}
}

func TestTarballRoundtrip(t *testing.T) {
	s := newTestStore(t)
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
	s := newTestStore(t)
	ctx := context.Background()

	if _, _, err := s.GetTarball(ctx, "pkg", "p-1.0.0.tgz"); !errors.Is(err, ErrNotExist) {
		t.Fatalf("missing file err = %v, want ErrNotExist", err)
	}
	if _, _, err := s.GetTarball(ctx, "@no/such", "p-1.0.0.tgz"); !errors.Is(err, ErrNotExist) {
		t.Fatalf("missing pkg err = %v, want ErrNotExist", err)
	}
}

func TestInvalidInputRejected(t *testing.T) {
	s := newTestStore(t)
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
		{"put tarball manifest name", func() error {
			_, err := s.PutTarball(ctx, "pkg", "package.json", strings.NewReader("x"))
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
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.PutManifest(ctx, "@scope/foo", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if err := s.PutManifest(ctx, "bar", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PutTarball(ctx, "bar", "bar-1.0.0.tgz", strings.NewReader("t")); err != nil {
		t.Fatal(err)
	}

	got, err := s.ListPackages(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	slices.Sort(got)

	want := []string{"@scope/foo", "bar"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestListPackagesEmpty(t *testing.T) {
	s := newTestStore(t)

	got, err := s.ListPackages(context.Background())
	if err != nil {
		t.Fatalf("list on empty store: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %v, want empty", got)
	}
}

func TestLock(t *testing.T) {
	s := newTestStore(t)

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

func TestDeletePackage(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.PutTarball(ctx, "@acme/lib", "lib-1.0.0.tgz", strings.NewReader("tar")); err != nil {
		t.Fatalf("seed tarball: %v", err)
	}
	if err := s.PutManifest(ctx, "@acme/lib", []byte(`{"name":"@acme/lib"}`)); err != nil {
		t.Fatalf("seed manifest: %v", err)
	}
	if err := s.PutManifest(ctx, "@acme/other", []byte(`{"name":"@acme/other"}`)); err != nil {
		t.Fatalf("seed other: %v", err)
	}

	if err := s.DeletePackage(ctx, "@acme/lib"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if _, err := s.GetManifest(ctx, "@acme/lib"); !errors.Is(err, ErrNotExist) {
		t.Errorf("manifest after delete: err = %v, want ErrNotExist", err)
	}
	if _, _, err := s.GetTarball(ctx, "@acme/lib", "lib-1.0.0.tgz"); !errors.Is(err, ErrNotExist) {
		t.Errorf("tarball after delete: err = %v, want ErrNotExist", err)
	}
	pkgs, err := s.ListPackages(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !slices.Equal(pkgs, []string{"@acme/other"}) {
		t.Errorf("packages = %v, want [@acme/other]", pkgs)
	}

	// deleting an unknown package is a no-op
	if err := s.DeletePackage(ctx, "@acme/lib"); err != nil {
		t.Errorf("repeat delete: %v", err)
	}
}

// TestKeysMirrorDiskLayout pins the object layout to the one the local
// driver used before the blob refactor, so existing data stays readable.
func TestKeysMirrorDiskLayout(t *testing.T) {
	root := t.TempDir()
	s := New(blob.NewLocal(root))
	ctx := context.Background()

	if err := s.PutManifest(ctx, "@scope/foo", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PutTarball(ctx, "@scope/foo", "foo-1.0.0.tgz", strings.NewReader("t")); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{
		"@scope/foo/package.json",
		"@scope/foo/foo-1.0.0.tgz",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(path))); err != nil {
			t.Errorf("expected file at %s: %v", path, err)
		}
	}
}
