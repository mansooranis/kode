package explain

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mansooranis/kode/internal/annotate"
	"github.com/mansooranis/kode/internal/update"
)

func TestRefreshFilesDedupesAndSorts(t *testing.T) {
	s := annotate.NewStore()
	s.Add(annotate.Annotation{File: "b.go", Line: 1, Author: annotate.Human, Text: "x"})
	s.Add(annotate.Annotation{File: "a.go", Line: 1, Author: annotate.Human, Text: "y"})
	s.Add(annotate.Annotation{File: "a.go", Line: 2, Author: annotate.Human, Text: "z"})

	a := NewApp(s, "/tmp/does-not-exist.json")

	if len(a.files) != 2 {
		t.Fatalf("expected 2 distinct files, got %d: %v", len(a.files), a.files)
	}
	if a.files[0] != "a.go" || a.files[1] != "b.go" {
		t.Fatalf("expected sorted [a.go b.go], got %v", a.files)
	}
}

func TestRenderNoteCardDiagramNotWrapped(t *testing.T) {
	a := annotate.Annotation{
		Author: annotate.KodeAgent,
		Kind:   annotate.KindDiagram,
		Text:   "+---+\n| A |\n+---+",
	}
	lines := renderNoteCard(a, 40)

	var joined strings.Builder
	for _, l := range lines {
		joined.WriteString(l)
		joined.WriteString("\n")
	}
	if !strings.Contains(joined.String(), "+---+") {
		t.Fatalf("expected raw diagram art preserved verbatim, got:\n%s", joined.String())
	}
}

func TestUpdateAvailableShowsBottomRightBanner(t *testing.T) {
	s := annotate.NewStore()
	a := NewApp(s, "/tmp/does-not-exist.json")

	m, _ := a.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	a = m.(App)

	m, _ = a.Update(update.AvailableMsg{Latest: "v9.9.9"})
	a = m.(App)

	footer := lastLine(a.View())
	if !strings.Contains(footer, "v9.9.9") {
		t.Fatalf("expected the update banner on the footer's own row, got footer:\n%q", footer)
	}
	if !strings.Contains(footer, "navigate") {
		t.Fatalf("expected the keybinding hints to still be visible alongside the banner, got footer:\n%q", footer)
	}
	if !strings.HasSuffix(strings.TrimRight(footer, " "), "v9.9.9 available") {
		t.Fatalf("expected the banner flush against the right edge, got footer:\n%q", footer)
	}
}

func lastLine(s string) string {
	lines := strings.Split(s, "\n")
	return lines[len(lines)-1]
}

func TestRenderSnippetOutOfRangeReturnsNil(t *testing.T) {
	if got := renderSnippet([]string{"a", "b"}, 0); got != nil {
		t.Fatalf("expected nil for line 0, got %v", got)
	}
	if got := renderSnippet([]string{"a", "b"}, 5); got != nil {
		t.Fatalf("expected nil for out-of-range line, got %v", got)
	}
	if got := renderSnippet(nil, 1); got != nil {
		t.Fatalf("expected nil when source is unreadable, got %v", got)
	}
}
