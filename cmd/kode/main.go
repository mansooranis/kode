package main

import (
	"context"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mansooranis/kode/internal/annotate"
	"github.com/mansooranis/kode/internal/config"
	"github.com/mansooranis/kode/internal/diffparse"
	"github.com/mansooranis/kode/internal/mcpserver"
	"github.com/mansooranis/kode/internal/ui"
	"github.com/mansooranis/kode/internal/vcs/git"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "kode: failed to load config: %v\n", err)
		os.Exit(1)
	}

	src := git.New("")

	var diffText []byte
	args := os.Args[1:]
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
		mcpSrv = mcpserver.New(store, changeset, cfg.MCPServer.Port)
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
