# kode

kode is a terminal app for reviewing code changes, with an AI agent built in
to help you understand them. Point it at a git diff and it shows you a fast,
readable side by side (or stacked) view with syntax highlighting, lets you
leave inline comments as you go, and can call on an embedded coding agent to
explain a hunk, answer questions about it, or write annotations and Mermaid
diagrams of its own.

> **Early release.** kode is at v0.0.1, expect rough edges and breaking
> changes between versions.

## Getting started

The easiest way to install kode is via Homebrew:

```sh
brew tap mansooranis/kode
brew install kode
```

You can also build it from source if you have Go installed. Clone the repo
and build it:

```sh
git clone https://github.com/mansooranis/kode.git
cd kode
make build
```

This produces a `kode` binary in the current directory with a real version
baked in (`kode version` to check). You can also run it straight from source
while developing, without a build step:

```sh
go run ./cmd/kode
```

Once you have a binary, put it somewhere on your `PATH` (for example
`make install`, which runs `go install` instead of writing a local binary).

To try it out, go into any git repository that has some uncommitted changes
or a commit you want to look at, and run:

```sh
kode
```

That opens the diff of your working tree in the TUI. To look at a specific
commit instead, run `kode show <ref>`, for example `kode show HEAD~1`. To
review a GitHub pull request, run `kode pr <number>` (or just `kode pr` to
review the PR for your current branch) — this requires the
[GitHub CLI](https://cli.github.com) (`gh`) to be installed and
authenticated (`gh auth login`); kode will tell you if either is missing.

Run `kode help` at any time to see every command kode understands.

## Features

- **Diff review TUI.** Browse every changed file in a sidebar, jump between
  hunks, and read the diff with syntax highlighting. Toggle between a split
  layout and a stacked one with `m`, switch focus between the sidebar and
  the diff with `tab`, and quit with `q`.
- **Inline comments.** Press `c` on a line to leave a note directly on the
  diff, the same way you would leave a comment in a code review tool.
  Comments are saved to a JSON file in the repo (`.kode/annotations.json` by
  default) so they survive a restart and can be read or written by anything
  else that touches that file, including the embedded agent.
- **Built-in AI agent.** kode ships with an agent that can read the diff you
  have open, explain what changed, and answer questions about it. The agent
  supports MCP tool servers and a pluggable model provider (Anthropic by
  default), configurable under the `[agent]` section of your config.
- **Skills.** The agent loads Claude-Code-style skills (markdown files with
  frontmatter) from a global skills folder, normally
  `~/.config/kode/skills`. kode ships a few of its own and keeps them up to
  date automatically: every time you run kode after an upgrade, it notices
  its version changed and re-copies its bundled skills into that folder, so
  you never have to do it by hand. Run `kode skills sync` to force it right
  away.
- **Comments from another agent session.** kode's comments and diagrams live
  in a JSON file in the repo (`.kode/annotations.json` by default — see
  below), so a separate agent session, such as Claude Code, can read and
  write them directly without any live connection to kode. Run
  `kode skill install` once to copy the `kode-comments` skill into
  `~/.claude/skills`, so any `claude` session knows the file format and how
  to render Mermaid diagrams into it; press `r` in kode to pick up what it
  writes.
- **Codebase walkthroughs.** `kode explain` opens a read-only viewer for
  annotations and Mermaid diagrams an agent has already written about a
  codebase, useful for onboarding or documenting how something works without
  touching a diff at all.
- **Works with git, jj, and Sapling.** kode shells out to whichever version
  control tool your project uses to produce the diff it renders.
- **GitHub PR review.** `kode pr [number]` fetches a pull request's diff via
  the `gh` CLI and opens it in the same TUI as a local diff, with the same
  split view, inline comments, and agent support.

## Configuration

kode reads settings from `.kode/config.toml` in your project first, then
falls back to `~/.config/kode/config.toml`, then to built in defaults.
Anything you don't set keeps its default value. Notable settings:

- `theme`, `mode` (`auto`, `split`, or `stack`), `vcs` (`auto`, `git`, `jj`,
  or `sapling`), `line_numbers`, `tab_width`, `wrap_lines`
- `check_updates` (default `true`): on startup, check GitHub once a day for a
  newer release and show a banner in the bottom-right corner if one's out.
  Only compares against tagged release builds (`make build`/`make install`);
  a `dev` build never checks. No network call, download, or install happens
  beyond that GET — set to `false` to disable entirely.
- `[agent]`: `enabled`, `provider`, `model`, `effort`, `skills_path`
- `[annotations]`: `file`, where comments and diagrams are persisted, and
  where another agent session should read/write them directly
- `[keybindings]`: remap any key to a different action

## Development

kode is a Go TUI built with [Bubble Tea](https://github.com/charmbracelet/bubbletea).
Run `go run ./cmd/kode` inside a git repo with changes to try your edits
without building a binary first.

Run the test suite with:

```sh
make test
```

Build a version stamped binary with `make build` or `make install`.
`VERSION` defaults to `git describe` and is baked into the binary with
`-ldflags`, so `kode version` reports something meaningful even before there
is a tagged release. This is also the command a future Homebrew formula's
`install` step should run.

`.claude/skills/kode-comments/SKILL.md` in this repo is a symlink into
`internal/agent/skills/bundled/kode-comments/SKILL.md`. That way there is
only one copy of the skill to edit, and it works both as a project skill for
Claude Code and as one of the skills kode bundles into its own binary.
