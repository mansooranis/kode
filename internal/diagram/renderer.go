// Package diagram converts Mermaid diagram source into terminal
// ASCII/Unicode art. It isolates the renderer behind an interface per
// docs/PLAN.md's guidance, since the underlying tool is small/young.
package diagram

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// Renderer turns Mermaid source into ready-to-display terminal art.
type Renderer interface {
	Render(mermaidSource string) (string, error)
}

// CLIRenderer shells out to the mermaid-ascii binary
// (github.com/AlexanderGrooff/mermaid-ascii), the same way internal/vcs/git
// shells out to git — mermaid-ascii's own rendering logic lives in an
// unexported, cobra-coupled package not meant for import as a library, so a
// CLI call is the stable integration point.
type CLIRenderer struct {
	// Bin is the binary to invoke. Empty means "mermaid-ascii" on $PATH.
	Bin string
}

func NewCLIRenderer() CLIRenderer {
	return CLIRenderer{}
}

func (r CLIRenderer) Render(mermaidSource string) (string, error) {
	bin := r.Bin
	if bin == "" {
		bin = "mermaid-ascii"
	}

	cmd := exec.Command(bin, "--file", "-")
	cmd.Stdin = strings.NewReader(mermaidSource)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("mermaid-ascii: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	return strings.TrimRight(stdout.String(), "\n"), nil
}
