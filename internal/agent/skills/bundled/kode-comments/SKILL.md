---
name: kode-comments
description: Use when the user wants to read reviewer comments/annotations from a running kode TUI session, wants Claude to add/reply to comments on a diff open in kode, or wants Claude to explain/document a codebase by leaving annotations and Mermaid diagrams for `kode explain` to display. Covers connecting via `claude mcp add` and using kode's list_files, get_diff, list_annotations, add_annotation, and add_diagram MCP tools.
---

# kode comments

kode is this repo's Go TUI diff reviewer, plus a `kode explain` read-only viewer for
codebase-learning notes. Either mode, kode hosts a local MCP server so this session can read
and add comments/diagrams live — they show up in the running kode session immediately.

## One-time connection setup

kode prints its own setup command on startup:

```
claude mcp add --transport http kode http://127.0.0.1:<port>/mcp
```

Default port is `7378` (`internal/config/config.go`). Ask the user for the exact command
kode printed if unsure, then run it once to register the server.

## Tools

- `list_files` — files in the diff currently open in kode, with status (A/M/D/R). Empty in
  `kode explain` mode (no diff is loaded) — use your own Read/Grep/Glob tools instead.
- `get_diff(file?)` — unified diff text for one file, or the whole changeset if omitted.
  Also not meaningful in `kode explain` mode.
- `list_annotations(file?)` — existing comments/diagrams, formatted
  `[id] file:line (author, kind): text`. Use this first to see what's already there.
- `add_annotation(file, line, text)` — adds a comment on a specific line.
- `add_diagram(file, line, mermaid)` — renders Mermaid diagram source to ASCII/Unicode art
  (via kode's `internal/diagram` renderer) and attaches it at a file/line, same as a comment
  but shown as a diagram card.

## Workflow: answering a reviewer (diff mode)

1. Call `list_annotations` to see what reviewers have already asked.
2. Use `get_diff`/`list_files` to pull the context needed to answer.
3. Reply with `add_annotation` on the same `file`/`line` as the question, rather than
   starting an unrelated top-level comment.

## Workflow: explaining a codebase (`kode explain`)

When the user wants help learning/understanding a codebase rather than reviewing a diff:

1. Explore the repo with your normal Read/Grep/Glob tools — `kode explain` mode has no diff
   loaded, so `get_diff`/`list_files` won't help here.
2. As you build understanding, narrate it back into kode: `add_annotation` for prose
   explanations anchored at the most relevant `file`/`line`, `add_diagram` for control-flow or
   architecture flowcharts (Mermaid `graph`/`flowchart` syntax).
3. Tell the user to run `kode explain` (or refresh with `r` if it's already open) to browse the
   result as a walkthrough — files with notes in the left pane, each note shown with a small
   code snippet for context.

Annotations also persist to `.kode/annotations.json` if direct file access is ever more
convenient than going through MCP.
