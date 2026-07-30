package bundled

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSyncWritesFlatMarkdownFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "skills")

	names, err := Sync(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) == 0 {
		t.Fatal("expected at least one bundled skill")
	}

	for _, name := range names {
		path := filepath.Join(dir, name+".md")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("expected %s to exist: %v", path, err)
		}
		if len(data) == 0 {
			t.Fatalf("expected %s to be non-empty", path)
		}
	}
}

func TestEnsureSyncedSkipsUnchangedVersion(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "skills")

	synced, _, err := EnsureSynced(dir, "1.0.0", false)
	if err != nil {
		t.Fatal(err)
	}
	if !synced {
		t.Fatal("expected first EnsureSynced call to sync")
	}

	synced, _, err = EnsureSynced(dir, "1.0.0", false)
	if err != nil {
		t.Fatal(err)
	}
	if synced {
		t.Fatal("expected EnsureSynced to no-op for an unchanged version")
	}

	synced, _, err = EnsureSynced(dir, "2.0.0", false)
	if err != nil {
		t.Fatal(err)
	}
	if !synced {
		t.Fatal("expected EnsureSynced to sync again after a version bump")
	}

	synced, _, err = EnsureSynced(dir, "2.0.0", true)
	if err != nil {
		t.Fatal(err)
	}
	if !synced {
		t.Fatal("expected force=true to sync even with an unchanged version")
	}
}
