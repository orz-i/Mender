import type { AgentBackend } from "./agent-backends";

// Local, offline-safe third-party marks. Source and license:
// packages/app/src/assets/agent-icons/SOURCES.md
const marks = {
  codex: { src: new URL("../../assets/agent-icons/codex.svg", import.meta.url).href, invertInDark: false },
  grok: { src: new URL("../../assets/agent-icons/grok.svg", import.meta.url).href, invertInDark: true },
  opencode: { src: new URL("../../assets/agent-icons/opencode.svg", import.meta.url).href, invertInDark: true },
  antigravity: { src: new URL("../../assets/agent-icons/antigravity.svg", import.meta.url).href, invertInDark: false },
} as const satisfies Record<AgentBackend, { src: string; invertInDark: boolean }>;

export function AgentPresetIcon({
  backend,
  className = "size-[27px]",
}: {
  backend: AgentBackend;
  className?: string;
}) {
  const mark = marks[backend];
  return (
    <img src={mark.src} alt={""} draggable={false} aria-hidden="true"
      className={className + " shrink-0 object-contain " + (mark.invertInDark ? "dark:invert" : "")} />
  );
}
