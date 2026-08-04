import { Terminal, type TerminalLine } from "./Terminal";

const brewLines: TerminalLine[] = [
  { type: "command", text: "brew tap mansooranis/kode" },
  { type: "command", text: "brew install kode" },
  { type: "command", text: "kode" },
];

const sourceLines: TerminalLine[] = [
  { type: "command", text: "git clone https://github.com/mansooranis/kode.git" },
  { type: "command", text: "cd kode && make build" },
  { type: "command", text: "./kode" },
];

const skillLines: TerminalLine[] = [
  { type: "command", text: "kode skill install" },
];

const nextStepLines: TerminalLine[] = [
  { type: "comment", text: "# jump into a diff of your working tree" },
  { type: "command", text: "kode" },
  { type: "blank" },
  { type: "comment", text: "# or a specific commit" },
  { type: "command", text: "kode show HEAD~1" },
  { type: "blank" },
  { type: "comment", text: "# review a GitHub pull request (needs `gh` installed & authenticated)" },
  { type: "command", text: "kode pr 128" },
  { type: "blank" },
  { type: "comment", text: "# see every command kode understands" },
  { type: "command", text: "kode help" },
];

export function Install() {
  return (
    <section id="install" className="border-t border-white/10 px-6 py-20">
      <div className="mx-auto max-w-5xl">
        <div className="mx-auto max-w-2xl text-center">
          <span className="text-xl font-semibold tracking-wide text-emerald-400 uppercase">
            Getting started
          </span>
        </div>

        <div className="mt-12 space-y-10">
          <div>
            <h3 className="mb-4 flex items-center gap-3 text-sm font-semibold text-white">
              <span className="flex h-6 w-6 items-center justify-center rounded-full bg-emerald-400/15 text-xs text-emerald-300">
                1
              </span>
              Install kode
            </h3>
            <div className="grid gap-6 md:grid-cols-2">
              <Terminal title="Homebrew (recommended)" lines={brewLines} />
              <Terminal title="From source" lines={sourceLines} />
            </div>
          </div>

          <div>
            <h3 className="mb-4 flex items-center gap-3 text-sm font-semibold text-white">
              <span className="flex h-6 w-6 items-center justify-center rounded-full bg-emerald-400/15 text-xs text-emerald-300">
                2
              </span>
              Install the annotations skill
            </h3>
            <p className="mb-4 text-sm text-zinc-400">
              Run this once so an agent session — like Claude Code — knows how to write notes and
              Mermaid diagrams into kode's shared annotations file. It copies the{" "}
              <code className="text-emerald-300">kode-comments</code> skill into{" "}
              <code className="text-emerald-300">~/.claude/skills</code>. This is what powers{" "}
              <code className="text-emerald-300">kode explain</code>.
            </p>
            <Terminal title="Skill setup" lines={skillLines} />
          </div>

          <div>
            <h3 className="mb-4 flex items-center gap-3 text-sm font-semibold text-white">
              <span className="flex h-6 w-6 items-center justify-center rounded-full bg-emerald-400/15 text-xs text-emerald-300">
                3
              </span>
              Try it
            </h3>
            <Terminal title="Next steps" lines={nextStepLines} />
          </div>
        </div>
      </div>
    </section>
  );
}
