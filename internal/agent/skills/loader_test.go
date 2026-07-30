package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadParsesFrontmatterAndBody(t *testing.T) {
	dir := t.TempDir()
	content := "---\nname: explain-diff\ndescription: Explains a diff hunk in plain English.\ntriggers:\n  - explain\n---\nFull instructions go here.\n"
	if err := os.WriteFile(filepath.Join(dir, "explain-diff.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	lib, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}

	summaries := lib.Summaries()
	if len(summaries) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(summaries))
	}
	if summaries[0].Name != "explain-diff" || summaries[0].Description != "Explains a diff hunk in plain English." {
		t.Fatalf("unexpected summary: %+v", summaries[0])
	}

	full, ok := lib.Get("explain-diff")
	if !ok {
		t.Fatal("expected to find skill by name")
	}
	if full.Body != "Full instructions go here.\n" {
		t.Fatalf("unexpected body: %q", full.Body)
	}
}

func TestLoadMissingDirReturnsEmptyLibrary(t *testing.T) {
	lib, err := Load(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatal(err)
	}
	if len(lib.Summaries()) != 0 {
		t.Fatalf("expected empty library, got %d skills", len(lib.Summaries()))
	}
}
