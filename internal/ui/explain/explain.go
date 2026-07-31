// Package explain renders kode's read-only "learning" viewer: a feed of
// explanations (and Mermaid-rendered diagrams) an agent has left in the
// shared annotate.Store, grouped by file and ordered by line — a generated
// walkthrough, not a diff review. It has no local comment authoring; notes
// are meant to arrive from an external agent writing directly to the
// annotations JSON file (see .claude/skills/kode-comments/SKILL.md), and
// this view just displays whatever's there once refreshed ("r").
package explain

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mansooranis/kode/internal/annotate"
)

// AnnotationAddedMsg is sent whenever the shared store gains a new entry
// (from a JSON file reload), mirroring ui.AnnotationAddedMsg so the viewer
// repaints immediately without polling.
type AnnotationAddedMsg annotate.Annotation

const titleHeight = 1
const contextLines = 2 // lines of surrounding code shown above/below an annotation

type focusTarget int

const (
	focusFiles focusTarget = iota
	focusMain
)

type box struct{ x, y, w, h int }

func (b box) contains(x, y int) bool {
	return x >= b.x && x < b.x+b.w && y >= b.y && y < b.y+b.h
}

type App struct {
	store           *annotate.Store
	annotationsPath string

	files      []string
	fileCursor int

	viewport viewport.Model

	width, height int
	focus         focusTarget

	filesBox box
	mainBox  box

	statusMsg string
}

func NewApp(store *annotate.Store, annotationsPath string) App {
	a := App{
		store:           store,
		annotationsPath: annotationsPath,
		viewport:        viewport.New(0, 0),
		focus:           focusFiles,
	}
	a.refreshFiles()
	return a
}

func (a App) Init() tea.Cmd {
	return nil
}

func (a *App) refreshFiles() {
	seen := map[string]bool{}
	var files []string
	for _, an := range a.store.All() {
		if !seen[an.File] {
			seen[an.File] = true
			files = append(files, an.File)
		}
	}
	sort.Strings(files)
	a.files = files
	if a.fileCursor >= len(a.files) {
		a.fileCursor = 0
	}
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.layout()
		a.render()
		return a, nil

	case AnnotationAddedMsg:
		a.refreshFiles()
		a.render()
		return a, nil

	case tea.KeyMsg:
		if msg.String() != "r" {
			a.statusMsg = ""
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return a, tea.Quit
		case "tab":
			if a.focus == focusFiles {
				a.focus = focusMain
			} else {
				a.focus = focusFiles
			}
			return a, nil
		case "r":
			return a.reload()
		}

		if a.focus == focusFiles {
			switch msg.String() {
			case "up", "k":
				if a.fileCursor > 0 {
					a.fileCursor--
					a.render()
				}
			case "down", "j":
				if a.fileCursor < len(a.files)-1 {
					a.fileCursor++
					a.render()
				}
			}
			return a, nil
		}

		var cmd tea.Cmd
		a.viewport, cmd = a.viewport.Update(msg)
		return a, cmd

	case tea.MouseMsg:
		switch {
		case a.filesBox.contains(msg.X, msg.Y):
			a.focus = focusFiles
			if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
				idx := msg.Y - a.filesBox.y - titleHeight
				if idx >= 0 && idx < len(a.files) {
					a.fileCursor = idx
					a.render()
				}
			}
			return a, nil
		case a.mainBox.contains(msg.X, msg.Y):
			a.focus = focusMain
			var cmd tea.Cmd
			a.viewport, cmd = a.viewport.Update(msg)
			return a, cmd
		}
	}
	return a, nil
}

// reload re-reads the persisted annotations JSON file, merging in anything
// pushed there directly. New entries also arrive live via AnnotationAddedMsg
// (the store's OnChange bridge, wired in cmd/kode/main.go) — this is the
// manual fallback.
func (a App) reload() (tea.Model, tea.Cmd) {
	added, err := a.store.Reload(a.annotationsPath)
	switch {
	case err != nil:
		a.statusMsg = fmt.Sprintf("reload failed: %v", err)
	case added == 0:
		a.statusMsg = "no new annotations"
	case added == 1:
		a.statusMsg = "loaded 1 new annotation"
	default:
		a.statusMsg = fmt.Sprintf("loaded %d new annotations", added)
	}
	a.refreshFiles()
	a.render()
	return a, nil
}

func (a *App) layout() {
	statusHeight := 1
	contentHeight := a.height - statusHeight
	if contentHeight < 0 {
		contentHeight = 0
	}

	filesWidth := a.width / 4
	if filesWidth < 20 {
		filesWidth = 20
	}
	if filesWidth > 40 {
		filesWidth = 40
	}
	if filesWidth > a.width {
		filesWidth = a.width
	}
	mainWidth := a.width - filesWidth - 1
	if mainWidth < 0 {
		mainWidth = 0
	}

	a.filesBox = box{x: 0, y: 0, w: filesWidth, h: contentHeight}
	a.mainBox = box{x: filesWidth + 1, y: 0, w: mainWidth, h: contentHeight}

	a.viewport.Width = a.mainBox.w
	a.viewport.Height = a.mainBox.h - titleHeight
}

func (a *App) currentFile() (string, bool) {
	if a.fileCursor < 0 || a.fileCursor >= len(a.files) {
		return "", false
	}
	return a.files[a.fileCursor], true
}

// render rebuilds the main pane's content for the currently selected file.
func (a *App) render() {
	file, ok := a.currentFile()
	if !ok {
		a.viewport.SetContent(emptyStateText(a.annotationsPath))
		return
	}

	annotations := a.store.ForFile(file)
	sort.Slice(annotations, func(i, j int) bool { return annotations[i].Line < annotations[j].Line })

	source, _ := os.ReadFile(file)
	var sourceLines []string
	if source != nil {
		sourceLines = strings.Split(string(source), "\n")
	}

	var b strings.Builder
	for i, ann := range annotations {
		if i > 0 {
			b.WriteString("\n")
		}
		for _, line := range renderSnippet(sourceLines, ann.Line) {
			b.WriteString(line)
			b.WriteString("\n")
		}
		for _, line := range renderNoteCard(ann, a.viewport.Width) {
			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	a.viewport.SetContent(strings.TrimRight(b.String(), "\n"))
}

var (
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	statusStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	selectedFile = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	normalFile   = lipgloss.NewStyle()
	gutterStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	lineNoStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
)

func (a App) View() string {
	if a.width == 0 {
		return "loading..."
	}

	var filesList strings.Builder
	for i, f := range a.files {
		style := normalFile
		prefix := "  "
		if i == a.fileCursor {
			style = selectedFile
			prefix = "> "
		}
		filesList.WriteString(style.Render(prefix + f))
		filesList.WriteString("\n")
	}
	filesPane := lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render(fmt.Sprintf("Explained files (%d)", len(a.files))),
		lipgloss.NewStyle().Width(a.filesBox.w).Height(a.filesBox.h-titleHeight).Render(strings.TrimRight(filesList.String(), "\n")),
	)

	mainTitle := "(select a file)"
	if f, ok := a.currentFile(); ok {
		mainTitle = f
	}
	mainPane := lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render(mainTitle),
		a.viewport.View(),
	)

	body := lipgloss.JoinHorizontal(lipgloss.Top, filesPane, " ", mainPane)

	var footer string
	if a.statusMsg != "" {
		footer = statusStyle.Render(a.statusMsg)
	} else {
		footer = statusStyle.Render(strings.Join([]string{
			"↑/↓ or j/k: navigate", "tab: switch pane", "r: refresh", "q: quit",
		}, "  •  "))
	}

	return lipgloss.JoinVertical(lipgloss.Left, body, footer)
}

// renderSnippet shows a few lines of source around line for context, best
// effort: a missing file or out-of-range line just means no snippet, never
// an error — this view must stay usable even for deleted/renamed files.
func renderSnippet(sourceLines []string, line int) []string {
	if sourceLines == nil || line < 1 || line > len(sourceLines) {
		return nil
	}
	start := line - contextLines
	if start < 1 {
		start = 1
	}
	end := line + contextLines
	if end > len(sourceLines) {
		end = len(sourceLines)
	}

	var out []string
	for n := start; n <= end; n++ {
		style := lineNoStyle
		marker := "  "
		if n == line {
			marker = "> "
		}
		out = append(out, fmt.Sprintf("%s%s%s", marker, style.Render(fmt.Sprintf("%4d ", n)), sourceLines[n-1]))
	}
	return out
}

// renderNoteCard draws one annotation as a bordered card: a comment gets its
// text word-wrapped, a diagram's pre-rendered ASCII art is shown verbatim
// (unwrapped) so its alignment isn't corrupted.
func renderNoteCard(a annotate.Annotation, width int) []string {
	style := authorStyle(a.Author)
	innerWidth := width - 2
	if innerWidth < 1 {
		innerWidth = 1
	}

	kind := a.Kind
	if kind == "" {
		kind = annotate.KindComment
	}
	header := fmt.Sprintf(" %s · %s · %s ", a.Author, kind, a.CreatedAt.Format("Jan 2 15:04"))
	if len(header) > innerWidth {
		header = header[:innerWidth]
	}
	topPad := innerWidth - len(header)
	if topPad < 0 {
		topPad = 0
	}

	var out []string
	out = append(out, style.Render("╭─"+header+strings.Repeat("─", topPad)+"╮"))

	var contentLines []string
	if kind == annotate.KindDiagram {
		contentLines = strings.Split(a.Text, "\n")
	} else {
		contentLines = wrapText(a.Text, innerWidth-2)
	}
	for _, line := range contentLines {
		pad := innerWidth - 2 - lipgloss.Width(line)
		if pad < 0 {
			pad = 0
		}
		out = append(out, style.Render("│ ")+line+strings.Repeat(" ", pad)+style.Render(" │"))
	}
	out = append(out, style.Render("╰"+strings.Repeat("─", innerWidth)+"╯"))
	return out
}

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

func emptyStateText(annotationsPath string) string {
	return strings.Join([]string{
		"No annotations yet.",
		"",
		"Ask an agent (e.g. Claude Code) to explain this codebase — with the",
		"kode-comments skill installed (\"kode skill install\"), it will write",
		fmt.Sprintf("notes and diagrams directly to %s as it goes.", annotationsPath),
		"",
		"Press r to refresh once it has written some.",
	}, "\n")
}
