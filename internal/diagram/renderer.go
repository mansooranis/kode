// Package diagram converts Mermaid diagram source into terminal
// ASCII/Unicode art. It isolates the renderer behind an interface per
// docs/PLAN.md's guidance, since the underlying tool is small/young.
package diagram

import (
	"fmt"
	"strings"

	mmascii "github.com/AlexanderGrooff/mermaid-ascii/cmd"
	mmconfig "github.com/AlexanderGrooff/mermaid-ascii/pkg/diagram"
)

// Renderer turns Mermaid source into ready-to-display terminal art.
type Renderer interface {
	Render(mermaidSource string) (string, error)
}

// LibRenderer calls mermaid-ascii's rendering package
// (github.com/AlexanderGrooff/mermaid-ascii) in-process, so users don't need
// a separate mermaid-ascii binary on $PATH.
type LibRenderer struct{}

func NewLibRenderer() LibRenderer {
	return LibRenderer{}
}

func (LibRenderer) Render(mermaidSource string) (string, error) {
	output, err := mmascii.RenderDiagram(mermaidSource, mmconfig.DefaultConfig())
	if err != nil {
		return "", fmt.Errorf("mermaid-ascii: %w", err)
	}

	return strings.TrimRight(output, "\n"), nil
}
