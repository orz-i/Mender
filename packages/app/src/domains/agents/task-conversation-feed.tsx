import { useEffect, useState } from "react";
import type { AgentTask, AgentTaskEvent, ManagementClient } from "@mender/sdk";
import {
  BotIcon,
  CheckIcon,
  ChevronDownIcon,
  CopyIcon,
  ShieldCheckIcon,
  SparklesIcon,
  TerminalIcon,
  UserIcon,
  WrenchIcon,
} from "lucide-react";

import { formatDateTime, t } from "../../i18n/i18n";
import { StatusBadge } from "../../shared/status-badge";
import { taskDiagnosticText, taskStatusText } from "./agent-detail-page";

function MarkdownInline({ text }: { text: string }) {
  const parts: React.ReactNode[] = [];
  const regex = /(`[^`]+`|\*\*[^*]+\*\*|\*[^*]+\*)/g;
  let lastIndex = 0;
  let match: RegExpExecArray | null;

  while ((match = regex.exec(text)) !== null) {
    if (match.index > lastIndex) {
      parts.push(text.slice(lastIndex, match.index));
    }
    const token = match[0];
    if (token.startsWith("`") && token.endsWith("`")) {
      parts.push(
        <code
          key={match.index}
          className="rounded bg-muted px-1.5 py-0.5 font-mono text-xs text-foreground"
        >
          {token.slice(1, -1)}
        </code>,
      );
    } else if (token.startsWith("**") && token.endsWith("**")) {
      parts.push(
        <strong key={match.index} className="font-semibold">
          {token.slice(2, -2)}
        </strong>,
      );
    } else if (token.startsWith("*") && token.endsWith("*")) {
      parts.push(
        <em key={match.index} className="italic">
          {token.slice(1, -1)}
        </em>,
      );
    }
    lastIndex = regex.lastIndex;
  }
  if (lastIndex < text.length) {
    parts.push(text.slice(lastIndex));
  }
  return <>{parts}</>;
}

function CodeBlock({ code, language }: { code: string; language?: string }) {
  const [copied, setCopied] = useState(false);

  const copy = () => {
    void navigator.clipboard.writeText(code).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    });
  };

  return (
    <div className="relative my-2 overflow-hidden rounded-lg border border-border bg-muted/60">
      <div className="flex items-center justify-between border-b border-border/40 bg-muted px-3 py-1 text-[11px] text-muted-foreground">
        <span className="font-mono uppercase">{language || "text"}</span>
        <button
          type="button"
          onClick={copy}
          className="inline-flex cursor-pointer items-center gap-1 text-xs hover:text-foreground"
          aria-label={t("Copy code")}
        >
          {copied ? (
            <>
              <CheckIcon className="size-3 text-green-500" />
              <span>{t("Copied")}</span>
            </>
          ) : (
            <>
              <CopyIcon className="size-3" />
              <span>{t("Copy")}</span>
            </>
          )}
        </button>
      </div>
      <pre className="overflow-x-auto p-3 font-mono text-xs leading-5">
        <code>{code}</code>
      </pre>
    </div>
  );
}

export function MarkdownContent({ content }: { content: string }) {
  if (!content) return null;

  const segments = content.split(/(```[\s\S]*?```)/g);

  return (
    <div className="space-y-2 text-sm leading-6">
      {segments.map((segment, idx) => {
        if (segment.startsWith("```") && segment.endsWith("```")) {
          const inner = segment.slice(3, -3);
          const firstLineEnd = inner.indexOf("\n");
          let lang = "";
          let code = inner;
          if (firstLineEnd !== -1) {
            lang = inner.slice(0, firstLineEnd).trim();
            code = inner.slice(firstLineEnd + 1);
          }
          return <CodeBlock key={idx} code={code} language={lang} />;
        }

        const lines = segment.split("\n");
        return (
          <div key={idx} className="space-y-1">
            {lines.map((line, lineIdx) => {
              const trimmed = line.trim();
              if (!trimmed) {
                return <div key={lineIdx} className="h-1" />;
              }
              if (trimmed.startsWith("### ")) {
                return (
                  <h4 key={lineIdx} className="pt-1 text-sm font-bold text-foreground">
                    <MarkdownInline text={trimmed.slice(4)} />
                  </h4>
                );
              }
              if (trimmed.startsWith("## ")) {
                return (
                  <h3 key={lineIdx} className="pt-2 text-base font-bold text-foreground">
                    <MarkdownInline text={trimmed.slice(3)} />
                  </h3>
                );
              }
              if (trimmed.startsWith("# ")) {
                return (
                  <h2 key={lineIdx} className="pt-2 text-lg font-bold text-foreground">
                    <MarkdownInline text={trimmed.slice(2)} />
                  </h2>
                );
              }
              if (trimmed.startsWith("> ")) {
                return (
                  <blockquote
                    key={lineIdx}
                    className="border-l-2 border-primary/60 pl-3 italic text-muted-foreground"
                  >
                    <MarkdownInline text={trimmed.slice(2)} />
                  </blockquote>
                );
              }
              if (trimmed.startsWith("- ") || trimmed.startsWith("* ")) {
                return (
                  <div key={lineIdx} className="flex items-start gap-2 pl-2">
                    <span className="text-muted-foreground">•</span>
                    <span>
                      <MarkdownInline text={trimmed.slice(2)} />
                    </span>
                  </div>
                );
              }
              return (
                <p key={lineIdx} className="whitespace-pre-wrap break-words">
                  <MarkdownInline text={line} />
                </p>
              );
            })}
          </div>
        );
      })}
    </div>
  );
}

function formatResultOutput(payload: Record<string, any> | undefined): string {
  if (!payload) return "";
  const out = payload.output ?? payload.result ?? payload.content ?? payload.error ?? payload;
  if (typeof out === "string") return out;
  try {
    return JSON.stringify(out, null, 2);
  } catch {
    return String(out);
  }
}

type FeedGroup =
  | { kind: "user_prompt"; event: AgentTaskEvent }
  | { kind: "thinking"; event: AgentTaskEvent }
  | { kind: "tool_call"; callEvent: AgentTaskEvent; resultEvent?: AgentTaskEvent | undefined }
  | { kind: "tool_result"; event: AgentTaskEvent }
  | { kind: "message"; events: AgentTaskEvent[]; text: string; createdAt: string }
  | { kind: "approval"; event: AgentTaskEvent }
  | { kind: "terminal"; event: AgentTaskEvent };

function groupEvents(events: AgentTaskEvent[]): FeedGroup[] {
  const toolResultsByCallId = new Map<string, AgentTaskEvent>();
  const claimedResultEventIds = new Set<string>();

  for (const ev of events) {
    if (ev.type === "tool_result" && ev.payload?.toolCallId) {
      toolResultsByCallId.set(String(ev.payload.toolCallId), ev);
    }
  }

  const groups: FeedGroup[] = [];
  let pendingChunks: AgentTaskEvent[] = [];

  const flushChunks = () => {
    if (pendingChunks.length > 0) {
      const text = pendingChunks
        .map((e) => String(e.payload?.text ?? e.payload?.chunk ?? e.payload?.content ?? e.payload?.delta ?? ""))
        .join("");
      const first = pendingChunks[0];
      groups.push({
        kind: "message",
        events: pendingChunks,
        text,
        createdAt: first?.createdAt ?? "",
      });
      pendingChunks = [];
    }
  };

  for (const ev of events) {
    if (ev.type === "message_chunk") {
      pendingChunks.push(ev);
      continue;
    }

    flushChunks();

    if (ev.type === "user_prompt") {
      groups.push({ kind: "user_prompt", event: ev });
    } else if (ev.type === "thinking") {
      groups.push({ kind: "thinking", event: ev });
    } else if (ev.type === "tool_call") {
      const callId = ev.payload?.toolCallId ? String(ev.payload.toolCallId) : "";
      const resultEvent = callId ? toolResultsByCallId.get(callId) : undefined;
      if (resultEvent) claimedResultEventIds.add(resultEvent.eventId);
      groups.push({ kind: "tool_call", callEvent: ev, resultEvent });
    } else if (ev.type === "tool_result") {
      if (!claimedResultEventIds.has(ev.eventId)) {
        groups.push({ kind: "tool_result", event: ev });
      }
    } else if (ev.type === "approval") {
      groups.push({ kind: "approval", event: ev });
    } else if (ev.type === "terminal") {
      groups.push({ kind: "terminal", event: ev });
    }
  }

  flushChunks();
  return groups;
}

export function TaskConversationFeed({
  task,
  client,
}: {
  task: AgentTask;
  client: Pick<ManagementClient, "getAgentTaskTranscript">;
}) {
  const [events, setEvents] = useState<AgentTaskEvent[] | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let active = true;
    const fetchTranscript = async () => {
      try {
        const res = await client.getAgentTaskTranscript(task.taskId);
        if (!active) return;
        setEvents(res.events ?? []);
      } catch {
        if (!active) return;
        setEvents([]);
      } finally {
        if (active) setLoading(false);
      }
    };

    void fetchTranscript();

    if (task.status === "running" || task.status === "queued") {
      const timer = setInterval(() => {
        void fetchTranscript();
      }, 2000);
      return () => {
        active = false;
        clearInterval(timer);
      };
    }

    return () => {
      active = false;
    };
  }, [client, task.taskId, task.status]);

  if (!loading && (!events || events.length === 0)) {
    return (
      <div className="space-y-3">
        {task.result ? (
          <pre className="max-h-[38vh] overflow-auto whitespace-pre-wrap break-words rounded-xl bg-muted p-4 text-xs leading-5">
            {task.result}
          </pre>
        ) : (
          <p className="text-sm leading-6 text-muted-foreground">
            {task.diagnostic ? taskDiagnosticText(task.diagnostic) : t("No result available.")}
          </p>
        )}
      </div>
    );
  }

  if (loading && events === null) {
    return (
      <div className="flex items-center justify-center p-6 text-xs text-muted-foreground">
        <span>{t("Loading conversation transcript...")}</span>
      </div>
    );
  }

  const groups = groupEvents(events ?? []);

  const terminalTitle =
    task.status === "completed"
      ? t("Task completed")
      : task.status === "failed"
        ? t("Task failed")
        : task.status === "cancelled"
          ? t("Task cancelled")
          : task.status === "running"
            ? t("Task running")
            : task.status === "queued"
              ? t("Task queued")
              : t("Task status unknown");

  return (
    <div className="flex max-h-[50vh] flex-col gap-4 overflow-y-auto pr-1">
      {groups.map((item, idx) => {
        switch (item.kind) {
          case "user_prompt": {
            const promptText =
              item.event.payload?.prompt ??
              item.event.payload?.text ??
              item.event.payload?.content ??
              "";
            return (
              <div key={item.event.eventId || idx} className="flex flex-col items-end gap-1">
                <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
                  <UserIcon className="size-3.5" />
                  <span>{t("User prompt")}</span>
                  {item.event.createdAt ? <span>· {formatDateTime(item.event.createdAt)}</span> : null}
                </div>
                <div className="max-w-[85%] rounded-2xl bg-primary px-4 py-2.5 text-sm text-primary-foreground shadow-xs">
                  <p className="whitespace-pre-wrap break-words">{promptText}</p>
                </div>
              </div>
            );
          }
          case "thinking": {
            const thinkingText =
              item.event.payload?.thought ??
              item.event.payload?.thinking ??
              item.event.payload?.text ??
              item.event.payload?.content ??
              "";
            return (
              <details
                key={item.event.eventId || idx}
                className="group rounded-xl border border-border/70 bg-muted/30 text-xs"
              >
                <summary className="flex cursor-pointer items-center gap-2 px-3.5 py-2.5 font-medium text-muted-foreground hover:text-foreground">
                  <SparklesIcon className="size-3.5 text-amber-500" />
                  <span>{t("Thinking process")}</span>
                  <ChevronDownIcon className="ml-auto size-3.5 transition-transform group-open:rotate-180" />
                </summary>
                <div className="border-t border-border/40 bg-muted/10 px-3.5 py-2.5 font-mono whitespace-pre-wrap break-words text-muted-foreground">
                  {thinkingText}
                </div>
              </details>
            );
          }
          case "tool_call": {
            const call = item.callEvent;
            const res = item.resultEvent;
            const toolName =
              call.payload?.name ??
              call.payload?.tool ??
              call.payload?.title ??
              call.payload?.kind ??
              t("Tool call");
            const args =
              call.payload?.args ??
              call.payload?.arguments ??
              call.payload?.input ??
              call.payload?.rawInput;
            const hasArgs = args !== undefined && args !== null && args !== "";
            const isFailed = res?.payload?.error !== undefined || res?.payload?.status === "error";

            return (
              <div
                key={call.eventId || idx}
                className="space-y-2 rounded-xl border border-border/70 bg-card p-3 shadow-xs"
              >
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2 text-xs font-semibold">
                    <WrenchIcon className="size-3.5 text-blue-500" />
                    <span className="font-mono">{String(toolName)}</span>
                  </div>
                  {res ? (
                    isFailed ? (
                      <span className="text-[11px] font-medium text-destructive">
                        {t("Tool call failed")}
                      </span>
                    ) : (
                      <span className="text-[11px] font-medium text-green-600 dark:text-green-400">
                        {t("Tool call succeeded")}
                      </span>
                    )
                  ) : (
                    <span className="text-[11px] font-medium text-muted-foreground">
                      {t("Running")}
                    </span>
                  )}
                </div>
                {hasArgs ? (
                  <details className="text-xs">
                    <summary className="cursor-pointer text-muted-foreground hover:text-foreground">
                      {t("Tool arguments")}
                    </summary>
                    <pre className="mt-1 max-h-36 overflow-auto rounded bg-muted/60 p-2 font-mono text-[11px]">
                      {typeof args === "object" ? JSON.stringify(args, null, 2) : String(args)}
                    </pre>
                  </details>
                ) : null}
                {res ? (
                  <div className="rounded-lg bg-muted/30 p-2 text-xs">
                    <div className="mb-1 text-[11px] font-medium text-muted-foreground">
                      {t("Tool output")}
                    </div>
                    <pre className="max-h-48 overflow-auto whitespace-pre-wrap font-mono text-[11px] leading-4 text-foreground">
                      {formatResultOutput(res.payload)}
                    </pre>
                  </div>
                ) : null}
              </div>
            );
          }
          case "tool_result": {
            return (
              <div
                key={item.event.eventId || idx}
                className="space-y-1 rounded-xl border border-border/70 bg-card p-3 shadow-xs"
              >
                <div className="flex items-center gap-2 text-xs font-semibold">
                  <WrenchIcon className="size-3.5 text-blue-500" />
                  <span>{t("Tool output")}</span>
                </div>
                <pre className="max-h-48 overflow-auto whitespace-pre-wrap font-mono text-[11px] leading-4 text-foreground">
                  {formatResultOutput(item.event.payload)}
                </pre>
              </div>
            );
          }
          case "message": {
            return (
              <div key={idx} className="flex items-start gap-3">
                <div className="mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-full bg-primary/10 text-primary">
                  <BotIcon className="size-4" />
                </div>
                <div className="flex-1 space-y-1">
                  <div className="flex items-center gap-2 text-xs font-semibold text-muted-foreground">
                    <span>{t("Assistant")}</span>
                    {item.createdAt ? <span>· {formatDateTime(item.createdAt)}</span> : null}
                  </div>
                  <div className="rounded-2xl rounded-tl-sm border border-border/50 bg-card p-4 shadow-xs">
                    <MarkdownContent content={item.text} />
                  </div>
                </div>
              </div>
            );
          }
          case "approval": {
            return (
              <div
                key={item.event.eventId || idx}
                className="space-y-1 rounded-xl border border-amber-500/40 bg-amber-500/5 p-3 text-xs"
              >
                <div className="flex items-center gap-2 font-semibold text-amber-700 dark:text-amber-400">
                  <ShieldCheckIcon className="size-4" />
                  <span>{t("Approval requested")}</span>
                </div>
                {item.event.payload?.target ? (
                  <p className="text-muted-foreground">
                    <span className="font-medium text-foreground">{t("Target")}:</span>{" "}
                    {String(item.event.payload.target)}
                  </p>
                ) : null}
                {item.event.payload?.decision ? (
                  <p className="text-muted-foreground">
                    <span className="font-medium text-foreground">{t("Decision")}:</span>{" "}
                    {String(item.event.payload.decision)}
                  </p>
                ) : null}
              </div>
            );
          }
          case "terminal": {
            return null; // Handled by terminal badge at bottom
          }
        }
      })}

      <div className="flex items-center justify-between rounded-xl border border-border/80 bg-muted/40 px-4 py-3">
        <div className="flex items-center gap-2.5">
          <TerminalIcon className="size-4 text-muted-foreground" />
          <div className="flex flex-col">
            <span className="text-xs font-semibold text-foreground">{terminalTitle}</span>
            {task.diagnostic ? (
              <span className="text-xs text-muted-foreground">
                {taskDiagnosticText(task.diagnostic)}
              </span>
            ) : null}
          </div>
        </div>
        <StatusBadge status={task.status}>{taskStatusText(task.status)}</StatusBadge>
      </div>
    </div>
  );
}
