package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mansooranis/kode/internal/agent/skills/bundled"
	"github.com/mansooranis/kode/internal/annotate"
	"github.com/mansooranis/kode/internal/buildinfo"
	"github.com/mansooranis/kode/internal/config"
	"github.com/mansooranis/kode/internal/diagram"
	"github.com/mansooranis/kode/internal/diffparse"
	"github.com/mansooranis/kode/internal/ui"
	"github.com/mansooranis/kode/internal/ui/explain"
	"github.com/mansooranis/kode/internal/vcs/git"
	"github.com/mansooranis/kode/internal/vcs/github"
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
	case len(args) >= 2 && args[0] == "skill" && args[1] == "install":
		if err := installClaudeSkill(); err != nil {
			fmt.Fprintf(os.Stderr, "kode: %v\n", err)
			os.Exit(1)
		}
		return
	case len(args) >= 1 && args[0] == "pr":
		runPR(cfg, args[1:])
		return
	case len(args) >= 1 && args[0] == "render-diagram":
		runRenderDiagram()
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

	if err := runReview(cfg, diffText); err != nil {
		fmt.Fprintf(os.Stderr, "kode: %v\n", err)
		os.Exit(1)
	}
}

// runReview parses diffText and opens the shared TUI review app. Both the
// local git path above and runPR below funnel into this, so a GitHub PR
// diff gets identical review functionality (split view, comments,
// annotations) to a local diff.
func runReview(cfg config.Config, diffText []byte) error {
	changeset, err := diffparse.Parse(diffText)
	if err != nil {
		return fmt.Errorf("failed to parse diff: %w", err)
	}

	if len(changeset.Files) == 0 {
		fmt.Println("kode: no changes to review")
		return nil
	}

	store := annotate.NewStore()
	store.SetPersistPath(cfg.Annotations.FilePath)
	if _, err := store.Reload(cfg.Annotations.FilePath); err != nil {
		return fmt.Errorf("failed to load %s: %w", cfg.Annotations.FilePath, err)
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

	_, err = p.Run()
	return err
}

// runPR reviews a GitHub pull request's diff, fetched via the gh CLI. It
// checks gh is installed and authenticated up front so a missing/unauthed
// gh fails with one clear instruction instead of a raw exec error.
func runPR(cfg config.Config, args []string) {
	if err := github.CheckAvailable(); err != nil {
		fmt.Fprintf(os.Stderr, "kode: %v\n", err)
		os.Exit(1)
	}

	var ref string
	if len(args) >= 1 {
		ref = args[0]
	}

	diffText, err := github.New("").Diff(ref)
	if err != nil {
		fmt.Fprintf(os.Stderr, "kode: %v\n", err)
		os.Exit(1)
	}

	if err := runReview(cfg, diffText); err != nil {
		fmt.Fprintf(os.Stderr, "kode: %v\n", err)
		os.Exit(1)
	}
}

// runRenderDiagram reads Mermaid source from stdin and prints its
// ASCII/Unicode rendering to stdout. It exists so external agent sessions
// (see .claude/skills/kode-comments) can pre-render diagrams for annotations
// via the kode binary itself, rather than requiring a separate mermaid-ascii
// install — diagram.LibRenderer calls mermaid-ascii's rendering package
// in-process.
func runRenderDiagram() {
	src, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "kode: failed to read mermaid source from stdin: %v\n", err)
		os.Exit(1)
	}

	out, err := diagram.NewLibRenderer().Render(string(src))
	if err != nil {
		fmt.Fprintf(os.Stderr, "kode: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(out)
}

// printHelp lists every command kode understands. Keep this in sync with the
// switch in main(): it's the one place a new user (or a Homebrew "kode
// --help" pipe) should be able to see everything the binary can do.
func printHelp() {
	fmt.Println(`kode is a terminal diff reviewer with a built-in AI agent.

Usage:
  kode                Review the current working diff (git diff) in the TUI
  kode show <ref>     Review a single commit, e.g. "kode show HEAD~1"
  kode pr [number]    Review a GitHub PR diff (current branch's PR if omitted).
                       Requires the gh CLI installed and authenticated.
  kode explain        Open the read-only codebase walkthrough viewer
  kode skills sync    Copy kode's bundled skills into your global skills folder now
  kode skill install  Copy the kode-comments skill into ~/.claude/skills, for Claude Code
  kode render-diagram Render Mermaid source (stdin) to ASCII/Unicode art (stdout)
  kode version        Print the installed version
  kode help           Show this message

Comments and diagrams a separate agent session (such as Claude Code) leaves
while exploring this repo are read from and written directly to the
annotations file (see the "annotations" section of your config), rather than
over any live connection to kode. Run "kode skill install" once so that
session knows the annotations file format; see .claude/skills/kode-comments.
Run "kode explain" to browse the resulting notes and diagrams as a
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
		fmt.Fprintf(os.Stderr, "kode: updated bundled skills in %s for %s\n", cfg.Agent.SkillsPath, version)
	}
	return nil
}

// runExplain starts kode's read-only "learning" viewer: no diff/VCS
// involved, just whatever annotations/diagrams already exist in the store
// (typically written directly to cfg.Annotations.FilePath by an external
// agent working off .claude/skills/kode-comments/SKILL.md). Press "r" to
// refresh and pick up anything written since the viewer opened.
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

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "kode: %v\n", err)
		os.Exit(1)
	}
}

// installClaudeSkill copies the kode-comments skill into
// ~/.claude/skills/kode-comments/SKILL.md — Claude Code's global skills
// folder — so a `claude` session started in any project (not just this repo,
// which already has it as a project skill via .claude/skills) knows how to
// read and write kode's annotations file directly.
func installClaudeSkill() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("install skill: %w", err)
	}
	dir := filepath.Join(home, ".claude", "skills")
	path, err := bundled.InstallClaudeSkill(dir, "kode-comments")
	if err != nil {
		return fmt.Errorf("install skill: %w", err)
	}
	fmt.Printf("kode: installed kode-comments skill to %s\n", path)
	return nil
}
