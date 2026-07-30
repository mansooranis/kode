// Package git implements vcs.Source by shelling out to the native git
// binary, so kode's diff output is byte-identical to what `git diff`/`git
// show` would print for the user.
package git

import (
	"bytes"
	"fmt"
	"os/exec"
)

type Source struct {
	// Dir is the repository root to run git commands in. Empty means the
	// current working directory.
	Dir string
}

func New(dir string) Source {
	return Source{Dir: dir}
}

func (s Source) Diff() ([]byte, error) {
	return s.run("diff", "--no-color")
}

func (s Source) Show(rev string) ([]byte, error) {
	return s.run("show", "--no-color", rev)
}

func (s Source) run(args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = s.Dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %v: %w: %s", args, err, stderr.String())
	}
	return stdout.Bytes(), nil
}
