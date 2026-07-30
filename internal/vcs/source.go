// Package vcs abstracts over version control backends. Every backend
// produces canonical git-unified-diff text; internal/diffparse is the single
// place that text gets turned into structured data.
package vcs

// Source produces unified-diff text for review.
type Source interface {
	// Diff returns the unified diff for the current working tree changes.
	Diff() ([]byte, error)
	// Show returns the unified diff introduced by a single revision.
	Show(rev string) ([]byte, error)
}
