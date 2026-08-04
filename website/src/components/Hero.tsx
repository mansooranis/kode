import { Terminal, type TerminalLine } from "./Terminal";
import { MediaPlaceholder } from "./MediaPlaceholder";

const heroLines: TerminalLine[] = [
  { type: "command", text: "brew tap mansooranis/kode" },
  { type: "command", text: "brew install kode" },
  { type: "command", text: "kode skill install" },
  { type: "blank" },
  { type: "comment", text: "# view local changes" },
  { type: "command", text: "kode diff" },
];

export function Hero() {
  return (
    <section id="top" className="relative overflow-hidden px-6 pt-20 pb-16">
      <div
        aria-hidden
        className="pointer-events-none absolute inset-x-0 -top-40 -z-10 h-[480px] bg-[radial-gradient(closest-side,rgba(16,185,129,0.18),transparent)]"
      />
      <div className="mx-auto max-w-4xl text-center">
        <div className="mx-auto mb-6 inline-flex items-center gap-2 rounded-full border border-white/10 bg-white/[0.03] px-3 py-1 text-xs font-medium text-zinc-400">
          <span className="h-1.5 w-1.5 rounded-full bg-emerald-400" />
          v 0.0.2
        </div>
        <h1 className="text-4xl font-bold tracking-tight text-white sm:text-5xl md:text-6xl">
          Learn complex codebases
          <br className="hidden sm:block" />using agents in your terminal.
        </h1>
        <p className="mx-auto mt-6 max-w-2xl text-lg text-zinc-400">
          Point an agent like Claude Code to leave notes and diagrams in your codebase as it explores.
          Use <code className="text-emerald-300">kode explain</code> to interact with those comments.
          Point kode at a diff instead, and get
          the same speed for reviewing local changes and GitHub PRs.
        </p>
      </div>

      <div className="mx-auto mt-12 max-w-2xl">
        <Terminal title="Install &amp; run" lines={heroLines} />
        <div className="mt-4 flex flex-wrap items-center justify-center gap-3 text-sm">
          <a
            href="#install"
            className="rounded-lg bg-emerald-400 px-5 py-2.5 font-semibold text-zinc-950 transition hover:bg-emerald-300"
          >
            Get started
          </a>
          <a
            href="#explain"
            className="rounded-lg border border-white/10 px-5 py-2.5 font-medium text-zinc-200 transition hover:border-white/20 hover:bg-white/5"
          >
            See kode explain
          </a>
        </div>
      </div>

      <div className="mx-auto mt-16 max-w-5xl">
        <MediaPlaceholder label="GIF · kode explain rendering a walkthrough with a Mermaid diagram" />
      </div>
    </section>
  );
}
