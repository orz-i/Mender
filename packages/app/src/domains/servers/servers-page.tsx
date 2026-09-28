import { useCallback, useEffect, useState } from "react";
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
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@mender/ui/components/card";
import {
  Dialog,
  DialogClose,
  DialogContent,
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

import { IconButton } from "../../shared/icon-button";
import { EmptyState, ErrorNotice, PageHeader } from "../../shared/page";
import { usePagination } from "../../shared/pagination";
import { TablePagination } from "../../shared/table-pagination";

import { t } from "../../i18n/i18n";

const defaultAddress = "127.0.0.1:8765";

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
  const [error, setError] = useState("");
  const [busy, setBusy] = useState("");
  const [copied, setCopied] = useState("");

  const [startOpen, setStartOpen] = useState(false);
  const [startProfile, setStartProfile] = useState("");
  const [startAddress, setStartAddress] = useState(defaultAddress);

  const refresh = useCallback(async () => {
    setBusy("refresh");
    try {
      const [profileResult, serverResult] = await Promise.all([
        client.listProfiles(),
        client.listMCPServers(),
      ]);
      setProfiles(profileResult.profiles);
      setServers(serverResult.servers);
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

  function openStartDialog() {
    setError("");
    setStartProfile(profiles[0]?.slug ?? "");
    setStartAddress(defaultAddress);
    setStartOpen(true);
  }

  async function startServer() {
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

  const serverPagination = usePagination(servers);

  async function restartServer(server: MCPServerInstance) {
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

  return (
    <>
      <PageHeader
        title="Servers"
        description="Expose a Profile as a local Streamable HTTP MCP server."
        actions={
          <div className="flex gap-2">
            <Button
              variant="outline"
              disabled={busy !== ""}
              onClick={() => void refresh()}
            >
              {t("Refresh")}
            </Button>
            <Button
              disabled={profiles.length === 0 || busy !== ""}
              onClick={openStartDialog}
            >
              {t("Start server")}
            </Button>
          </div>
        }
      />

      {error ? <ErrorNotice message={error} /> : null}

      <Card>
        <CardHeader className="border-b">
          <CardTitle>{t("Running MCP servers")}</CardTitle>
          <CardDescription>
            {t("Runtime-only loopback servers. They stop when Mender exits.")}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {!loaded ? (
            <div className="py-10 text-center text-sm text-muted-foreground">
              {t("Loading servers…")}
            </div>
          ) : servers.length ? (
            <>
              <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("Profile")}</TableHead>
                  <TableHead>{t("Status")}</TableHead>
                  <TableHead>{t("Endpoint")}</TableHead>
                  <TableHead>{t("Started")}</TableHead>
                  <TableHead className="text-right">{t("Actions")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {serverPagination.items.map((server) => (
                  <TableRow key={server.serverId}>
                    <TableCell>
                      <div className="font-medium">{server.profile}</div>
                      <div className="mt-1 text-xs text-muted-foreground">
                        {t("Streamable HTTP")}
                      </div>
                    </TableCell>
                    <TableCell>
                      <Badge variant="outline">{server.status}</Badge>
                    </TableCell>
                    <TableCell className="max-w-[360px]">
                      <div className="truncate font-mono text-xs">
                        {server.endpoint}
                      </div>
                      <div className="mt-1 font-mono text-[11px] text-muted-foreground">
                        {server.address}
                      </div>
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
              <TablePagination
                label="Servers"
                pagination={serverPagination}
              />
            </>
          ) : (
            <EmptyState
              title="No running MCP servers"
              description={
                profiles.length
                  ? "Start a Profile as a loopback Streamable HTTP server."
                  : "Create a Profile before starting an MCP server."
              }
            />
          )}
        </CardContent>
      </Card>

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
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{t("Start MCP server")}</DialogTitle>
            <DialogDescription>
              {t("Expose one Profile over local Streamable HTTP. Remote/wildcard bind addresses are intentionally rejected.")}
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-4">
            <Field label="Profile">
              <select
                aria-label={t("Server profile")}
                value={startProfile}
                onChange={(event) => setStartProfile(event.target.value)}
                className="h-8 w-full rounded-lg border border-input bg-background px-3 text-sm outline-none focus:ring-2 focus:ring-ring/40"
              >
                {profiles.map((profile) => (
                  <option key={profile.profileId} value={profile.slug}>
                    {profile.displayName} ({profile.slug}) · {profile.tools.length} {t("tools")}
                  </option>
                ))}
              </select>
            </Field>

            {selectedProfile !== null && selectedProfile.tools.length === 0 ? (
              <div className="rounded-lg border border-amber-500/30 bg-amber-500/10 p-3 text-xs leading-5">
                {t("This Profile currently contains 0 Tools. The server can start, but downstream discovery will not return Provider Tools until you add Tool membership on the Profiles page and Restart it.")}
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
          </div>

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
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString();
}

function errorMessage(reason: unknown) {
  return reason instanceof Error ? reason.message : t("Unexpected error");
}
