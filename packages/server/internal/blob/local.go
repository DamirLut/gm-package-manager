package blob

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Local is the FS driver over a directory: each object is a plain file
// named by its key, written atomically via a temp file and rename.
type Local struct {
	root string
}

func NewLocal(root string) *Local { return &Local{root: root} }

func (s *Local) Backend() string { return BackendLocal }

func (s *Local) Put(_ context.Context, key string, data io.Reader) (int64, error) {
	path, err := s.resolve(key)
	if err != nil {
		return 0, err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+"-*")
	if err != nil {
		return 0, err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	n, copyErr := io.Copy(tmp, data)
	closeErr := tmp.Close()
	if copyErr != nil || closeErr != nil {
		return n, errors.Join(copyErr, closeErr)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return n, err
	}
	return n, nil
}

func (s *Local) Get(_ context.Context, key string) (io.ReadCloser, int64, error) {
	path, err := s.resolve(key)
	if err != nil {
		return nil, 0, err
	}
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, 0, ErrNotExist
	}
	if err != nil {
		return nil, 0, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, 0, err
	}
	if info.IsDir() {
		f.Close()
		return nil, 0, ErrNotExist
	}
	return f, info.Size(), nil
}

func (s *Local) Delete(_ context.Context, key string) error {
	path, err := s.resolve(key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func (s *Local) List(_ context.Context, prefix string) ([]Object, error) {
	if !ValidPrefix(prefix) {
		return nil, fmt.Errorf("blob: invalid prefix %q", prefix)
	}
	var objs []Object
	err := filepath.WalkDir(s.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(s.root, path)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(rel)
		if strings.HasPrefix(key, prefix) {
			info, err := d.Info()
			if err != nil {
				return err
			}
			objs = append(objs, Object{Key: key, Size: info.Size(), Modified: info.ModTime()})
		}
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return objs, nil
	}
	if err != nil {
		return nil, err
	}
	sortObjects(objs)
	return objs, nil
}

func (s *Local) resolve(key string) (string, error) {
	if !ValidKey(key) {
		return "", fmt.Errorf("blob: invalid key %q", key)
	}
	return filepath.Join(s.root, filepath.FromSlash(key)), nil
}
