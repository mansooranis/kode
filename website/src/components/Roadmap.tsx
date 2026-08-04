const roadmap = [
  {
    title: "Live in-TUI agent chat",
    description:
      "Ask kode's own embedded agent to explain a hunk or answer questions right inside the diff view, instead of only reading notes another session already left.",
  },
  {
    title: "jj & Sapling support",
    description:
      "Extend diff sourcing beyond git to Jujutsu and Sapling repos, using the same review UI.",
  },
  {
    title: "Watch mode",
    description: "kode diff --watch, auto-reloading the diff as files change on disk.",
  },
  {
    title: "Session export",
    description: "Flatten annotations, diagrams, and a chat transcript into a shareable Markdown or HTML report.",
  },
  {
    title: "Local MCP server",
    description:
      "Kode hosts its own MCP server on localhost so an external agent can push annotations into a live kode session, not just the file on disk.",
  },
  {
    title: "Remappable keybindings & themes",
    description: "Full keybinding remapping and theme switching, beyond today's fixed defaults.",
  },
];

export function Roadmap() {
  return (
    <section className="border-t border-white/10 px-6 py-20">
      <div className="mx-auto max-w-6xl">
        <div className="mx-auto max-w-2xl text-center">
          <span className="text-sm font-semibold tracking-wide text-emerald-400 uppercase">
            Roadmap
          </span>
          <h2 className="mt-3 text-3xl font-bold text-white sm:text-4xl">What's next</h2>
          <p className="mt-4 text-zinc-400">
            kode is at v0.0.1. These are planned, not shipped yet.
          </p>
        </div>
        <div className="mt-12 grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
          {roadmap.map((item) => (
            <div
              key={item.title}
              className="rounded-xl border border-dashed border-white/10 bg-white/[0.01] p-6"
            >
              <span className="inline-block rounded-full bg-white/5 px-2.5 py-1 text-[11px] font-semibold tracking-wide text-zinc-400 uppercase">
                Planned
              </span>
              <h3 className="mt-4 font-semibold text-white">{item.title}</h3>
              <p className="mt-2 text-sm text-zinc-400">{item.description}</p>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}
