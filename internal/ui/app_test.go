package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mansooranis/kode/internal/annotate"
	"github.com/mansooranis/kode/internal/config"
	"github.com/mansooranis/kode/internal/diffparse"
	"github.com/mansooranis/kode/internal/update"
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

// TestKeyVTogglesSplitViewRegardlessOfFocus guards a bug where "v" only
// toggled the diff pane's split view when a.focus == focusDiff, because it
// was handled inside diffview.Update and only reached there via
// forwardToDiffview. The app starts with focus on the sidebar (see
// NewApp), so pressing "v" right after launch — before ever pressing "tab"
// — silently did nothing, unlike "m" (layout toggle) which is handled here
// in app.go regardless of focus.
func TestKeyVTogglesSplitViewRegardlessOfFocus(t *testing.T) {
	store := annotate.NewStore()
	app := NewApp(config.Default(), testChangeset(), store, t.TempDir()+"/annotations.json")

	m, _ := app.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	app = m.(App)

	if app.focus != focusSidebar {
		t.Fatalf("expected focus to default to the sidebar, got %v", app.focus)
	}
	if app.diffview.SplitView() {
		t.Fatal("expected unified view by default")
	}

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")}
	m, _ = app.Update(msg)
	app = m.(App)

	if !app.diffview.SplitView() {
		t.Fatal("expected \"v\" to switch to split view even while the sidebar has focus")
	}
}

// TestUpdateAvailableShowsBottomRightBanner guards the placement: the banner
// must land at the far right of the footer's own row, not push a new row or
// overwrite the keybinding hints entirely at normal terminal widths.
func TestUpdateAvailableShowsBottomRightBanner(t *testing.T) {
	store := annotate.NewStore()
	app := NewApp(config.Default(), testChangeset(), store, t.TempDir()+"/annotations.json")

	// Wide enough that the footer's keybinding hints (~143 cols) and the
	// banner both fit on the same row with room to spare.
	m, _ := app.Update(tea.WindowSizeMsg{Width: 220, Height: 30})
	app = m.(App)

	m, _ = app.Update(update.AvailableMsg{Latest: "v9.9.9"})
	app = m.(App)

	view := app.View()
	if !strings.Contains(view, "v9.9.9") {
		t.Fatalf("expected the rendered view to contain the available version; view:\n%s", view)
	}

	lines := strings.Split(view, "\n")
	footer := lines[len(lines)-1]
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
