package contracts

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A rename is a directory modification, so the directory is synced too. A
// caller told its write succeeded and finding no file after power loss is the
// failure this prevents; no black-box test can observe that, so what is
// asserted here is that the sync happens and its errors are surfaced.
func TestWriteAtomicSyncsTheDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "record.pb")
	if err := WriteAtomic(path, []byte("payload")); err != nil {
		t.Fatalf("WriteAtomic: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "payload" {
		t.Errorf("content = %q, want payload", got)
	}
	// No temporary file is left behind to be mistaken for a record.
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			t.Errorf("temporary file %s survived the write", e.Name())
		}
	}
	if err := syncDir(filepath.Join(dir, "does-not-exist")); err == nil {
		t.Error("syncing a missing directory reported success")
	}
}

// Two directories are created here, and both are new names in their own
// parents. Syncing only the innermost would leave the record reachable through
// a path that did not survive.
func TestWriteAtomicSyncsEveryDirectoryItCreates(t *testing.T) {
	var synced []string
	original := syncDirectory
	syncDirectory = func(d string) error {
		synced = append(synced, d)
		return nil
	}
	defer func() { syncDirectory = original }()

	root := t.TempDir()
	deep := filepath.Join(root, "a", "b")
	if err := WriteAtomic(filepath.Join(deep, "record.pb"), []byte("payload")); err != nil {
		t.Fatalf("WriteAtomic: %v", err)
	}
	for _, want := range []string{deep, filepath.Join(root, "a"), root} {
		found := false
		for _, got := range synced {
			if got == want {
				found = true
			}
		}
		if !found {
			t.Errorf("%s was never synced (synced %v)", want, synced)
		}
	}
}

func TestWriteAtomicReportsAFailedDirectorySync(t *testing.T) {
	var synced string
	original := syncDirectory
	syncDirectory = func(d string) error {
		synced = d
		return errBoom
	}
	defer func() { syncDirectory = original }()

	dir := t.TempDir()
	err := WriteAtomic(filepath.Join(dir, "record.pb"), []byte("payload"))
	if synced == "" {
		t.Fatal("the containing directory was never synced")
	}
	if synced != dir {
		t.Errorf("synced %q, want %q", synced, dir)
	}
	// A write reported as successful has to have been durably named.
	if !errors.Is(err, errBoom) {
		t.Errorf("err = %v, want the sync failure surfaced", err)
	}
}

var errBoom = errors.New("sync refused")
