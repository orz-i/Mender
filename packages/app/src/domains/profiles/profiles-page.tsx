import { useCallback, useEffect, useMemo, useState } from "react";
import { ChevronRightIcon, Layers3Icon } from "lucide-react";
import { Link, useNavigate } from "react-router";

import type { ManagementClient, Profile } from "@mender/sdk";
import { Button } from "@mender/ui/components/button";

import { t, tCount } from "../../i18n/i18n";
import { errorMessage } from "../../shared/errors";
import { EmptyState, ErrorNotice } from "../../shared/page";
import { PageToolbar, ResultPanel } from "../../shared/page-toolbar";
import { usePagination } from "../../shared/pagination";
import { TablePagination } from "../../shared/table-pagination";
import { EmptyPanelState, SearchField } from "../../shared/workbench";

export function ProfilesPage({ client }: { client: ManagementClient }) {
  const navigate = useNavigate();
  const [profiles, setProfiles] = useState<Profile[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [hasSnapshot, setHasSnapshot] = useState(false);
  const [error, setError] = useState("");
  const [retryVersion, setRetryVersion] = useState(0);
  const [query, setQuery] = useState("");

  const refresh = useCallback(() => {
    let active = true;
    void client.listProfiles()
      .then((profileResult) => {
        if (!active) return;
        setProfiles(profileResult.profiles);
        setHasSnapshot(true);
        setError("");
      })
      .catch((reason: unknown) => {
        if (active) setError(errorMessage(reason));
      })
      .finally(() => {
        if (active) setLoaded(true);
      });
    return () => { active = false; };
  }, [client]);

  useEffect(() => refresh(), [refresh, retryVersion]);

  const filteredProfiles = useMemo(() => {
    const needle = query.trim().toLocaleLowerCase();
    return needle
      ? profiles.filter((profile) =>
          [profile.displayName, profile.slug].some((value) => value.toLocaleLowerCase().includes(needle)))
      : profiles;
  }, [profiles, query]);
  const pagination = usePagination(filteredProfiles);

  return (
    <>
      <PageToolbar
        tabs={<span className="text-xs text-muted-foreground">
          {hasSnapshot
            ? tCount(profiles.length, "{{count}} profile configured", "{{count}} profiles configured")
            : t("Loading profiles…")}
        </span>}
        filters={<div className="w-full min-w-0 sm:w-56 sm:shrink-0">
          <SearchField label="Search profiles" placeholder="Search profiles…" value={query}
            onChange={(next) => { setQuery(next); pagination.reset(); }} />
        </div>}
        actions={<>
          <Button variant="outline" size="sm" onClick={() => setRetryVersion((v) => v + 1)}>{t("Refresh")}</Button>
          <Button size="sm" onClick={() => void navigate("/profiles/new")}>{t("Create profile")}</Button>
        </>}
      />
      {error ? <ErrorNotice message={error} onRetry={() => setRetryVersion((v) => v + 1)} /> : null}
      {!hasSnapshot && error ? null : <ResultPanel title="Profile inventory">
        {!loaded && !hasSnapshot ? (
          <div className="px-4 py-12 text-center text-sm text-muted-foreground">{t("Loading profiles…")}</div>
        ) : filteredProfiles.length ? (
          <div role="list" aria-label={t("Profile inventory")} data-slot="profile-collection"
            className="divide-y divide-border/80">
            {pagination.items.map((profile) => (
              <div role="listitem" key={profile.profileId}>
                <Link to={"/profiles/" + encodeURIComponent(profile.slug)}
                  aria-label={profile.displayName || profile.slug}
                  className="group flex min-h-[76px] min-w-0 items-center gap-3 px-4 py-3 transition-colors hover:bg-muted/40 focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-ring sm:gap-4 sm:px-5">
                  <span className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-secondary text-muted-foreground">
                    <Layers3Icon className="size-5" aria-hidden="true" />
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-[14px] font-semibold text-foreground group-hover:text-primary-ink">{profile.displayName || profile.slug}</span>
                    <span className="mt-1 block truncate text-[12px] text-muted-foreground">{profile.slug}</span>
                  </span>
                  <span className="shrink-0 text-[12px] font-medium text-muted-foreground">
                    {tCount(profile.tools.length, "{{count}} selected Tool", "{{count}} selected Tools")}
                  </span>
                  <ChevronRightIcon className="size-4 shrink-0 text-muted-foreground/60" aria-hidden="true" />
                </Link>
              </div>
            ))}
          </div>
        ) : <EmptyPanelState><EmptyState
          title={profiles.length ? t("No matching profiles") : t("No profiles")}
          description={profiles.length ? t("Try a different search or filter.") : t("Create a Profile before exposing Tools through MCP.")}
        /></EmptyPanelState>}
      </ResultPanel>}
      {hasSnapshot ? <TablePagination label="Profiles" pagination={pagination} /> : null}
    </>
  );
}
