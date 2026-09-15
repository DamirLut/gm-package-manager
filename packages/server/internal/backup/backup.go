package backup

import (
	"archive/zip"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"server/internal/blob"
)

var (
	ErrNotFound     = errors.New("backup: not found")
	ErrInvalidName  = errors.New("backup: invalid name")
	ErrNotAvailable = errors.New("backup: restore is not available")
)

const (
	prefix   = "backups/"
	dbName   = "metadata.db"
	auditLog = "audit.jsonl"
)

// Backup is a stored archive without the .zip suffix in Name.
type Backup struct {
	Name     string    `json:"name"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
}

type Service struct {
	fs        blob.FS
	db        *sql.DB
	dbPath    string
	auditPath string
	localRoot string
	log       *slog.Logger

	// shutdown is called by Restore to start the graceful stop; main
	// applies the pending restore after the DB and audit log are closed.
	shutdown func()
	pending  atomic.Pointer[string]

	cronStoper func()
}

func New(fs blob.FS, db *sql.DB, dbPath, auditPath, localRoot string, log *slog.Logger) *Service {
	return &Service{fs: fs, db: db, dbPath: dbPath, auditPath: auditPath, localRoot: localRoot, log: log}
}

// OnShutdown wires the callback Restore uses to begin the process stop.
func (s *Service) OnShutdown(fn func()) { s.shutdown = fn }

// Create archives the current data and uploads it as backups/<name>.zip.
// An empty name becomes the current UTC timestamp.
func (s *Service) Create(ctx context.Context, name string) error {
	if name == "" {
		name = time.Now().UTC().Format("2006-01-02T15-04-05Z")
	}
	if !validName(name) {
		return fmt.Errorf("%w: %q", ErrInvalidName, name)
	}

	tmpDir, err := os.MkdirTemp("", "gmpm-backup-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	snapshot := filepath.Join(tmpDir, dbName)
	if _, err := s.db.ExecContext(ctx, "VACUUM INTO ?", snapshot); err != nil {
		return fmt.Errorf("backup: sqlite snapshot: %w", err)
	}

	zipPath := filepath.Join(tmpDir, "backup.zip")
	if err := s.buildZip(ctx, zipPath, snapshot); err != nil {
		return err
	}

	f, err := os.Open(zipPath)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := s.fs.Put(ctx, prefix+name+".zip", f); err != nil {
		return fmt.Errorf("backup: upload: %w", err)
	}
	return nil
}

func (s *Service) List(ctx context.Context) ([]Backup, error) {
	objs, err := s.fs.List(ctx, prefix)
	if err != nil {
		return nil, err
	}
	backups := make([]Backup, 0, len(objs))
	for _, o := range objs {
		name := strings.TrimSuffix(strings.TrimPrefix(o.Key, prefix), ".zip")
		if name == "" {
			continue
		}
		backups = append(backups, Backup{Name: name, Size: o.Size, Modified: o.Modified})
	}
	return backups, nil
}

// Get streams the zip archive for download.
func (s *Service) Get(ctx context.Context, name string) (io.ReadCloser, int64, error) {
	key, err := s.key(name)
	if err != nil {
		return nil, 0, err
	}
	rc, size, err := s.fs.Get(ctx, key)
	if err != nil {
		if errors.Is(err, blob.ErrNotExist) {
			return nil, 0, ErrNotFound
		}
		return nil, 0, err
	}
	return rc, size, nil
}

func (s *Service) Delete(ctx context.Context, name string) error {
	key, err := s.key(name)
	if err != nil {
		return err
	}
	if err := s.fs.Delete(ctx, key); err != nil {
		return err
	}
	return nil
}

// Restore stages the archive and starts the graceful shutdown; the actual
// file swap happens in ApplyRestore once main closed the DB and audit log.
func (s *Service) Restore(ctx context.Context, name string) error {
	if s.shutdown == nil {
		return ErrNotAvailable
	}
	if s.pending.Load() != nil {
		return errors.New("backup: restore already pending")
	}

	tmpDir, err := os.MkdirTemp("", "gmpm-restore-")
	if err != nil {
		return err
	}
	cleanup := func() { os.RemoveAll(tmpDir) }

	rc, _, err := s.Get(ctx, name)
	if err != nil {
		cleanup()
		return err
	}
	zipPath := filepath.Join(tmpDir, "restore.zip")
	if err := save(rc, zipPath); err != nil {
		cleanup()
		return fmt.Errorf("backup: download: %w", err)
	}
	dataDir := filepath.Join(tmpDir, "data")
	if err := extractZip(zipPath, dataDir); err != nil {
		cleanup()
		return err
	}
	if _, err := os.Stat(filepath.Join(dataDir, dbName)); err != nil {
		cleanup()
		return fmt.Errorf("backup: archive has no %s", dbName)
	}

	s.pending.Store(&tmpDir)
	s.shutdown()
	return nil
}

// PendingRestore reports the staged restore, if any.
func (s *Service) PendingRestore() bool { return s.pending.Load() != nil }

// ApplyRestore swaps the database file, the audit log and — for local
// blob storage — the storage tree. It runs after the process stopped
// serving: the DB and audit log must already be closed.
func (s *Service) ApplyRestore() error {
	dir := s.pending.Swap(nil)
	if dir == nil {
		return errors.New("backup: no restore pending")
	}
	defer os.RemoveAll(*dir)
	data := filepath.Join(*dir, "data")

	// a stale WAL would resurrect pre-restore writes over the new file
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := os.Remove(s.dbPath + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("backup: remove %s%s: %w", s.dbPath, suffix, err)
		}
	}
	if err := os.Rename(filepath.Join(data, dbName), s.dbPath); err != nil {
		return fmt.Errorf("backup: swap database: %w", err)
	}

	if s.auditPath != "" {
		if err := os.Rename(filepath.Join(data, auditLog), s.auditPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("backup: swap audit log: %w", err)
		}
	}

	storageDir := filepath.Join(data, "storage")
	if info, err := os.Stat(storageDir); err != nil || !info.IsDir() {
		return nil
	}
	if err := s.swapLocalRoot(storageDir); err != nil {
		return fmt.Errorf("backup: swap storage: %w", err)
	}
	return nil
}

func (s *Service) buildZip(ctx context.Context, zipPath, dbSnapshot string) error {
	f, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)

	if err := addFile(zw, dbName, dbSnapshot); err != nil {
		return err
	}
	if s.auditPath != "" {
		if _, err := os.Stat(s.auditPath); err == nil {
			if err := addFile(zw, auditLog, s.auditPath); err != nil {
				return err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}

	// With a remote blob backend the objects already live outside this
	// machine (PocketBase does the same); the archive holds only the DB.
	if s.fs.Backend() == blob.BackendLocal {
		objs, err := s.fs.List(ctx, "")
		if err != nil {
			return err
		}
		for _, o := range objs {
			if strings.HasPrefix(o.Key, prefix) || isDBFile(o.Key) || o.Key == auditLog {
				continue
			}
			if err := addObject(zw, "storage/"+o.Key, ctx, s.fs, o.Key, o.Size); err != nil {
				return err
			}
		}
	}

	return zw.Close()
}

// isDBFile keeps the live database files out of the walk: the archive
// carries the VACUUM INTO snapshot instead.
func isDBFile(key string) bool {
	return key == dbName || strings.HasPrefix(key, dbName+"-")
}

func (s *Service) key(name string) (string, error) {
	if !validName(name) {
		return "", fmt.Errorf("%w: %q", ErrInvalidName, name)
	}
	return prefix + name + ".zip", nil
}

func validName(name string) bool {
	if name == "" || len(name) > 128 {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
		default:
			return false
		}
	}
	return name != "." && name != ".."
}

func save(rc io.ReadCloser, path string) error {
	defer rc.Close()
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(f, rc)
	closeErr := f.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
