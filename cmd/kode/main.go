package main

import (
	"context"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mansooranis/kode/internal/agent/skills/bundled"
	"github.com/mansooranis/kode/internal/annotate"
	"github.com/mansooranis/kode/internal/buildinfo"
	"github.com/mansooranis/kode/internal/config"
	"github.com/mansooranis/kode/internal/diagram"
	"github.com/mansooranis/kode/internal/diffparse"
	"github.com/mansooranis/kode/internal/mcpserver"
	"github.com/mansooranis/kode/internal/ui"
	"github.com/mansooranis/kode/internal/ui/explain"
	"github.com/mansooranis/kode/internal/vcs/git"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "kode: failed to load config: %v\n", err)
		os.Exit(1)
	}

	args := os.Args[1:]
	switch {
	case len(args) >= 1 && (args[0] == "help" || args[0] == "-h" || args[0] == "--help"):
		printHelp()
		return
	case len(args) >= 1 && args[0] == "explain":
		runExplain(cfg)
		return
	case len(args) >= 1 && args[0] == "version":
		fmt.Println(buildinfo.Version)
		return
	case len(args) >= 2 && args[0] == "skills" && args[1] == "sync":
		if err := syncSkills(cfg, true); err != nil {
			fmt.Fprintf(os.Stderr, "kode: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Best-effort: keep the global skills dir in step with this binary. A
	// Homebrew upgrade just swaps the binary, so this is what actually
	// propagates bundled skill updates, since there's no separate install hook.
	_ = syncSkills(cfg, false)

	src := git.New("")

	var diffText []byte
	switch {
	case len(args) >= 2 && args[0] == "show":
		diffText, err = src.Show(args[1])
	default:
		diffText, err = src.Diff()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "kode: %v\n", err)
		os.Exit(1)
	}

	changeset, err := diffparse.Parse(diffText)
	if err != nil {
		fmt.Fprintf(os.Stderr, "kode: failed to parse diff: %v\n", err)
		os.Exit(1)
	}

	if len(changeset.Files) == 0 {
		fmt.Println("kode: no changes to review")
		return
	}

	store := annotate.NewStore()
	store.SetPersistPath(cfg.Annotations.FilePath)
	if _, err := store.Reload(cfg.Annotations.FilePath); err != nil {
		fmt.Fprintf(os.Stderr, "kode: failed to load %s: %v\n", cfg.Annotations.FilePath, err)
		os.Exit(1)
	}

	p := tea.NewProgram(ui.NewApp(cfg, changeset, store, cfg.Annotations.FilePath), tea.WithAltScreen(), tea.WithMouseCellMotion())

	store.OnChange(func(a annotate.Annotation) {
		// Add can be called synchronously from within Update itself (a local
		// "c" comment), and p.Send blocks until the program's event loop
		// goroutine reads it — which is exactly the goroutine stuck inside
		// Update in that case. Sending from a fresh goroutine every time
		// avoids that self-deadlock regardless of the caller's origin.
		go p.Send(ui.AnnotationAddedMsg(a))
	})

	var mcpSrv *mcpserver.Server
	if cfg.MCPServer.Enabled {
		mcpSrv = mcpserver.New(store, changeset, diagram.NewCLIRenderer(), cfg.MCPServer.Port)
		mcpSrv.Start()
		fmt.Printf("kode: mcp server listening on http://%s/mcp\n", mcpSrv.Addr())
		fmt.Printf("kode: one-time setup: claude mcp add --transport http kode http://%s/mcp\n", mcpSrv.Addr())
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			mcpSrv.Close(ctx)
		}()
	}

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "kode: %v\n", err)
		os.Exit(1)
	}
}

// printHelp lists every command kode understands. Keep this in sync with the
// switch in main(): it's the one place a new user (or a Homebrew "kode
// --help" pipe) should be able to see everything the binary can do.
func printHelp() {
	fmt.Println(`kode is a terminal diff reviewer with a built-in AI agent.

Usage:
  kode                Review the current working diff (git diff) in the TUI
  kode show <ref>     Review a single commit, e.g. "kode show HEAD~1"
  kode explain        Open the read-only codebase walkthrough viewer
  kode skills sync    Copy kode's bundled skills into your global skills folder now
  kode version        Print the installed version
  kode help           Show this message

While the diff viewer is open, kode also starts a local MCP server (see the
"mcp_server" section of your config) so a separate agent session, such as
Claude Code, can read the diff and leave comments on it live. Run
"kode explain" the same way to get annotations and diagrams for a codebase
walkthrough instead of a diff.

Configuration lives at .kode/config.toml in your project, falling back to
~/.config/kode/config.toml, then built-in defaults. See the README for the
full list of settings.`)
}

// syncSkills writes kode's bundled skills (internal/agent/skills/bundled)
// into cfg.Agent.SkillsPath. force always re-syncs, e.g. for the explicit
// `kode skills sync` command; otherwise it's a no-op once per version, per
// bundled.EnsureSynced.
func syncSkills(cfg config.Config, force bool) error {
	version := buildinfo.Version
	synced, names, err := bundled.EnsureSynced(cfg.Agent.SkillsPath, version, force)
	if err != nil {
		return fmt.Errorf("sync skills: %w", err)
	}
	if force {
		fmt.Printf("kode: synced %d skill(s) to %s: %v\n", len(names), cfg.Agent.SkillsPath, names)
	} else if synced && len(names) > 0 {
		fmt.Fprintf(os.Stderr, "kode: updated bundled skills in %s for v%s\n", cfg.Agent.SkillsPath, version)
	}
	return nil
}

// runExplain starts kode's read-only "learning" viewer: no diff/VCS
// involved, just whatever annotations/diagrams already exist in the store
// (typically pushed there by an external agent connected over MCP, per
// .claude/skills/kode-comments/SKILL.md) plus the same MCP server so an
// agent can keep adding to it live while it's open.
func runExplain(cfg config.Config) {
	store := annotate.NewStore()
	store.SetPersistPath(cfg.Annotations.FilePath)
	if _, err := store.Reload(cfg.Annotations.FilePath); err != nil {
		fmt.Fprintf(os.Stderr, "kode: failed to load %s: %v\n", cfg.Annotations.FilePath, err)
		os.Exit(1)
	}

	p := tea.NewProgram(explain.NewApp(store, cfg.Annotations.FilePath), tea.WithAltScreen(), tea.WithMouseCellMotion())

	store.OnChange(func(a annotate.Annotation) {
		go p.Send(explain.AnnotationAddedMsg(a))
	})

	var mcpSrv *mcpserver.Server
	if cfg.MCPServer.Enabled {
		mcpSrv = mcpserver.New(store, diffparse.Changeset{}, diagram.NewCLIRenderer(), cfg.MCPServer.Port)
		mcpSrv.Start()
		fmt.Printf("kode: mcp server listening on http://%s/mcp\n", mcpSrv.Addr())
		fmt.Printf("kode: one-time setup: claude mcp add --transport http kode http://%s/mcp\n", mcpSrv.Addr())
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			mcpSrv.Close(ctx)
		}()
	}

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "kode: %v\n", err)
		os.Exit(1)
	}
}
