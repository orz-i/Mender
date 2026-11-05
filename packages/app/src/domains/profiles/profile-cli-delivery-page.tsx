import { useCallback, useEffect, useState } from "react";
import { ChevronLeftIcon, CopyIcon } from "lucide-react";
import { useNavigate, useParams } from "react-router";

import type { ProfileCLIInstructions } from "@mender/sdk";
import { Button } from "@mender/ui/components/button";
import { t } from "../../i18n/i18n";
import { errorMessage } from "../../shared/errors";
import { ErrorNotice } from "../../shared/page";
import { DataPanel } from "../../shared/workbench";

function quoteArg(value: string, platform: string): string {
  if (/^[a-zA-Z0-9_./:=-]+$/.test(value)) return value;
  if (platform === "windows") return '"' + value.replaceAll('"', '\\"') + '"';
  return "'" + value.replaceAll("'", "'\"'\"'") + "'";
}
function commandText(values: string[], platform: string) {
  return values.map((value) => quoteArg(value, platform)).join(" ");
}

type CommandField = "search" | "describe" | "call" | "mcpStdio";
const commands: { label: string; field: CommandField }[] = [
  { label: "Search Tools", field: "search" },
  { label: "Describe Tool", field: "describe" },
  { label: "Call Tool", field: "call" },
  { label: "MCP stdio", field: "mcpStdio" },
];

function commandHelp(field: CommandField): string {
  switch (field) {
    case "search": return t("Search only this Profile's selected Tools.");
    case "describe": return t("Inspect one Tool only if it belongs to this Profile.");
    case "call": return t("Calls still pass through Mender Policy, approval, and Audit.");
    case "mcpStdio": return t("The client launches MCP stdio on demand; this is not a running server.");
  }
}

/** Local-only command recipes pinned to the current host DB and Profile. */
export function ProfileCLIDeliveryPage({
  getLocalProfileCLI, copyText,
}: {
  getLocalProfileCLI?: ((slug: string) => Promise<ProfileCLIInstructions>) | undefined;
  copyText?: (value: string) => Promise<void>;
}) {
  const { slug } = useParams();
  return <ProfileCLIDeliveryContent key={slug} slug={slug} getLocalProfileCLI={getLocalProfileCLI}
    {...(copyText === undefined ? {} : { copyText })} />;
}

function ProfileCLIDeliveryContent({
  getLocalProfileCLI, copyText, slug,
}: {
  getLocalProfileCLI?: ((slug: string) => Promise<ProfileCLIInstructions>) | undefined;
  copyText?: (value: string) => Promise<void>;
  slug: string | undefined;
}) {
  const navigate = useNavigate();
  const [recipe, setRecipe] = useState<ProfileCLIInstructions | null>(null);
  const [busy, setBusy] = useState(false);
  const [loadError, setLoadError] = useState("");
  const [copyError, setCopyError] = useState("");
  const [copied, setCopied] = useState("");
  const [loaded, setLoaded] = useState(false);
  const [version, setVersion] = useState(0);
  const visibleLoadError = getLocalProfileCLI ? loadError : "";

  useEffect(() => {
    if (!slug || !getLocalProfileCLI) return;
    let active = true;
    void getLocalProfileCLI(slug).then((item) => {
      if (!active) return;
      setRecipe(item);
      setLoadError("");
    }).catch((reason: unknown) => {
      if (active) setLoadError(errorMessage(reason));
    }).finally(() => { if (active) setLoaded(true); });
    return () => { active = false; };
  }, [getLocalProfileCLI, slug, version]);

  const refresh = useCallback(() => {
    setLoaded(false);
    setRecipe(null);
    setVersion((current) => current + 1);
  }, []);

  async function copy(field: string, text: string) {
    if (!copyText || busy) return;
    setBusy(true);
    setCopyError("");
    try {
      await copyText(text);
      setCopied(field);
    } catch (reason) {
      setCopyError(errorMessage(reason));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <Button size="sm" variant="outline" onClick={() => void navigate(slug
          ? "/profiles/" + encodeURIComponent(slug) : "/profiles",
          { state: { initialSection: "deliveries" } })}>
          <ChevronLeftIcon data-icon="inline-start" />
          {t("Back to profile")}
        </Button>
        {getLocalProfileCLI && !loadError ? <Button size="sm" variant="outline" onClick={refresh}>
          {t("Refresh")}
        </Button> : null}
      </div>
      <DataPanel title={t("Local CLI delivery")} description={slug}>
        <div className="space-y-5 p-5">
          <p className="text-sm text-muted-foreground">
            {t("These commands use the current Desktop database and this exact Profile. Mender CLI must be installed locally.")}
          </p>
          {!getLocalProfileCLI ?
            <p role="note" className="text-sm text-muted-foreground">
              {t("Local CLI instructions are available only on the Mender Desktop host, not through remote management.")}
            </p> : null}
          {getLocalProfileCLI && !loaded ? <p className="text-sm text-muted-foreground">{t("Loading Profile delivery…")}</p> : null}
          {visibleLoadError || !slug ? <ErrorNotice message={visibleLoadError || t("The requested item was not found.")}
            onRetry={slug && getLocalProfileCLI ? refresh : undefined} /> : null}
          {getLocalProfileCLI && copyError ? <ErrorNotice message={copyError} /> : null}
          {getLocalProfileCLI && recipe ? <>
            <div className="rounded-lg border border-border p-3">
              <p className="text-xs font-medium text-muted-foreground">{t("Local database")}</p>
              <p className="mt-1 break-all font-mono text-xs">{recipe.databasePath}</p>
            </div>
            <div className="divide-y divide-border rounded-lg border border-border">
              {commands.map((item) => {
                const value = commandText(recipe[item.field], recipe.platform);
                return <div key={item.field} className="space-y-2 px-4 py-4">
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <div className="min-w-0">
                      <p className="text-sm font-medium">{t(item.label)}</p>
                      <p className="mt-1 text-xs text-muted-foreground">{commandHelp(item.field)}</p>
                    </div>
                    {copyText ? <Button size="sm" variant="outline" disabled={busy}
                      onClick={() => void copy(item.field, value)}>
                      <CopyIcon data-icon="inline-start" />
                      {copied === item.field ? t("Copied") : t("Copy")}
                    </Button> : null}
                  </div>
                  <code className="block overflow-x-auto whitespace-pre-wrap break-all rounded-lg bg-muted/50 p-3 text-xs">{value}</code>
                </div>;
              })}
            </div>
            <p className="text-xs text-muted-foreground">
              {t("Replace example query and Tool IDs before use. Profile-scoped CLI is not an OS sandbox.")}
            </p>
          </> : null}
        </div>
      </DataPanel>
    </div>
  );
}
