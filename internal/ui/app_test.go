package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mansooranis/kode/internal/annotate"
	"github.com/mansooranis/kode/internal/config"
	"github.com/mansooranis/kode/internal/diffparse"
)

func testChangeset() diffparse.Changeset {
	return diffparse.Changeset{Files: []diffparse.FileDiff{{
		NewName: "example.go",
		Hunks: []diffparse.Hunk{{
			Header: "@@ -1,1 +1,2 @@",
			Lines: []diffparse.Line{
				{Op: diffparse.OpContext, OldLineNo: 1, NewLineNo: 1, Content: "package main"},
				{Op: diffparse.OpAdd, NewLineNo: 2, Content: "// new line"},
			},
		}},
	}}}
}

// TestMouseClickOnDiffButtonRowOpensDraft guards the title-row offset bug:
// diffview.Update expects mouse Y relative to ITS OWN row 0, but app.go's
// diffBox spans the same rows as the pane's title (rendered separately, one
// row above diffview.View()). Forwarding raw coordinates without subtracting
// titleHeight put every click one row below where the user actually clicked
// — silently breaking "[+]"/"[Reply]" for anyone driving the real mouse
// (the diffview-package tests never caught this since they call
// diffview.Update directly, bypassing app.go's coordinate translation).
func TestMouseClickOnDiffButtonRowOpensDraft(t *testing.T) {
	store := annotate.NewStore()
	app := NewApp(config.Default(), testChangeset(), store, t.TempDir()+"/annotations.json")

	m, _ := app.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	app = m.(App)

	// diffview's own rows: 0 = hunk header, 1 = context line (ordinal 0),
	// 2 = add line (ordinal 1, which gets a "[+]" button). Its row 0 renders
	// at absolute screen row (diffBox.y + titleHeight), so the add line is
	// at absolute row diffBox.y + titleHeight + 2.
	x := app.diffBox.x + 1 // within the "[+]" button's own columns
	y := app.diffBox.y + titleHeight + 2

	msg := tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}
	m, _ = app.Update(msg)
	app = m.(App)

	if !strings.Contains(app.View(), "Draft note") {
		t.Fatalf("expected clicking the add line's [+] button to open the draft box; view:\n%s", app.View())
	}
}

// TestMouseClickOneRowHigherMissesButton pins down exactly what the bug
// looked like: before the fix, a click on the button's row landed on the
// hunk-header row instead (one row too high in diffview's own numbering),
// which has no button and does nothing.
func TestMouseClickOneRowHigherMissesButton(t *testing.T) {
	store := annotate.NewStore()
	app := NewApp(config.Default(), testChangeset(), store, t.TempDir()+"/annotations.json")

	m, _ := app.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	app = m.(App)

	x := app.diffBox.x + 1
	y := app.diffBox.y + titleHeight + 1 // the context line, ordinal 0 — no button

	msg := tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}
	m, _ = app.Update(msg)
	app = m.(App)

	if strings.Contains(app.View(), "Draft note") {
		t.Fatal("clicking the context line (no button) should not open the draft box")
	}
}
