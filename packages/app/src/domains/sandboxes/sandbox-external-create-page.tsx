import { useEffect, useMemo, useState } from "react";
import { useNavigate, useSearchParams } from "react-router";
import type { MenderClient, PluginCatalogueEntry } from "@mender/sdk";
import { Button } from "@mender/ui/components/button";
import { Input } from "@mender/ui/components/input";
import { t } from "../../i18n/i18n";
import { errorMessage } from "../../shared/errors";
import { EmptyState, ErrorNotice } from "../../shared/page";
import { PageToolbar } from "../../shared/page-toolbar";
import { DataPanel, EmptyPanelState } from "../../shared/workbench";
import { GenericProviderConfigForm } from "../providers/generic-provider-config-form";
import { genericConfigDefaults, genericConfigValidationError } from "../providers/generic-provider-config-schema";

// Each view owns exactly one resource creation flow; package installation and
// backend selection are completed before navigating here.
export function ExternalSandboxCreatePage({ client }: { client: MenderClient }) {
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const installationId = params.get("installationId") ?? "";
  const pluginId = params.get("pluginId") ?? "";
  const [catalogue, setCatalogue] = useState<PluginCatalogueEntry[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [name, setName] = useState("");
  const [value, setValue] = useState<Record<string, unknown>>({});
  const [step, setStep] = useState<"configure" | "review">("configure");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    let live = true;
    void client.listPluginCatalogue().then((result) => {
      if (!live) return;
      setCatalogue(result.plugins);
      const selected = result.plugins.find((plugin) =>
        plugin.installationId === installationId &&
        plugin.pluginId === pluginId &&
        plugin.kind === "sandbox.backend" && plugin.enabled &&
        plugin.status === "enabled");
      setValue(genericConfigDefaults(selected?.configSchema));
      setLoaded(true);
    }).catch((reason: unknown) => {
      if (!live) return;
      setError(errorMessage(reason));
      setLoaded(true);
    });
    return () => { live = false; };
  }, [client, installationId, pluginId]);

  const plugin = catalogue.find((item) =>
    item.kind === "sandbox.backend" && item.installationId === installationId &&
    item.pluginId === pluginId && item.enabled && item.status === "enabled");
  const validation = useMemo(() =>
    genericConfigValidationError(plugin?.configSchema, value),
  [plugin?.configSchema, value]);

  async function create() {
    if (!plugin?.installationId || !name.trim() || validation || busy || step !== "review") return;
    setBusy(true);
    setError("");
    try {
      const resource = await client.createExternalSandbox({
        name: name.trim(), installationId: plugin.installationId, config: value,
      });
      void navigate("/sandboxes/" + resource.sandboxId);
    } catch (reason) {
      setError(errorMessage(reason));
      setBusy(false);
    }
  }

  return <>
    <PageToolbar actions={
      <Button variant="outline" onClick={() => void navigate("/sandboxes")}>{t("Back to sandboxes")}</Button>
    } />
    {error ? <ErrorNotice message={error} /> : null}
    {!loaded ? (
      <DataPanel title="Create sandbox">
        <div className="p-8 text-sm text-muted-foreground">{t("Loading plugins…")}</div>
      </DataPanel>
    ) : !plugin ? (
      <DataPanel title="Create sandbox">
        <EmptyPanelState>
          <EmptyState title="No matching plugins" description="The selected Sandbox backend is unavailable." />
        </EmptyPanelState>
      </DataPanel>
    ) : (
      <DataPanel title={plugin.displayName}>
        <div className="mx-auto flex max-w-2xl flex-col gap-5 p-5 sm:p-7">
          <p className="text-sm text-muted-foreground">
            {t(step === "configure"
              ? "Configure the exact installed Sandbox backend."
              : "Review the Sandbox authority before provisioning.")}
          </p>
          {step === "configure" ? <>
            <label className="grid gap-2 text-sm">
              <span className="font-medium">{t("Sandbox name")}</span>
              <Input aria-label={t("Sandbox name")} value={name} maxLength={160}
                onChange={(event) => setName(event.target.value)} />
            </label>
            <GenericProviderConfigForm
              schema={plugin.configSchema}
              uiSchema={undefined}
              value={value}
              credentials={[]}
              onChange={setValue}
            />
            {validation ? <p role="alert" className="text-xs text-destructive">{t(validation)}</p> : null}
            <div className="flex justify-end">
              <Button disabled={!name.trim() || Boolean(validation)}
                onClick={() => setStep("review")}>{t("Review")}</Button>
            </div>
          </> : <>
            <dl className="grid gap-3 text-sm">
              <div><dt className="text-muted-foreground">{t("Sandbox name")}</dt><dd className="font-medium">{name}</dd></div>
              <div><dt className="text-muted-foreground">{t("Backend")}</dt><dd className="font-medium">{plugin.displayName}</dd></div>
              <div><dt className="text-muted-foreground">{t("Installation ID")}</dt><dd className="break-all font-mono text-xs">{plugin.installationId}</dd></div>
              <div><dt className="text-muted-foreground">{t("Isolation")}</dt><dd>{t("Remote backend; native plugin executes under the host account.")}</dd></div>
              <div><dt className="text-muted-foreground">{t("Configuration")}</dt><dd className="break-all font-mono text-xs">{JSON.stringify(value)}</dd></div>
            </dl>
            <div className="flex justify-between gap-2">
              <Button variant="outline" onClick={() => setStep("configure")}>{t("Back")}</Button>
              <Button disabled={busy} onClick={() => void create()}>
                {t(busy ? "Creating…" : "Create sandbox")}
              </Button>
            </div>
          </>}
        </div>
      </DataPanel>
    )}
  </>;
}
