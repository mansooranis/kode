// Package diffview renders a single file's diff: a scrollable, syntax
// highlighted view over a diffparse.FileDiff, with a movable cursor line and
// inline annotation threads. It's built as a standalone Bubble Tea component
// (its own Update/View) so it can be embedded by other programs, per Hunk's
// reusable-component goal.
package diffview

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mansooranis/kode/internal/annotate"
	"github.com/mansooranis/kode/internal/diffparse"
)

var (
	addStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	delStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("204"))
	hunkStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	gutterStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	cursorLineBg    = lipgloss.Color("237")
	commentBtnStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	draftBorderClr  = lipgloss.Color("214")
	draftBorder     = lipgloss.NewStyle().Foreground(draftBorderClr)
	draftButton     = lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(draftBorderClr).Bold(true)
)

const commentButtonGlyph = "[+]"
const commentButtonBlank = "   "
const replyButtonGlyph = "[Reply]"
const draftTextareaHeight = 3
const commentThreadIndent = "      " // aligns cards/reply button under the line's gutter+button columns
const minBoxWidth = 20               // floor so a narrow pane can't drive width negative

// boxWidth returns how wide a box (draft note or comment card) may render
// given the pane's actual current width, so it can never overflow past the
// diff pane's real boundary — a fixed width here previously caused the
// terminal to wrap overflowing lines, corrupting the whole layout and
// throwing off the row/column math mouse clicks rely on.
func (m Model) boxWidth(indent int) int {
	w := m.width - indent - 1
	if w < minBoxWidth {
		w = minBoxWidth
	}
	return w
}

// authorStyle returns the color used for a comment thread line, based on
// who (or what) wrote it.
func authorStyle(author string) lipgloss.Style {
	switch {
	case author == annotate.Human:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	case author == annotate.KodeAgent:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("135"))
	case strings.HasPrefix(author, "mcp:"):
		return lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	}
}

// lineKey identifies the diff line an annotation or draft attaches to: a
// canonical line number plus which coordinate space it's from. Old- and
// new-file line numbers are independent sequences that can land on the same
// integer (e.g. a deleted line's old-file number and some other, unrelated
// line's new-file number), so the number alone isn't a unique key within a
// file's diff — Old disambiguates them.
type lineKey struct {
	line int
	old  bool
}

// rowMeta records what a single rendered row corresponds to, so mouse clicks
// (given only an absolute row number) can be mapped back to a diff line.
type rowMeta struct {
	ordinal  int     // index into the flat list of diff lines; -1 if not a code line
	changed  bool    // true for add/delete lines, i.e. lines that get a comment button
	replyKey lineKey // replyKey.line > 0 if this row is the "[Reply]" button for this line

	// The fields below apply only when splitRow is true, i.e. this row is a
	// side-by-side code row rendered by renderSplitHunk. Unlike unified rows
	// (one diff line per row), a split row can hold two distinct diff lines
	// (a deletion on the left paired with its replacement on the right), so
	// clicks need their own ordinal/changed pair per side plus the column
	// boundary (midX) to know which side msg.X landed in.
	splitRow     bool
	leftOrdinal  int
	rightOrdinal int
	leftChanged  bool
	rightChanged bool
	midX         int
}

type Model struct {
	viewport    viewport.Model
	file        diffparse.FileDiff
	annotations []annotate.Annotation
	lineNumbers bool
	splitView   bool // GitHub-style side-by-side rendering, vs. the default unified view
	width       int
	height      int

	cursor     int // ordinal index into commentable diff lines, 0-based
	totalLines int
	cursorRow  int // row within rendered content where the cursor line starts
	rows       []rowMeta

	draftActive   bool
	draftOrdinal  int  // which diff line ordinal the open draft is attached to
	draftKey      lineKey
	draftIsReply  bool // true if the line already had a thread when the draft opened
	draftTextarea textarea.Model

	submittedKey  lineKey
	submittedText string

	style chroma.Style
}

// fileStatus is the word shown in the draft box title, e.g.
// "Draft note - main.go (new) R11".
func fileStatus(f diffparse.FileDiff) string {
	switch {
	case f.IsNew:
		return "new"
	case f.IsDelete:
		return "deleted"
	case f.IsRename:
		return "renamed"
	default:
		return "modified"
	}
}

func New() Model {
	style := styles.Get("monokai")
	if style == nil {
		style = styles.Fallback
	}
	return Model{
		viewport:    viewport.New(0, 0),
		lineNumbers: true,
		style:       *style,
	}
}

func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.viewport.Width = width
	m.viewport.Height = height
	m.render()
}

func (m *Model) SetLineNumbers(on bool) {
	m.lineNumbers = on
	m.render()
}

// SplitView reports whether the diff is currently rendered side-by-side.
func (m Model) SplitView() bool {
	return m.splitView
}

// SetSplitView switches between unified (default) and GitHub-style
// side-by-side rendering.
func (m *Model) SetSplitView(on bool) {
	m.splitView = on
	m.render()
}

// SetFile loads a new file's diff into the view and resets scroll/cursor.
func (m *Model) SetFile(f diffparse.FileDiff) {
	m.file = f
	m.annotations = nil
	m.cursor = 0
	m.render()
	m.viewport.GotoTop()
}

// SetAnnotations replaces the annotation threads shown for the current file.
func (m *Model) SetAnnotations(annotations []annotate.Annotation) {
	m.annotations = annotations
	m.render()
}

// CursorTarget returns the canonical line number under the cursor (new-file
// line number if present, else old-file line number), plus whether that
// number is from the old-file space (true only for a pure deletion), so the
// caller can attach a new comment to it. ok is false if the file has no
// lines.
func (m Model) CursorTarget() (line int, old bool, ok bool) {
	i := 0
	for _, hunk := range m.file.Hunks {
		for _, l := range hunk.Lines {
			if i == m.cursor {
				key := canonicalKey(l)
				return key.line, key.old, true
			}
			i++
		}
	}
	return 0, false, false
}

// ordinalForLine finds the diff-line ordinal whose canonical key matches
// key, so a click on a "[Reply]" button (which isn't itself the line's own
// row) can move the cursor there before opening the draft box.
func (m Model) ordinalForLine(key lineKey) (int, bool) {
	i := 0
	for _, hunk := range m.file.Hunks {
		for _, l := range hunk.Lines {
			if canonicalKey(l) == key {
				return i, true
			}
			i++
		}
	}
	return 0, false
}

func (m *Model) MoveCursor(delta int) {
	if m.totalLines == 0 {
		return
	}

	// In split view, a delete/add pair shares one visual row (see
	// renderSplitHunk), so it claims two ordinals for one row. Moving by a
	// single ordinal can then land back on the same row the cursor started
	// on, which makes "up"/"down" feel broken (the highlight barely seems
	// to move). Keep stepping in the same direction until the row actually
	// changes, or until the cursor stops moving (start/end of file), so
	// every keypress moves the cursor to a different row when one exists.
	startRow := m.cursorRow
	for {
		prev := m.cursor
		m.cursor += delta
		if m.cursor < 0 {
			m.cursor = 0
		}
		if m.cursor >= m.totalLines {
			m.cursor = m.totalLines - 1
		}
		m.render()
		if m.cursor == prev || !m.splitView || m.cursorRow != startRow {
			break
		}
	}

	if m.cursorRow < m.viewport.YOffset {
		m.viewport.YOffset = m.cursorRow
	} else if m.cursorRow >= m.viewport.YOffset+m.viewport.Height {
		m.viewport.YOffset = m.cursorRow - m.viewport.Height + 1
	}
	if m.viewport.YOffset < 0 {
		m.viewport.YOffset = 0
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles input for the diff pane, including the inline draft-note
// box. The returned bool is true exactly when a note was just submitted
// (ctrl+s) — the caller should immediately call TakeSubmission to retrieve
// it and persist it (diffview has no store reference of its own).
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd, bool) {
	if m.draftActive {
		return m.updateDraft(msg)
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			m.MoveCursor(-1)
			return m, nil, false
		case "down", "j":
			m.MoveCursor(1)
			return m, nil, false
		case "c":
			return m.activateDraft(), nil, false
		case "v":
			m.splitView = !m.splitView
			m.render()
			return m, nil, false
		}
	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			absRow := msg.Y + m.viewport.YOffset
			if absRow >= 0 && absRow < len(m.rows) {
				meta := m.rows[absRow]

				if meta.replyKey.line > 0 {
					onButton := msg.X < len(commentThreadIndent)+len(replyButtonGlyph)
					if onButton {
						if ord, ok := m.ordinalForLine(meta.replyKey); ok {
							m.cursor = ord
							return m.activateDraft(), nil, false
						}
					}
					return m, nil, false
				}

				if meta.splitRow {
					var ord int
					var changed bool
					var colStart int
					switch {
					case msg.X < meta.midX-1:
						ord, changed, colStart = meta.leftOrdinal, meta.leftChanged, 0
					case msg.X >= meta.midX:
						ord, changed, colStart = meta.rightOrdinal, meta.rightChanged, meta.midX
					default:
						return m, nil, false // clicked the column separator itself
					}
					if ord >= 0 {
						m.cursor = ord
						onButton := changed && msg.X < colStart+len(commentButtonGlyph)+1
						if onButton {
							return m.activateDraft(), nil, false
						}
						m.render()
					}
					return m, nil, false
				}

				if meta.ordinal >= 0 {
					m.cursor = meta.ordinal
					// Only a click within the "[+]" button's own columns
					// opens the draft box, matching GitHub's affordance —
					// clicking elsewhere on a changed line just moves the
					// cursor there.
					onButton := meta.changed && msg.X < len(commentButtonGlyph)+1
					if onButton {
						return m.activateDraft(), nil, false
					}
					m.render()
					return m, nil, false
				}
			}
		}
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd, false
}

// activateDraft opens the inline note box on the current cursor line, if
// that line has a valid target (i.e. the file isn't empty).
func (m Model) activateDraft() Model {
	line, old, ok := m.CursorTarget()
	if !ok {
		return m
	}

	ta := textarea.New()
	ta.Placeholder = "Write a note..."
	ta.SetWidth(m.boxWidth(len(commentThreadIndent)) - 4)
	ta.SetHeight(draftTextareaHeight)
	ta.ShowLineNumbers = false
	ta.Focus()

	m.draftActive = true
	m.draftOrdinal = m.cursor
	m.draftKey = lineKey{line: line, old: old}
	m.draftIsReply = false
	for _, a := range m.annotations {
		if a.Line == line && a.OldLine == old {
			m.draftIsReply = true
			break
		}
	}
	m.draftTextarea = ta
	m.render()
	return m
}

func (m Model) updateDraft(msg tea.Msg) (Model, tea.Cmd, bool) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc":
			m.draftActive = false
			m.render()
			return m, nil, false
		case "ctrl+s":
			text := strings.TrimSpace(m.draftTextarea.Value())
			m.draftActive = false
			m.render()
			if text == "" {
				return m, nil, false
			}
			m.submittedKey = m.draftKey
			m.submittedText = text
			return m, nil, true
		}
	}

	var cmd tea.Cmd
	m.draftTextarea, cmd = m.draftTextarea.Update(msg)
	m.render()
	return m, cmd, false
}

// DraftActive reports whether the inline note box is open, so the parent
// model knows to route every keystroke here instead of treating them as
// global shortcuts (e.g. typing "q" while writing a note must not quit).
func (m Model) DraftActive() bool {
	return m.draftActive
}

// TakeSubmission returns the note text and target line (plus whether that
// line number is from the old-file space) captured by the most recent
// ctrl+s submission. Call this once, immediately after Update returns true,
// to persist it — the values are not cleared automatically since diffview
// has no store of its own to write to.
func (m Model) TakeSubmission() (line int, old bool, text string) {
	return m.submittedKey.line, m.submittedKey.old, m.submittedText
}

func (m Model) View() string {
	return m.viewport.View()
}

func (m *Model) render() {
	lexer := lexers.Match(m.file.Name())
	if lexer == nil {
		lexer = lexers.Fallback
	}

	annotationsByLine := map[lineKey][]annotate.Annotation{}
	for _, a := range m.annotations {
		key := lineKey{line: a.Line, old: a.OldLine}
		annotationsByLine[key] = append(annotationsByLine[key], a)
	}

	if m.draftActive {
		// The pane may have been resized since the draft was opened.
		m.draftTextarea.SetWidth(m.boxWidth(len(commentThreadIndent)) - 4)
	}

	var b strings.Builder
	row := 0
	ordinal := 0
	m.cursorRow = 0
	m.rows = m.rows[:0]

	writeLine := func(s string, meta rowMeta) {
		b.WriteString(s)
		b.WriteString("\n")
		row++
		m.rows = append(m.rows, meta)
	}

	if m.file.IsBinary {
		writeLine(gutterStyle.Render("(binary file, no textual diff)"), rowMeta{ordinal: -1})
	}
	for hi, hunk := range m.file.Hunks {
		if hi > 0 {
			writeLine("", rowMeta{ordinal: -1})
		}
		writeLine(hunkStyle.Render(hunk.Header), rowMeta{ordinal: -1})

		if m.splitView {
			ordinal = m.renderSplitHunk(hunk, ordinal, lexer, annotationsByLine, writeLine)
		} else {
			ordinal = m.renderUnifiedHunk(hunk, ordinal, lexer, annotationsByLine, writeLine)
		}
	}
	m.totalLines = ordinal

	m.viewport.SetContent(strings.TrimRight(b.String(), "\n"))
}

// canonicalKey is the key an annotation attaches to: the new-file line if
// the line exists there, else the old-file line (with old set so it isn't
// confused with an unrelated new-file line of the same number elsewhere in
// the diff). Matches how CursorTarget picks a target for a freshly created
// comment.
func canonicalKey(l diffparse.Line) lineKey {
	if l.NewLineNo > 0 {
		return lineKey{line: l.NewLineNo}
	}
	return lineKey{line: l.OldLineNo, old: true}
}

// emitThreadRows writes the existing comment cards, "[Reply]" button, and
// (if open) the draft box for the diff line at ordinal/key. Shared by both
// unified and split rendering so the two stay in sync.
func (m *Model) emitThreadRows(writeLine func(string, rowMeta), annotationsByLine map[lineKey][]annotate.Annotation, key lineKey, ordinal int) {
	thread := annotationsByLine[key]
	for _, a := range thread {
		for _, line := range renderCommentCard(a, m.boxWidth(len(commentThreadIndent))) {
			writeLine(line, rowMeta{ordinal: -1})
		}
	}
	// The reply button is suppressed while a draft is already open on this
	// exact line — the open draft box IS the reply affordance at that point,
	// showing both would be redundant.
	if len(thread) > 0 && !(m.draftActive && ordinal == m.draftOrdinal) {
		writeLine(commentThreadIndent+commentBtnStyle.Render(replyButtonGlyph), rowMeta{ordinal: -1, replyKey: key})
	}
	if m.draftActive && ordinal == m.draftOrdinal {
		for _, line := range m.renderDraftBox() {
			writeLine(line, rowMeta{ordinal: -1})
		}
	}
}

// renderUnifiedHunk renders one hunk's lines the traditional way: one row
// per diff line, sign-prefixed, in original patch order. Returns the ordinal
// just past this hunk's lines.
func (m *Model) renderUnifiedHunk(hunk diffparse.Hunk, ordinal int, lexer chroma.Lexer, annotationsByLine map[lineKey][]annotate.Annotation, writeLine func(string, rowMeta)) int {
	for _, l := range hunk.Lines {
		isCursor := ordinal == m.cursor
		if isCursor {
			m.cursorRow = len(m.rows)
		}
		changed := l.Op == diffparse.OpAdd || l.Op == diffparse.OpDelete
		writeLine(m.renderLine(lexer, l, isCursor, changed), rowMeta{ordinal: ordinal, changed: changed})
		m.emitThreadRows(writeLine, annotationsByLine, canonicalKey(l), ordinal)
		ordinal++
	}
	return ordinal
}

// splitPair is one row of a side-by-side hunk: li/ri index into the hunk's
// Lines slice (-1 if that side is blank for this row).
type splitPair struct{ li, ri int }

// pairSplitRows groups a hunk's flat, patch-ordered lines into side-by-side
// rows: context lines appear on both sides of their own row, and each
// contiguous run of deletions is zipped index-wise against the run of
// additions that follows it (GitHub's split-diff heuristic) — extra lines on
// the longer side get a blank counterpart rather than a matching pair.
func pairSplitRows(lines []diffparse.Line) []splitPair {
	var rows []splitPair
	i := 0
	for i < len(lines) {
		if lines[i].Op == diffparse.OpContext {
			rows = append(rows, splitPair{li: i, ri: i})
			i++
			continue
		}

		delStart := i
		for i < len(lines) && lines[i].Op == diffparse.OpDelete {
			i++
		}
		delEnd := i
		addStart := i
		for i < len(lines) && lines[i].Op == diffparse.OpAdd {
			i++
		}
		addEnd := i

		nDel := delEnd - delStart
		nAdd := addEnd - addStart
		n := nDel
		if nAdd > n {
			n = nAdd
		}
		for k := 0; k < n; k++ {
			pr := splitPair{li: -1, ri: -1}
			if k < nDel {
				pr.li = delStart + k
			}
			if k < nAdd {
				pr.ri = addStart + k
			}
			rows = append(rows, pr)
		}
	}
	return rows
}

// renderSplitHunk renders one hunk's lines side-by-side, GitHub style.
// Returns the ordinal just past this hunk's lines.
func (m *Model) renderSplitHunk(hunk diffparse.Hunk, ordinal int, lexer chroma.Lexer, annotationsByLine map[lineKey][]annotate.Annotation, writeLine func(string, rowMeta)) int {
	base := ordinal
	for _, pr := range pairSplitRows(hunk.Lines) {
		var leftLine, rightLine diffparse.Line
		hasLeft, hasRight := pr.li >= 0, pr.ri >= 0
		leftOrdinal, rightOrdinal := -1, -1
		if hasLeft {
			leftLine = hunk.Lines[pr.li]
			leftOrdinal = base + pr.li
		}
		if hasRight {
			rightLine = hunk.Lines[pr.ri]
			rightOrdinal = base + pr.ri
		}

		isCursorLeft := hasLeft && leftOrdinal == m.cursor
		isCursorRight := hasRight && rightOrdinal == m.cursor
		if isCursorLeft || isCursorRight {
			m.cursorRow = len(m.rows)
		}

		leftChanged := hasLeft && leftLine.Op != diffparse.OpContext
		rightChanged := hasRight && rightLine.Op != diffparse.OpContext

		line, midX := m.renderSplitLine(lexer, leftLine, hasLeft, isCursorLeft, rightLine, hasRight, isCursorRight)
		writeLine(line, rowMeta{
			ordinal:      -1,
			splitRow:     true,
			leftOrdinal:  leftOrdinal,
			rightOrdinal: rightOrdinal,
			leftChanged:  leftChanged,
			rightChanged: rightChanged,
			midX:         midX,
		})

		if pr.li == pr.ri {
			// Context line: one underlying diff line shown on both sides,
			// so it gets exactly one thread, not two.
			if hasLeft {
				m.emitThreadRows(writeLine, annotationsByLine, canonicalKey(leftLine), leftOrdinal)
			}
		} else {
			if hasLeft {
				m.emitThreadRows(writeLine, annotationsByLine, canonicalKey(leftLine), leftOrdinal)
			}
			if hasRight {
				m.emitThreadRows(writeLine, annotationsByLine, canonicalKey(rightLine), rightOrdinal)
			}
		}
	}
	return base + len(hunk.Lines)
}

// boxTopBorder renders "╭─<label><fill>╮" at exactly total visible columns,
// truncating label if it doesn't fit. Shared by the draft box and comment
// cards so both draw from identical width arithmetic — the two previously
// used separate ad-hoc formulas that didn't quite agree, which is what made
// the draft box render narrower/raggeder than the comment cards above it.
func boxTopBorder(label string, total int, style lipgloss.Style) string {
	inner := max(total-2, 0)
	// Measure by visual width, not len() (byte count) — labels can contain
	// multi-byte-but-single-column runes (e.g. the "·" separator in a
	// comment card's author/date header), and byte-counting them silently
	// under-fills the border by one column per such rune, exactly the bug
	// that made comment cards render one column narrower than the draft box
	// (whose title happens to be pure ASCII, so it never showed the drift).
	if lipgloss.Width(label) > inner-1 {
		label = truncateToWidth(label, max(inner-1, 0))
	}
	fill := max(inner-1-lipgloss.Width(label), 0)
	return style.Render("╭─" + label + strings.Repeat("─", fill) + "╮")
}

// truncateToWidth cuts s down to at most width visual columns, rune-safe.
func truncateToWidth(s string, width int) string {
	if width <= 0 {
		return ""
	}
	w := 0
	for i, r := range s {
		rw := lipgloss.Width(string(r))
		if w+rw > width {
			return s[:i]
		}
		w += rw
	}
	return s
}

// padToWidth right-pads s with spaces until it's width visual columns,
// measuring by visual width (not byte count) so ANSI-colored strings pad
// correctly. Returns s unchanged if it's already at or past width.
func padToWidth(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

// boxBottomBorder renders "╰<fill> <trailer>╯" at exactly total visible
// columns, with trailer (e.g. Save/Cancel buttons) right-aligned. An empty
// trailer draws a plain "╰────╯" line.
func boxBottomBorder(trailer string, total int, style lipgloss.Style) string {
	inner := max(total-2, 0)
	if trailer == "" {
		return style.Render("╰" + strings.Repeat("─", inner) + "╯")
	}
	fill := max(inner-lipgloss.Width(trailer)-1, 0)
	return style.Render("╰"+strings.Repeat("─", fill)+" ") + trailer + style.Render("╯")
}

// boxContentLine renders "│ <content><pad> │" at exactly total visible
// columns, right-padding short content so the box's right border forms a
// clean vertical line regardless of how wide each row's content happens to
// be — the draft box previously skipped this padding for textarea lines,
// which is what made its right edge look broken/ragged.
func boxContentLine(content string, total int, style lipgloss.Style) string {
	width := max(total-4, 0)
	pad := max(width-lipgloss.Width(content), 0)
	return style.Render("│ ") + content + strings.Repeat(" ", pad) + style.Render(" │")
}

// renderDraftBox draws the floating "Draft note"/"Reply" box: a bordered
// frame with the file/status/line in its top border, the note textarea in
// the middle, and Save/Cancel affordances in its bottom border — rendered
// inline in the diff right after the line it's attached to, at the same
// indent and width as renderCommentCard so a draft nests visually like any
// other entry in the same thread rather than looking like a different size.
func (m Model) renderDraftBox() []string {
	total := m.boxWidth(len(commentThreadIndent))

	title := fmt.Sprintf(" Draft note - %s (%s) R%d ", m.file.Name(), fileStatus(m.file), m.draftKey.line)
	if m.draftIsReply {
		title = fmt.Sprintf(" Reply - %s R%d ", m.file.Name(), m.draftKey.line)
	}

	var lines []string
	lines = append(lines, commentThreadIndent+boxTopBorder(title, total, draftBorder))
	for _, taLine := range strings.Split(m.draftTextarea.View(), "\n") {
		lines = append(lines, commentThreadIndent+boxContentLine(taLine, total, draftBorder))
	}
	buttons := draftButton.Render(" Save (^S) ") + " " + draftButton.Render(" Cancel (Esc) ")
	lines = append(lines, commentThreadIndent+boxBottomBorder(buttons, total, draftBorder))
	return lines
}

// renderCommentCard draws one existing annotation as a GitHub-style bordered
// comment box: an author/timestamp header in the top border, the (word
// wrapped) comment text inside, colored per source like everywhere else
// annotations are distinguished (human/kode-agent/mcp:<client>).
func renderCommentCard(a annotate.Annotation, total int) []string {
	style := authorStyle(a.Author)
	header := fmt.Sprintf(" %s · %s ", a.Author, a.CreatedAt.Format("Jan 2 15:04"))

	var out []string
	out = append(out, commentThreadIndent+boxTopBorder(header, total, style))
	for _, line := range wrapText(a.Text, max(total-4, 1)) {
		out = append(out, commentThreadIndent+boxContentLine(line, total, style))
	}
	out = append(out, commentThreadIndent+boxBottomBorder("", total, style))
	return out
}

// wrapText greedily word-wraps s to width columns, splitting on existing
// newlines first so multi-paragraph notes keep their paragraph breaks.
func wrapText(s string, width int) []string {
	if width < 1 {
		width = 1
	}
	var out []string
	for _, paragraph := range strings.Split(s, "\n") {
		if paragraph == "" {
			out = append(out, "")
			continue
		}
		var line strings.Builder
		for _, word := range strings.Fields(paragraph) {
			switch {
			case line.Len() == 0:
				line.WriteString(word)
			case line.Len()+1+len(word) <= width:
				line.WriteString(" ")
				line.WriteString(word)
			default:
				out = append(out, line.String())
				line.Reset()
				line.WriteString(word)
			}
		}
		out = append(out, line.String())
	}
	return out
}

func (m *Model) renderLine(lexer chroma.Lexer, l diffparse.Line, isCursor, changed bool) string {
	sign := " "
	lineStyle := lipgloss.NewStyle()
	switch l.Op {
	case diffparse.OpAdd:
		sign = "+"
		lineStyle = addStyle
	case diffparse.OpDelete:
		sign = "-"
		lineStyle = delStyle
	}
	if isCursor {
		lineStyle = lineStyle.Background(cursorLineBg)
	}

	button := commentButtonBlank
	if changed {
		button = commentBtnStyle.Render(commentButtonGlyph)
	}

	// Long source lines must not be allowed to overflow the pane: the
	// viewport doesn't wrap, so an untruncated line just spills out past
	// the box border. Budget the remaining width after the button/gutter/
	// sign prefix, same as renderSplitSide does for the split view.
	code := highlight(lexer, &m.style, l.Content)
	if code == "" {
		code = l.Content
	}

	if !m.lineNumbers {
		return fmt.Sprintf("%s %s %s", button, lineStyle.Render(sign), code)
	}

	old := "    "
	if l.OldLineNo > 0 {
		old = fmt.Sprintf("%4d", l.OldLineNo)
	}
	newCol := "    "
	if l.NewLineNo > 0 {
		newCol = fmt.Sprintf("%4d", l.NewLineNo)
	}

	gutterText := old + " " + newCol
	gutter := gutterStyle.Render(gutterText)
	if isCursor {
		gutter = gutterStyle.Background(cursorLineBg).Render(gutterText)
	}
	return fmt.Sprintf("%s %s %s %s", button, gutter, lineStyle.Render(sign), code)
}

// renderSplitLine draws one side-by-side row: left and right columns of
// equal (±1) width separated by a gutter rule. Returns the row and the
// column index where the right half starts, so mouse clicks can be routed
// to the correct side.
func (m *Model) renderSplitLine(lexer chroma.Lexer, left diffparse.Line, hasLeft, isCursorLeft bool, right diffparse.Line, hasRight, isCursorRight bool) (string, int) {
	leftWidth := (m.width - 1) / 2
	if leftWidth < 0 {
		leftWidth = 0
	}
	rightWidth := m.width - 1 - leftWidth
	if rightWidth < 0 {
		rightWidth = 0
	}

	leftStr := m.renderSplitSide(lexer, left, hasLeft, isCursorLeft, leftWidth)
	rightStr := m.renderSplitSide(lexer, right, hasRight, isCursorRight, rightWidth)
	return leftStr + gutterStyle.Render("│") + rightStr, leftWidth + 1
}

// renderSplitSide draws one column (old or new) of a side-by-side row: the
// comment button, line number, +/- sign, and (width-truncated) code — or
// blank space if this side has no line, e.g. an added line has no old-file
// counterpart.
func (m *Model) renderSplitSide(lexer chroma.Lexer, l diffparse.Line, has, isCursor bool, width int) string {
	if !has {
		return padToWidth("", width)
	}

	sign := " "
	lineStyle := lipgloss.NewStyle()
	switch l.Op {
	case diffparse.OpAdd:
		sign = "+"
		lineStyle = addStyle
	case diffparse.OpDelete:
		sign = "-"
		lineStyle = delStyle
	}
	if isCursor {
		lineStyle = lineStyle.Background(cursorLineBg)
	}

	changed := l.Op != diffparse.OpContext
	button := commentButtonBlank
	if changed {
		button = commentBtnStyle.Render(commentButtonGlyph)
	}

	prefixWidth := lipgloss.Width(commentButtonGlyph) + 1 + 1 + 1 // button + space + sign + space
	var gutter string
	if m.lineNumbers {
		lineNo := "    "
		n := l.NewLineNo
		if l.Op == diffparse.OpDelete {
			n = l.OldLineNo
		}
		if n > 0 {
			lineNo = fmt.Sprintf("%4d", n)
		}
		gutter = gutterStyle.Render(lineNo)
		if isCursor {
			gutter = gutterStyle.Background(cursorLineBg).Render(lineNo)
		}
		prefixWidth += 4 + 1 // number + space
	}

	budget := max(width-prefixWidth, 0)
	truncated := truncateToWidth(l.Content, budget)
	code := highlight(lexer, &m.style, truncated)
	if code == "" {
		code = truncated
	}

	var line string
	if m.lineNumbers {
		line = fmt.Sprintf("%s %s %s %s", button, gutter, lineStyle.Render(sign), code)
	} else {
		line = fmt.Sprintf("%s %s %s", button, lineStyle.Render(sign), code)
	}
	return padToWidth(line, width)
}

// highlight renders a single line of source with chroma, returning ANSI
// terminal output. Each line is tokenised independently (rather than the
// whole file) since diff hunks interleave added/removed/context lines out of
// their original contiguous order.
func highlight(lexer chroma.Lexer, style *chroma.Style, line string) string {
	iterator, err := lexer.Tokenise(nil, line)
	if err != nil {
		return ""
	}
	var buf bytes.Buffer
	if err := formatters.TTY256.Format(&buf, style, iterator); err != nil {
		return ""
	}
	return strings.TrimRight(buf.String(), "\n")
}
