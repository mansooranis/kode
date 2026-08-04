export function MediaPlaceholder({
  label,
  className = "",
}: {
  label: string;
  className?: string;
}) {
  return (
    <div
      className={`flex aspect-video w-full items-center justify-center rounded-xl border border-dashed border-white/15 bg-white/[0.02] ${className}`}
    >
      <div className="flex flex-col items-center gap-2 px-6 text-center">
        <svg
          className="h-8 w-8 text-zinc-600"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.5"
        >
          <rect x="3" y="5" width="18" height="14" rx="2" />
          <path d="m3 15 4.5-4.5a2 2 0 0 1 2.8 0L15 15" />
          <circle cx="16.5" cy="9.5" r="1.5" />
        </svg>
        <span className="text-xs font-medium text-zinc-600">{label}</span>
      </div>
    </div>
  );
}
