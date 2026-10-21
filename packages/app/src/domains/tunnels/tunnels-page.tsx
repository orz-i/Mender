import { useCallback, useEffect, useMemo, useState } from "react";
import { PlusIcon, Trash2Icon } from "lucide-react";
import { Link, useSearchParams } from "react-router";

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
import { FormHeading } from "../../shared/form-info";
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

function tunnelCreateError(reason: unknown): string {
  const detail = (typeof reason === "string" ? reason : reason instanceof Error ? reason.message : "").toLowerCase();
  // Transport plugins may return untrusted diagnostics. Do not echo credential material into a form alert.
  if (detail.includes("already exists") || detail.includes("already registered") || detail.includes("duplicate")) {
    return t("This Tunnel ID may already exist. Verify the external Tunnel ID before retrying.");
  }
  if (detail.includes("credential") || detail.includes("keyring")) {
    return t("The selected connection credential is not available or authorized.");
  }
  return t("Tunnel creation failed. Check the external ID and selected credential, then try again.");
}

export function TunnelsPage({ client }: { client: MenderClient }) {
  const [searchParams]=useSearchParams();
  const [tunnels, setTunnels] = useState<Tunnel[]>([]);
  const [availablePlugins,setAvailablePlugins]=useState<PluginCatalogueEntry[]>([]);
  const [selectedPluginKey,setSelectedPluginKey]=useState("");
  const [selectionOpen,setSelectionOpen]=useState(false);
  const [credentials, setCredentials] = useState<Credential[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [hasSnapshot, setHasSnapshot] = useState(false);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState("");
  const [removeTarget,setRemoveTarget]=useState<Tunnel|null>(null);
  const [inventoryQuery, setInventoryQuery] = useState("");
  const [deepLink] = useState(() => ({
    create:searchParams.get("create")==="1",
    pluginId:searchParams.get("pluginId")??"",
    installationId:searchParams.get("installationId")??"",
  }));
  const [createOpen, setCreateOpen] = useState(false);
  const [createError, setCreateError] = useState("");
  const [createForm, setCreateForm] = useState(emptyCreateForm);

  const refresh = useCallback(async () => {
    try {
      const [tunnelResult, credentialResult, catalogue] = await Promise.all([
        client.listTunnels(),client.listCredentials(),client.listPluginCatalogue(),
      ]);
      setAvailablePlugins(catalogue.plugins.filter((item)=>item.kind==="tunnel.transport"));
      setTunnels(tunnelResult.tunnels);
      setCredentials(credentialResult.credentials);
      setHasSnapshot(true);
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
          setHasSnapshot(true);
          if(deepLink.create){
            const selected=catalogue.plugins.find(item=>item.kind==="tunnel.transport" &&
              item.pluginId===deepLink.pluginId &&
              (item.installationId??"")===deepLink.installationId &&
              item.enabled && item.status==="enabled");
            if(selected){
              setSelectedPluginKey(selected.key);
              setCreateForm(emptyCreateForm);
              setCreateError("");
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
    setCreateForm(emptyCreateForm);
    setCreateError("");
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
    if (!createValid || busy !== "") return;
    setBusy("create");
    setCreateError("");
    try {
      if(!selectedPlugin)throw new Error("Selected Tunnel transport is unavailable");
      const input={
        displayName:createForm.displayName.trim(),
        externalTunnelId:createForm.externalTunnelId.trim(),
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
      setCreateError("");
      await refresh();
    } catch (reason) {
      setCreateError(tunnelCreateError(reason));
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

  if (error && !hasSnapshot) {
    return <div className="pt-4"><ErrorNotice message={error} onRetry={() => {setError("");void refresh();}} /></div>;
  }

  return (
    <>
      <PageToolbar
        tabs={
          <span className="text-sm text-muted-foreground">
            {hasSnapshot ? tCount(tunnels.length, "{{count}} Tunnel configured", "{{count}} Tunnels configured") : t("Loading Tunnels…")}
          </span>
        }
        filters={
          <div className="w-full min-w-0 sm:w-56 md:w-64 sm:shrink-0">
            <SearchField
              label="Search tunnels"
              placeholder="Search tunnels…"
              value={inventoryQuery}
              onChange={setInventoryQuery}
            />
          </div>
        }
        actions={
          <Button
            size="sm"
            disabled={busy !== ""}
            onClick={() => {
              setError("");
              setSelectionOpen(true);
            }}
          >
            <PlusIcon data-icon="inline-start" />
            {t("Add tunnel")}
          </Button>
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
                  <TableHead className="w-[28%] min-w-[140px]">{t("Tunnel")}</TableHead>
                  <TableHead className="w-[18%] min-w-[110px]">{t("Provider")}</TableHead>
                  <TableHead className="w-[27%] min-w-[160px]">{t("External Tunnel ID")}</TableHead>
                  <TableHead className="w-[27%] min-w-[160px]">{t("Runtime credential")}</TableHead>
                  <TableHead className="w-16 text-right">{t("Actions")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {pagination.items.map((item) => (
                  <TableRow key={item.tunnelId}>
                    <TableCell className="font-medium text-foreground">
                      <span className="block truncate font-medium text-foreground" title={item.displayName}>
                        {item.displayName}
                      </span>
                    </TableCell>
                    <TableCell>
                      <Badge
                        variant={item.provider === "external" ? "secondary" : "default"}
                        className="shrink-0"
                      >
                        {item.provider === "external" ? t("External plugin") : t("OpenAI")}
                      </Badge>
                    </TableCell>
                    <TableCell className="max-w-[200px] truncate font-mono text-xs text-muted-foreground sm:max-w-[240px]">
                      <span className="block truncate font-mono text-xs text-muted-foreground" title={item.externalTunnelId}>
                        {item.externalTunnelId}
                      </span>
                    </TableCell>
                    <TableCell className="max-w-[220px] truncate font-mono text-xs text-muted-foreground sm:max-w-[260px]">
                      <span className="block truncate font-mono text-xs text-muted-foreground" title={item.runtimeCredentialRef}>
                        {item.runtimeCredentialRef}
                      </span>
                    </TableCell>
                    <TableCell className="text-right">
                      <IconButton
                        variant="ghost"
                        size="icon-sm"
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
              description="Add a Tunnel to make a managed local Profile MCP server reachable from external clients."
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
            <DialogTitle className="text-lg font-semibold tracking-tight text-foreground">
              {t("Confirm Tunnel removal")}
            </DialogTitle>
            <DialogDescription className="text-xs leading-5 text-muted-foreground">
              {removeTarget?t('Delete Tunnel "{{name}}"? Profiles that reference it will block deletion.',{
                name:removeTarget.displayName,
              }):""}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter className="gap-2 sm:gap-2">
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
            <DialogTitle className="text-lg font-semibold tracking-tight text-foreground">
              {t("Choose Tunnel transport")}
            </DialogTitle>
            <DialogDescription className="text-xs leading-5 text-muted-foreground">
              {t("Select one installed or built-in Tunnel transport.")}
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-2.5">
            {enabledPlugins.map((plugin)=>(
              <button
                key={plugin.key}
                type="button"
                className="group flex flex-col items-start rounded-xl border border-border/80 bg-card p-4 text-left transition-all hover:border-primary/40 hover:bg-muted/30 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50 focus-visible:ring-offset-1"
                onClick={()=>choosePlugin(plugin)}
              >
                <span className="flex items-center gap-2 font-medium text-foreground">
                  {plugin.displayName}
                  <Badge variant={plugin.installationId ? "secondary" : "default"} className="shrink-0 text-xs">
                    {plugin.installationId ? t("External plugin") : t("Built-in")}
                  </Badge>
                </span>
                <span className="mt-1.5 text-xs leading-5 text-muted-foreground">{plugin.description}</span>
              </button>
            ))}
            {enabledPlugins.length===0?<p className="py-6 text-center text-xs text-muted-foreground">{t("No available Tunnel transport plugins.")}</p>:null}
          </div>
        </DialogContent>
      </Dialog>

      <Dialog
        open={createOpen}
        onOpenChange={(open) => {
          setCreateOpen(open);
          if (!open) {
            setCreateForm(emptyCreateForm);
            setCreateError("");
          }
        }}
      >
        <DialogContent className="grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden sm:max-w-xl">
          <DialogHeader>
            <DialogTitle className="text-lg font-semibold tracking-tight text-foreground">
              {t("Create Tunnel")}
            </DialogTitle>
            <DialogDescription className="sr-only">
              {t("Enter the Tunnel details provided by your transport service. Select an existing connection credential; Mender never copies its secret into the Tunnel.")}
            </DialogDescription>
            {createError ? (
              <div role="alert" className="rounded-lg border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive">{createError}</div>
            ) : null}
          </DialogHeader>

          <DialogBody className="space-y-4">
            <div className="flex flex-wrap items-center gap-x-2 text-sm">
              <span className="text-muted-foreground">{t("Provider")}</span>
              <span className="font-medium">{selectedPlugin?.displayName ?? t("Unavailable")}</span>
            </div>

            <Field label="Tunnel display name">
              <Input
                aria-label={t("Tunnel display name")}
                placeholder={t("OpenAI Work")}
                value={createForm.displayName}
                onChange={(event) => {
                  setCreateError("");
                  setCreateForm((current) => ({
                    ...current,
                    displayName: event.target.value,
                  }));
                }}
              />
            </Field>

            <Field label="External Tunnel ID" help={t("Copy this ID from your Tunnel provider. It is issued outside Mender and cannot be generated here.")}>
              <Input
                aria-label={t("External Tunnel ID")}
                className="font-mono text-xs"
                placeholder={t("External Tunnel ID")}
                value={createForm.externalTunnelId}
                onChange={(event) => {
                  setCreateError("");
                  setCreateForm((current) => ({
                    ...current,
                    externalTunnelId: event.target.value,
                  }));
                }}
              />
            </Field>

            <Field label="Connection credential" help={t("Use an inventory Credential authorized for the selected Tunnel transport.")}>
              <select
                aria-label={t("Connection credential")}
                value={createForm.runtimeCredentialRef}
                onChange={(event) => {
                  setCreateError("");
                  setCreateForm((current) => ({
                    ...current,
                    runtimeCredentialRef: event.target.value,
                  }));
                }}
                className="h-9 w-full rounded-lg border border-input bg-background px-3 text-sm text-foreground outline-none transition-colors focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50"
              >
                <option value="">{t("Choose a Credential")}</option>
                {credentials.map((credential) => (
                  <option key={credential.reference} value={credential.reference}>
                    {credential.displayName} · {credential.reference}
                  </option>
                ))}
              </select>
              {credentials.length === 0 ? (
                <p role="status" className="text-xs leading-5 text-muted-foreground">
                  {t("No connection credential is available yet. Add an API-key Credential before creating this Tunnel.")}
                </p>
              ) : null}
            </Field>
            {credentials.length === 0 ? (
              <Link to="/credentials" className="inline-flex min-h-9 items-center text-sm font-medium text-primary-ink underline-offset-4 hover:underline">
                {t("Go to Credentials")}
              </Link>
            ) : null}
          </DialogBody>

          <DialogFooter className="gap-2 sm:gap-2">
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
  help,
  children,
}: {
  label: string;
  help?: string;
  children: React.ReactNode;
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <FormHeading label={t(label)} help={help} />
      {children}
    </div>
  );
}
