// Package diffparse turns raw unified-diff text (from any vcs.Source) into
// structured data for rendering. Every VCS backend and, later, GitHub PR
// fetching funnels through Parse so the UI only ever deals with one shape.
package diffparse

import (
	"bytes"

	"github.com/bluekeyes/go-gitdiff/gitdiff"
)

type LineOp int

const (
	OpContext LineOp = iota
	OpAdd
	OpDelete
)

// Line is a single rendered line within a hunk, carrying both old- and
// new-file line numbers so the gutter can show whichever applies.
type Line struct {
	Op        LineOp
	OldLineNo int // 0 if this line doesn't exist in the old file (added lines)
	NewLineNo int // 0 if this line doesn't exist in the new file (deleted lines)
	Content   string
}

type Hunk struct {
	Header string
	Lines  []Line
}

type FileDiff struct {
	OldName  string
	NewName  string
	IsNew    bool
	IsDelete bool
	IsRename bool
	IsBinary bool
	Hunks    []Hunk
}

// Name is the path to display for this file (new name, falling back to old
// name for deletions).
func (f FileDiff) Name() string {
	if f.NewName != "" {
		return f.NewName
	}
	return f.OldName
}

type Changeset struct {
	Files []FileDiff
}

func Parse(diffText []byte) (Changeset, error) {
	files, _, err := gitdiff.Parse(bytes.NewReader(diffText))
	if err != nil {
		return Changeset{}, err
	}

	cs := Changeset{Files: make([]FileDiff, 0, len(files))}
	for _, f := range files {
		fd := FileDiff{
			OldName:  f.OldName,
			NewName:  f.NewName,
			IsNew:    f.IsNew,
			IsDelete: f.IsDelete,
			IsRename: f.IsRename,
			IsBinary: f.IsBinary,
		}

		for _, frag := range f.TextFragments {
			hunk := Hunk{Header: frag.Header()}
			oldLine := frag.OldPosition
			newLine := frag.NewPosition

			for _, l := range frag.Lines {
				line := Line{Content: l.Line}
				switch l.Op {
				case gitdiff.OpAdd:
					line.Op = OpAdd
					line.NewLineNo = int(newLine)
					newLine++
				case gitdiff.OpDelete:
					line.Op = OpDelete
					line.OldLineNo = int(oldLine)
					oldLine++
				default:
					line.Op = OpContext
					line.OldLineNo = int(oldLine)
					line.NewLineNo = int(newLine)
					oldLine++
					newLine++
				}
				hunk.Lines = append(hunk.Lines, line)
			}
			fd.Hunks = append(fd.Hunks, hunk)
		}

		cs.Files = append(cs.Files, fd)
	}

	return cs, nil
}
