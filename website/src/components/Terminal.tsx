import { useState } from "react";

export type TerminalLine =
  | { type: "comment"; text: string }
  | { type: "command"; text: string }
  | { type: "blank" };

export function CopyIcon({ copied }: { copied: boolean }) {
  if (copied) {
    return (
      <svg className="h-3.5 w-3.5" viewBox="0 0 20 20" fill="currentColor">
        <path
          fillRule="evenodd"
          d="M16.7 5.3a1 1 0 0 1 0 1.4l-7.5 7.5a1 1 0 0 1-1.4 0l-3.5-3.5a1 1 0 1 1 1.4-1.4l2.8 2.8 6.8-6.8a1 1 0 0 1 1.4 0Z"
          clipRule="evenodd"
        />
      </svg>
    );
  }
  return (
    <svg className="h-3.5 w-3.5" viewBox="0 0 20 20" fill="none" stroke="currentColor" strokeWidth="1.5">
      <rect x="7" y="7" width="10" height="10" rx="1.5" />
      <path d="M4.5 12.5H4a1.5 1.5 0 0 1-1.5-1.5V4A1.5 1.5 0 0 1 4 2.5h7A1.5 1.5 0 0 1 12.5 4v.5" />
    </svg>
  );
}

export async function copyText(text: string) {
  try {
    await navigator.clipboard.writeText(text);
  } catch {
    // clipboard API unavailable (e.g. insecure context) — fail silently
  }
}

export function Terminal({ title, lines }: { title: string; lines: TerminalLine[] }) {
  const [copiedAll, setCopiedAll] = useState(false);
  const [copiedIndex, setCopiedIndex] = useState<number | null>(null);

  const commands = lines.filter((l) => l.type === "command").map((l) => l.text);

  function handleCopyAll() {
    copyText(commands.join("\n"));
    setCopiedAll(true);
    setTimeout(() => setCopiedAll(false), 1500);
  }

  function handleCopyLine(index: number, text: string) {
    copyText(text);
    setCopiedIndex(index);
    setTimeout(() => setCopiedIndex(null), 1500);
  }

  return (
    <div className="overflow-hidden rounded-xl border border-white/10 bg-zinc-900/80 shadow-2xl shadow-black/40">
      <div className="flex items-center gap-2 border-b border-white/10 bg-white/[0.03] px-4 py-2.5">
        <span className="h-2.5 w-2.5 shrink-0 rounded-full bg-red-500/70" />
        <span className="h-2.5 w-2.5 shrink-0 rounded-full bg-yellow-500/70" />
        <span className="h-2.5 w-2.5 shrink-0 rounded-full bg-green-500/70" />
        <span className="ml-2 min-w-0 truncate font-mono text-xs text-zinc-500">{title}</span>
        {commands.length > 0 && (
          <button
            type="button"
            onClick={handleCopyAll}
            className="ml-auto flex shrink-0 items-center gap-1.5 rounded-md border border-white/10 px-2 py-1 text-[11px] font-medium text-zinc-400 transition hover:border-white/20 hover:text-white"
          >
            <CopyIcon copied={copiedAll} />
            <span className="hidden sm:inline">{copiedAll ? "Copied" : "Copy all"}</span>
          </button>
        )}
      </div>
      <div className="overflow-x-auto px-5 py-4 font-mono text-[13px] leading-relaxed text-zinc-300">
        {lines.map((line, i) => {
          if (line.type === "blank") return <div key={i} className="h-3" />;
          if (line.type === "comment") {
            return (
              <div key={i} className="text-zinc-600">
                {line.text}
              </div>
            );
          }
          return (
            <div key={i} className="group -mx-2 flex items-center justify-between gap-3 rounded px-2 py-0.5 hover:bg-white/[0.03]">
              <span className="whitespace-pre">
                <span className="text-emerald-400">$</span> {line.text}
              </span>
              <button
                type="button"
                onClick={() => handleCopyLine(i, line.text)}
                aria-label={`Copy "${line.text}"`}
                className="shrink-0 text-zinc-600 opacity-60 transition hover:text-white group-hover:opacity-100"
              >
                <CopyIcon copied={copiedIndex === i} />
              </button>
            </div>
          );
        })}
      </div>
    </div>
  );
}
