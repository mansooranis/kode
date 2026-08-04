import type { ReactNode } from "react";
import { MediaPlaceholder } from "./MediaPlaceholder";

export function FeatureSpotlight({
  id,
  eyebrow,
  title,
  description,
  points,
  command,
  mediaLabel,
  reverse = false,
}: {
  id: string;
  eyebrow: string;
  title: string;
  description: ReactNode;
  points: string[];
  command: string;
  mediaLabel: string;
  reverse?: boolean;
}) {
  return (
    <section id={id} className="border-t border-white/10 px-6 py-20">
      <div
        className={`mx-auto grid max-w-6xl items-center gap-12 md:grid-cols-2 ${
          reverse ? "md:[&>*:first-child]:order-2" : ""
        }`}
      >
        <div>
          <span className="text-sm font-semibold tracking-wide text-emerald-400 uppercase">
            {eyebrow}
          </span>
          <h2 className="mt-3 text-3xl font-bold text-white sm:text-4xl">{title}</h2>
          <p className="mt-4 text-zinc-400">{description}</p>
          <ul className="mt-6 space-y-3">
            {points.map((point) => (
              <li key={point} className="flex gap-3 text-sm text-zinc-300">
                <svg
                  className="mt-0.5 h-4 w-4 shrink-0 text-emerald-400"
                  viewBox="0 0 20 20"
                  fill="currentColor"
                >
                  <path
                    fillRule="evenodd"
                    d="M16.7 5.3a1 1 0 0 1 0 1.4l-7.5 7.5a1 1 0 0 1-1.4 0l-3.5-3.5a1 1 0 1 1 1.4-1.4l2.8 2.8 6.8-6.8a1 1 0 0 1 1.4 0Z"
                    clipRule="evenodd"
                  />
                </svg>
                {point}
              </li>
            ))}
          </ul>
          <code className="mt-6 inline-block rounded-lg border border-white/10 bg-white/[0.03] px-3 py-1.5 font-mono text-sm text-emerald-300">
            {command}
          </code>
        </div>
        <MediaPlaceholder label={mediaLabel} />
      </div>
    </section>
  );
}
