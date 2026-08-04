import { Nav } from "./components/Nav";
import { Hero } from "./components/Hero";
import { Install } from "./components/Install";
import { AgentComments } from "./components/AgentComments";
import { FeatureSpotlight } from "./components/FeatureSpotlight";
import { FeatureGrid } from "./components/FeatureGrid";
import { Roadmap } from "./components/Roadmap";
import { Footer } from "./components/Footer";

function App() {
  return (
    <div className="min-h-screen bg-zinc-950">
      <Nav />
      <main>
        <Hero />
        <Install />
        <AgentComments />

        <FeatureSpotlight
          id="explain"
          eyebrow="kode explain"
          title="A guided walkthrough, written by an agent."
          description={
            <>
              <code className="text-emerald-300">kode explain</code> is a read-only viewer for
              annotations and Mermaid diagrams an agent has left about your code — Claude Code,
              for instance, once you run{" "}
              <code className="text-emerald-300">kode skill install</code> so it knows the
              format. Great for onboarding, or documenting how something works without a diff in
              flight. Notes are grouped by file and rendered in place, right next to the code
              they're about.
            </>
          }
          points={[
            "Notes and Mermaid diagrams rendered as terminal art, grouped by file",
            "Written by any agent session that can write JSON — no live connection to kode required",
            "Persisted to .kode/annotations.json, so it survives restarts and is diffable/shareable",
            "Press r to reload and pick up new notes without restarting",
          ]}
          command="kode explain"
          mediaLabel="GIF · kode explain rendering a walkthrough with a Mermaid diagram"
        />

        <FeatureSpotlight
          id="diff"
          eyebrow="kode diff"
          title="A diff review TUI built for reading, not just scrolling."
          description={
            <>
              Running <code className="text-emerald-300">kode</code> opens the diff of your
              working tree with a file sidebar, syntax highlighting, and a split or stacked
              layout you can toggle on the fly. Press <code className="text-emerald-300">c</code>{" "}
              to leave an inline comment anywhere, the same way you'd leave a review comment on
              GitHub — saved to the same annotations file <code className="text-emerald-300">kode explain</code> reads from.
            </>
          }
          points={[
            "File sidebar with keyboard and mouse navigation",
            "Split or stacked layout, toggle with m",
            "Syntax highlighting via Chroma",
            "Inline comments saved to .kode/annotations.json, reload with r",
          ]}
          command="kode show HEAD~1"
          mediaLabel="Screenshot · split-view diff with an inline comment open"
          reverse
        />

        <FeatureSpotlight
          id="pr"
          eyebrow="kode pr"
          title="Review a GitHub pull request without leaving the terminal."
          description={
            <>
              <code className="text-emerald-300">kode pr &lt;number&gt;</code> fetches a pull
              request's diff via the GitHub CLI and opens it in the same TUI as a local diff —
              same split view, same inline comments. Run{" "}
              <code className="text-emerald-300">kode pr</code> with no arguments to review the
              PR for your current branch.
            </>
          }
          points={[
            "Uses gh under the hood — kode tells you if it's missing or unauthenticated",
            "Identical review UI to a local diff: sidebar, layout toggle, inline comments",
            "No context switch to a browser to leave a first pass of review",
          ]}
          command="kode pr 128"
          mediaLabel="GIF · kode pr fetching and rendering a GitHub PR"
        />

        <FeatureGrid />
        <Roadmap />
      </main>
      <Footer />
    </div>
  );
}

export default App;
