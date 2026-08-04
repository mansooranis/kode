---
name: kode-comments
description: Use when the user wants to read reviewer comments/annotations from a kode session (diff review or `kode explain`), wants Claude to add/reply to comments on a diff open in kode, or wants Claude to explain/document a codebase by leaving annotations and Mermaid diagrams for `kode explain` to display. Covers reading and writing kode's annotations.json file directly, and rendering Mermaid diagrams to terminal art with `kode render-diagram`.
---

# kode comments

kode is this repo's Go TUI diff reviewer, plus a `kode explain` read-only viewer for
codebase-learning notes. Both modes read their comments/diagrams from a JSON file on disk —
`.kode/annotations.json` by default (`[annotations].file` in `.kode/config.toml`, falling back
to `~/.config/kode/config.toml`, then that default). There is no live connection to a running
kode process: write straight to that file, then tell the user to press `r` in kode to refresh.

If this skill isn't already available in a project (i.e. you're not reading it from
`.claude/skills/kode-comments`), it can be installed globally by running `kode skill install`,
which copies it to `~/.claude/skills/kode-comments/SKILL.md`.

## The annotations file

A JSON array of objects, pretty-printed, at the path above. Read it with your normal file
tools first (it may not exist yet — treat that the same as an empty array) to see what's
already there and to match its formatting when you add to it:

```json
[
  {
    "id": "a1b2c3d4e5f6",
    "file": "internal/foo/bar.go",
    "line": 42,
    "author": "kode-agent",
    "kind": "comment",
    "text": "Explanation text goes here.",
    "created_at": "2026-07-30T12:00:00Z"
  }
]
```

Field notes:

- `id` — omit it when adding a new entry; kode derives a stable content hash itself on load.
  Don't invent one, and don't reuse an existing `id` unless you're intentionally deduplicating.
- `file` — path relative to the repo root, matching what's shown in the diff or by your own
  Glob/Grep tools.
- `line` — the line number to anchor the note to (the new-file line number if the file has
  pending changes there, otherwise the current line number).
- `author` — use `"claude"` (or another short, identifying string) so entries you add are
  visually distinguishable from human and kode's own embedded-agent notes.
- `kind` — `"comment"` for prose (the default — you may omit the field entirely), or
  `"diagram"` for a Mermaid diagram (see below). Any other value is invalid.
- `text` — the comment body, or for a diagram, its **pre-rendered ASCII/Unicode art** (kode
  displays this verbatim, unwrapped, so it must already be laid out — see below).
- `source` — only for `kind: "diagram"`: the raw Mermaid source that produced `text`, kept so
  a human can see/edit the original.
- `created_at` — an RFC 3339 timestamp. Omit it and kode will stamp it on load; only set it
  yourself if you need annotations added in the same batch to sort in a specific order.

To add an entry: read the file, append your object to the array (or start a new `[...]` array
if the file doesn't exist), and write it back with correct JSON formatting. Adding several
notes in one pass (e.g. while exploring a codebase) — read once, append everything, write once.

## Diagrams

For flowcharts/control-flow/architecture diagrams, kode renders Mermaid source into terminal art
using [`mermaid-ascii`](https://github.com/AlexanderGrooff/mermaid-ascii)'s rendering package,
bundled directly into the `kode` binary. Do the same rendering step yourself with `kode render-diagram`
(kode expects diagrams to already be pre-rendered in `text`, since the TUI itself has no Mermaid
renderer built in):

```sh
echo '<mermaid source>' | kode render-diagram
```

This requires no separate install — it's the same `kode` binary already on `$PATH`. If it fails
(e.g. unsupported diagram syntax), fall back to plain `comment` annotations describing the flow
in prose — don't hand-draw ASCII art as a substitute; it won't match kode's rendering.

Once you have the rendered output, write an annotation with `"kind": "diagram"`, `"text"` set
to that rendered output exactly as produced (including its own internal spacing/newlines —
don't re-wrap or re-indent it), and `"source"` set to the Mermaid source you fed in.

## Workflow: answering a reviewer (diff mode)

1. Read the annotations file to see what reviewers have already asked.
2. Use your normal Read/Grep/Glob tools (and `git diff`/`git show` if useful) to pull the
   context needed to answer.
3. Append your reply as a new annotation on the same `file`/`line` as the question, rather
   than starting an unrelated top-level comment.
4. Tell the user to press `r` in kode to see it.

## Workflow: explaining a codebase (`kode explain`)

When the user wants help learning/understanding a codebase rather than reviewing a diff:

1. Explore the repo with your normal Read/Grep/Glob tools.
2. As you build understanding, narrate it back into the annotations file: a `comment` entry
   for prose explanations anchored at the most relevant `file`/`line`, a `diagram` entry
   (rendered per the section above) for control-flow or architecture flowcharts.
3. Tell the user to run `kode explain` (or press `r` to refresh if it's already open) to browse
   the result as a walkthrough — files with notes in the left pane, each note shown with a
   small code snippet for context.
