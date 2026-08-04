const features = [
  {
    icon: "⇄",
    title: "Shared annotations file",
    description:
      "Notes, replies, and diagrams live in one JSON file (.kode/annotations.json), source-tagged by who wrote them. Anything that can write JSON can add a note — no live connection to kode required.",
  },
  {
    icon: "✎",
    title: "Skills for external agents",
    description:
      "kode skill install copies the kode-comments skill into ~/.claude/skills, so a Claude Code session knows the annotations file format and how to render Mermaid diagrams into it.",
  },
  {
    icon: "◫",
    title: "Split & stacked layout",
    description: "Toggle between a side-by-side and a stacked diff layout on the fly with m, in both kode and kode pr.",
  },
  {
    icon: "◆",
    title: "Mermaid diagrams, in-terminal",
    description: "Diagrams left by an agent render as ASCII/Unicode art inside kode explain — no browser needed.",
  },
  {
    icon: "▤",
    title: "Fast & local",
    description: "A single Go binary. No account, no server — point it at a repo and go.",
  },
  {
    icon: "⚙",
    title: "Project config",
    description: "Layout and annotations file path are configurable via .kode/config.toml, falling back to a user-level config.",
  },
];

export function FeatureGrid() {
  return (
    <section className="border-t border-white/10 px-6 py-20">
      <div className="mx-auto max-w-6xl">
        <h2 className="text-center text-3xl font-bold text-white sm:text-4xl">
          Everything else, briefly
        </h2>
        <div className="mt-12 grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
          {features.map((f) => (
            <div
              key={f.title}
              className="rounded-xl border border-white/10 bg-white/[0.02] p-6 transition hover:border-white/20 hover:bg-white/[0.04]"
            >
              <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-emerald-400/10 text-lg text-emerald-400">
                {f.icon}
              </div>
              <h3 className="mt-4 font-semibold text-white">{f.title}</h3>
              <p className="mt-2 text-sm text-zinc-400">{f.description}</p>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}
