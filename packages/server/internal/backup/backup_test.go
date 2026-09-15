package backup

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"server/internal/blob"
)

type fixture struct {
	svc     *Service
	fs      blob.FS
	db      *sql.DB
	dbPath  string
	root    string
	audit   string
	message string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dir := t.TempDir()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	dbPath := filepath.Join(dir, "metadata.db")
	db, err := sql.Open("sqlite", "file:"+dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec("CREATE TABLE t (v TEXT); INSERT INTO t VALUES ('before')"); err != nil {
		t.Fatalf("seed db: %v", err)
	}

	root := filepath.Join(dir, "packages")
	fs := blob.NewLocal(root)
	auditPath := filepath.Join(dir, "audit.jsonl")
	if err := os.WriteFile(auditPath, []byte("{\"action\":\"x\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	return &fixture{
		svc:     New(fs, db, dbPath, auditPath, root, log),
		fs:      fs,
		db:      db,
		dbPath:  dbPath,
		root:    root,
		audit:   auditPath,
		message: "archive payload",
	}
}

func (f *fixture) seedPackage(t *testing.T, pkg, version string) {
	t.Helper()
	ctx := context.Background()
	if _, err := f.fs.Put(ctx, pkg+"/"+pkg+"-"+version+".tgz", strings.NewReader(f.message)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.fs.Put(ctx, pkg+"/package.json", strings.NewReader(`{"name":"`+pkg+`"}`)); err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) mustCreate(t *testing.T, name string) {
	t.Helper()
	if err := f.svc.Create(context.Background(), name); err != nil {
		t.Fatalf("create backup: %v", err)
	}
}

func TestCreateListDownload(t *testing.T) {
	f := newFixture(t)
	f.seedPackage(t, "@acme/lib", "1.0.0")
	f.mustCreate(t, "snap-1")

	backups, err := f.svc.List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(backups) != 1 || backups[0].Name != "snap-1" || backups[0].Size == 0 {
		t.Fatalf("backups = %+v", backups)
	}

	rc, size, err := f.svc.Get(context.Background(), "snap-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer rc.Close()
	if size != backups[0].Size {
		t.Fatalf("download size = %d, want %d", size, backups[0].Size)
	}
	names := zipEntryNames(t, rc)
	want := []string{"audit.jsonl", "metadata.db", "storage/@acme/lib/@acme/lib-1.0.0.tgz", "storage/@acme/lib/package.json"}
	slicesEqual(t, names, want)
}

func TestCreateDefaultNameAndInvalidNames(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	if err := f.svc.Create(ctx, ""); err != nil {
		t.Fatalf("create default name: %v", err)
	}
	backups, err := f.svc.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 1 || !strings.HasPrefix(backups[0].Name, "20") {
		t.Fatalf("expected timestamp name, got %+v", backups)
	}

	for _, name := range []string{"..", "a/b", "спейс", "with space"} {
		if err := f.svc.Create(ctx, name); !errors.Is(err, ErrInvalidName) {
			t.Errorf("Create(%q) err = %v, want ErrInvalidName", name, err)
		}
		if _, _, err := f.svc.Get(ctx, name); !errors.Is(err, ErrInvalidName) {
			t.Errorf("Get(%q) err = %v, want ErrInvalidName", name, err)
		}
	}
}

// The archive must not contain the live DB files, the audit log twice or
// previous backups — only the snapshot, the audit log and the packages.
func TestCreateExclusions(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.seedPackage(t, "lib", "1.0.0")

	// junk that must not end up in the archive
	f.mustCreate(t, "old")
	if _, err := f.fs.Put(ctx, "metadata.db-wal", strings.NewReader("wal")); err != nil {
		t.Fatal(err)
	}
	if _, err := f.fs.Put(ctx, "audit.jsonl", strings.NewReader("{\"tampered\"}")); err != nil {
		t.Fatal(err)
	}

	f.mustCreate(t, "snap")
	rc, _, err := f.svc.Get(context.Background(), "snap")
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	names := zipEntryNames(t, rc)
	for _, name := range names {
		if strings.HasPrefix(name, "storage/backups/") || strings.HasPrefix(name, "storage/metadata.db") || name == "storage/audit.jsonl" {
			t.Errorf("archive contains excluded entry %q", name)
		}
	}
	if !slicesContain(names, "metadata.db") || !slicesContain(names, "audit.jsonl") {
		t.Fatalf("archive entries = %v", names)
	}
}

func TestCreateS3BackendSkipsStorage(t *testing.T) {
	f := newFixture(t)
	f.svc.fs = newMemFS()
	f.seedPackage(t, "lib", "1.0.0")

	f.mustCreate(t, "snap")
	rc, _, err := f.svc.Get(context.Background(), "snap")
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	names := zipEntryNames(t, rc)
	if slicesContain(names, "storage/lib/package.json") {
		t.Fatalf("remote objects must not be archived, entries = %v", names)
	}
	if !slicesContain(names, "metadata.db") {
		t.Fatalf("entries = %v", names)
	}
}

func TestDelete(t *testing.T) {
	f := newFixture(t)
	f.mustCreate(t, "snap")

	if err := f.svc.Delete(context.Background(), "snap"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := f.svc.Delete(context.Background(), "snap"); err != nil {
		t.Fatalf("repeat delete should be a no-op: %v", err)
	}
	if _, _, err := f.svc.Get(context.Background(), "snap"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get after delete = %v, want ErrNotFound", err)
	}
}

func TestRestoreSwapsDatabaseStorageAndAudit(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.seedPackage(t, "lib", "1.0.0")
	f.mustCreate(t, "snap")

	// damage everything after the backup was taken
	if err := f.fs.Delete(ctx, "lib/package.json"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec("UPDATE t SET v = 'after'"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.audit, []byte("{\"rotated\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	shutdown := make(chan struct{}, 1)
	f.svc.OnShutdown(func() { shutdown <- struct{}{} })
	if err := f.svc.Restore(ctx, "snap"); err != nil {
		t.Fatalf("restore: %v", err)
	}
	select {
	case <-shutdown:
	default:
		t.Fatal("restore did not trigger shutdown")
	}
	if !f.svc.PendingRestore() {
		t.Fatal("no pending restore after Restore")
	}

	if err := f.svc.Restore(ctx, "snap"); err == nil {
		t.Fatal("second Restore while pending must fail")
	}

	// main's sequence: checkpoint, close, then swap
	if _, err := f.db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		t.Fatal(err)
	}
	f.db.Close()
	if err := f.svc.ApplyRestore(); err != nil {
		t.Fatalf("apply restore: %v", err)
	}

	reopened, err := sql.Open("sqlite", "file:"+f.dbPath+"?_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	var v string
	if err := reopened.QueryRow("SELECT v FROM t").Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v != "before" {
		t.Fatalf("restored db value = %q, want %q", v, "before")
	}

	if _, _, err := f.fs.Get(ctx, "lib/package.json"); err != nil {
		t.Fatalf("restored manifest missing: %v", err)
	}
	auditData, err := os.ReadFile(f.audit)
	if err != nil || string(auditData) != "{\"action\":\"x\"}\n" {
		t.Fatalf("restored audit = %q, err = %v", auditData, err)
	}

	if _, err := os.Stat(f.root + ".old"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("old storage tree not cleaned: %v", err)
	}
	if f.svc.PendingRestore() {
		t.Error("pending restore not cleared after ApplyRestore")
	}
}

func TestRestoreWithoutArchive(t *testing.T) {
	f := newFixture(t)
	f.svc.OnShutdown(func() {})
	if err := f.svc.Restore(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// The default layout nests the database, audit log and backups INSIDE the
// local storage root; a restore must swap only the package objects and
// leave those in place.
func TestRestoreNestedLayoutPreservesDBAuditAndBackups(t *testing.T) {
	dir := t.TempDir()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	dbPath := filepath.Join(dir, "data", "metadata.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE t (v TEXT); INSERT INTO t VALUES ('before')"); err != nil {
		t.Fatal(err)
	}

	root := filepath.Join(dir, "data")
	fs := blob.NewLocal(root)
	svc := New(fs, db, dbPath, filepath.Join(dir, "data", "audit.jsonl"), root, log)
	ctx := context.Background()

	if _, err := fs.Put(ctx, "lib/package.json", strings.NewReader(`{}`)); err != nil {
		t.Fatal(err)
	}
	if err := svc.Create(ctx, "snap"); err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Put(ctx, "backups/manual.zip", strings.NewReader("manual")); err != nil {
		t.Fatal(err)
	}

	// damage everything
	if err := fs.Delete(ctx, "lib/package.json"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE t SET v = 'after'"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "backups", "manual.zip"), []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc.OnShutdown(func() {})
	if err := svc.Restore(ctx, "snap"); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if _, err := db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if err := svc.ApplyRestore(); err != nil {
		t.Fatalf("apply restore: %v", err)
	}

	if _, _, err := fs.Get(ctx, "lib/package.json"); err != nil {
		t.Fatalf("package object not restored: %v", err)
	}
	manual, err := os.ReadFile(filepath.Join(root, "backups", "manual.zip"))
	if err != nil || string(manual) != "tampered" {
		t.Fatalf("backups dir not preserved: %q, err = %v", manual, err)
	}

	reopened, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	var v string
	if err := reopened.QueryRow("SELECT v FROM t").Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v != "before" {
		t.Fatalf("restored db value = %q, want %q", v, "before")
	}
	if _, err := os.Stat(root + ".old"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("old entries not cleaned: %v", err)
	}
}

func TestPruneAutoKeepsManualBackups(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	f.mustCreate(t, "auto-1")
	time.Sleep(10 * time.Millisecond)
	f.mustCreate(t, "manual")
	time.Sleep(10 * time.Millisecond)
	f.mustCreate(t, "auto-2")
	time.Sleep(10 * time.Millisecond)
	f.mustCreate(t, "auto-3")

	if err := f.svc.pruneAuto(ctx, 2); err != nil {
		t.Fatalf("prune: %v", err)
	}
	backups, err := f.svc.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(backups))
	for _, b := range backups {
		names = append(names, b.Name)
	}
	if !slicesContain(names, "manual") {
		t.Errorf("manual backup pruned: %v", names)
	}
	if len(names) != 3 || slicesContain(names, "auto-1") {
		t.Errorf("expected auto-1 pruned, got %v", names)
	}
}

func TestSchedulerInvalidSpec(t *testing.T) {
	f := newFixture(t)
	if err := f.svc.StartScheduler("not a cron", 0); err == nil {
		t.Fatal("expected error for invalid cron spec")
	}
}

// --- helpers ---

func zipEntryNames(t *testing.T, r io.Reader) []string {
	t.Helper()
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	names := make([]string, 0, len(zr.File))
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	sortStrings(names)
	return names
}

func slicesContain(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

func sortStrings(list []string) {
	for i := 1; i < len(list); i++ {
		for j := i; j > 0 && list[j] < list[j-1]; j-- {
			list[j], list[j-1] = list[j-1], list[j]
		}
	}
}

func slicesEqual(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

// memFS stands in for the S3 backend; only Backend() and generic object
// operations are used by the service under test.
type memFS struct {
	backend string
	objs    map[string][]byte
}

func newMemFS() *memFS { return &memFS{backend: blob.BackendS3, objs: map[string][]byte{}} }

func (m *memFS) Backend() string { return m.backend }

func (m *memFS) Put(_ context.Context, key string, data io.Reader) (int64, error) {
	b, err := io.ReadAll(data)
	if err != nil {
		return 0, err
	}
	m.objs[key] = b
	return int64(len(b)), nil
}

func (m *memFS) Get(_ context.Context, key string) (io.ReadCloser, int64, error) {
	b, ok := m.objs[key]
	if !ok {
		return nil, 0, blob.ErrNotExist
	}
	return io.NopCloser(bytes.NewReader(b)), int64(len(b)), nil
}

func (m *memFS) Delete(_ context.Context, key string) error {
	delete(m.objs, key)
	return nil
}

func (m *memFS) List(_ context.Context, prefix string) ([]blob.Object, error) {
	var objs []blob.Object
	for key, data := range m.objs {
		if strings.HasPrefix(key, prefix) {
			objs = append(objs, blob.Object{Key: key, Size: int64(len(data))})
		}
	}
	return objs, nil
}
