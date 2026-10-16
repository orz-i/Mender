import { useCallback, useEffect, useMemo, useState } from "react";
import { CableIcon, Trash2Icon } from "lucide-react";
import { useSearchParams } from "react-router";

import type { Credential, MenderClient, Tunnel, PluginCatalogueEntry } from "@mender/sdk";
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

import { t, tCount } from "../../i18n/i18n";
import { errorMessage } from "../../shared/errors";
import { IconButton } from "../../shared/icon-button";
import { EmptyState, ErrorNotice } from "../../shared/page";
import { PageToolbar, ResultPanel } from "../../shared/page-toolbar";
import { usePagination } from "../../shared/pagination";
import { TablePagination } from "../../shared/table-pagination";
import { EmptyPanelState, SearchField } from "../../shared/workbench";

const emptyCreateForm = {
  displayName: "",
  externalTunnelId: "",
  runtimeCredentialRef: "",
};

export function TunnelsPage({ client }: { client: MenderClient }) {
  const [searchParams]=useSearchParams();
  const [tunnels, setTunnels] = useState<Tunnel[]>([]);
  const [availablePlugins,setAvailablePlugins]=useState<PluginCatalogueEntry[]>([]);
  const [selectedPluginKey,setSelectedPluginKey]=useState("");
  const [selectionOpen,setSelectionOpen]=useState(false);
  const [credentials, setCredentials] = useState<Credential[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState("");
  const [removeTarget,setRemoveTarget]=useState<Tunnel|null>(null);
  const [inventoryQuery, setInventoryQuery] = useState("");
  const [deepLink] = useState(() => ({
    create:searchParams.get("create")==="1",
    pluginId:searchParams.get("pluginId")??"",
    installationId:searchParams.get("installationId")??"",
  }));
  const [helpOpen, setHelpOpen] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);
  const [createForm, setCreateForm] = useState(emptyCreateForm);

  const refresh = useCallback(async () => {
    try {
      const [tunnelResult, credentialResult, catalogue] = await Promise.all([
        client.listTunnels(),client.listCredentials(),client.listPluginCatalogue(),
      ]);
      setAvailablePlugins(catalogue.plugins.filter((item)=>item.kind==="tunnel.transport"));
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
    void Promise.all([client.listTunnels(), client.listCredentials(),client.listPluginCatalogue()])
      .then(([tunnelResult, credentialResult,catalogue]) => {
        if (active) {
          setTunnels(tunnelResult.tunnels);
          setCredentials(credentialResult.credentials);
          setAvailablePlugins(catalogue.plugins.filter((item)=>item.kind==="tunnel.transport"));
          if(deepLink.create){
            const selected=catalogue.plugins.find(item=>item.kind==="tunnel.transport" &&
              item.pluginId===deepLink.pluginId &&
              (item.installationId??"")===deepLink.installationId &&
              item.enabled && item.status==="enabled");
            if(selected){
              setSelectedPluginKey(selected.key);
              setCreateForm({...emptyCreateForm,runtimeCredentialRef:credentialResult.credentials[0]?.reference??""});
              setCreateOpen(true);
            }
          }
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
  }, [client,deepLink]);

  const selectedPlugin=availablePlugins.find((item)=>item.key===selectedPluginKey);
  const enabledPlugins=availablePlugins.filter((item)=>item.enabled&&item.status==="enabled");

  function choosePlugin(plugin:PluginCatalogueEntry){
    if(!plugin.enabled || plugin.status!=="enabled")return;
    setSelectedPluginKey(plugin.key);
    setCreateForm({
      ...emptyCreateForm,
      runtimeCredentialRef: credentials[0]?.reference ?? "",
    });
    setSelectionOpen(false);
    setCreateOpen(true);
    setError("");
  }

  const visibleTunnels = useMemo(() => {
    const needle = inventoryQuery.trim().toLocaleLowerCase();
    return needle ? tunnels.filter((item) =>
      [item.displayName, item.externalTunnelId, item.runtimeCredentialRef]
        .some((value) => value.toLocaleLowerCase().includes(needle))
    ) : tunnels;
  }, [inventoryQuery, tunnels]);
  const pagination = usePagination(visibleTunnels);

  const createValid =
    selectedPlugin!==undefined && selectedPlugin.enabled &&
    createForm.displayName.trim() !== "" &&
    createForm.externalTunnelId.trim() !== "" &&
    createForm.runtimeCredentialRef !== "";

  async function createTunnel() {
    setBusy("create");
    setError("");
    try {
      if(!selectedPlugin)throw new Error("Selected Tunnel transport is unavailable");
      const input={
        displayName:createForm.displayName,
        externalTunnelId:createForm.externalTunnelId,
        runtimeCredentialRef:createForm.runtimeCredentialRef,
      };
      if(selectedPlugin.installationId){
        await client.createExternalTunnel({...input,installationId:selectedPlugin.installationId});
      }else if(selectedPlugin.pluginId==="io.mender.tunnel.openai"){
        await client.createOpenAITunnel(input);
      }else{
        throw new Error("Unknown built-in Tunnel transport");
      }
      setCreateOpen(false);
      setCreateForm(emptyCreateForm);
      await refresh();
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy("");
    }
  }

  async function remove(item:Tunnel){
    if(busy!=="" || removeTarget?.tunnelId!==item.tunnelId)return;
    setBusy(item.tunnelId);
    setError("");
    try {
      await client.deleteTunnel(item.tunnelId);
      setRemoveTarget(null);
      await refresh();
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy("");
    }
  }

  return (
    <>
      <PageToolbar
        tabs={
          <span className="text-xs text-muted-foreground">
            {tCount(tunnels.length, "{{count}} Tunnel configured", "{{count}} Tunnels configured")}
          </span>
        }
        filters={
          <div className="w-full min-w-0 sm:w-56 sm:shrink-0">
            <SearchField label="Search tunnels" placeholder="Search tunnels…" value={inventoryQuery} onChange={setInventoryQuery} />
          </div>
        }
        actions={
          <>
            <Button size="sm" variant="outline" onClick={() => setHelpOpen(true)}>
              <CableIcon data-icon="inline-start" />
              {t("OpenAI Secure MCP Tunnel")}
            </Button>
            <Button
              size="sm"
              disabled={busy !== ""}
              onClick={() => {
                setError("");
                setSelectionOpen(true);
              }}
            >
              {t("Add tunnel")}
            </Button>
          </>
        }
      />
      {error ? <ErrorNotice message={error} /> : null}
      <ResultPanel title="Tunnel inventory">
        {!loaded ? (
          <div className="px-4 py-12 text-center text-sm text-muted-foreground">
            {t("Loading Tunnels…")}
          </div>
        ) : visibleTunnels.length ? (
          <>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("Tunnel")}</TableHead>
                  <TableHead>{t("Provider")}</TableHead>
                  <TableHead>{t("External Tunnel ID")}</TableHead>
                  <TableHead>{t("Runtime credential")}</TableHead>
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
                      <Badge variant="outline">{item.provider==="external"?t("External plugin"):t("OpenAI")}</Badge>
                    </TableCell>
                    <TableCell className="max-w-[240px] truncate font-mono text-xs">
                      {item.externalTunnelId}
                    </TableCell>
                    <TableCell className="max-w-[260px] truncate font-mono text-xs text-muted-foreground">
                      {item.runtimeCredentialRef}
                    </TableCell>
                    <TableCell className="text-right">
                      <IconButton
                        variant="outline"
                        label={t("Delete {{name}}", { name: item.displayName })}
                        disabled={busy !== ""}
                        onClick={() => setRemoveTarget(item)}
                      >
                        <Trash2Icon />
                      </IconButton>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </>
        ) : (
          <EmptyPanelState>
            <EmptyState
              title="No Tunnels"
              description="Add an OpenAI Secure MCP Tunnel to make a managed local Profile MCP server reachable from supported OpenAI surfaces."
            />
          </EmptyPanelState>
        )}
      </ResultPanel>

      <TablePagination label="Tunnels" pagination={pagination} />

      <Dialog open={removeTarget!==null} onOpenChange={(open)=>{
        if(!open && busy==="")setRemoveTarget(null);
      }}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{t("Confirm Tunnel removal")}</DialogTitle>
            <DialogDescription>
              {removeTarget?t('Delete Tunnel "{{name}}"? Profiles that reference it will block deletion.',{
                name:removeTarget.displayName,
              }):""}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button type="button" variant="outline" disabled={busy!==""}
              onClick={()=>setRemoveTarget(null)}>{t("Cancel")}</Button>
            <Button type="button" variant="destructive" disabled={busy!=="" || !removeTarget}
              onClick={()=>{if(removeTarget)void remove(removeTarget);}}>{t("Delete")}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={selectionOpen} onOpenChange={setSelectionOpen}>
        <DialogContent className="sm:max-w-xl">
          <DialogHeader>
            <DialogTitle>{t("Choose Tunnel transport")}</DialogTitle>
            <DialogDescription>{t("Select one installed or built-in Tunnel transport.")}</DialogDescription>
          </DialogHeader>
          <div className="grid gap-2">
            {enabledPlugins.map((plugin)=>(
              <button key={plugin.key} type="button"
                className="rounded-xl border border-border p-4 text-left transition-colors hover:bg-accent"
                onClick={()=>choosePlugin(plugin)}>
                <span className="block font-medium">{plugin.displayName}</span>
                <span className="mt-1 block text-xs text-muted-foreground">{plugin.description}</span>
              </button>
            ))}
            {enabledPlugins.length===0?<p className="text-sm text-muted-foreground">{t("No available Tunnel transport plugins.")}</p>:null}
          </div>
        </DialogContent>
      </Dialog>

      <Dialog open={helpOpen} onOpenChange={setHelpOpen}>
        <DialogContent className="sm:max-w-xl" showCloseButton={false}>
          <DialogHeader>
            <DialogTitle>{t("OpenAI Secure MCP Tunnel")}</DialogTitle>
            <DialogDescription>
              {t("OpenAI Secure MCP Tunnel keeps the MCP server private and carries traffic through outbound HTTPS to supported OpenAI surfaces. It does not create a general public MCP URL.")}
            </DialogDescription>
          </DialogHeader>
          <div className="rounded-xl border border-border bg-background p-4 text-sm">
            {t("Install the client first:")}{" "}
            <code className="break-all font-mono text-xs">{t("brew install openai/tools/tunnel-client")}</code>
          </div>
          <DialogFooter>
            <DialogClose render={<Button variant="outline" />}>{t("Close")}</DialogClose>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog
        open={createOpen}
        onOpenChange={(open) => {
          setCreateOpen(open);
          if (!open) {
            setCreateForm(emptyCreateForm);
          }
        }}
      >
        <DialogContent className="grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden sm:max-w-xl">
          <DialogHeader>
            <DialogTitle>{t("Create Tunnel")}</DialogTitle>
            <DialogDescription>
              {t(
                "Mender stores only a Credential Reference and resolves the secret when connecting the selected transport.",
              )}
            </DialogDescription>
          </DialogHeader>

          <DialogBody className="space-y-4">
            <ReadOnlyField label="Provider" value={selectedPlugin?.displayName??t("Unavailable")} />

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

            <Field label="External Tunnel ID">
              <Input
                aria-label={t("External Tunnel ID")}
                className="font-mono"
                placeholder={t("External Tunnel ID")}
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
                      "Use an inventory Credential authorized for the selected Tunnel transport.",
                    )
                  : t(
                      "Create a Credential first. Tunnel configuration stores only its reference.",
                    )}
              </p>
            </Field>
          </DialogBody>

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
