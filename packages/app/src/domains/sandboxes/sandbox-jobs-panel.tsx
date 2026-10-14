import { useEffect, useState } from "react";
import type { MenderClient, Sandbox, SandboxToolSuite } from "@mender/sdk";
import { Badge } from "@mender/ui/components/badge";
import { Button } from "@mender/ui/components/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@mender/ui/components/dialog";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@mender/ui/components/table";
import { t } from "../../i18n/i18n";
import { errorMessage } from "../../shared/errors";
import { ErrorNotice } from "../../shared/page";
import { DataPanel, EmptyPanelState } from "../../shared/workbench";

// Only the reviewed, resource-scoped facade appears here. No task-submission
// form is embedded in the Sandbox management view; Profile bindings and Policy
// remain separate administrative steps.
export function SandboxJobsPanel({ client, sandbox }: { client: MenderClient; sandbox: Sandbox }) {
  const [suite, setSuite] = useState<SandboxToolSuite | null>(null);
  const [busy, setBusy] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState("");
  const [confirmRemove, setConfirmRemove] = useState(false);
  useEffect(() => {
    let active = true;
    void client.getSandboxJobTools(sandbox.sandboxId).then((item) => {
      if (active) { setSuite(item); setLoaded(true); setError(""); }
    }).catch((reason: unknown) => { if (active) { setLoaded(true); setError(errorMessage(reason)); } });
    return () => { active = false; };
  }, [client, sandbox.sandboxId]);
  async function prepare() {
    if (busy) return;
    setBusy(true); setError("");
    try { setSuite(await client.ensureSandboxJobTools(sandbox.sandboxId)); }
    catch (reason) { setError(errorMessage(reason)); }
    finally { setBusy(false); }
  }
  async function retire() {
    if (busy || !confirmRemove) return;
    setBusy(true); setError("");
    try {
      await client.removeSandboxJobTools(sandbox.sandboxId);
      setSuite(await client.getSandboxJobTools(sandbox.sandboxId));
      setConfirmRemove(false);
    } catch (reason) { setError(errorMessage(reason)); }
    finally { setBusy(false); }
  }
  return <>
    <DataPanel title={t("SSH Durable Jobs")} description={sandbox.name}>
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border px-5 py-4">
        <div className="min-w-0 space-y-2">
          <p className="text-sm font-medium">{t("Jobs Tool Suite")}</p>
          {suite ? <Badge variant="outline">{t(suite.status === "ready" ? "Ready" : suite.status === "conflict" ? "Conflict" :
            suite.status === "incomplete" ? "Incomplete" : suite.status === "incompatible" ? "Incompatible" : "Not prepared")}</Badge> : null}
          <p className="text-xs leading-5 text-muted-foreground">
            {suite?.issue || t("Jobs tools require an explicit Profile grant and ToolBinding. Root SSH is not isolated.")}
          </p>
        </div>
        <div className="flex gap-2">
          {suite?.available && suite.status !== "ready" && suite.status !== "conflict" ?
            <Button size="sm" disabled={busy} onClick={() => void prepare()}>{t("Prepare Jobs Tools")}</Button> : null}
          {suite?.providerId && suite.status === "ready" ? <Button size="sm" variant="outline" disabled={busy} onClick={() => setConfirmRemove(true)}>
            {t("Retire Jobs Tools")}
          </Button> : null}
        </div>
      </div>
      {error ? <div className="p-4"><ErrorNotice message={error}/></div> : null}
      {!loaded ? <p className="p-5 text-sm text-muted-foreground">{t("Loading Sandbox tools…")}</p> : suite?.tools.length ?
        <Table><TableHeader><TableRow><TableHead>{t("Tool")}</TableHead><TableHead>{t("Source key")}</TableHead></TableRow></TableHeader>
          <TableBody>{suite.tools.map(tool => <TableRow key={tool.toolId}>
            <TableCell className="font-medium" title={tool.toolId}>{tool.title}</TableCell>
            <TableCell className="font-mono text-xs">{tool.sourceKey}</TableCell>
          </TableRow>)}</TableBody></Table> :
        <EmptyPanelState><p className="text-sm text-muted-foreground">{t("No Jobs Tools are prepared for this Sandbox.")}</p></EmptyPanelState>}
    </DataPanel>
    <Dialog open={confirmRemove} onOpenChange={open => { if (!busy) setConfirmRemove(open); }}>
      <DialogContent><DialogHeader><DialogTitle>{t("Retire Jobs Tools")}</DialogTitle>
        <DialogDescription>{t("Retirement requires no active or uncertain jobs and no Profile or Policy references. Remote TaskIDs are preserved.")}</DialogDescription>
      </DialogHeader>
      <DialogFooter><Button variant="outline" disabled={busy} onClick={() => setConfirmRemove(false)}>{t("Cancel")}</Button>
        <Button variant="destructive" disabled={busy} onClick={() => void retire()}>{t("Confirm")}</Button></DialogFooter>
      </DialogContent>
    </Dialog>
  </>;
}
