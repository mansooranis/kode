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

// rowMeta records what a single rendered row corresponds to, so mouse clicks
// (given only an absolute row number) can be mapped back to a diff line.
type rowMeta struct {
	ordinal   int  // index into the flat list of diff lines; -1 if not a code line
	changed   bool // true for add/delete lines, i.e. lines that get a comment button
	replyLine int  // >0 if this row is the "[Reply]" button for this canonical line number
}

type Model struct {
	viewport    viewport.Model
	file        diffparse.FileDiff
	annotations []annotate.Annotation
	lineNumbers bool
	width       int
	height      int

	cursor     int // ordinal index into commentable diff lines, 0-based
	totalLines int
	cursorRow  int // row within rendered content where the cursor line starts
	rows       []rowMeta

	draftActive   bool
	draftOrdinal  int  // which diff line ordinal the open draft is attached to
	draftLine     int  // canonical line number, captured at activation time
	draftIsReply  bool // true if the line already had a thread when the draft opened
	draftTextarea textarea.Model

	submittedLine int
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
// line number if present, else old-file line number) so the caller can
// attach a new comment to it. ok is false if the file has no lines.
func (m Model) CursorTarget() (line int, ok bool) {
	i := 0
	for _, hunk := range m.file.Hunks {
		for _, l := range hunk.Lines {
			if i == m.cursor {
				if l.NewLineNo > 0 {
					return l.NewLineNo, true
				}
				return l.OldLineNo, true
			}
			i++
		}
	}
	return 0, false
}

// ordinalForLine finds the diff-line ordinal whose canonical line number
// (new-file if present, else old-file) matches line, so a click on a
// "[Reply]" button (which isn't itself the line's own row) can move the
// cursor there before opening the draft box.
func (m Model) ordinalForLine(line int) (int, bool) {
	i := 0
	for _, hunk := range m.file.Hunks {
		for _, l := range hunk.Lines {
			key := l.NewLineNo
			if key == 0 {
				key = l.OldLineNo
			}
			if key == line {
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
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= m.totalLines {
		m.cursor = m.totalLines - 1
	}
	m.render()

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
		}
	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			absRow := msg.Y + m.viewport.YOffset
			if absRow >= 0 && absRow < len(m.rows) {
				meta := m.rows[absRow]

				if meta.replyLine > 0 {
					onButton := msg.X < len(commentThreadIndent)+len(replyButtonGlyph)
					if onButton {
						if ord, ok := m.ordinalForLine(meta.replyLine); ok {
							m.cursor = ord
							return m.activateDraft(), nil, false
						}
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
	line, ok := m.CursorTarget()
	if !ok {
		return m
	}

	ta := textarea.New()
	ta.Placeholder = "Write a note..."
	ta.SetWidth(m.boxWidth(0) - 4)
	ta.SetHeight(draftTextareaHeight)
	ta.ShowLineNumbers = false
	ta.Focus()

	m.draftActive = true
	m.draftOrdinal = m.cursor
	m.draftLine = line
	m.draftIsReply = false
	for _, a := range m.annotations {
		if a.Line == line {
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
			m.submittedLine = m.draftLine
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

// TakeSubmission returns the note text and target line captured by the most
// recent ctrl+s submission. Call this once, immediately after Update returns
// true, to persist it — the values are not cleared automatically since
// diffview has no store of its own to write to.
func (m Model) TakeSubmission() (line int, text string) {
	return m.submittedLine, m.submittedText
}

func (m Model) View() string {
	return m.viewport.View()
}

func (m *Model) render() {
	lexer := lexers.Match(m.file.Name())
	if lexer == nil {
		lexer = lexers.Fallback
	}

	annotationsByLine := map[int][]annotate.Annotation{}
	for _, a := range m.annotations {
		annotationsByLine[a.Line] = append(annotationsByLine[a.Line], a)
	}

	if m.draftActive {
		// The pane may have been resized since the draft was opened.
		m.draftTextarea.SetWidth(m.boxWidth(0) - 4)
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

		for _, l := range hunk.Lines {
			isCursor := ordinal == m.cursor
			if isCursor {
				m.cursorRow = row
			}
			changed := l.Op == diffparse.OpAdd || l.Op == diffparse.OpDelete
			writeLine(m.renderLine(lexer, l, isCursor, changed), rowMeta{ordinal: ordinal, changed: changed})

			key := l.NewLineNo
			if key == 0 {
				key = l.OldLineNo
			}
			thread := annotationsByLine[key]
			for _, a := range thread {
				for _, line := range renderCommentCard(a, m.boxWidth(len(commentThreadIndent))) {
					writeLine(line, rowMeta{ordinal: -1})
				}
			}
			// The reply button is suppressed while a draft is already open on
			// this exact line — the open draft box IS the reply affordance
			// at that point, showing both would be redundant.
			if len(thread) > 0 && !(m.draftActive && ordinal == m.draftOrdinal) {
				writeLine(commentThreadIndent+commentBtnStyle.Render(replyButtonGlyph), rowMeta{ordinal: -1, replyLine: key})
			}
			if m.draftActive && ordinal == m.draftOrdinal {
				for _, line := range m.renderDraftBox() {
					writeLine(line, rowMeta{ordinal: -1})
				}
			}
			ordinal++
		}
	}
	m.totalLines = ordinal

	m.viewport.SetContent(strings.TrimRight(b.String(), "\n"))
}

// renderDraftBox draws the floating "Draft note" box: a bordered frame with
// the file/status/line in its top border, the note textarea in the middle,
// and Save/Cancel affordances in its bottom border — rendered inline in the
// diff right after the line it's attached to, rather than as a screen-level
// overlay, so it reads like a threaded reply the same way submitted notes do.
func (m Model) renderDraftBox() []string {
	innerWidth := m.boxWidth(0)

	title := fmt.Sprintf(" Draft note - %s (%s) R%d ", m.file.Name(), fileStatus(m.file), m.draftLine)
	if m.draftIsReply {
		title = fmt.Sprintf(" Reply - %s R%d ", m.file.Name(), m.draftLine)
	}
	if len(title) > innerWidth-2 {
		title = title[:innerWidth-2]
	}
	topPad := innerWidth - 2 - len(title)
	if topPad < 0 {
		topPad = 0
	}
	top := draftBorder.Render("╭─"+title) + draftBorder.Render(strings.Repeat("─", topPad)+"╮")

	var lines []string
	lines = append(lines, top)
	for _, taLine := range strings.Split(m.draftTextarea.View(), "\n") {
		lines = append(lines, draftBorder.Render("│ ")+taLine+draftBorder.Render(" │"))
	}

	buttons := draftButton.Render(" Save (^S) ") + " " + draftButton.Render(" Cancel (Esc) ")
	bottomFill := innerWidth - 2 - lipgloss.Width(buttons) - 1
	if bottomFill < 0 {
		bottomFill = 0
	}
	bottom := draftBorder.Render("╰"+strings.Repeat("─", bottomFill)+" ") + buttons + draftBorder.Render("╯")
	lines = append(lines, bottom)

	return lines
}

// renderCommentCard draws one existing annotation as a GitHub-style bordered
// comment box: an author/timestamp header in the top border, the (word
// wrapped) comment text inside, colored per source like everywhere else
// annotations are distinguished (human/kode-agent/mcp:<client>).
func renderCommentCard(a annotate.Annotation, width int) []string {
	style := authorStyle(a.Author)
	innerWidth := width - 2
	if innerWidth < 1 {
		innerWidth = 1
	}

	header := fmt.Sprintf(" %s · %s ", a.Author, a.CreatedAt.Format("Jan 2 15:04"))
	if len(header) > innerWidth {
		header = header[:innerWidth]
	}
	topPad := innerWidth - len(header)
	if topPad < 0 {
		topPad = 0
	}

	var out []string
	out = append(out, commentThreadIndent+style.Render("╭─"+header+strings.Repeat("─", topPad)+"╮"))
	for _, line := range wrapText(a.Text, innerWidth-2) {
		pad := innerWidth - 2 - lipgloss.Width(line)
		if pad < 0 {
			pad = 0
		}
		out = append(out, commentThreadIndent+style.Render("│ ")+line+strings.Repeat(" ", pad)+style.Render(" │"))
	}
	out = append(out, commentThreadIndent+style.Render("╰"+strings.Repeat("─", innerWidth)+"╯"))
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
