import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  CheckIcon,
  CopyIcon,
  RefreshCwIcon,
  SquareIcon,
} from "lucide-react";

import type { MCPServerInstance, MenderClient, Profile } from "@mender/sdk";
import { Badge } from "@mender/ui/components/badge";
import { Button } from "@mender/ui/components/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogBody,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@mender/ui/components/dialog";
import { Input } from "@mender/ui/components/input";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@mender/ui/components/table";

import { errorMessage } from "../../shared/errors";
import { IconButton } from "../../shared/icon-button";
import { EmptyState, ErrorNotice } from "../../shared/page";
import { PageToolbar, ResultPanel } from "../../shared/page-toolbar";
import { usePagination } from "../../shared/pagination";
import { TablePagination } from "../../shared/table-pagination";
import { EmptyPanelState, SearchField } from "../../shared/workbench";

import { formatDateTime, t, tCount } from "../../i18n/i18n";

const defaultAddress = "127.0.0.1:8765";
const serverStatusPollIntervalMs = 2000;

function tunnelStatusLabel(status: NonNullable<MCPServerInstance["tunnelStatus"]>): string {
  switch (status) {
    case "starting": return t("Starting");
    case "ready": return t("Ready");
    case "degraded": return t("Degraded");
    case "failed": return t("Failed");
    case "exited": return t("Exited");
    case "running": return t("Running");
  }
}
function tunnelHint(server: MCPServerInstance): string {
  const provider = server.tunnelProvider === "external"
    ? t("External plugin")
    : server.tunnelProvider === "openai" ? t("OpenAI") : "";
  return [
    server.tunnelDisplayName || t("Tunnel"),
    provider,
    server.tunnelStatus ? tunnelStatusLabel(server.tunnelStatus) : "",
  ].filter(Boolean).join(" · ");
}

export function ServersPage({
  client,
  copyText = browserCopyText,
}: {
  client: MenderClient;
  copyText?: (value: string) => Promise<void>;
}) {
  const [profiles, setProfiles] = useState<Profile[]>([]);
  const [servers, setServers] = useState<MCPServerInstance[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [hasSnapshot, setHasSnapshot] = useState(false);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState("");
  const [inventoryQuery, setInventoryQuery] = useState("");
  const [copied, setCopied] = useState("");
  // Fences a background inventory read that began before a management
  // operation. An old response must never resurrect a stopped server.
  const serverOperationEpoch = useRef(0);

  const [startOpen, setStartOpen] = useState(false);
  const [startProfile, setStartProfile] = useState("");
  const [startAddress, setStartAddress] = useState(defaultAddress);

  const refresh = useCallback(async () => {
    serverOperationEpoch.current += 1;
    setBusy("refresh");
    try {
      const [profileResult, serverResult] = await Promise.all([
        client.listProfiles(),
        client.listMCPServers(),
      ]);
      setProfiles(profileResult.profiles);
      setServers(serverResult.servers);
      setHasSnapshot(true);
      setError("");
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setLoaded(true);
      setBusy("");
    }
  }, [client]);

  useEffect(() => {
    let active = true;

    void Promise.all([client.listProfiles(), client.listMCPServers()])
      .then(([profileResult, serverResult]) => {
        if (active) {
          setProfiles(profileResult.profiles);
          setServers(serverResult.servers);
          setHasSnapshot(true);
          setError("");
          setLoaded(true);
        }
      })
      .catch((reason: unknown) => {
        if (active) {
          setError(errorMessage(reason));
          setLoaded(true);
        }
      });

    return () => {
      active = false;
    };
  }, [client]);

  // The native Tunnel's ready/degraded phases update asynchronously after
  // the initial Start response. Poll only this read-only Mender inventory
  // while a tunneled server is visible; no Tunnel credentials or endpoints
  // are queried by the frontend.
  const hasManagedTunnel = servers.some((server) => Boolean(server.tunnelId));
  useEffect(() => {
    if (!loaded || !hasManagedTunnel || busy !== "") return;

    let active = true;
    let requestInFlight = false;
    const timer = window.setInterval(() => {
      if (requestInFlight) return;
      requestInFlight = true;
      const requestEpoch = serverOperationEpoch.current;
      void client.listMCPServers().then(({ servers: current }) => {
        if (!active || requestEpoch !== serverOperationEpoch.current) return;
        setServers(current);
      }).catch(() => {
        // A transient management read failure is not evidence of native
        // Tunnel failure. Preserve the last observed state and manual Refresh.
      }).finally(() => { requestInFlight = false; });
    }, serverStatusPollIntervalMs);

    return () => {
      active = false;
      window.clearInterval(timer);
    };
  }, [client, loaded, hasManagedTunnel, busy]);

  function openStartDialog() {
    setError("");
    setStartProfile(profiles[0]?.slug ?? "");
    setStartAddress(defaultAddress);
    setStartOpen(true);
  }

  async function startServer() {
    serverOperationEpoch.current += 1;
    setBusy("start");
    setError("");
    try {
      const instance = await client.startMCPServer({
        profile: startProfile,
        address: startAddress,
      });
      setServers((current) => [...current, instance]);
      setStartOpen(false);
      setCopied("");
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy("");
    }
  }

  const visibleServers = useMemo(() => {
    const needle = inventoryQuery.trim().toLocaleLowerCase();
    return needle ? servers.filter((server) =>
      [server.profile, server.endpoint, server.status, server.tunnelDisplayName ?? ""]
        .some((value) => value.toLocaleLowerCase().includes(needle))
    ) : servers;
  }, [servers, inventoryQuery]);
  const serverPagination = usePagination(visibleServers);

  async function restartServer(server: MCPServerInstance) {
    serverOperationEpoch.current += 1;
    setBusy(server.serverId);
    setError("");
    try {
      const restarted = await client.restartMCPServer(server.serverId);
      setServers((current) =>
        current.map((item) =>
          item.serverId === server.serverId ? restarted : item,
        ),
      );
      setCopied("");
    } catch (reason) {
      setError(errorMessage(reason));
      await refresh();
    } finally {
      setBusy("");
    }
  }

  async function stopServer(server: MCPServerInstance) {
    serverOperationEpoch.current += 1;
    setBusy(server.serverId);
    setError("");
    try {
      await client.stopMCPServer(server.serverId);
      setServers((current) =>
        current.filter((item) => item.serverId !== server.serverId),
      );
      setCopied("");
    } catch (reason) {
      setError(errorMessage(reason));
      await refresh();
    } finally {
      setBusy("");
    }
  }

  const selectedProfile =
    profiles.find((profile) => profile.slug === startProfile) ?? null;

  async function copy(value: string, key: string) {
    try {
      await copyText(value);
      setCopied(key);
      setError("");
    } catch (reason) {
      setError(errorMessage(reason));
    }
  }

  if (error && !hasSnapshot) {
    return <div className="pt-4"><ErrorNotice message={error} onRetry={() => {setError("");void refresh();}} /></div>;
  }

  return (
    <>
      <PageToolbar
        filters={
          <div className="w-full min-w-0 sm:w-56 sm:shrink-0">
            <SearchField label="Search servers" placeholder="Search servers…" value={inventoryQuery} onChange={setInventoryQuery} />
          </div>
        }
        actions={
          <>
            <Button size="sm" variant="outline" disabled={busy !== ""} onClick={() => void refresh()}>{t("Refresh")}</Button>
            <Button size="sm" disabled={profiles.length === 0 || busy !== ""} onClick={openStartDialog}>{t("Start server")}</Button>
          </>
        }
      />
      {error ? <ErrorNotice message={error} /> : null}
      <ResultPanel title="Running MCP servers">
          {!loaded ? (
            <div className="px-4 py-12 text-center text-sm text-muted-foreground">
              {t("Loading servers…")}
            </div>
          ) : visibleServers.length ? (
            <>
              <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("Profile")}</TableHead>
                  <TableHead>{t("Status")}</TableHead>
                  <TableHead>{t("Tunnel")}</TableHead>
                  <TableHead>{t("Endpoint")}</TableHead>
                  <TableHead>{t("Started")}</TableHead>
                  <TableHead className="text-right">{t("Actions")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {serverPagination.items.map((server) => (
                  <TableRow key={server.serverId}>
                    <TableCell className="font-medium">
                      {server.profile}
                    </TableCell>
                    <TableCell>
                      <Badge variant="information">{t("Server running")}</Badge>
                    </TableCell>
                    <TableCell>
                      {server.tunnelId ? (
                        <div className="flex min-w-0 max-w-[220px] items-center gap-2">
                          <span className="min-w-0 truncate text-sm font-medium" title={tunnelHint(server)}>
                            {server.tunnelDisplayName || t("Tunnel")}
                          </span>
                          {server.tunnelStatus === "degraded" ||
                          server.tunnelStatus === "failed" ||
                          server.tunnelStatus === "exited" ? (
                            <Badge
                              variant={server.tunnelStatus === "failed" ? "destructive" : "caution"}
                              className="shrink-0"
                              aria-live="polite"
                            >
                              {tunnelStatusLabel(server.tunnelStatus)}
                            </Badge>
                          ) : null}
                        </div>
                      ) : (
                        <span className="text-sm text-muted-foreground">
                          {t("Local only")}
                        </span>
                      )}
                    </TableCell>
                    <TableCell className="max-w-[360px] truncate font-mono text-xs">
                      {server.endpoint}
                    </TableCell>
                    <TableCell className="text-sm text-muted-foreground">
                      {formatTimestamp(server.startedAt)}
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="flex justify-end gap-1">
                        <IconButton
                          variant="ghost"
                          label={t(
                            copied === server.serverId + ":endpoint"
                              ? "Copied"
                              : "Copy endpoint",
                          )}
                          onClick={() =>
                            void copy(
                              server.endpoint,
                              server.serverId + ":endpoint",
                            )
                          }
                        >
                          {copied === server.serverId + ":endpoint" ? (
                            <CheckIcon />
                          ) : (
                            <CopyIcon />
                          )}
                        </IconButton>
                        <IconButton
                          variant="ghost"
                          label={t(
                            copied === server.serverId + ":config"
                              ? "Copied"
                              : "Copy config",
                          )}
                          onClick={() =>
                            void copy(
                              clientConfig(server),
                              server.serverId + ":config",
                            )
                          }
                        >
                          {copied === server.serverId + ":config" ? (
                            <CheckIcon />
                          ) : (
                            <CopyIcon />
                          )}
                        </IconButton>
                        <IconButton
                          variant="outline"
                          label={t(
                            busy === server.serverId ? "Working…" : "Restart",
                          )}
                          disabled={busy !== ""}
                          onClick={() => void restartServer(server)}
                        >
                          <RefreshCwIcon />
                        </IconButton>
                        <IconButton
                          variant="destructive"
                          label={t("Stop")}
                          disabled={busy !== ""}
                          onClick={() => void stopServer(server)}
                        >
                          <SquareIcon />
                        </IconButton>
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
              </Table>
            </>
          ) : (
            <EmptyPanelState>
              <EmptyState
                title="No running MCP servers"
                description={
                  profiles.length
                    ? t("Start a Profile as a loopback Streamable HTTP server.")
                    : t("Create a Profile before starting an MCP server.")
                }
              />
            </EmptyPanelState>
          )}
      </ResultPanel>

      <TablePagination
        label="Servers"
        pagination={serverPagination}
      />

      <Dialog
        open={startOpen}
        onOpenChange={(open) => {
          setStartOpen(open);
          if (!open) {
            setStartProfile("");
            setStartAddress(defaultAddress);
          }
        }}
      >
        <DialogContent className="grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{t("Start MCP server")}</DialogTitle>
            <DialogDescription>
              {t("Expose one Profile over local Streamable HTTP. Remote/wildcard bind addresses are intentionally rejected.")}
            </DialogDescription>
          </DialogHeader>

          <DialogBody className="space-y-4">
            <Field label="Profile">
              <select
                aria-label={t("Server profile")}
                value={startProfile}
                onChange={(event) => setStartProfile(event.target.value)}
                className="h-8 w-full rounded-lg border border-input bg-background px-3 text-sm outline-none focus:ring-2 focus:ring-ring/40"
              >
                {profiles.map((profile) => (
                  <option key={profile.profileId} value={profile.slug}>
                    {profile.displayName} ({profile.slug}) ·{" "}
                    {tCount(profile.tools.length, "{{count}} tool", "{{count}} tools")}
                  </option>
                ))}
              </select>
            </Field>

            {selectedProfile !== null && selectedProfile.tools.length === 0 ? (
              <div className="rounded-lg border border-amber-500/30 bg-amber-500/10 p-3 text-xs leading-5">
                {t("This Profile currently contains 0 Tools. The server can start, but downstream discovery will not return Provider Tools until you add Tool membership on the Profiles page and Restart it.")}
              </div>
            ) : null}

            {selectedProfile?.tunnelId ? (
              <div className="rounded-lg border border-border bg-muted/40 p-3 text-xs leading-5 text-muted-foreground">
                {t(
                  "Starting this Profile also starts its configured OpenAI Secure MCP Tunnel. The MCP server itself remains loopback-only.",
                )}
              </div>
            ) : null}

            <Field label="Listen address">
              <Input
                aria-label={t("Listen address")}
                className="font-mono"
                value={startAddress}
                onChange={(event) => setStartAddress(event.target.value)}
              />
              <p className="mt-1 text-xs leading-5 text-muted-foreground">
                {t("Use a loopback host such as 127.0.0.1 or localhost. Port 0 asks the OS to choose a free port.")}
              </p>
            </Field>

            <div className="rounded-lg border border-border bg-muted/40 p-3 text-xs leading-5 text-muted-foreground">
              {t("Desktop-managed servers use Streamable HTTP. stdio remains client-launched through")}{" "}
              <code>mender serve mcp --profile … --stdio</code>.
            </div>
          </DialogBody>

          <DialogFooter>
            <DialogClose render={<Button variant="outline" />}>
              {t("Cancel")}
            </DialogClose>
            <Button
              disabled={
                busy === "start" ||
                !startProfile.trim() ||
                !startAddress.trim()
              }
              onClick={() => void startServer()}
            >
              {t(busy === "start" ? "Starting…" : "Start server")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

async function browserCopyText(value: string) {
  if (navigator.clipboard === undefined) {
    throw new Error(t("Clipboard API is unavailable in this host."));
  }
  await navigator.clipboard.writeText(value);
}

function Field({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <label className="block space-y-1.5">
      <span className="text-sm font-medium">{t(label)}</span>
      {children}
    </label>
  );
}

function clientConfig(server: MCPServerInstance) {
  return JSON.stringify(
    {
      mcpServers: {
        ["mender-" + server.profile]: {
          type: "streamable-http",
          url: server.endpoint,
        },
      },
    },
    null,
    2,
  );
}

function formatTimestamp(value: string) {
  return formatDateTime(value);
}
