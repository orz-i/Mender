import { useCallback, useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router";

import type { MenderClient, Profile, Tunnel } from "@mender/sdk";
import { Badge } from "@mender/ui/components/badge";
import { Button } from "@mender/ui/components/button";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@mender/ui/components/table";

import { formatNumber, t, tCount } from "../../i18n/i18n";
import { errorMessage } from "../../shared/errors";
import { EmptyState, ErrorNotice } from "../../shared/page";
import { PageToolbar, ResultPanel } from "../../shared/page-toolbar";
import { usePagination } from "../../shared/pagination";
import { TablePagination } from "../../shared/table-pagination";
import { EmptyPanelState, SearchField } from "../../shared/workbench";

export function ProfilesPage({ client }: { client: MenderClient }) {
  const navigate = useNavigate();
  const [profiles, setProfiles] = useState<Profile[]>([]);
  const [tunnels, setTunnels] = useState<Tunnel[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [hasSnapshot, setHasSnapshot] = useState(false);
  const [error, setError] = useState("");
  const [retryVersion, setRetryVersion] = useState(0);
  const [query, setQuery] = useState("");

  const refresh = useCallback(() => {
    let active = true;
    void Promise.all([client.listProfiles(), client.listTunnels()])
      .then(([profileResult, tunnelResult]) => {
        if (!active) return;
        setProfiles(profileResult.profiles);
        setTunnels(tunnelResult.tunnels);
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
          [profile.displayName, profile.slug, profile.mode].some((value) => value.toLocaleLowerCase().includes(needle)))
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
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("Profile")}</TableHead>
                <TableHead>{t("Mode")}</TableHead>
                <TableHead>{t("Tools")}</TableHead>
                <TableHead>{t("Tunnel")}</TableHead>
                <TableHead className="text-right">{t("Actions")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {pagination.items.map((profile) => (
                <TableRow key={profile.profileId}>
                  <TableCell>
                    <button type="button" className="text-left font-medium hover:underline focus-visible:rounded focus-visible:outline-2 focus-visible:outline-ring"
                      onClick={() => void navigate("/profiles/" + encodeURIComponent(profile.slug))}>
                      {profile.displayName || profile.slug}
                    </button>
                    <div className="mt-1 text-xs text-muted-foreground">{profile.slug}</div>
                  </TableCell>
                  <TableCell><Badge variant="outline">{t(profile.mode)}</Badge></TableCell>
                  <TableCell>{formatNumber(profile.tools.length)}</TableCell>
                  <TableCell>{profile.tunnelId
                    ? (tunnels.find((tunnel) => tunnel.tunnelId === profile.tunnelId)?.displayName ?? t("Configured"))
                    : <span className="text-muted-foreground">{t("Local only")}</span>}</TableCell>
                  <TableCell className="text-right">
                    <Button size="sm" variant="outline" onClick={() => void navigate("/profiles/" + encodeURIComponent(profile.slug))}>
                      {t("View details")}
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        ) : <EmptyPanelState><EmptyState
          title={profiles.length ? t("No matching profiles") : t("No profiles")}
          description={profiles.length ? t("Try a different search or filter.") : t("Create a Profile before exposing Tools through MCP.")}
        /></EmptyPanelState>}
      </ResultPanel>}
      {hasSnapshot ? <TablePagination label="Profiles" pagination={pagination} /> : null}
    </>
  );
}
