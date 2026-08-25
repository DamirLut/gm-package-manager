package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const manifestName = "package.json"

type Local struct {
	root    string
	lockMu  sync.Mutex
	pkgLock map[string]*sync.Mutex
}

func NewLocal(root string) *Local {
	return &Local{root: root, pkgLock: make(map[string]*sync.Mutex)}
}

func (s *Local) Root() string { return s.root }

func (s *Local) GetManifest(_ context.Context, pkg string) ([]byte, error) {
	if !validPkg(pkg) {
		return nil, ErrNotExist
	}
	data, err := os.ReadFile(filepath.Join(s.root, filepath.FromSlash(pkg), manifestName))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotExist
	}
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (s *Local) PutManifest(_ context.Context, pkg string, data []byte) error {
	if !validPkg(pkg) {
		return fmt.Errorf("storage: invalid package name %q", pkg)
	}
	_, err := s.putFile(pkg, manifestName, bytes.NewReader(data))
	return err
}

func (s *Local) GetTarball(_ context.Context, pkg, filename string) (io.ReadCloser, int64, error) {
	if !validPkg(pkg) || !validFilename(filename) {
		return nil, 0, ErrNotExist
	}
	f, err := os.Open(filepath.Join(s.root, filepath.FromSlash(pkg), filename))
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

func (s *Local) PutTarball(_ context.Context, pkg, filename string, data io.Reader) (int64, error) {
	if !validPkg(pkg) {
		return 0, fmt.Errorf("storage: invalid package name %q", pkg)
	}
	if !validFilename(filename) {
		return 0, fmt.Errorf("storage: invalid filename %q", filename)
	}
	return s.putFile(pkg, filename, data)
}

func (s *Local) ListPackages(_ context.Context) ([]string, error) {
	var pkgs []string
	err := filepath.WalkDir(s.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != manifestName {
			return nil
		}
		rel, err := filepath.Rel(s.root, filepath.Dir(path))
		if err != nil {
			return err
		}
		pkgs = append(pkgs, filepath.ToSlash(rel))
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return pkgs, nil
	}
	if err != nil {
		return nil, err
	}
	return pkgs, nil
}

// DeletePackage removes the manifest, all tarballs and the package
// directory itself; an emptied scope directory is cleaned up best-effort.
func (s *Local) DeletePackage(_ context.Context, pkg string) error {
	if !validPkg(pkg) {
		return fmt.Errorf("storage: invalid package name %q", pkg)
	}
	dir := filepath.Join(s.root, filepath.FromSlash(pkg))
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	os.Remove(filepath.Dir(dir))
	return nil
}

func (s *Local) Lock(pkg string) func() {
	s.lockMu.Lock()
	mu, ok := s.pkgLock[pkg]
	if !ok {
		mu = &sync.Mutex{}
		s.pkgLock[pkg] = mu
	}
	s.lockMu.Unlock()

	mu.Lock()
	return mu.Unlock
}

func (s *Local) putFile(pkg, name string, data io.Reader) (int64, error) {
	dir := filepath.Join(s.root, filepath.FromSlash(pkg))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, err
	}
	tmp, err := os.CreateTemp(dir, "."+name+"-*")
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
	if err := os.Rename(tmpName, filepath.Join(dir, name)); err != nil {
		return n, err
	}
	return n, nil
}

func validPkg(pkg string) bool {
	if pkg == "" {
		return false
	}
	for seg := range strings.SplitSeq(pkg, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return false
		}
	}
	return true
}

func validFilename(name string) bool {
	return name != "" &&
		name != "." &&
		name != ".." &&
		filepath.Base(name) == name
}
