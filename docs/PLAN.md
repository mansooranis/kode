# kode — Go TUI diff reviewer with embedded agent

## Context

The repo is currently empty (just LICENSE/README/.gitignore) — this is a greenfield Go project. The goal is a terminal UI, feature-equivalent to [Hunk](https://github.com/modem-dev/hunk) (a TypeScript/OpenTUI diff reviewer), rebuilt in Go, with three additions layered on top: a chat panel to ask an embedded coding agent to explain the diff, the ability for that agent to write its own inline annotations on specific lines, and the ability for it to generate Mermaid diagrams rendered as ASCII/Unicode art. The agent core must support MCP (to pull in external tool servers), a Claude-Code-style skills system, and a pluggable LLM provider (default Anthropic, swappable) — confirmed directly with the user, not an assumption.

Confirmed with the user:
- Diff sources: local git/jj/sapling diffs (Hunk parity) **plus** direct GitHub PR fetching by URL/number.
- Annotations/diagrams/chat persist for the session and are **exportable as a Markdown/HTML report** at the end — not written back into the diff, not discarded on exit.
- Agent backend: MCP client + skills system + pluggable multi-provider LLM abstraction (default Anthropic Claude).

## Hunk feature set to match

- Sidebar navigation across files in a changeset, continuous review stream, watch mode (auto-reload, fs-watch + polling fallback)
- Split/stack responsive layouts, syntax highlighting, mouse + keyboard nav
- VCS support: Git (native), Jujutsu (revsets), Sapling (revsets), direct file comparison, stdin patch input
- CLI: `kode diff`, `kode diff --watch`, `kode show HEAD~1`; usable as a git pager (`git config --global core.pager "kode pager"`)
- Config at `.kode/config.toml` (project) → `~/.config/kode/config.toml` (user): theme, mode, vcs, watch, exclude_untracked, line_numbers, tab_width, wrap_lines, menu_bar, agent_notes, transparent_background
- Fully remappable keybindings (`[keybindings]` table, exclusive claims, unbinding)
- Experimental extension loading from `~/.config/kode/extensions/`
- Diff-view component reusable/embeddable by other programs

## Library choices

- **TUI**: `github.com/charmbracelet/bubbletea` + `bubbles` + `lipgloss` (+ `glamour` for markdown in chat). Actively maintained, large ecosystem (`gh`, `glow`, `crush` all use it); `tview` is comparatively stale. Risk: Bubble Tea's mouse model has no built-in drag-continuation primitive, so drag-to-resize between split panes must be hand-rolled off raw `tea.MouseMsg` — budget explicit time for this in Phase 1.
- **Diff parsing**: shell out to native VCS binaries (`git diff --no-color`, `git show`, `jj diff --git`, `sl diff --git`) to get canonical unified-diff text (matches what the user's VCS already produces, avoids reimplementing rename detection/histogram diff/etc.), then parse with `github.com/bluekeyes/go-gitdiff` into structured `[]*gitdiff.File`. Do not use `go-git` for diff generation — it lags native git in flag parity and adds dependency weight for no benefit.
- **Syntax highlighting**: `github.com/alecthomas/chroma` (v2) — de facto standard, terminal ANSI formatter, already a `glamour` dependency so it's shared with the chat markdown renderer.
- **Diagrams**: `github.com/AlexanderGrooff/mermaid-ascii` as primary (purpose-built for terminal output: LR/TD/TB/BT/RL, subgraphs, node shapes, edge labels, styling). Keep `github.com/yashikota/mermaigo` as a documented fallback for diagram types mermaid-ascii doesn't cover (verify sequence-diagram support before promising full Mermaid coverage). Both are small projects — wrap behind an internal `Renderer` interface so either can be swapped or forked without touching call sites; confirm license/maintenance activity before vendoring.
- **GitHub PR fetching**: shell out to `gh` CLI (`gh pr diff <n>`, `gh api ...`) as primary — it already handles auth/rate-limiting/GHE. Fall back to `github.com/google/go-github` + a token when `gh` isn't on `$PATH`. Either path feeds the same unified-diff text into `go-gitdiff`, so PR review reuses 100% of the diff rendering pipeline.
- **MCP client**: `github.com/modelcontextprotocol/go-sdk` — the official Go SDK (Google-co-maintained), supports stdio and HTTP/SSE transports. Young SDK — pin a version, isolate behind an internal wrapper package.
- **Default LLM provider**: `github.com/anthropics/anthropic-sdk-go`, using streaming (`Messages.NewStreaming`) for all chat responses, adaptive thinking with a configurable `effort`, and its Tool Runner for the agentic tool-use loop (native tools and MCP-sourced tools share the same tool schema format).

## Provider abstraction

```go
package provider

type Provider interface {
    Stream(ctx context.Context, req Request) (<-chan StreamEvent, error)
    Name() string
    SupportsTools() bool
    SupportsThinking() bool
}

type Request struct {
    System   string
    Messages []Message
    Tools    []ToolDef   // shared shape for native kode tools AND MCP-sourced tools
    Effort   string       // ignored by providers that don't support it
}
```

`internal/agent/provider/anthropic` is the default implementation. Build a minimal `internal/agent/provider/openai` implementation *during* Phase 4 (not deferred) — this is the real test that the interface isn't accidentally Anthropic-shaped (e.g. baking in `thinking`/`effort` semantics OpenAI doesn't have).

MCP tools are discovered once per session (`ListTools`) and mapped into `[]ToolDef` — from the model's point of view there's no difference between a native tool (`add_annotation`, `render_diagram`) and an MCP tool; `internal/agent/mcp` just routes `tool_use` calls to the right handler.

**Skills**: markdown files under `~/.config/kode/skills/*.md` with YAML frontmatter (`name`, `description`, `triggers`), mirroring Claude Code's skill convention. Only name+description go into the system prompt at session start; the full body loads on demand via a `load_skill` tool call (progressive disclosure, keeps the system prompt small).

## Local MCP server (kode hosts, Claude Code connects) — confirmed with user

Distinct from the MCP *client* work above: kode also **hosts its own MCP server** so a separately-running Claude Code session can call into a live kode TUI and add annotations/comments to the diff currently being reviewed.

- **Transport**: HTTP/SSE on `127.0.0.1:<fixed configurable port, default 7378>` — not stdio, since kode's own stdio is occupied by the TUI. Not ephemeral, so registration (`claude mcp add --transport http kode http://127.0.0.1:7378/mcp`) only has to happen once and works across future sessions.
- **Package**: `internal/mcpserver`, wrapping `modelcontextprotocol/go-sdk`'s server-side API (`mcp.NewServer`/`AddTool`), started as a goroutine alongside the Bubble Tea program.
- **Tools exposed**: `add_annotation(file, line, text)` (writes into the same `internal/annotate.Store` the embedded agent uses), plus read-only `list_annotations()`, `list_files()`, `get_diff(file?)` so an external caller can see what's open before commenting.
- **Live UI bridge**: the server holds a reference to the running `tea.Program`; on `add_annotation` it writes the store and calls `p.Send(annotationAddedMsg{...})` — same bridge pattern as streamed chat tokens — so the diffview repaints immediately regardless of who added the annotation.
- **Source tagging**: `annotate.Store`'s `Author` field becomes a source string (`"human"`, `"kode-agent"`, `"mcp:<client-name>"` — taken from the MCP `initialize` handshake's `clientInfo.name`) so external annotations get a visually distinct marker/color from kode's own agent's.
- **Activation**: auto-starts with kode by default, config-toggleable via `[mcp_server] enabled = false`.

```toml
[mcp_server]
enabled = true
port = 7378
```

## Package layout

```
cmd/kode/                    main(), CLI dispatch, wiring
internal/
  vcs/                       Source interface: Diff(), Show(rev)
    git/ jj/ sapling/ filecompare/ stdin/ githubpr/
    watch/                   fsnotify + polling fallback, shared across sources
  diffparse/                 wraps go-gitdiff: []byte -> Changeset{Files[]FileDiff{Hunks}}
  ui/
    app.go                   root model, routes to children, owns split/stack layout
    sidebar/ diffview/ chat/ diagram/ theme/ keymap/ statusline/ menubar/
  agent/
    core.go                  Agent: conversation state, drives tool-use loop
    provider/                interface + anthropic/ + openai/
    mcp/                     MCP CLIENT: wraps modelcontextprotocol/go-sdk, connects OUT to external servers, tool routing
    skills/                  loader + load_skill tool
    tools/                   add_annotation, render_diagram, read_context
  mcpserver/                 MCP SERVER: kode hosts its own MCP server (HTTP/SSE, localhost) so Claude Code can call add_annotation etc. against a live session
  annotate/                  session annotation store (File, Line, Author [source-tagged: human|kode-agent|mcp:<client>], Text, Timestamp); also persists to / reloads from a JSON file (annotate.LoadFile, Store.Reload) so annotations can be pushed by anything that can write a file, not just live MCP callers
  diagram/                   Renderer interface over mermaid-ascii (+ mermaigo fallback)
  export/                    markdown.go, html.go — session -> shareable report
  config/                    TOML schema + load precedence, keybindings
  extensions/                (later) plugin loader mirroring Hunk's extension system
```

Critical files to get right early: `internal/agent/provider/provider.go` (everything else depends on this shape), `internal/diffparse/diffparse.go` (every VCS/PR path funnels through it), `internal/ui/diffview/diffview.go` (reusable component + biggest perf risk), `internal/agent/mcp/client.go` (isolates the young external SDK), `internal/config/config.go` (every later phase extends this struct), `internal/diagram/renderer.go` (isolates a maturity-risk dependency).

## Config schema (extends Hunk's TOML)

```toml
theme = "dark"
mode = "auto"              # auto | split | stack
vcs = "auto"
watch = false
exclude_untracked = false
line_numbers = true
tab_width = 4
wrap_lines = false
menu_bar = true
agent_notes = true
transparent_background = false

[keybindings]
# remapping, exclusive claims, unbinding — Hunk-compatible

[agent]
enabled = true
provider = "anthropic"          # anthropic | openai | ...
model = "claude-opus-5"
effort = "medium"                # low|medium|high|xhigh|max
skills_path = "~/.config/kode/skills"
annotations_enabled = true
diagrams_enabled = true

[agent.provider.anthropic]
api_key_env = "ANTHROPIC_API_KEY"

[agent.provider.openai]
api_key_env = "OPENAI_API_KEY"
base_url = ""

[[mcp.servers]]
name = "github"
transport = "stdio"
command = "npx"
args = ["-y", "@modelcontextprotocol/server-github"]

[mcp_server]
enabled = true
port = 7378

[annotations]
file = ".kode/annotations.json"   # pushed to directly by scripts/agents; "r" in the TUI reloads it

[export]
default_format = "markdown"      # markdown | html
output_dir = "./kode-reports"
```

## Phased milestones

**Reprioritized per user request (this order supersedes the original sequential rationale below): get the core diff viewer and the agent core working first — VCS breadth, theming, and other polish come later once those two are solid.**

0. **Scaffolding** — module setup, `internal/config` TOML load, static Bubble Tea shell. *(done)*
1. **Core diff viewer parity** — `vcs/git`, `diffparse`, `sidebar` + `diffview` with chroma highlighting, split/stack layout, keyboard+mouse nav. *(done — verified live via tmux: sidebar nav, split/stack toggle, syntax highlighting all working)*
4. **Agent core** — `provider` interface + Anthropic impl + minimal OpenAI impl, `agent/mcp` client wrapping the official SDK, `agent/skills`, `agent.Core` tool-use loop. *(done — unit-tested against a mock provider; no UI dependency yet, wires into the chat panel in Phase 5)*
5a. **Terminal comments + local MCP server** — `internal/annotate` store (source-tagged: human/kode-agent/mcp:\<client\>); `diffview` gained a movable cursor line, a `[+]` comment button on every changed line (and a `c` keybinding), and an inline floating "Draft note" box (bordered, multi-line textarea, `Save (^S)`/`Cancel (Esc)`) matching Hunk's UI, rendered right after the target line rather than in a footer; `internal/mcpserver` hosts kode's own MCP server (HTTP/SSE, `127.0.0.1:7378`, auto-starts, config-toggleable via `[mcp_server]`) exposing `add_annotation`/`list_annotations`/`get_diff`/`list_files` so an external Claude Code session can read a reviewer's comment and answer it back into the live TUI. *(done — verified end-to-end live via tmux, including the box's exact render output matching the reference screenshot)*
5a-json. **JSON-backed annotation persistence + refresh** — `annotate.Store` now persists every new annotation (from any source: keypress, MCP call, or file reload) to a JSON file (default `.kode/annotations.json`, configurable via `[annotations] file`), using a content-hash ID (`computeID`) so repeated reloads of an unchanged file never duplicate entries. `annotate.LoadFile`/`Store.Reload` let an agent (or a script, or a human) push notes directly into that file with no live MCP connection at all; pressing `r` in the TUI calls `Reload` and reports how many were new in the status line. *(done — verified live: pushed 3 annotations directly into `.kode/annotations.json` for this repo's own README.md diff, confirmed 2 appeared on launch and the 3rd appeared after pressing `r` with zero restart, and that a second `r` correctly reports "no new annotations")*

*(deferred until 1 and 4 are solid)*

2. **VCS breadth + watch + pager** — `jj`, `sapling`, `filecompare`, `stdin`, `watch`; `kode diff/--watch/show`; `kode pager`. Independent of agent work; stress-tests the diffparse abstraction early.
3. **Config, keybindings, theming, extensions** — full remapping, themes, extension loader.
5. **Chat panel** — `ui/chat` bridged to `agent.Core` via channel → `tea.Program.Send` (build streaming as the first code path, not a retrofit) for freeform conversation with kode's own embedded agent, as opposed to 5a's comment-thread flow which needs no chat UI at all.
6. **Diagram generation/rendering** — `diagram.Renderer` over mermaid-ascii; `render_diagram` tool; `ui/diagram` panel. Can run parallel to Phase 5 (only depends on Phase 4).
7. **GitHub PR fetching** — `vcs/githubpr` (gh CLI + go-github fallback), `kode diff --pr <url|number>`. Reuses Phase 1's diffview untouched.
8. **Export/report generation** — `export/markdown.go` + `html.go` flattening annotations + diagrams + chat transcript. Necessarily last.

## Key risks

- Chroma-highlighting + re-render-on-every-`View()` can get slow on large diffs — mitigate with `bubbles/viewport`-based rendering of only visible lines; benchmark in Phase 1, not later.
- MCP Go SDK and mermaid-ascii/mermaigo are both young/small — isolate each behind an internal interface so upstream breakage or a needed fork is a one-package fix.
- Bubble Tea drag-to-resize between panes needs hand-rolling; don't assume it's free.
- `gh` CLI as a hard dependency for PR fetch — decide explicitly whether v1 ships the `go-github` fallback or documents `gh`-only, rather than leaving it an undocumented gap.
- The provider abstraction is only proven if a second provider is actually built during Phase 4, not deferred "for later."
- **`tea.Program.Send` deadlocks if called synchronously from within `Update` on the same goroutine** — hit this for real in 5a: `annotate.Store`'s `onChange` callback fires synchronously inside `Store.Add`, and a local "c" comment calls `Store.Add` directly from `App.Update`, so the naive `p.Send(msg)` in that callback blocked the entire event loop forever (confirmed via `SIGQUIT` goroutine dump). Fixed by wrapping every `OnChange` send in its own goroutine (`go p.Send(...)`) regardless of caller origin, in `cmd/kode/main.go`. Anything that bridges external state into the TUI (the chat panel's streaming tokens in Phase 5, the diagram panel in Phase 6) must follow the same rule: never assume the call site is off the Update goroutine.
- **Hardcoded box widths overflow and corrupt the layout on any pane narrower than the constant** — the draft-note box and comment cards originally used a fixed `draftBoxWidth = 70`; on a pane narrower than that (the common case once the sidebar eats its share of the terminal), the terminal wrapped the overflowing lines, visibly breaking the box's borders into the row below. Fixed by computing box width from `m.width` every render (`Model.boxWidth(indent int)`, floored at `minBoxWidth`), including refreshing the open draft textarea's width on every render so a live terminal resize doesn't leave it stale. Locked in with width-invariant tests (`assertNoLineExceedsWidth`) at multiple pane sizes, including a resize-while-drafting case. Any future panel that draws a box (diagram panel in Phase 6, chat panel in Phase 5) must size off the actual pane width, never a constant.
- **Mouse clicks into the diff pane were off by one row, independent of the width bug above** — `sidebar.Update` compensates for its own title row internally (`idx := msg.Y - 1`), but `diffview.Update` was built title-agnostic (an embeddable component with no knowledge of what wraps it), and `app.go`'s mouse-forwarding code only subtracted `diffBox.y` (0 in split mode) before handing coordinates to `diffview` — never the 1-row title ("Files (N)" / current filename) both panes render above their content. Every click into the diff pane landed exactly one row below what the user clicked, which is why `[Reply]`/`[+]` looked broken even after the width fix. Fixed by subtracting a shared `titleHeight` constant in `app.go`'s diffBox mouse case. Caught because a real synthetic mouse click via tmux's raw SGR escape sequence (`\x1b[<0;col;rowM`) was tested end-to-end, not just `diffview`'s own unit tests — those call `diffview.Update` directly and so never exercise `app.go`'s coordinate translation at all. Locked in with `internal/ui/app_test.go` (`TestMouseClickOnDiffButtonRowOpensDraft`, `TestMouseClickOneRowHigherMissesButton`) which drive the click through the full `App.Update` path. Any future pane that reserves a title row and forwards mouse coordinates to a title-agnostic child component needs the same offset.
- **The draft/reply box and comment cards used two different, subtly-disagreeing width formulas** (`m.boxWidth(0)` with no left indent vs. `m.boxWidth(len(indent))` with one), so a reply visually nested at a different width than the thread it was replying to. Fixed by unifying both onto shared `boxTopBorder`/`boxContentLine`/`boxBottomBorder` helpers and the same `total := m.boxWidth(len(commentThreadIndent))`. That surfaced a second, sneakier bug in the process: `boxTopBorder` measured the label with `len()` (byte count) instead of `lipgloss.Width()` (visual column count) — harmless for the draft box's pure-ASCII title, but the comment card header contains `·` (2 UTF-8 bytes, 1 visual column), so cards silently rendered exactly one column narrower than everything else. Byte-counting a string for display-width purposes will always be wrong the moment non-ASCII text is involved (usernames, MCP client names, comment text); use `lipgloss.Width` (or the new `truncateToWidth` helper for rune-safe truncation) instead of `len()` anywhere a string's *visual* width matters. Locked in with `TestDraftBoxSameWidthAndIndentAsCommentCard`, which failed with a 1-column mismatch before this fix and asserts exact width+indent equality.

## Verification

- Phase 1: run `kode diff` and `kode show HEAD~1` against a real repo with a multi-file, multi-hundred-line changeset; confirm sidebar nav, split/stack toggle, mouse scroll/click, and syntax highlighting all work, and check render latency on a large diff (e.g. a vendored dependency bump).
- Phase 2: verify `--watch` reflects live file edits, and pager mode via `git config --global core.pager "kode pager"` followed by `git log -p` / `git diff`.
- Phase 4: unit tests against a mock `Provider` for the tool-use loop; a manual smoke test hitting the real Anthropic API with `ANTHROPIC_API_KEY` set, asking a scripted question and confirming a tool call round-trips.
- Phase 5: in the TUI, open a diff, ask the chat panel to "explain this file," confirm streaming tokens render live and an `add_annotation` call shows an inline marker at the right line.
- Phase 6: ask the agent to "diagram this function's control flow," confirm a Mermaid diagram renders as ASCII in the diagram panel.
- Phase 7: run `kode diff --pr <github-pr-url>` against a real PR and confirm it renders through the same diffview as local diffs.
- Phase 8: export a session with at least one annotation, one diagram, and a chat exchange to both Markdown and HTML, and open the output to confirm it's readable and complete.
