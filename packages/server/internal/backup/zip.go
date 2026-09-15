package backup

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"server/internal/blob"
)

func addFile(zw *zip.Writer, name, path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return copyZipEntry(zw, name, f, info.Size(), info.ModTime())
}

func addObject(zw *zip.Writer, name string, ctx context.Context, src blob.FS, key string, size int64) error {
	rc, _, err := src.Get(ctx, key)
	if err != nil {
		return fmt.Errorf("backup: fetch %q: %w", key, err)
	}
	defer rc.Close()
	return copyZipEntry(zw, name, rc, size, time.Time{})
}

func copyZipEntry(zw *zip.Writer, name string, r io.Reader, size int64, modified time.Time) error {
	header := &zip.FileHeader{Name: name, Method: zip.Deflate}
	header.SetModTime(modified)
	header.SetMode(0o644)
	w, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, r)
	return err
}

// extractZip unpacks archive into dir, rejecting unsafe entry names.
func extractZip(zipPath, dir string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("backup: open archive: %w", err)
	}
	defer reader.Close()

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, f := range reader.File {
		name, err := safeZipPath(f.Name)
		if err != nil {
			return err
		}
		target := filepath.Join(dir, name)
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		src, err := f.Open()
		if err != nil {
			return err
		}
		dst, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			src.Close()
			return err
		}
		_, copyErr := io.Copy(dst, src)
		closeErr := dst.Close()
		src.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func safeZipPath(name string) (string, error) {
	name = filepath.ToSlash(name)
	if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, "\\") {
		return "", fmt.Errorf("backup: unsafe archive path %q", name)
	}
	for seg := range strings.SplitSeq(name, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return "", fmt.Errorf("backup: unsafe archive path %q", name)
		}
	}
	return name, nil
}

// swapLocalRoot replaces the package objects under the local root with the
// restored tree. The backups directory, the database and the audit log —
// which may live inside the same root (the default layout) — are not
// restore-owned and survive. Entries not restored are kept in "<root>.old" until
// the copy succeeds, so a failure leaves the data recoverable.
func (s *Service) swapLocalRoot(restored string) error {
	old := s.localRoot + ".old"
	if err := os.RemoveAll(old); err != nil {
		return err
	}
	if err := os.MkdirAll(old, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(s.localRoot)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	for _, e := range entries {
		full := filepath.Join(s.localRoot, e.Name())
		if s.preservedByRestore(full) {
			continue
		}
		if err := os.Rename(full, filepath.Join(old, e.Name())); err != nil {
			return err
		}
	}
	if err := copyTree(restored, s.localRoot); err != nil {
		return err
	}
	os.RemoveAll(old)
	return nil
}

// preservedByRestore reports whether path is managed outside the package
// storage swap: the backups directory, the SQLite database (+ sidecars)
// and the audit log.
func (s *Service) preservedByRestore(path string) bool {
	if path == filepath.Join(s.localRoot, "backups") {
		return true
	}
	if path == s.dbPath || path == s.auditPath {
		return true
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if path == s.dbPath+suffix {
			return true
		}
	}
	return false
}

func copyTree(srcDir, dstDir string) error {
	return filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dstDir, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		src, err := os.Open(path)
		if err != nil {
			return err
		}
		defer src.Close()
		info, err := d.Info()
		if err != nil {
			return err
		}
		dst, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(dst, src)
		closeErr := dst.Close()
		return errors.Join(copyErr, closeErr)
	})
}
