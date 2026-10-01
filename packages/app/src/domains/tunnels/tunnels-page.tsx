import { useCallback, useEffect, useState } from "react";
import { CableIcon, Trash2Icon } from "lucide-react";

import type { Credential, MenderClient, Tunnel } from "@mender/sdk";
import { Badge } from "@mender/ui/components/badge";
import { Button } from "@mender/ui/components/button";
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

import { t, tCount } from "../../i18n/i18n";
import { errorMessage } from "../../shared/errors";
import { IconButton } from "../../shared/icon-button";
import { EmptyState, ErrorNotice, PageHeader } from "../../shared/page";
import { usePagination } from "../../shared/pagination";
import { TablePagination } from "../../shared/table-pagination";
import { DataPanel, EmptyPanelState } from "../../shared/workbench";

const emptyCreateForm = {
  displayName: "",
  externalTunnelId: "",
  runtimeCredentialRef: "",
  executable: "",
};

export function TunnelsPage({ client }: { client: MenderClient }) {
  const [tunnels, setTunnels] = useState<Tunnel[]>([]);
  const [credentials, setCredentials] = useState<Credential[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState("");
  const [createOpen, setCreateOpen] = useState(false);
  const [createForm, setCreateForm] = useState(emptyCreateForm);

  const refresh = useCallback(async () => {
    try {
      const [tunnelResult, credentialResult] = await Promise.all([
        client.listTunnels(),
        client.listCredentials(),
      ]);
      setTunnels(tunnelResult.tunnels);
      setCredentials(credentialResult.credentials);
      setError("");
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setLoaded(true);
    }
  }, [client]);

  useEffect(() => {
    let active = true;
    void Promise.all([client.listTunnels(), client.listCredentials()])
      .then(([tunnelResult, credentialResult]) => {
        if (active) {
          setTunnels(tunnelResult.tunnels);
          setCredentials(credentialResult.credentials);
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

  const pagination = usePagination(tunnels);

  const createValid =
    createForm.displayName.trim() !== "" &&
    createForm.externalTunnelId.trim() !== "" &&
    createForm.runtimeCredentialRef !== "";

  async function createTunnel() {
    setBusy("create");
    setError("");
    try {
      await client.createOpenAITunnel({
        displayName: createForm.displayName,
        externalTunnelId: createForm.externalTunnelId,
        runtimeCredentialRef: createForm.runtimeCredentialRef,
        ...(createForm.executable.trim()
          ? { executable: createForm.executable.trim() }
          : {}),
      });
      setCreateOpen(false);
      setCreateForm(emptyCreateForm);
      await refresh();
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy("");
    }
  }

  async function remove(item: Tunnel) {
    if (
      !window.confirm(
        t('Delete Tunnel "{{name}}"? Profiles that reference it will block deletion.', {
          name: item.displayName,
        }),
      )
    ) {
      return;
    }

    setBusy(item.tunnelId);
    setError("");
    try {
      await client.deleteTunnel(item.tunnelId);
      await refresh();
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy("");
    }
  }

  return (
    <>
      <PageHeader
        actions={
          <Button
            disabled={busy !== ""}
            onClick={() => {
              setError("");
              setCreateForm({
                ...emptyCreateForm,
                runtimeCredentialRef: credentials[0]?.reference ?? "",
              });
              setCreateOpen(true);
            }}
          >
            {t("Add tunnel")}
          </Button>
        }
      />

      {error ? <ErrorNotice message={error} /> : null}

      <div className="mb-4 rounded-xl border border-border bg-muted/35 px-4 py-3">
        <div className="flex gap-3">
          <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg border border-border bg-background">
            <CableIcon className="size-4" aria-hidden="true" />
          </div>
          <div className="min-w-0">
            <p className="text-sm font-medium">{t("OpenAI Secure MCP Tunnel")}</p>
            <p className="mt-1 text-xs leading-5 text-muted-foreground">
              {t(
                "OpenAI Secure MCP Tunnel keeps the MCP server private and carries traffic through outbound HTTPS to supported OpenAI surfaces. It does not create a general public MCP URL.",
              )}
            </p>
            <p className="mt-2 text-xs text-muted-foreground">
              {t("Install the client first:")}{" "}
              <code className="rounded bg-background px-1.5 py-0.5 font-mono">
                {t("brew install openai/tools/tunnel-client")}
              </code>
            </p>
          </div>
        </div>
      </div>

      <DataPanel
        title="Tunnel inventory"
        description={tCount(
          tunnels.length,
          "{{count}} Tunnel configured",
          "{{count}} Tunnels configured",
        )}
      >
        {!loaded ? (
          <div className="px-4 py-12 text-center text-sm text-muted-foreground">
            {t("Loading Tunnels…")}
          </div>
        ) : tunnels.length ? (
          <>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("Tunnel")}</TableHead>
                  <TableHead>{t("Provider")}</TableHead>
                  <TableHead>{t("OpenAI Tunnel ID")}</TableHead>
                  <TableHead>{t("Runtime credential")}</TableHead>
                  <TableHead>{t("Executable")}</TableHead>
                  <TableHead className="text-right">{t("Actions")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {pagination.items.map((item) => (
                  <TableRow key={item.tunnelId}>
                    <TableCell className="font-medium">
                      {item.displayName}
                    </TableCell>
                    <TableCell>
                      <Badge variant="outline">{t("OpenAI")}</Badge>
                    </TableCell>
                    <TableCell className="max-w-[240px] truncate font-mono text-xs">
                      {item.externalTunnelId}
                    </TableCell>
                    <TableCell className="max-w-[260px] truncate font-mono text-xs text-muted-foreground">
                      {item.runtimeCredentialRef}
                    </TableCell>
                    <TableCell className="max-w-[220px] truncate font-mono text-xs text-muted-foreground">
                      {item.executable}
                    </TableCell>
                    <TableCell className="text-right">
                      <IconButton
                        variant="outline"
                        label={t("Delete {{name}}", { name: item.displayName })}
                        disabled={busy !== ""}
                        onClick={() => void remove(item)}
                      >
                        <Trash2Icon />
                      </IconButton>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
            <div className="px-4 pb-3">
              <TablePagination label="Tunnels" pagination={pagination} />
            </div>
          </>
        ) : (
          <EmptyPanelState>
            <EmptyState
              title="No Tunnels"
              description="Add an OpenAI Secure MCP Tunnel to make a managed local Profile MCP server reachable from supported OpenAI surfaces."
            />
          </EmptyPanelState>
        )}
      </DataPanel>

      <Dialog
        open={createOpen}
        onOpenChange={(open) => {
          setCreateOpen(open);
          if (!open) {
            setCreateForm(emptyCreateForm);
          }
        }}
      >
        <DialogContent className="sm:max-w-xl">
          <DialogHeader>
            <DialogTitle>{t("Create OpenAI Tunnel")}</DialogTitle>
            <DialogDescription>
              {t(
                "Tunnel configuration stores only a Credential Reference. Mender resolves the API key only when starting tunnel-client.",
              )}
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-4">
            <ReadOnlyField label="Provider" value={t("OpenAI Secure MCP Tunnel")} />

            <Field label="Display name">
              <Input
                aria-label={t("Tunnel display name")}
                placeholder={t("OpenAI Work")}
                value={createForm.displayName}
                onChange={(event) =>
                  setCreateForm((current) => ({
                    ...current,
                    displayName: event.target.value,
                  }))
                }
              />
            </Field>

            <Field label="OpenAI Tunnel ID">
              <Input
                aria-label={t("OpenAI Tunnel ID")}
                className="font-mono"
                placeholder="tunnel_..."
                value={createForm.externalTunnelId}
                onChange={(event) =>
                  setCreateForm((current) => ({
                    ...current,
                    externalTunnelId: event.target.value,
                  }))
                }
              />
            </Field>

            <Field label="Runtime API-key Credential">
              <select
                aria-label={t("Runtime API-key Credential")}
                value={createForm.runtimeCredentialRef}
                onChange={(event) =>
                  setCreateForm((current) => ({
                    ...current,
                    runtimeCredentialRef: event.target.value,
                  }))
                }
                className="h-9 w-full rounded-lg border border-input bg-background px-3 text-sm outline-none focus:ring-2 focus:ring-ring/40"
              >
                <option value="">{t("Choose a Credential")}</option>
                {credentials.map((credential) => (
                  <option key={credential.reference} value={credential.reference}>
                    {credential.displayName} · {credential.reference}
                  </option>
                ))}
              </select>
              <p className="mt-1 text-xs leading-5 text-muted-foreground">
                {credentials.length
                  ? t(
                      "Use an inventory Credential containing the OpenAI tunnel runtime API key.",
                    )
                  : t(
                      "Create a Credential first. Tunnel configuration stores only its reference; Mender resolves the secret when starting tunnel-client.",
                    )}
              </p>
            </Field>

            <Field label="Tunnel client executable">
              <Input
                aria-label={t("Tunnel client executable")}
                className="font-mono"
                placeholder="tunnel-client"
                value={createForm.executable}
                onChange={(event) =>
                  setCreateForm((current) => ({
                    ...current,
                    executable: event.target.value,
                  }))
                }
              />
              <p className="mt-1 text-xs leading-5 text-muted-foreground">
                {t("Leave blank to use tunnel-client from PATH.")}
              </p>
            </Field>
          </div>

          <DialogFooter>
            <DialogClose render={<Button variant="outline" />}>
              {t("Cancel")}
            </DialogClose>
            <Button
              disabled={!createValid || busy !== ""}
              onClick={() => void createTunnel()}
            >
              {t(busy === "create" ? "Creating…" : "Create tunnel")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
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

function ReadOnlyField({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <p className="text-xs font-medium text-muted-foreground">{t(label)}</p>
      <p className="mt-1 text-sm">{value}</p>
    </div>
  );
}
