// Package sidebar renders the file-list navigation pane: one entry per
// changed file, with the currently selected file highlighted.
package sidebar

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mansooranis/kode/internal/diffparse"
)

var (
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	normalStyle   = lipgloss.NewStyle()
	addedStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	deletedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("204"))
)

type Model struct {
	files   []diffparse.FileDiff
	cursor  int
	width   int
	height  int
	focused bool
}

func New() Model {
	return Model{}
}

func (m *Model) SetFiles(files []diffparse.FileDiff) {
	m.files = files
	if m.cursor >= len(files) {
		m.cursor = 0
	}
}

func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
}

func (m *Model) SetFocused(f bool) {
	m.focused = f
}

// Selected returns the currently selected file and whether one exists.
func (m Model) Selected() (diffparse.FileDiff, bool) {
	if m.cursor < 0 || m.cursor >= len(m.files) {
		return diffparse.FileDiff{}, false
	}
	return m.files[m.cursor], true
}

func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles navigation keys. It returns whether the selection changed,
// so the parent model knows to refresh the diff view.
func (m Model) Update(msg tea.Msg) (Model, bool) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				return m, true
			}
		case "down", "j":
			if m.cursor < len(m.files)-1 {
				m.cursor++
				return m, true
			}
		}
	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			// Row 0 is the pane title; file rows start at row 1.
			idx := msg.Y - 1
			if idx >= 0 && idx < len(m.files) {
				m.cursor = idx
				return m, true
			}
		}
	}
	return m, false
}

// namePrefixWidth is how many columns "%s%s %s" burns on the prefix, status
// glyph, and separating space before the file name itself starts.
const namePrefixWidth = 4

// truncateName cuts name down to fit within width visual columns, replacing
// the cut tail with an ellipsis so long paths don't wrap onto a second row —
// wrapping there shifts every row after it and corrupts the whole pane's
// row-to-file mapping that mouse clicks and rendering rely on.
func truncateName(name string, width int) string {
	if width <= 0 || lipgloss.Width(name) <= width {
		return name
	}
	if width <= 1 {
		return strings.Repeat("…", width)
	}
	w := 0
	cut := len(name)
	for i, r := range name {
		rw := lipgloss.Width(string(r))
		if w+rw > width-1 {
			cut = i
			break
		}
		w += rw
	}
	return name[:cut] + "…"
}

func (m Model) View() string {
	var b strings.Builder
	for i, f := range m.files {
		style := normalStyle
		prefix := "  "
		if i == m.cursor {
			style = selectedStyle
			prefix = "> "
		}

		status := " "
		statusStyle := normalStyle
		switch {
		case f.IsNew:
			status, statusStyle = "A", addedStyle
		case f.IsDelete:
			status, statusStyle = "D", deletedStyle
		case f.IsRename:
			status = "R"
		}

		name := truncateName(f.Name(), max(m.width-namePrefixWidth, 0))
		line := fmt.Sprintf("%s%s %s", prefix, statusStyle.Render(status), name)
		b.WriteString(style.Render(line))
		b.WriteString("\n")
	}
	content := strings.TrimRight(b.String(), "\n")
	return lipgloss.NewStyle().Width(m.width).Height(m.height).Render(content)
}
