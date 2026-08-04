import { useState } from "react";
import { CopyIcon, copyText, Terminal, type TerminalLine } from "./Terminal";

const prompts = [
  "/kode-comments explain how the auth middleware works, and diagram the request flow",
  "/kode-comments document this package for a new engineer onboarding",
];

const viewLines: TerminalLine[] = [
  { type: "comment", text: "# once the agent is done writing" },
  { type: "command", text: "kode explain" },
];

function PromptBubble({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);

  function handleCopy() {
    copyText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  }

  return (
    <div className="group flex items-start justify-between gap-3 rounded-xl border border-white/10 bg-zinc-900/80 px-4 py-3 font-mono text-sm text-zinc-300">
      <span>
        <span className="text-emerald-400">›</span> {text}
      </span>
      <button
        type="button"
        onClick={handleCopy}
        aria-label={`Copy "${text}"`}
        className="mt-0.5 shrink-0 text-zinc-600 opacity-60 transition hover:text-white group-hover:opacity-100"
      >
        <CopyIcon copied={copied} />
      </button>
    </div>
  );
}

export function AgentComments() {
  return (
    <section id="comments" className="border-t border-white/10 px-6 py-20">
      <div className="mx-auto max-w-5xl">
        <div className="mx-auto max-w-2xl text-center">
          <span className="text-sm font-semibold tracking-wide text-emerald-400 uppercase">
            Write comments
          </span>
          <h2 className="mt-3 text-3xl font-bold text-white sm:text-4xl">
            Ask an agent to document your code
          </h2>
          <p className="mt-4 text-zinc-400">
            Once the <code className="text-emerald-300">kode-comments</code> skill is installed
            (step 2 above), open Claude Code — or any agent that supports Claude-Code-style
            skills — in your repo and invoke it by name.
          </p>
        </div>

        <div className="mt-12 grid gap-10 md:grid-cols-2">
          <div>
            <h3 className="mb-4 flex items-center gap-3 text-sm font-semibold text-white">
              <span className="flex h-6 w-6 items-center justify-center rounded-full bg-emerald-400/15 text-xs text-emerald-300">
                1
              </span>
              Tell it what to explore
            </h3>
            <p className="mb-4 text-sm text-zinc-400">
              Type <code className="text-emerald-300">/kode-comments</code> followed by what you
              want documented. The agent reads the code, then writes notes — and, for control
              flow, Mermaid diagrams rendered to terminal art — straight into{" "}
              <code className="text-emerald-300">.kode/annotations.json</code>.
            </p>
            <div className="space-y-3">
              {prompts.map((prompt) => (
                <PromptBubble key={prompt} text={prompt} />
              ))}
            </div>
          </div>

          <div>
            <h3 className="mb-4 flex items-center gap-3 text-sm font-semibold text-white">
              <span className="flex h-6 w-6 items-center justify-center rounded-full bg-emerald-400/15 text-xs text-emerald-300">
                2
              </span>
              View the walkthrough
            </h3>
            <p className="mb-4 text-sm text-zinc-400">
              Open the read-only viewer to browse what the agent wrote. Already have{" "}
              <code className="text-emerald-300">kode explain</code> open? Press{" "}
              <code className="text-emerald-300">r</code> to reload without restarting.
            </p>
            <Terminal title="View it" lines={viewLines} />
          </div>
        </div>
      </div>
    </section>
  );
}
