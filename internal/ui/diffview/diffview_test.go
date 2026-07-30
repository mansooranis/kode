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

// TestUnifiedViewRespectsNarrowPaneWidth guards a bug where renderLine (the
// unified/default view) never truncated a diff line's content to the pane
// width — unlike renderSplitSide, which already did — so a long source line
// spilled out past the box border instead of being cut off.
func TestUnifiedViewRespectsNarrowPaneWidth(t *testing.T) {
	m := New()
	m.SetSize(40, 20)
	m.SetFile(diffparse.FileDiff{
		NewName: "example.go",
		Hunks: []diffparse.Hunk{{
			Header: "@@ -1,1 +1,1 @@",
			Lines: []diffparse.Line{
				{Op: diffparse.OpAdd, NewLineNo: 1, Content: strings.Repeat("x", 200)},
			},
		}},
	})

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

// TestDraftBoxSameWidthAndIndentAsCommentCard guards the regression where the
// draft box used its own separate width formula (m.boxWidth(0), no indent)
// while comment cards used a different one (m.boxWidth(len(indent)), with
// indent) — the two didn't agree, so a reply visually nested at a different
// width than the thread it was replying to. Both must now render identical
// borders (same indent, same total width) via the shared boxTopBorder etc.
// helpers.
func TestDraftBoxSameWidthAndIndentAsCommentCard(t *testing.T) {
	m := newTestModel()
	m.SetAnnotations([]annotate.Annotation{{File: "example.go", Line: 1, Author: annotate.Human, Text: "why?"}})

	card := renderCommentCard(annotate.Annotation{Author: annotate.Human, Text: "x"}, m.boxWidth(len(commentThreadIndent)))
	cardTop := card[0]

	m, _, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	draftTop := m.renderDraftBox()[0]

	if lipgloss.Width(cardTop) != lipgloss.Width(draftTop) {
		t.Fatalf("card top border width %d != draft box top border width %d\ncard:  %q\ndraft: %q",
			lipgloss.Width(cardTop), lipgloss.Width(draftTop), cardTop, draftTop)
	}
	if !strings.HasPrefix(cardTop, commentThreadIndent) || !strings.HasPrefix(draftTop, commentThreadIndent) {
		t.Fatalf("expected both to share the same left indent %q\ncard:  %q\ndraft: %q", commentThreadIndent, cardTop, draftTop)
	}
}

func TestKeyVTogglesSplitView(t *testing.T) {
	m := newTestModel()
	if m.SplitView() {
		t.Fatal("expected unified view by default")
	}

	m, _, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	if !m.SplitView() {
		t.Fatal("expected \"v\" to switch to split view")
	}

	m, _, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	if m.SplitView() {
		t.Fatal("expected a second \"v\" to switch back to unified view")
	}
}

func TestSplitViewRespectsNarrowPaneWidth(t *testing.T) {
	m := New()
	m.SetSize(40, 20)
	m.SetFile(testFile())
	m.SetSplitView(true)

	assertNoLineExceedsWidth(t, m)
}

// TestSplitViewPairsDeleteAndAddRuns guards the row-pairing heuristic: the
// test file has one delete run (1 line) and one add run (2 lines) — GitHub's
// split view zips them index-wise into shared rows, so the first add lines
// up next to the delete, and the extra add gets a blank left side.
func TestSplitViewPairsDeleteAndAddRuns(t *testing.T) {
	m := newTestModel()
	m.SetSplitView(true)

	var splitRows []rowMeta
	for _, r := range m.rows {
		if r.splitRow {
			splitRows = append(splitRows, r)
		}
	}
	if len(splitRows) != 3 {
		t.Fatalf("expected 3 split rows (1 context + 1 paired delete/add + 1 add-only), got %d", len(splitRows))
	}

	context, pair, addOnly := splitRows[0], splitRows[1], splitRows[2]
	if context.leftOrdinal != 0 || context.rightOrdinal != 0 {
		t.Fatalf("expected the context row's left and right ordinal to both be 0, got left=%d right=%d", context.leftOrdinal, context.rightOrdinal)
	}
	if pair.leftOrdinal != 1 || pair.rightOrdinal != 2 {
		t.Fatalf("expected the delete (ordinal 1) paired with the first add (ordinal 2), got left=%d right=%d", pair.leftOrdinal, pair.rightOrdinal)
	}
	if addOnly.leftOrdinal != -1 || addOnly.rightOrdinal != 3 {
		t.Fatalf("expected the second add (ordinal 3) alone on the right with a blank left, got left=%d right=%d", addOnly.leftOrdinal, addOnly.rightOrdinal)
	}
}

func TestSplitViewClickOnLeftButtonOpensDraftOnDeleteLine(t *testing.T) {
	m := newTestModel()
	m.SetSplitView(true)

	// Row 0 is the hunk header, row 1 the context row, row 2 the
	// delete/add pair row. Click within the left button's columns.
	msg := tea.MouseMsg{X: 1, Y: 2, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}
	m, _, _ = m.Update(msg)
	if !m.DraftActive() {
		t.Fatal("expected click on left button column to open the draft box")
	}
	line, ok := m.CursorTarget()
	if !ok || line != 2 {
		t.Fatalf("expected cursor on the delete line (old-file line 2), got %d (ok=%v)", line, ok)
	}
}

func TestSplitViewClickOnRightButtonOpensDraftOnAddLine(t *testing.T) {
	m := newTestModel()
	m.SetSplitView(true)

	midX := m.rows[2].midX
	msg := tea.MouseMsg{X: midX + 1, Y: 2, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}
	m, _, _ = m.Update(msg)
	if !m.DraftActive() {
		t.Fatal("expected click on right button column to open the draft box")
	}
	line, ok := m.CursorTarget()
	if !ok || line != 2 {
		t.Fatalf("expected cursor on the add line (new-file line 2), got %d (ok=%v)", line, ok)
	}
}

func TestSplitViewClickOnSeparatorDoesNothing(t *testing.T) {
	m := newTestModel()
	m.SetSplitView(true)
	before := m.cursor

	midX := m.rows[2].midX
	msg := tea.MouseMsg{X: midX - 1, Y: 2, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}
	m, _, _ = m.Update(msg)
	if m.DraftActive() {
		t.Fatal("clicking the column separator should never open the draft box")
	}
	if m.cursor != before {
		t.Fatalf("cursor should not move when clicking the separator, was %d now %d", before, m.cursor)
	}
}

// TestSplitViewArrowKeysAlwaysMoveToADifferentRow guards a bug where a
// delete/add pair sharing one visual row (ordinals 1 and 2 both render on
// row 2, see TestSplitViewPairsDeleteAndAddRuns) made a "down" press from
// ordinal 1 to ordinal 2 look like it did nothing: the cursor ordinal moved
// but cursorRow stayed put, so the highlighted row never changed. Every
// keypress should move the highlighted row when there is a next/previous
// row to move to.
func TestSplitViewArrowKeysAlwaysMoveToADifferentRow(t *testing.T) {
	m := newTestModel()
	m.SetSplitView(true)

	// testFile's one hunk renders as 3 split rows: the context line, the
	// delete/add pair sharing a row, and the trailing add-only line. So
	// starting from the context row, there are exactly 2 more distinct rows
	// to move through.
	rows := []int{m.cursorRow}
	for i := 0; i < 2; i++ {
		m.MoveCursor(1)
		rows = append(rows, m.cursorRow)
	}

	for i := 1; i < len(rows); i++ {
		if rows[i] == rows[i-1] {
			t.Fatalf("pressing down should always change the highlighted row, but step %d stayed on row %d (all rows: %v)", i, rows[i], rows)
		}
	}

	// And walking back up should retrace the same rows in reverse.
	for i := 0; i < 2; i++ {
		m.MoveCursor(-1)
		got := m.cursorRow
		want := rows[len(rows)-2-i]
		if got != want {
			t.Fatalf("step %d up: expected to be back on row %d, got %d", i, want, got)
		}
	}
}
