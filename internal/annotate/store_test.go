package annotate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAddAssignsIDAndFiresOnChange(t *testing.T) {
	s := NewStore()

	var got Annotation
	calls := 0
	s.OnChange(func(a Annotation) {
		got = a
		calls++
	})

	added := s.Add(Annotation{File: "a.go", Line: 5, Author: Human, Text: "why?"})

	if added.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if calls != 1 {
		t.Fatalf("expected onChange called once, got %d", calls)
	}
	if got.ID != added.ID || got.Text != "why?" {
		t.Fatalf("onChange got %+v, want %+v", got, added)
	}
}

func TestForFileFiltersByFile(t *testing.T) {
	s := NewStore()
	s.Add(Annotation{File: "a.go", Line: 1, Author: Human, Text: "x"})
	s.Add(Annotation{File: "b.go", Line: 1, Author: Human, Text: "y"})
	s.Add(Annotation{File: "a.go", Line: 2, Author: KodeAgent, Text: "z"})

	got := s.ForFile("a.go")
	if len(got) != 2 {
		t.Fatalf("expected 2 annotations for a.go, got %d", len(got))
	}
	for _, a := range got {
		if a.File != "a.go" {
			t.Fatalf("ForFile leaked annotation from %q", a.File)
		}
	}
}

func TestAllReturnsEverything(t *testing.T) {
	s := NewStore()
	s.Add(Annotation{File: "a.go", Line: 1, Author: Human, Text: "x"})
	s.Add(Annotation{File: "b.go", Line: 1, Author: "mcp:claude-code", Text: "y"})

	if len(s.All()) != 2 {
		t.Fatalf("expected 2 total annotations, got %d", len(s.All()))
	}
}

func TestAddPersistsToFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "annotations.json")
	s := NewStore()
	s.SetPersistPath(path)

	s.Add(Annotation{File: "a.go", Line: 1, Author: Human, Text: "hello"})

	list, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Text != "hello" {
		t.Fatalf("expected persisted file to contain the annotation, got %+v", list)
	}
}

func TestReloadPicksUpAnnotationsPushedDirectlyToFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "annotations.json")
	s := NewStore()
	s.SetPersistPath(path)
	s.Add(Annotation{File: "a.go", Line: 1, Author: Human, Text: "existing"})

	// Simulate an agent pushing a note directly to the file, outside of
	// this Store instance entirely (as if written by a separate process).
	pushed := Annotation{File: "a.go", Line: 2, Author: KodeAgent, Text: "pushed note"}
	pushed.ID = computeID(pushed)
	current, _ := LoadFile(path)
	data := append(current, pushed)
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	added, err := s.Reload(path)
	if err != nil {
		t.Fatal(err)
	}
	if added != 1 {
		t.Fatalf("expected 1 newly loaded annotation, got %d", added)
	}
	if len(s.ForFile("a.go")) != 2 {
		t.Fatalf("expected 2 annotations for a.go after reload, got %d", len(s.ForFile("a.go")))
	}

	// Reloading again with no file changes must not duplicate anything.
	added, err = s.Reload(path)
	if err != nil {
		t.Fatal(err)
	}
	if added != 0 {
		t.Fatalf("expected 0 newly loaded annotations on repeat reload, got %d", added)
	}
	if len(s.All()) != 2 {
		t.Fatalf("expected still 2 total annotations after repeat reload, got %d", len(s.All()))
	}
}

func TestReloadFiresOnChangeForNewEntriesOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "annotations.json")
	s := NewStore()
	s.SetPersistPath(path)

	calls := 0
	s.OnChange(func(Annotation) { calls++ })

	s.Add(Annotation{File: "a.go", Line: 1, Author: Human, Text: "one"})
	if calls != 1 {
		t.Fatalf("expected 1 onChange call after Add, got %d", calls)
	}

	if _, err := s.Reload(path); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("reloading unchanged file should not fire onChange again, got %d calls", calls)
	}
}
