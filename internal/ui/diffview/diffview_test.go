package diffview

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mansooranis/kode/internal/annotate"
	"github.com/mansooranis/kode/internal/diffparse"
)

// findReplyRow returns the absolute row (0-indexed, matching m.rows) of the
// "[Reply]" button attached to the given canonical line number.
func findReplyRow(m Model, line int) (int, bool) {
	for i, r := range m.rows {
		if r.replyLine == line {
			return i, true
		}
	}
	return 0, false
}

func testFile() diffparse.FileDiff {
	return diffparse.FileDiff{
		NewName: "example.go",
		Hunks: []diffparse.Hunk{{
			Header: "@@ -1,2 +1,3 @@",
			Lines: []diffparse.Line{
				{Op: diffparse.OpContext, OldLineNo: 1, NewLineNo: 1, Content: "package main"},
				{Op: diffparse.OpDelete, OldLineNo: 2, Content: "old line"},
				{Op: diffparse.OpAdd, NewLineNo: 2, Content: "new line"},
				{Op: diffparse.OpAdd, NewLineNo: 3, Content: "another new line"},
			},
		}},
	}
}

func newTestModel() Model {
	m := New()
	m.SetSize(80, 20)
	m.SetFile(testFile())
	return m
}

func typeString(m Model, s string) Model {
	for _, r := range s {
		m, _, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return m
}

func TestKeyCOpensDraftOnCursorLine(t *testing.T) {
	m := newTestModel()
	// cursor starts at ordinal 0 (the context line, line 1).
	m, _, submitted := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	if submitted {
		t.Fatal("opening a draft should not itself report a submission")
	}
	if !m.DraftActive() {
		t.Fatal("expected \"c\" to open the draft box")
	}
}

func TestArrowKeysMoveCursorAndCursorTargetTracksIt(t *testing.T) {
	m := newTestModel()
	m, _, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	line, ok := m.CursorTarget()
	if !ok || line != 2 {
		t.Fatalf("after moving down once, expected line 2 (old-file line of the delete), got %d", line)
	}
}

func TestClickOnButtonColumnOfChangedLineOpensDraft(t *testing.T) {
	m := newTestModel()

	// Row 0 is the hunk header; row 1 is the context line (ordinal 0); row 2
	// is the delete line (ordinal 1, changed=true). Click within the button's
	// columns (X < len(commentButtonGlyph)+1).
	msg := tea.MouseMsg{X: 1, Y: 2, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}
	m, _, submitted := m.Update(msg)
	if submitted {
		t.Fatal("opening a draft via button click should not itself report a submission")
	}
	if !m.DraftActive() {
		t.Fatal("expected click on button column of a changed line to open the draft box")
	}
	line, ok := m.CursorTarget()
	if !ok || line != 2 {
		t.Fatalf("expected cursor to move to the delete line (old-file line 2), got %d (ok=%v)", line, ok)
	}
}

func TestClickOutsideButtonColumnJustMovesCursor(t *testing.T) {
	m := newTestModel()

	// Same row (the delete line) but clicking well past the button's columns
	// — should move the cursor there but NOT open the draft box.
	msg := tea.MouseMsg{X: 20, Y: 2, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}
	m, _, _ = m.Update(msg)
	if m.DraftActive() {
		t.Fatal("click outside the button column should not open the draft box")
	}
	line, ok := m.CursorTarget()
	if !ok || line != 2 {
		t.Fatalf("expected cursor to still move to line 2, got %d (ok=%v)", line, ok)
	}
}

func TestClickOnContextLineNeverOpensDraft(t *testing.T) {
	m := newTestModel()

	// Row 1 is the context line (ordinal 0, changed=false); even a click
	// within the button's column range must not open a draft, since context
	// lines don't get a button.
	msg := tea.MouseMsg{X: 1, Y: 1, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}
	m, _, _ = m.Update(msg)
	if m.DraftActive() {
		t.Fatal("context lines should never offer a comment button")
	}
	line, ok := m.CursorTarget()
	if !ok || line != 1 {
		t.Fatalf("expected cursor at line 1, got %d (ok=%v)", line, ok)
	}
}

func TestClickOnHunkHeaderRowIsIgnored(t *testing.T) {
	m := newTestModel()
	before := m.cursor

	msg := tea.MouseMsg{X: 1, Y: 0, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}
	m, _, _ = m.Update(msg)
	if m.DraftActive() {
		t.Fatal("clicking the hunk header row should never open the draft box")
	}
	if m.cursor != before {
		t.Fatalf("cursor should not move when clicking a non-code row, was %d now %d", before, m.cursor)
	}
}

func TestDraftSubmitFlow(t *testing.T) {
	m := newTestModel()
	m, _, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	if !m.DraftActive() {
		t.Fatal("expected draft to be active")
	}

	m = typeString(m, "why did we remove this?")

	m, _, submitted := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if !submitted {
		t.Fatal("expected ctrl+s to report a submission")
	}
	if m.DraftActive() {
		t.Fatal("expected draft to close after submit")
	}

	line, text := m.TakeSubmission()
	if line != 1 {
		t.Fatalf("expected submission line 1, got %d", line)
	}
	if text != "why did we remove this?" {
		t.Fatalf("expected submitted text to match typed text, got %q", text)
	}
}

func TestDraftEscCancelsWithoutSubmitting(t *testing.T) {
	m := newTestModel()
	m, _, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = typeString(m, "discard me")

	m, _, submitted := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if submitted {
		t.Fatal("esc should never report a submission")
	}
	if m.DraftActive() {
		t.Fatal("expected draft to close after esc")
	}
}

func TestDraftSubmitWithEmptyTextDoesNotSubmit(t *testing.T) {
	m := newTestModel()
	m, _, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})

	m, _, submitted := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if submitted {
		t.Fatal("submitting an empty note should not report a submission")
	}
	if m.DraftActive() {
		t.Fatal("expected draft to still close on ctrl+s even when empty")
	}
}

func TestTypingWhileDraftActiveDoesNotMoveCursorOrQuit(t *testing.T) {
	m := newTestModel()
	m, _, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})

	// "j"/"k" would normally move the cursor; while drafting they must be
	// typed into the textarea instead.
	m = typeString(m, "jk")

	if !m.DraftActive() {
		t.Fatal("draft should still be active")
	}
	line, _ := m.CursorTarget()
	if line != 1 {
		t.Fatalf("cursor should not have moved while drafting, got target line %d", line)
	}
	if m.draftTextarea.Value() != "jk" {
		t.Fatalf("expected \"jk\" typed into the textarea, got %q", m.draftTextarea.Value())
	}
}

func TestFreshDraftIsNotMarkedAsReply(t *testing.T) {
	m := newTestModel()
	m, _, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	if m.draftIsReply {
		t.Fatal("opening a draft on a line with no existing thread should not be marked as a reply")
	}
}

func TestDraftOnLineWithExistingThreadIsMarkedAsReply(t *testing.T) {
	m := newTestModel()
	m.SetAnnotations([]annotate.Annotation{{File: "example.go", Line: 1, Author: annotate.Human, Text: "why?"}})

	m, _, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	if !m.draftIsReply {
		t.Fatal("opening a draft on a line with an existing thread should be marked as a reply")
	}
}

func TestReplyButtonRenderedOnlyForLinesWithThreads(t *testing.T) {
	m := newTestModel()
	m.SetAnnotations([]annotate.Annotation{{File: "example.go", Line: 1, Author: annotate.Human, Text: "why?"}})

	if _, ok := findReplyRow(m, 1); !ok {
		t.Fatal("expected a [Reply] button row for line 1, which has a thread")
	}
	if _, ok := findReplyRow(m, 2); ok {
		t.Fatal("line 2 has no thread and should not get a [Reply] button")
	}
}

func TestClickOnReplyButtonOpensDraftAttachedToThatLine(t *testing.T) {
	m := newTestModel()
	m.SetAnnotations([]annotate.Annotation{{File: "example.go", Line: 1, Author: annotate.Human, Text: "why?"}})

	row, ok := findReplyRow(m, 1)
	if !ok {
		t.Fatal("expected to find the [Reply] button row for line 1")
	}

	msg := tea.MouseMsg{X: 1, Y: row, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}
	m, _, submitted := m.Update(msg)
	if submitted {
		t.Fatal("clicking [Reply] should not itself report a submission")
	}
	if !m.DraftActive() {
		t.Fatal("expected clicking [Reply] to open the draft box")
	}
	if !m.draftIsReply {
		t.Fatal("expected the draft opened via [Reply] to be marked as a reply")
	}
	line, ok := m.CursorTarget()
	if !ok || line != 1 {
		t.Fatalf("expected draft attached to line 1, got %d (ok=%v)", line, ok)
	}
}

// assertNoLineExceedsWidth guards against the regression where boxes used a
// fixed width regardless of the pane's actual size: on a narrow pane, lines
// wider than the terminal wrap and corrupt the whole layout, and the
// resulting row/column drift is exactly what made mouse clicks (including
// "[Reply]") land on the wrong target.
func assertNoLineExceedsWidth(t *testing.T, m Model) {
	t.Helper()
	for i, line := range strings.Split(m.viewport.View(), "\n") {
		if w := lipgloss.Width(line); w > m.width {
			t.Fatalf("rendered line %d exceeds pane width %d (got %d): %q", i, m.width, w, line)
		}
	}
}

func TestDraftBoxRespectsNarrowPaneWidth(t *testing.T) {
	m := New()
	m.SetSize(40, 20)
	m.SetFile(testFile())

	m, _, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = typeString(m, "a very long note that would have overflowed the old fixed-width box on a narrow pane")

	assertNoLineExceedsWidth(t, m)
}

func TestCommentCardRespectsNarrowPaneWidth(t *testing.T) {
	m := New()
	m.SetSize(40, 20)
	m.SetFile(testFile())
	m.SetAnnotations([]annotate.Annotation{{
		File:   "example.go",
		Line:   1,
		Author: "mcp:claude-code",
		Text:   "This is a fairly long review comment that needs to wrap across several lines without ever exceeding the pane width.",
	}})

	assertNoLineExceedsWidth(t, m)
}

func TestBoxWidthResizesLiveWhileDraftOpen(t *testing.T) {
	m := New()
	m.SetSize(100, 20)
	m.SetFile(testFile())
	m, _, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})

	// Shrink the pane while the draft is still open (e.g. terminal resize).
	m.SetSize(30, 20)
	assertNoLineExceedsWidth(t, m)
}

func TestClickPastReplyButtonColumnsDoesNothing(t *testing.T) {
	m := newTestModel()
	m.SetAnnotations([]annotate.Annotation{{File: "example.go", Line: 1, Author: annotate.Human, Text: "why?"}})

	row, ok := findReplyRow(m, 1)
	if !ok {
		t.Fatal("expected to find the [Reply] button row for line 1")
	}

	msg := tea.MouseMsg{X: 50, Y: row, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}
	m, _, _ = m.Update(msg)
	if m.DraftActive() {
		t.Fatal("clicking past the [Reply] button's own columns should not open the draft box")
	}
}
