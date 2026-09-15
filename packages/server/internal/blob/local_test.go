package blob

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestLocalPutGet(t *testing.T) {
	s := NewLocal(t.TempDir())
	ctx := context.Background()

	n, err := s.Put(ctx, "@scope/foo/foo-1.0.0.tgz", strings.NewReader("content"))
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if n != int64(len("content")) {
		t.Fatalf("written = %d, want %d", n, len("content"))
	}

	rc, size, err := s.Get(ctx, "@scope/foo/foo-1.0.0.tgz")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer rc.Close()
	if size != int64(len("content")) {
		t.Fatalf("size = %d", size)
	}
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "content" {
		t.Fatalf("content = %q", got)
	}
}

func TestLocalPutOverwritesAtomically(t *testing.T) {
	s := NewLocal(t.TempDir())
	ctx := context.Background()

	if _, err := s.Put(ctx, "a/b", strings.NewReader("old")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Put(ctx, "a/b", strings.NewReader("brand new")); err != nil {
		t.Fatal(err)
	}
	rc, _, err := s.Get(ctx, "a/b")
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	data, _ := io.ReadAll(rc)
	if !bytes.Equal(data, []byte("brand new")) {
		t.Fatalf("content = %q", data)
	}
}

func TestLocalGetNotExist(t *testing.T) {
	s := NewLocal(filepath.Join(t.TempDir(), "empty"))
	if _, _, err := s.Get(context.Background(), "nope"); !errors.Is(err, ErrNotExist) {
		t.Fatalf("err = %v, want ErrNotExist", err)
	}
}

func TestLocalDeleteIdempotent(t *testing.T) {
	s := NewLocal(t.TempDir())
	ctx := context.Background()

	if _, err := s.Put(ctx, "x", strings.NewReader("v")); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, "x"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := s.Delete(ctx, "x"); err != nil {
		t.Fatalf("repeat delete: %v", err)
	}
	if _, _, err := s.Get(ctx, "x"); !errors.Is(err, ErrNotExist) {
		t.Fatalf("get after delete: %v", err)
	}
}

func TestLocalList(t *testing.T) {
	s := NewLocal(t.TempDir())
	ctx := context.Background()

	for _, key := range []string{"b/two", "a/one", "backups/x.zip", "a/sub/three"} {
		if _, err := s.Put(ctx, key, strings.NewReader("v")); err != nil {
			t.Fatal(err)
		}
	}

	all, err := s.List(ctx, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	keys := keysOf(all)
	if !slices.Equal(keys, []string{"a/one", "a/sub/three", "b/two", "backups/x.zip"}) {
		t.Fatalf("all = %v", keys)
	}
	if all[0].Size != 1 {
		t.Fatalf("size = %d, want 1", all[0].Size)
	}
	if all[0].Modified.IsZero() {
		t.Fatal("modified is zero")
	}

	sub, err := s.List(ctx, "a/")
	if err != nil {
		t.Fatalf("list prefix: %v", err)
	}
	if !slices.Equal(keysOf(sub), []string{"a/one", "a/sub/three"}) {
		t.Fatalf("prefix = %v", keysOf(sub))
	}

	empty, err := s.List(ctx, "zzz/")
	if err != nil {
		t.Fatalf("list missing prefix: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("missing prefix = %v", keysOf(empty))
	}
}

func TestKeyValidation(t *testing.T) {
	s := NewLocal(t.TempDir())
	ctx := context.Background()

	for _, key := range []string{
		"", "/abs", "trailing/", "../up", "a/../b", "a//b", ".", "..", `back\slash`,
	} {
		if _, err := s.Put(ctx, key, strings.NewReader("v")); err == nil {
			t.Errorf("Put(%q) accepted", key)
		}
	}

	for _, p := range []string{"ok", "a/b", "prefix/", ""} {
		if !ValidPrefix(p) {
			t.Errorf("ValidPrefix(%q) = false", p)
		}
	}
	for _, p := range []string{"a//b/", "/lead", ".."} {
		if ValidPrefix(p) {
			t.Errorf("ValidPrefix(%q) = true", p)
		}
	}
}

func TestFromEnvLocal(t *testing.T) {
	t.Setenv("STORAGE_BACKEND", "")
	t.Setenv("STORAGE_PATH", t.TempDir())

	fs, err := FromEnv(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if fs.Backend() != BackendLocal {
		t.Fatalf("backend = %q", fs.Backend())
	}
}

func TestFromEnvUnknown(t *testing.T) {
	t.Setenv("STORAGE_BACKEND", "gcs")
	if _, err := FromEnv(slog.New(slog.NewTextHandler(io.Discard, nil))); err == nil {
		t.Fatal("expected error for unknown backend")
	}
}

func TestFromEnvS3MissingConfig(t *testing.T) {
	t.Setenv("STORAGE_BACKEND", "s3")
	_, err := FromEnv(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil || !strings.Contains(err.Error(), "S3_ENDPOINT") {
		t.Fatalf("err = %v, want missing S3_* hint", err)
	}
}

func keysOf(objs []Object) []string {
	out := make([]string, len(objs))
	for i, o := range objs {
		out[i] = o.Key
	}
	return out
}
