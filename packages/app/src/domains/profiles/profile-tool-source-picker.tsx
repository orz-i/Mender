import { useMemo } from "react";
import { ChevronLeftIcon, ChevronRightIcon } from "lucide-react";

import type { Profile, Provider, ToolSummary } from "@mender/sdk";
import { Button } from "@mender/ui/components/button";
import { Input } from "@mender/ui/components/input";

import { t, tCount } from "../../i18n/i18n";
import { usePagination } from "../../shared/pagination";
import { TablePagination } from "../../shared/table-pagination";
import { profileSourceLabels } from "./profile-source-label";

/**
 * Profile selection mirrors Catalog /tools -> /tools/:providerId.
 * The source is only navigation: persisted grants remain exact ToolIDs.
 */
export function ProfileToolSourcePicker({
  open,
  profile,
  providers,
  tools,
  hasMore,
  query,
  sourceQuery,
  sourceId,
  selected,
  selectedCounts,
  dirty,
  direct,
  busy,
  error,
  onSourceQueryChange,
  onOpenSource,
  onBackToSources,
  onQueryChange,
  onSelectedChange,
  onDirectChange,
  onApply,
  onLoadMore,
  onSave,
  onClose,
}: {
  open: boolean;
  profile: Profile | null;
  providers: Provider[];
  tools: ToolSummary[];
  hasMore: boolean;
  query: string;
  sourceQuery: string;
  sourceId: string;
  selected: Set<string>;
  selectedCounts: Map<string, number>;
  dirty: boolean;
  direct: Record<string, boolean>;
  busy: boolean;
  error: string;
  onSourceQueryChange: (value: string) => void;
  onOpenSource: (sourceId: string) => void;
  onBackToSources: () => void;
  onQueryChange: (value: string) => void;
  onSelectedChange: (value: Set<string>) => void;
  onDirectChange: (value: Record<string, boolean>) => void;
  onApply: () => void;
  onLoadMore: () => void;
  onSave: () => void;
  onClose: () => void;
}) {
  const selectedProvider = providers.find((provider) => provider.providerId === sourceId);
  const sourceLabels = useMemo(() => profileSourceLabels(providers), [providers]);
  const selectableSources = useMemo(() => {
    const needle = sourceQuery.trim().toLocaleLowerCase();
    return providers
      .filter((provider) => provider.toolSourceSelectable === true && provider.toolCount > 0)
      .filter((provider) => !needle ||
        (sourceLabels.get(provider.providerId) ?? "").toLocaleLowerCase().includes(needle))
      .sort((left, right) =>
        (sourceLabels.get(left.providerId) ?? "").localeCompare(sourceLabels.get(right.providerId) ?? "") ||
        left.providerId.localeCompare(right.providerId));
  }, [providers, sourceQuery, sourceLabels]);

  // Hooks remain unconditional while only one information level is rendered.
  const sourcesPagination = usePagination(selectableSources);
  const toolsPagination = usePagination(tools);
  const selectedHere = selectedCounts.get(sourceId) ?? 0;

  return (
    open ? <section data-slot="profile-tool-picker" aria-label={t("Choose tool sources")}
      className="flex min-h-0 max-h-[calc(100dvh-12rem)] min-w-0 flex-col overflow-hidden rounded-xl border border-border bg-card p-5">
      <div className="flex min-h-0 flex-1 flex-col gap-4">
        <div className="flex shrink-0 flex-col gap-1.5">
          {sourceId ? (
            <>
              <div className="flex min-w-0 items-center gap-2">
                <Button
                  variant="ghost" size="sm" className="gap-1.5 px-2"
                  aria-label={t("Back to sources")}
                  disabled={busy}
                  onClick={() => { onBackToSources(); sourcesPagination.reset(); }}
                >
                  <ChevronLeftIcon className="size-4 shrink-0" />
                  {t("Sources")}
                </Button>
                <ChevronRightIcon className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
                <h2 className="min-w-0 truncate text-base font-semibold">
                  {selectedProvider ? sourceLabels.get(selectedProvider.providerId) : t("Service unavailable")}
                </h2>
              </div>
              <p className="text-xs text-muted-foreground">
                {t("Select the individual tools this Profile may expose. New tools are never granted automatically.")}
              </p>
            </>
          ) : (
            <h2 className="text-base font-semibold">{t("Choose tool sources")}</h2>
          )}
        </div>
        {error ? (
          <div role="alert" className="shrink-0 rounded-lg border border-destructive/20 bg-destructive/10 px-3 py-2 text-sm text-destructive">
            {error}
          </div>
        ) : null}

        {sourceId ? (
          <div className="flex min-h-0 flex-1 flex-col gap-3">
            <div className="flex flex-wrap items-center gap-2">
              <Input
                aria-label={t("Profile tool search")}
                placeholder={t("Search tools")}
                className="min-w-0 flex-1"
                value={query}
                onChange={(event) => { onQueryChange(event.target.value); toolsPagination.reset(); }}
                onKeyDown={(event) => { if (event.key === "Enter") onApply(); }}
              />
              <Button variant="outline" disabled={busy} onClick={onApply}>
                {t(busy ? "Loading…" : "Apply")}
              </Button>
              <Button variant="outline" disabled={busy || tools.length === 0 || tools.every((tool) => selected.has(tool.toolId))} onClick={() => {
                const next = new Set(selected);
                for (const tool of tools) next.add(tool.toolId);
                onSelectedChange(next);
              }}>
                {t("Select loaded tools")}
              </Button>
            </div>
            <div className="flex justify-between gap-3 text-xs text-muted-foreground">
              <span>{tCount(tools.length, "{{count}} loaded Tool", "{{count}} loaded Tools")}</span>
              <span>{tCount(selectedHere, "{{count}} selected", "{{count}} selected")}</span>
            </div>
            <div data-slot="profile-tool-picker-list" className="min-h-0 flex-1 divide-y divide-border overflow-y-auto overscroll-contain rounded-lg border border-border">
              {toolsPagination.items.length ? toolsPagination.items.map((tool) => {
                const checked = selected.has(tool.toolId);
                return (
                  <div key={tool.toolId} className="flex items-start gap-3 px-4 py-3">
                    <input
                      type="checkbox"
                      aria-label={t("Select {{tool}}", { tool: tool.title || tool.name })}
                      className="mt-1"
                      disabled={busy}
                      checked={checked}
                      onChange={(event) => {
                        const next = new Set(selected);
                        if (event.target.checked) next.add(tool.toolId);
                        else next.delete(tool.toolId);
                        onSelectedChange(next);
                      }}
                    />
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm font-medium">{tool.title || tool.name}</p>
                      {tool.description ? (
                        <p className="mt-1 line-clamp-2 text-xs leading-5 text-muted-foreground">{tool.description}</p>
                      ) : null}
                    </div>
                    {profile?.mode === "hybrid" && checked ? (
                      <label className="flex shrink-0 items-center gap-2 text-xs text-muted-foreground">
                        <input
                          type="checkbox"
                          checked={direct[tool.toolId] ?? false}
                          onChange={(event) => onDirectChange({ ...direct, [tool.toolId]: event.target.checked })}
                        />
                        {t("Direct export")}
                      </label>
                    ) : null}
                  </div>
                );
              }) : error ? null : (
                <div className="p-8 text-center text-sm text-muted-foreground">
                  {t(busy ? "Loading…" : "No matching Tools.")}
                </div>
              )}
            </div>
            <div className="shrink-0">
              <TablePagination label="Catalog Tools" pagination={toolsPagination} />
              {hasMore ? (
                <div className="flex justify-center py-2">
                  <Button type="button" size="sm" variant="outline" disabled={busy} onClick={onLoadMore}>
                    {t("Load more tools")}
                  </Button>
                </div>
              ) : null}
            </div>
          </div>
        ) : (
          <div className="flex min-h-0 flex-1 flex-col gap-3">
            <div className="flex flex-wrap items-center gap-3">
              <Input
                aria-label={t("Search sources")}
                placeholder={t("Search sources")}
                className="min-w-0 flex-1"
                value={sourceQuery}
                onChange={(event) => { onSourceQueryChange(event.target.value); sourcesPagination.reset(); }}
              />
              <span className="shrink-0 text-xs text-muted-foreground">
                {tCount(selectableSources.length, "{{count}} source", "{{count}} sources")}
              </span>
            </div>
            <div data-slot="profile-source-picker-list" className="min-h-0 flex-1 divide-y divide-border overflow-y-auto overscroll-contain rounded-lg border border-border">
              {sourcesPagination.items.length ? sourcesPagination.items.map((provider) => (
                <button
                  key={provider.providerId}
                  type="button"
                  disabled={busy}
                  aria-label={t("Open source {{source}}", { source: sourceLabels.get(provider.providerId) ?? "" })}
                  className="flex w-full items-center gap-3 px-4 py-4 text-left hover:bg-muted/40 focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-ring disabled:opacity-50"
                  onClick={() => { onOpenSource(provider.providerId); toolsPagination.reset(); }}
                >
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-sm font-medium">{sourceLabels.get(provider.providerId)}</span>
                    <span className="mt-1 block text-xs text-muted-foreground">
                      {tCount(provider.toolCount, "{{count}} tool", "{{count}} tools")}
                    </span>
                  </span>
                  <span className="text-xs text-muted-foreground">
                    {tCount(selectedCounts.get(provider.providerId) ?? 0, "{{count}} selected", "{{count}} selected")}
                  </span>
                  <ChevronRightIcon className="size-4 shrink-0 text-muted-foreground" />
                </button>
              )) : (
                <div className="p-8 text-center text-sm text-muted-foreground">{t("No available sources")}</div>
              )}
            </div>
            <TablePagination label="Sources" pagination={sourcesPagination} />
          </div>
        )}

        <div className="flex shrink-0 flex-wrap items-center justify-between gap-3 border-t border-border pt-4">
          <p aria-live="polite" className="text-xs text-muted-foreground">
            {tCount(selected.size, "{{count}} tools selected", "{{count}} tools selected")}
            {dirty ? <span className="ml-2 font-medium text-foreground">· {t("Unsaved changes")}</span> : null}
          </p>
          <div className="flex flex-wrap gap-2">
            <Button variant="outline" onClick={onClose}>{t("Cancel")}</Button>
            <Button disabled={busy || !dirty} onClick={onSave}>
              {t(busy ? "Saving…" : "Save selection")}
            </Button>
          </div>
        </div>
      </div>
    </section> : null
  );
}
