// Package storage maps the npm registry concepts — package manifests and
// tarballs — onto the shared blob filesystem. Keys mirror the on-disk
// layout: <package>/package.json and <package>/<name>-<version>.tgz.
package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"server/internal/blob"
)

var ErrNotExist = errors.New("storage: entry does not exist")

const manifestName = "package.json"

type Storage interface {
	GetManifest(ctx context.Context, pkg string) ([]byte, error)
	PutManifest(ctx context.Context, pkg string, data []byte) error
	GetTarball(ctx context.Context, pkg, filename string) (io.ReadCloser, int64, error)
	PutTarball(ctx context.Context, pkg, filename string, data io.Reader) (int64, error)
	DeletePackage(ctx context.Context, pkg string) error
	ListPackages(ctx context.Context) ([]string, error)
	Lock(pkg string) (unlock func())
}

// Store is the npm package store on top of any blob backend. Per-package
// locking is in-process: the registry runs as a single instance.
type Store struct {
	fs      blob.FS
	lockMu  sync.Mutex
	pkgLock map[string]*sync.Mutex
}

func New(fs blob.FS) *Store {
	return &Store{fs: fs, pkgLock: make(map[string]*sync.Mutex)}
}

func (s *Store) GetManifest(ctx context.Context, pkg string) ([]byte, error) {
	key, err := manifestKey(pkg)
	if err != nil {
		return nil, err
	}
	rc, _, err := s.fs.Get(ctx, key)
	if err != nil {
		return nil, mapNotExist(err)
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func (s *Store) PutManifest(ctx context.Context, pkg string, data []byte) error {
	key, err := manifestKey(pkg)
	if err != nil {
		return err
	}
	if _, err := s.fs.Put(ctx, key, bytes.NewReader(data)); err != nil {
		return err
	}
	return nil
}

func (s *Store) GetTarball(ctx context.Context, pkg, filename string) (io.ReadCloser, int64, error) {
	key, err := tarballKey(pkg, filename)
	if err != nil {
		return nil, 0, err
	}
	rc, size, err := s.fs.Get(ctx, key)
	if err != nil {
		return nil, 0, mapNotExist(err)
	}
	return rc, size, nil
}

func (s *Store) PutTarball(ctx context.Context, pkg, filename string, data io.Reader) (int64, error) {
	key, err := tarballKey(pkg, filename)
	if err != nil {
		return 0, err
	}
	return s.fs.Put(ctx, key, data)
}

func (s *Store) ListPackages(ctx context.Context) ([]string, error) {
	objs, err := s.fs.List(ctx, "")
	if err != nil {
		return nil, err
	}
	var pkgs []string
	for _, o := range objs {
		if suffix := "/" + manifestName; strings.HasSuffix(o.Key, suffix) {
			pkgs = append(pkgs, strings.TrimSuffix(o.Key, suffix))
		}
	}
	return pkgs, nil
}

// DeletePackage removes the manifest and all tarballs; a missing package
// is not an error.
func (s *Store) DeletePackage(ctx context.Context, pkg string) error {
	if !validPkg(pkg) {
		return fmt.Errorf("storage: invalid package name %q", pkg)
	}
	objs, err := s.fs.List(ctx, pkg+"/")
	if err != nil {
		return err
	}
	for _, o := range objs {
		if err := s.fs.Delete(ctx, o.Key); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Lock(pkg string) func() {
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

func manifestKey(pkg string) (string, error) {
	if !validPkg(pkg) {
		return "", fmt.Errorf("storage: invalid package name %q", pkg)
	}
	return pkg + "/" + manifestName, nil
}

func tarballKey(pkg, filename string) (string, error) {
	if !validPkg(pkg) {
		return "", fmt.Errorf("storage: invalid package name %q", pkg)
	}
	if !validFilename(filename) || filename == manifestName {
		return "", fmt.Errorf("storage: invalid filename %q", filename)
	}
	return pkg + "/" + filename, nil
}

func mapNotExist(err error) error {
	if errors.Is(err, blob.ErrNotExist) {
		return ErrNotExist
	}
	return err
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
		strings.Contains(name, "/") == false &&
		strings.Contains(name, "\\") == false
}
