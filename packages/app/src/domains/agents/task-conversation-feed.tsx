import { useEffect, useRef, useState } from "react";
import type { AgentApprovalCapability, AgentTask, AgentTaskEvent, ManagementClient } from "@mender/sdk";
import { Button } from "@mender/ui/components/button";
import { cn } from "@mender/ui/lib/utils";
import {
  ArrowDownIcon,
  BotIcon,
  CheckIcon,
  ChevronDownIcon,
  CopyIcon,
  SendIcon,
  ShieldAlertIcon,
  ShieldCheckIcon,
  SparklesIcon,
  TerminalIcon,
  UserIcon,
  WrenchIcon,
} from "lucide-react";

import { formatDateTime, t } from "../../i18n/i18n";
import { errorMessage } from "../../shared/errors";
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

function InlineApprovalCard({
  event,
  approvals,
  onDecided,
}: {
  event: AgentTaskEvent;
  approvals?: Pick<AgentApprovalCapability, "decideAgentApproval"> | undefined;
  onDecided?: ((approvalId: string, choice: "allow_once" | "reject") => void) | undefined;
}) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const payload = event.payload ?? {};
  const approvalId = String(payload.approvalId ?? event.eventId);
  const rawStatus = String(payload.status ?? "pending");
  const [localStatus, setLocalStatus] = useState<string | null>(null);

  const status = localStatus ?? rawStatus;
  const isPending = status === "pending";
  const isAllowed = status === "allowed_once" || status === "allowed";
  const isRejected = status === "rejected";
  const isExpired = status === "expired";

  const handleDecision = async (choice: "allow_once" | "reject") => {
    if (busy || !approvals) return;
    setBusy(true);
    setError("");
    try {
      await approvals.decideAgentApproval(approvalId, choice);
      setLocalStatus(choice === "allow_once" ? "allowed_once" : "rejected");
      onDecided?.(approvalId, choice);
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy(false);
    }
  };

  const kindLabel =
    payload.kind === "read"
      ? t("Read")
      : payload.kind === "edit"
        ? t("Edit")
        : payload.kind === "execute"
          ? t("Execute")
          : payload.kind
            ? t("Other")
            : null;

  const decisionDisplay = payload.decision ?? (localStatus ? (localStatus === "allowed_once" ? "allow_once" : "reject") : null);

  return (
    <div
      className={cn(
        "space-y-2 rounded-xl border p-3.5 text-xs transition-colors",
        isPending && "border-amber-500/40 bg-amber-500/5",
        isAllowed && "border-green-500/40 bg-green-500/5",
        isRejected && "border-destructive/40 bg-destructive/5",
        isExpired && "border-border/60 bg-muted/20 text-muted-foreground",
      )}
    >
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2 font-semibold">
          {isAllowed ? (
            <ShieldCheckIcon className="size-4 text-green-600 dark:text-green-400" />
          ) : isRejected ? (
            <ShieldAlertIcon className="size-4 text-destructive" />
          ) : (
            <ShieldCheckIcon className="size-4 text-amber-600 dark:text-amber-400" />
          )}
          <span
            className={cn(
              isAllowed && "text-green-700 dark:text-green-400",
              isRejected && "text-destructive",
              isPending && "text-amber-700 dark:text-amber-400",
            )}
          >
            {isAllowed
              ? t("Approval granted")
              : isRejected
                ? t("Approval rejected")
                : isExpired
                  ? t("Approval expired")
                  : t("Approval requested")}
          </span>
        </div>
        <div className="flex items-center gap-1.5">
          {isAllowed ? (
            <span className="rounded-full bg-green-500/10 px-2 py-0.5 text-[11px] font-medium text-green-700 dark:text-green-300">
              {t("Allow once")}
            </span>
          ) : isRejected ? (
            <span className="rounded-full bg-destructive/10 px-2 py-0.5 text-[11px] font-medium text-destructive">
              {t("Reject")}
            </span>
          ) : isExpired ? (
            <span className="rounded-full bg-muted px-2 py-0.5 text-[11px] font-medium text-muted-foreground">
              {t("Approval expired")}
            </span>
          ) : (
            <span className="rounded-full bg-amber-500/15 px-2 py-0.5 text-[11px] font-medium text-amber-700 dark:text-amber-300">
              {t("Awaiting owner approval")}
            </span>
          )}
        </div>
      </div>

      <div className="space-y-1 text-muted-foreground">
        {kindLabel ? (
          <p>
            <span className="font-medium text-foreground">{t("Requested tool category")}:</span>{" "}
            <span>{kindLabel}</span>
          </p>
        ) : null}
        {payload.target ? (
          <p className="break-all">
            <span className="font-medium text-foreground">{t("Target")}:</span>{" "}
            <span className="font-mono text-xs">{String(payload.target)}</span>
          </p>
        ) : null}
        {decisionDisplay ? (
          <p>
            <span className="font-medium text-foreground">{t("Decision")}:</span>{" "}
            <span>{String(decisionDisplay)}</span>
          </p>
        ) : null}
      </div>

      {isPending && approvals ? (
        <div className="flex flex-wrap items-center gap-2 pt-1">
          {payload.canAllow !== false ? (
            <Button
              size="sm"
              disabled={busy}
              onClick={() => void handleDecision("allow_once")}
            >
              {t("Allow once")}
            </Button>
          ) : null}
          <Button
            size="sm"
            variant="outline"
            disabled={busy}
            onClick={() => void handleDecision("reject")}
          >
            {t("Reject")}
          </Button>
        </div>
      ) : null}

      {error ? (
        <p role="alert" className="text-xs text-destructive">
          {error}
        </p>
      ) : null}
    </div>
  );
}

type FeedGroup =
  | { kind: "user_prompt"; event: AgentTaskEvent }
  | { kind: "thinking"; events: AgentTaskEvent[]; text: string; createdAt: string }
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
  let pendingMessageChunks: AgentTaskEvent[] = [];
  let pendingThinkingChunks: AgentTaskEvent[] = [];

  const flushMessageChunks = () => {
    if (pendingMessageChunks.length > 0) {
      const text = pendingMessageChunks
        .map((e) => String(e.payload?.text ?? e.payload?.chunk ?? e.payload?.content ?? e.payload?.delta ?? ""))
        .join("");
      const first = pendingMessageChunks[0];
      groups.push({
        kind: "message",
        events: pendingMessageChunks,
        text,
        createdAt: first?.createdAt ?? "",
      });
      pendingMessageChunks = [];
    }
  };

  const flushThinkingChunks = () => {
    if (pendingThinkingChunks.length > 0) {
      const text = pendingThinkingChunks
        .map((e) =>
          String(
            e.payload?.thought ??
            e.payload?.thinking ??
            e.payload?.text ??
            e.payload?.content ??
            e.payload?.delta ??
            "",
          ),
        )
        .join("");
      const first = pendingThinkingChunks[0];
      groups.push({
        kind: "thinking",
        events: pendingThinkingChunks,
        text,
        createdAt: first?.createdAt ?? "",
      });
      pendingThinkingChunks = [];
    }
  };

  for (const ev of events) {
    if (ev.type === "message_chunk") {
      flushThinkingChunks();
      pendingMessageChunks.push(ev);
      continue;
    }

    if (ev.type === "thinking") {
      flushMessageChunks();
      pendingThinkingChunks.push(ev);
      continue;
    }

    flushMessageChunks();
    flushThinkingChunks();

    if (ev.type === "user_prompt") {
      groups.push({ kind: "user_prompt", event: ev });
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

  flushMessageChunks();
  flushThinkingChunks();
  return groups;
}

function FollowUpReplyBar({
  taskId,
  client,
  onTaskResumed,
}: {
  taskId: string;
  client: { resumeAgentTask?: (taskId: string, prompt: string) => Promise<AgentTask> };
  onTaskResumed?: ((newTask: AgentTask) => void) | undefined;
}) {
  const [followUpPrompt, setFollowUpPrompt] = useState("");
  const [resuming, setResuming] = useState(false);
  const [resumeError, setResumeError] = useState("");

  const handleResume = async () => {
    const trimmed = followUpPrompt.trim();
    if (!trimmed || resuming || !client.resumeAgentTask) return;
    setResuming(true);
    setResumeError("");
    try {
      const newTask = await client.resumeAgentTask(taskId, trimmed);
      setFollowUpPrompt("");
      onTaskResumed?.(newTask);
    } catch (err) {
      setResumeError(errorMessage(err));
    } finally {
      setResuming(false);
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      void handleResume();
    }
  };

  if (!client.resumeAgentTask) return null;

  return (
    <div className="space-y-2 rounded-xl border border-border bg-card p-3 shadow-xs">
      <div className="flex items-center justify-between text-xs text-muted-foreground">
        <span className="font-medium text-foreground">{t("Continue session")}</span>
        <span className="text-[11px]">{t("Press Enter to send, Shift+Enter for new line")}</span>
      </div>
      <textarea
        value={followUpPrompt}
        onChange={(e) => setFollowUpPrompt(e.target.value)}
        onKeyDown={handleKeyDown}
        placeholder={t("Ask a follow-up or provide next instructions...")}
        rows={2}
        disabled={resuming}
        aria-label={t("Follow-up prompt")}
        className="w-full resize-none rounded-lg border border-input bg-background p-2.5 text-sm text-foreground placeholder:text-muted-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring disabled:cursor-not-allowed disabled:opacity-50"
      />
      {resumeError ? (
        <p role="alert" className="text-xs text-destructive">
          {resumeError}
        </p>
      ) : null}
      <div className="flex items-center justify-end">
        <Button
          size="sm"
          disabled={!followUpPrompt.trim() || resuming}
          onClick={() => void handleResume()}
          className="gap-1.5"
        >
          <SendIcon className="size-3.5" />
          <span>{resuming ? t("Sending...") : t("Send")}</span>
        </Button>
      </div>
    </div>
  );
}

export interface TaskConversationFeedProps {
  task: AgentTask;
  client: Pick<ManagementClient, "getAgentTaskTranscript"> &
    Partial<Pick<ManagementClient, "subscribeAgentTaskEvents" | "resumeAgentTask">>;
  approvals?: Pick<AgentApprovalCapability, "decideAgentApproval" | "listAgentApprovals"> | undefined;
  onTaskUpdated?: ((task: AgentTask) => void) | undefined;
  onHasInlineApproval?: ((hasInline: boolean) => void) | undefined;
  onTaskResumed?: ((newTask: AgentTask) => void) | undefined;
}

export function TaskConversationFeed({
  task,
  client,
  approvals,
  onTaskUpdated,
  onHasInlineApproval,
  onTaskResumed,
}: TaskConversationFeedProps) {
  const [events, setEvents] = useState<AgentTaskEvent[] | null>(null);
  const [loading, setLoading] = useState(true);
  const containerRef = useRef<HTMLDivElement>(null);
  const bottomRef = useRef<HTMLDivElement>(null);
  const [isPinnedToBottom, setIsPinnedToBottom] = useState(true);
  const [hasNewMessages, setHasNewMessages] = useState(false);
  const prevEventCountRef = useRef(0);

  useEffect(() => {
    let active = true;
    let unsubscribeStream: (() => void) | undefined;

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
      if (typeof client.subscribeAgentTaskEvents === "function") {
        unsubscribeStream = client.subscribeAgentTaskEvents(
          task.taskId,
          (newEvent) => {
            if (!active) return;
            setEvents((prev) => {
              if (!prev) return [newEvent];
              const existsIdx = prev.findIndex(
                (e) => (newEvent.eventId && e.eventId === newEvent.eventId) || (newEvent.seq && e.seq === newEvent.seq),
              );
              if (existsIdx >= 0) {
                const next = [...prev];
                next[existsIdx] = newEvent;
                return next;
              }
              return [...prev, newEvent];
            });
            if (newEvent.type === "terminal") {
              onTaskUpdated?.(task);
            }
          },
          () => {
            // Live stream error handled gracefully
          },
        );
      } else {
        const timer = setInterval(() => {
          void fetchTranscript();
        }, 2000);
        return () => {
          active = false;
          clearInterval(timer);
        };
      }
    }

    return () => {
      active = false;
      unsubscribeStream?.();
    };
  }, [client, task.taskId, task.status, onTaskUpdated]);

  useEffect(() => {
    const hasApproval = (events ?? []).some((e) => e.type === "approval");
    onHasInlineApproval?.(hasApproval);
  }, [events, onHasInlineApproval]);

  const handleScroll = () => {
    const el = containerRef.current;
    if (!el) return;
    const { scrollTop, scrollHeight, clientHeight } = el;
    const isAtBottom = scrollHeight - scrollTop - clientHeight <= 40;
    setIsPinnedToBottom(isAtBottom);
    if (isAtBottom) {
      setHasNewMessages(false);
    }
  };

  useEffect(() => {
    const currentCount = events?.length ?? 0;
    const isNew = currentCount > prevEventCountRef.current;
    prevEventCountRef.current = currentCount;

    if (isPinnedToBottom) {
      if (typeof bottomRef.current?.scrollIntoView === "function") {
        bottomRef.current.scrollIntoView({ behavior: "smooth" });
      } else if (containerRef.current) {
        containerRef.current.scrollTop = containerRef.current.scrollHeight;
      }
    } else if (isNew) {
      setHasNewMessages(true);
    }
  }, [events, isPinnedToBottom]);

  const scrollToBottom = () => {
    if (typeof bottomRef.current?.scrollIntoView === "function") {
      bottomRef.current.scrollIntoView({ behavior: "smooth" });
    } else if (containerRef.current) {
      containerRef.current.scrollTop = containerRef.current.scrollHeight;
    }
    setIsPinnedToBottom(true);
    setHasNewMessages(false);
  };

  const isTerminal =
    task.status === "completed" ||
    task.status === "failed" ||
    task.status === "cancelled" ||
    task.status === "unknown";

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
        {isTerminal && client.resumeAgentTask ? (
          <FollowUpReplyBar
            taskId={task.taskId}
            client={client}
            onTaskResumed={onTaskResumed}
          />
        ) : null}
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
    <div className="space-y-3">
      <div className="relative">
        <div
          ref={containerRef}
          onScroll={handleScroll}
          className="flex max-h-[50vh] flex-col gap-4 overflow-y-auto pr-1"
        >
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
              const isLatest =
                idx === groups.length - 1 ||
                (idx === groups.length - 2 && groups[groups.length - 1]?.kind === "terminal");
              const isTaskActive = task.status === "running" || task.status === "queued";
              return (
                <details
                  key={item.events[0]?.eventId || idx}
                  open={isLatest && isTaskActive}
                  className="group rounded-xl border border-border/70 bg-muted/30 text-xs"
                >
                  <summary className="flex cursor-pointer items-center gap-2 px-3.5 py-2.5 font-medium text-muted-foreground hover:text-foreground">
                    <SparklesIcon className="size-3.5 text-amber-500" />
                    <span>{t("Thinking process")}</span>
                    {isLatest && isTaskActive ? (
                      <span className="size-1.5 animate-pulse rounded-full bg-amber-500" />
                    ) : null}
                    <ChevronDownIcon className="ml-auto size-3.5 transition-transform group-open:rotate-180" />
                  </summary>
                  <div className="border-t border-border/40 bg-muted/10 px-3.5 py-2.5 font-mono whitespace-pre-wrap break-words text-muted-foreground">
                    {item.text}
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
                <InlineApprovalCard
                  key={item.event.eventId || idx}
                  event={item.event}
                  approvals={approvals}
                  onDecided={(approvalId, choice) => {
                    setEvents((prev) => {
                      if (!prev) return prev;
                      return prev.map((ev) => {
                        if (ev.type === "approval" && (ev.payload?.approvalId === approvalId || ev.eventId === approvalId)) {
                          return {
                            ...ev,
                            payload: {
                              ...ev.payload,
                              status: choice === "allow_once" ? "allowed_once" : "rejected",
                              decision: choice,
                            },
                          };
                        }
                        return ev;
                      });
                    });
                    onTaskUpdated?.(task);
                  }}
                />
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
        <div ref={bottomRef} className="h-0 w-0" />
      </div>

      {!isPinnedToBottom && hasNewMessages ? (
        <button
          type="button"
          onClick={scrollToBottom}
          className="absolute bottom-4 left-1/2 -translate-x-1/2 z-10 inline-flex items-center gap-1.5 rounded-full bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground shadow-md transition-all hover:bg-primary/90 cursor-pointer animate-in fade-in slide-in-from-bottom-2"
          aria-label={t("New messages")}
        >
          <span>{t("New messages")}</span>
          <ArrowDownIcon className="size-3.5" />
        </button>
      ) : null}
      </div>

      {isTerminal && client.resumeAgentTask ? (
        <FollowUpReplyBar
          taskId={task.taskId}
          client={client}
          onTaskResumed={onTaskResumed}
        />
      ) : null}
    </div>
  );
}
