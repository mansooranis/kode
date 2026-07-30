// Package bundled embeds the skills kode ships with, so a single binary
// install (e.g. via Homebrew) carries them without a separate asset step.
// Each subdirectory is a Claude-Code-style skill folder (SKILL.md with
// frontmatter); .claude/skills/<name>/SKILL.md in this repo symlinks back to
// the copy here, so there is exactly one file to edit.
package bundled

import (
	"embed"
	"io/fs"
)

//go:embed all:*/SKILL.md
var files embed.FS

// Skill is one bundled skill, keyed by its directory name.
type Skill struct {
	Name string
	Body string
}

// All returns every bundled skill.
func All() ([]Skill, error) {
	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		return nil, err
	}

	var out []Skill
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		data, err := fs.ReadFile(files, e.Name()+"/SKILL.md")
		if err != nil {
			continue
		}
		out = append(out, Skill{Name: e.Name(), Body: string(data)})
	}
	return out, nil
}
