export function Footer() {
  return (
    <footer className="border-t border-white/10 px-6 py-12">
      <div className="mx-auto flex max-w-6xl flex-col items-center gap-4 text-center">
        <a href="#top" className="flex items-center gap-2 font-semibold text-white">
          <span className="font-mono text-emerald-400">&gt;_</span>
          kode
        </a>
        <p className="text-sm text-zinc-500">Terminal-native diff review, with an agent built in.</p>
        <div className="flex gap-6 text-sm text-zinc-400">
          <a
            href="https://github.com/mansooranis/kode"
            target="_blank"
            rel="noopener"
            className="transition hover:text-white"
          >
            GitHub
          </a>
          <a
            href="https://github.com/mansooranis/kode/blob/main/README.md"
            target="_blank"
            rel="noopener"
            className="transition hover:text-white"
          >
            Docs
          </a>
          <a
            href="https://github.com/mansooranis/kode/blob/main/LICENSE"
            target="_blank"
            rel="noopener"
            className="transition hover:text-white"
          >
            License
          </a>
        </div>
      </div>
    </footer>
  );
}
