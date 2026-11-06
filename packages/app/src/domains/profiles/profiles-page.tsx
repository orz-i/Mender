import { useCallback, useEffect, useMemo, useState } from "react";
import { Layers3Icon } from "lucide-react";
import { useNavigate } from "react-router";

import type { ManagementClient, Profile } from "@mender/sdk";
import { Button } from "@mender/ui/components/button";

import { t, tCount } from "../../i18n/i18n";
import { errorMessage } from "../../shared/errors";
import { EmptyState, ErrorNotice } from "../../shared/page";
import { PageToolbar, ResultPanel } from "../../shared/page-toolbar";
import { usePagination } from "../../shared/pagination";
import { TablePagination } from "../../shared/table-pagination";
import { ResourceList, ResourceRow } from "../../shared/resource-list";
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
          <ResourceList label="Profile inventory" slot="profile-collection">
            {pagination.items.map((profile) => (
              <ResourceRow key={profile.profileId}
                to={"/profiles/" + encodeURIComponent(profile.slug)}
                label={profile.displayName || profile.slug}
                icon={Layers3Icon}
                title={profile.displayName || profile.slug}
                description={profile.slug}
                detail={tCount(profile.tools.length, "{{count}} selected Tool", "{{count}} selected Tools")}
              />
            ))}
          </ResourceList>
        ) : <EmptyPanelState><EmptyState
          title={profiles.length ? t("No matching profiles") : t("No profiles")}
          description={profiles.length ? t("Try a different search or filter.") : t("Create a Profile before exposing Tools through MCP.")}
        /></EmptyPanelState>}
      </ResultPanel>}
      {hasSnapshot ? <TablePagination label="Profiles" pagination={pagination} /> : null}
    </>
  );
}
