import { useMemo } from "react";
import type { AuditRecord } from "@mender/sdk";
import { formatNumber, t } from "../../i18n/i18n";

type DayUsage = { key: string; count: number; dateLabel: string };

/**
 * Audit API currently provides a bounded latest-record window, not full usage
 * aggregation. Display only the observed sample and label its limits.
 */
export function AuditUsageSummary({
  records,
  onReviewFailures,
}: {
  records: ReadonlyArray<AuditRecord>;
  onReviewFailures?: () => void;
}) {
  const usage = useMemo(() => {
    const dated = records.flatMap((record) => {
      const date = new Date(record.finishedAt);
      return Number.isFinite(date.getTime()) ? [{ record, date }] : [];
    });
    const latest = dated.reduce<Date | null>((current, item) =>
      current === null || item.date.getTime() > current.getTime() ? item.date : current, null);
    const end = latest === null
      ? null : Date.UTC(latest.getUTCFullYear(), latest.getUTCMonth(), latest.getUTCDate());
    const days: DayUsage[] = [];
    const counts = new Map<string, number>();
    for (const entry of dated) {
      const key = entry.date.toISOString().slice(0, 10);
      counts.set(key, (counts.get(key) ?? 0) + 1);
    }
    if (end !== null) {
      for (let i = 13; i >= 0; i -= 1) {
        const date = new Date(end - i * 86_400_000);
        const key = date.toISOString().slice(0, 10);
        const dateLabel = new Intl.DateTimeFormat(undefined, { month: "short", day: "numeric", timeZone: "UTC" }).format(date);
        days.push({ key, dateLabel, count: counts.get(key) ?? 0 });
      }
    }
    const max = Math.max(1, ...days.map((day) => day.count));
    return {
      days, max, count: records.length,
      completed: records.filter((record) => record.status === "completed").length,
      other: records.filter((record) => record.status === "failed" || record.status === "denied").length,
      tools: new Set(records.map((record) => record.toolId)).size,
    };
  }, [records]);

  if (!usage.count) return null;

  return (
    <section data-slot="audit-usage" aria-label={t("Recent invocation sample")} className="mb-5">
      <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
        <h2 className="text-[12px] font-semibold tracking-[0.04em] text-muted-foreground uppercase">{t("Recent invocation sample")}</h2>
        <div className="flex flex-wrap items-center gap-3">
          <p className="text-[11px] text-muted-foreground">
            {t("Based on up to 100 latest records, not complete historical totals.")}
          </p>
          {usage.other > 0 && onReviewFailures ? (
            <button type="button" onClick={onReviewFailures}
              className="rounded-md text-[12px] font-medium text-primary-ink hover:underline focus-visible:outline-2 focus-visible:outline-ring">
              {t("Review failures")}
            </button>
          ) : null}
        </div>
      </div>
      <div className="grid overflow-hidden rounded-xl border border-border bg-card sm:grid-cols-4">
        {([
          { label: "Observed calls", value: usage.count },
          { label: "Completed", value: usage.completed },
          { label: "Failed or denied", value: usage.other },
          { label: "Distinct tools", value: usage.tools },
        ] as const).map(({ label, value }, i) => (
          <div key={label} className={[
            "min-w-0 px-5 py-4",
            i > 0 ? "border-t border-border sm:border-t-0 sm:border-l" : "",
          ].join(" ")}>
            <p className="truncate text-[12px] text-muted-foreground">{t(label)}</p>
            <p className="mt-1.5 text-[24px] leading-7 font-semibold tracking-[-0.03em] tabular-nums">{formatNumber(value)}</p>
          </div>
        ))}
        <div className="col-span-full border-t border-border px-5 pt-5 pb-4">
          <div role="img" aria-label={t("Recorded invocations by day in the latest sample")}
            className="flex h-[144px] items-end gap-[5px] sm:gap-[9px]">
            {usage.days.map((day) => (
              <div key={day.key} className="group relative flex h-full min-w-0 flex-1 flex-col justify-end">
                <div className="relative flex min-h-0 items-end" style={{ height: "112px" }}>
                  {day.count > 0 ? (
                    <div title={day.dateLabel + ": " + day.count}
                      className="w-full rounded-t-[3px] border-t-[3px] border-primary bg-primary/30 transition-colors group-hover:bg-primary/45"
                      style={{ height: Math.max(6, 100 * day.count / usage.max) + "%" }} />
                  ) : (
                    <div className="h-px w-full bg-border/80" />
                  )}
                </div>
                <p aria-hidden="true" className="h-7 truncate pt-2 text-center text-[10px] text-muted-foreground">
                  {day.dateLabel}
                </p>
              </div>
            ))}
          </div>
        </div>
      </div>
    </section>
  );
}
