// Package buildinfo holds the version string stamped into the binary at
// build time via -ldflags, e.g.:
//
//	go build -ldflags "-X github.com/mansooranis/kode/internal/buildinfo.Version=1.2.3"
//
// A Homebrew formula's `go build` install step sets this from the release
// tag. It's also how kode notices it was upgraded: on startup it compares
// Version against the last version it synced bundled skills for, and
// re-syncs on a mismatch (see internal/agent/skills/bundled and
// cmd/kode/skillsync.go).
package buildinfo

var Version = "dev"
