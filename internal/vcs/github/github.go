// Package github fetches GitHub pull request diffs via the `gh` CLI, so
// kode's PR review reuses the exact same diffparse -> TUI pipeline as local
// git diffs (see internal/diffparse's package doc).
package github

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
)

// ErrNotInstalled means the gh CLI isn't on PATH.
var ErrNotInstalled = errors.New("gh CLI not found")

// ErrNotAuthenticated means gh is installed but has no logged-in account.
var ErrNotAuthenticated = errors.New("gh CLI not authenticated")

// CheckAvailable verifies gh is installed and authenticated before kode
// tries to shell out to it, so the user gets one clear instruction instead
// of a raw "exec: gh: not found" or an opaque API error.
func CheckAvailable() error {
	if _, err := exec.LookPath("gh"); err != nil {
		return fmt.Errorf(`%w: install the GitHub CLI to review PRs (e.g. "brew install gh", or see https://cli.github.com), then run "gh auth login"`, ErrNotInstalled)
	}

	cmd := exec.Command("gh", "auth", "status")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf(`%w: run "gh auth login" to authenticate the GitHub CLI`, ErrNotAuthenticated)
	}
	return nil
}

// Source produces unified-diff text for a pull request by shelling out to
// gh, mirroring vcs/git.Source's exec-and-wrap style.
type Source struct {
	// Dir is the repository to run gh in, so it can infer the right repo and
	// the current branch's PR. Empty means the current working directory.
	Dir string
}

func New(dir string) Source {
	return Source{Dir: dir}
}

// Diff returns the unified diff for a pull request. ref may be a PR number,
// URL, or branch name; empty uses the PR associated with the current branch.
func (s Source) Diff(ref string) ([]byte, error) {
	args := []string{"pr", "diff"}
	if ref != "" {
		args = append(args, ref)
	}
	return s.run(args...)
}

func (s Source) run(args ...string) ([]byte, error) {
	cmd := exec.Command("gh", args...)
	cmd.Dir = s.Dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("gh %v: %w: %s", args, err, stderr.String())
	}
	return stdout.Bytes(), nil
}
