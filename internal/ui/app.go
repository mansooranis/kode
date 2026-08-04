// Package ui holds kode's root Bubble Tea model, which owns overall layout
// (split/stack) and routes messages to child bubbles. Child bubbles (sidebar,
// diffview, chat, diagram) each own their own Update/View and are never
// mutated directly by the root — this keeps the Elm-architecture Update loop
// from becoming an unmanageable single switch statement as more panes land.
package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mansooranis/kode/internal/annotate"
	"github.com/mansooranis/kode/internal/config"
	"github.com/mansooranis/kode/internal/diffparse"
	"github.com/mansooranis/kode/internal/ui/diffview"
	"github.com/mansooranis/kode/internal/ui/sidebar"
	"github.com/mansooranis/kode/internal/update"
)

// AnnotationAddedMsg is sent into the running program whenever the shared
// annotate.Store gains a new entry — whether from a local draft note or
// from an external MCP caller (e.g. Claude Code) answering a comment. main
// wires annotate.Store.OnChange to send this so the diff view repaints
// regardless of where the annotation came from.
type AnnotationAddedMsg annotate.Annotation

type focusTarget int

const (
	focusSidebar focusTarget = iota
	focusDiff
)

// titleHeight is the one row each pane's title ("Files (N)" / the current
// filename) takes above its content. Both panes reserve it in layout(), but
// only sidebar.Update compensates for it internally (idx := msg.Y - 1);
// diffview.Update was built title-agnostic (embeddable component, no
// knowledge of what wraps it), so mouse coordinates forwarded to it must
// have this subtracted here — omitting it made every click into the diff
// pane land one row below what the user actually clicked.
const titleHeight = 1

type box struct{ x, y, w, h int }

func (b box) contains(x, y int) bool {
	return x >= b.x && x < b.x+b.w && y >= b.y && y < b.y+b.h
}

type App struct {
	cfg             config.Config
	changeset       diffparse.Changeset
	store           *annotate.Store
	annotationsPath string

	sidebar  sidebar.Model
	diffview diffview.Model

	width, height int
	mode          string // "split" | "stack"
	focus         focusTarget

	sidebarBox box
	diffBox    box

	statusMsg    string
	updateBanner string // e.g. "v0.2.0"; empty means no update available
}

func NewApp(cfg config.Config, changeset diffparse.Changeset, store *annotate.Store, annotationsPath string) App {
	sb := sidebar.New()
	sb.SetFiles(changeset.Files)
	sb.SetFocused(true)

	dv := diffview.New()

	mode := cfg.Mode
	if mode == "" || mode == "auto" {
		mode = "split"
	}

	a := App{
		cfg:             cfg,
		changeset:       changeset,
		store:           store,
		annotationsPath: annotationsPath,
		sidebar:         sb,
		diffview:        dv,
		mode:            mode,
		focus:           focusSidebar,
	}
	if f, ok := a.sidebar.Selected(); ok {
		a.diffview.SetFile(f)
		a.diffview.SetAnnotations(a.store.ForFile(f.Name()))
	}
	return a
}

func (a App) Init() tea.Cmd {
	return nil
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.layout()
		return a, nil

	case AnnotationAddedMsg:
		if f, ok := a.sidebar.Selected(); ok && f.Name() == msg.File {
			a.diffview.SetAnnotations(a.store.ForFile(f.Name()))
		}
		return a, nil

	case update.AvailableMsg:
		a.updateBanner = msg.Latest
		return a, nil

	case tea.KeyMsg:
		// While the diff pane's inline draft-note box is open, every key
		// (including "q") must go to it — global shortcuts only apply when
		// nothing is being typed.
		if a.diffview.DraftActive() {
			return a.forwardToDiffview(msg)
		}

		if msg.String() != "r" {
			a.statusMsg = ""
		}

		switch msg.String() {
		case "q", "ctrl+c":
			return a, tea.Quit
		case "tab":
			if a.focus == focusSidebar {
				a.focus = focusDiff
			} else {
				a.focus = focusSidebar
			}
			a.sidebar.SetFocused(a.focus == focusSidebar)
			return a, nil
		case "m":
			if a.mode == "split" {
				a.mode = "stack"
			} else {
				a.mode = "split"
			}
			a.layout()
			return a, nil
		case "v":
			// A pane-level view toggle, same as "m" above — handled here
			// rather than left to reach diffview only when it has focus
			// (a.focus == focusDiff), since the sidebar has focus by
			// default and would otherwise silently swallow the keypress.
			a.diffview.SetSplitView(!a.diffview.SplitView())
			return a, nil
		case "r":
			return a.reloadAnnotations()
		}

		if a.focus == focusSidebar {
			var changed bool
			a.sidebar, changed = a.sidebar.Update(msg)
			if changed {
				a.onFileSelected()
			}
			return a, nil
		}

		return a.forwardToDiffview(msg)

	case tea.MouseMsg:
		switch {
		case a.sidebarBox.contains(msg.X, msg.Y):
			a.focus = focusSidebar
			a.sidebar.SetFocused(true)
			local := msg
			local.X -= a.sidebarBox.x
			local.Y -= a.sidebarBox.y
			var changed bool
			a.sidebar, changed = a.sidebar.Update(local)
			if changed {
				a.onFileSelected()
			}
			return a, nil
		case a.diffBox.contains(msg.X, msg.Y):
			a.focus = focusDiff
			a.sidebar.SetFocused(false)
			local := msg
			local.X -= a.diffBox.x
			local.Y -= a.diffBox.y + titleHeight
			return a.forwardToDiffview(local)
		}
	}
	return a, nil
}

// forwardToDiffview routes a message to the diff pane and, if it reports a
// note was just submitted (ctrl+s), persists it to the shared store.
func (a App) forwardToDiffview(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd       tea.Cmd
		submitted bool
	)
	a.diffview, cmd, submitted = a.diffview.Update(msg)
	if submitted {
		if f, ok := a.sidebar.Selected(); ok {
			line, text := a.diffview.TakeSubmission()
			a.store.Add(annotate.Annotation{
				File:   f.Name(),
				Line:   line,
				Author: annotate.Human,
				Text:   text,
			})
		}
	}
	return a, cmd
}

// reloadAnnotations re-reads the annotations JSON file and merges in any
// entries pushed there directly (by a human, script, or agent not connected
// live over MCP). New entries reach the diff view via the store's onChange
// bridge (same path MCP-added annotations already use), so this just needs
// to trigger the reload and report how many were new.
func (a App) reloadAnnotations() (tea.Model, tea.Cmd) {
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
	return a, nil
}

func (a *App) onFileSelected() {
	if f, ok := a.sidebar.Selected(); ok {
		a.diffview.SetFile(f)
		a.diffview.SetAnnotations(a.store.ForFile(f.Name()))
	}
}

func (a *App) layout() {
	statusHeight := 1
	contentHeight := a.height - statusHeight
	if contentHeight < 0 {
		contentHeight = 0
	}

	if a.mode == "stack" {
		sidebarHeight := len(a.changeset.Files) + titleHeight
		maxSidebarHeight := contentHeight / 3
		if sidebarHeight > maxSidebarHeight {
			sidebarHeight = maxSidebarHeight
		}
		if sidebarHeight < titleHeight+1 {
			sidebarHeight = titleHeight + 1
		}

		a.sidebarBox = box{x: 0, y: 0, w: a.width, h: sidebarHeight}
		diffY := sidebarHeight
		diffHeight := contentHeight - sidebarHeight
		if diffHeight < 0 {
			diffHeight = 0
		}
		a.diffBox = box{x: 0, y: diffY, w: a.width, h: diffHeight}
	} else {
		sidebarWidth := a.width / 4
		if sidebarWidth < 20 {
			sidebarWidth = 20
		}
		if sidebarWidth > 40 {
			sidebarWidth = 40
		}
		if sidebarWidth > a.width {
			sidebarWidth = a.width
		}
		diffWidth := a.width - sidebarWidth - 1
		if diffWidth < 0 {
			diffWidth = 0
		}

		a.sidebarBox = box{x: 0, y: 0, w: sidebarWidth, h: contentHeight}
		a.diffBox = box{x: sidebarWidth + 1, y: 0, w: diffWidth, h: contentHeight}
	}

	a.sidebar.SetSize(a.sidebarBox.w, a.sidebarBox.h-titleHeight)
	a.diffview.SetSize(a.diffBox.w, a.diffBox.h-titleHeight)
}

var (
	titleStyle        = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	statusStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	updateBannerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("214")).Bold(true).Padding(0, 1)
)

// withUpdateBanner right-aligns banner within width columns, on the same row
// as left — the footer's keybinding hints or status message. If there isn't
// room for both, the banner wins (it's the more actionable of the two).
func withUpdateBanner(width int, left, banner string) string {
	if banner == "" {
		return left
	}
	lw := lipgloss.Width(left)
	bw := lipgloss.Width(banner)
	gap := width - lw - bw
	if gap < 1 {
		if bw >= width {
			return banner
		}
		return strings.Repeat(" ", width-bw) + banner
	}
	return left + strings.Repeat(" ", gap) + banner
}

func (a App) View() string {
	if a.width == 0 {
		return "loading..."
	}

	sidebarPane := lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render(fmt.Sprintf("Files (%d)", len(a.changeset.Files))),
		a.sidebar.View(),
	)
	diffPane := lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render(a.currentFileName()),
		a.diffview.View(),
	)

	var body string
	if a.mode == "stack" {
		body = lipgloss.JoinVertical(lipgloss.Left, sidebarPane, diffPane)
	} else {
		body = lipgloss.JoinHorizontal(lipgloss.Top, sidebarPane, " ", diffPane)
	}

	var footer string
	if a.statusMsg != "" {
		footer = statusStyle.Render(a.statusMsg)
	} else {
		footer = statusStyle.Render(strings.Join([]string{
			"↑/↓ or j/k: navigate", "tab: switch pane", "c or click [+]: comment", "r: refresh", "m: toggle layout", "v: unified/split diff", "q: quit",
		}, "  •  "))
	}
	if a.updateBanner != "" {
		footer = withUpdateBanner(a.width, footer, updateBannerStyle.Render(fmt.Sprintf("update %s available", a.updateBanner)))
	}

	return lipgloss.JoinVertical(lipgloss.Left, body, footer)
}

func (a App) currentFileName() string {
	if f, ok := a.sidebar.Selected(); ok {
		return f.Name()
	}
	return "(no file selected)"
}
