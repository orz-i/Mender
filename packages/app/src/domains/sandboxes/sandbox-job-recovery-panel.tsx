import { useEffect, useState } from "react";
import type { MenderClient, Sandbox, SSHJobAdmissionState, SSHJobRecoveryView } from "@mender/sdk";
import { Badge } from "@mender/ui/components/badge";
import { Button } from "@mender/ui/components/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@mender/ui/components/dialog";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@mender/ui/components/table";
import { t } from "../../i18n/i18n";
import { errorMessage } from "../../shared/errors";
import { ErrorNotice } from "../../shared/page";
import { DataPanel, EmptyPanelState } from "../../shared/workbench";

// One information category at a time. Entering this view fetches *only* local
// admission state. Contacting remote SSH is an explicit inspection action;
// neither observation nor pause changes plugin-owned Task outcome.
export function SandboxJobRecoveryPanel({ client, sandbox }: { client: MenderClient; sandbox: Sandbox }) {
  const [admission, setAdmission] = useState<SSHJobAdmissionState | null>(null);
  const [inspection, setInspection] = useState<SSHJobRecoveryView | null>(null);
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [pending, setPending] = useState<"pause" | "resume" | null>(null);

  useEffect(() => {
    let live = true;
    void client.getSSHJobAdmission(sandbox.sandboxId).then(value => {
      if (live) { setAdmission(value); setLoading(false); }
    }).catch((reason: unknown) => {
      if (live) { setError(errorMessage(reason)); setLoading(false); }
    });
    return () => { live = false; };
  }, [client, sandbox.sandboxId]);

  async function inspect() {
    if (busy) return;
    setBusy(true); setError("");
    try {
      const report = await client.getSSHJobRecovery(sandbox.sandboxId);
      setInspection(report);
      setAdmission({ sandboxId: report.sandboxId, generation: report.generation, paused: report.paused });
    } catch (reason) { setError(errorMessage(reason)); }
    finally { setBusy(false); }
  }

  async function changeAdmission() {
    if (busy || !pending) return;
    setBusy(true); setError("");
    try {
      setAdmission(await client.setSSHJobAdmission(sandbox.sandboxId, { paused: pending === "pause" }));
      setPending(null);
    } catch (reason) { setError(errorMessage(reason)); }
    finally { setBusy(false); }
  }

  return <>
    <DataPanel title={t("SSH Job Recovery")} description={sandbox.name}>
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border p-4">
        <div className="space-y-1">
          <p className="text-sm font-medium">{t("New submissions")}</p>
          {admission ? <Badge variant="outline">{t(admission.paused ? "Paused" : "Accepting")}</Badge> : null}
          <p className="text-xs text-muted-foreground">
            {t("Pausing only blocks new submissions. Existing tasks, logs, and cancellation remain independently governed.")}
          </p>
        </div>
        {admission ? <Button size="sm" variant="outline" disabled={busy} onClick={() => setPending(admission.paused ? "resume" : "pause")}>
          {t(admission.paused ? "Resume submissions" : "Pause submissions")}
        </Button> : null}
      </div>
      <div className="flex flex-wrap items-center justify-between gap-3 p-4">
        <div className="space-y-1">
          <p className="text-sm font-medium">{t("Recovery evidence")}</p>
          <p className="text-xs text-muted-foreground">
            {t("Inspection is read-only. Unknown legacy jobs cannot be marked completed or released automatically.")}
          </p>
        </div>
        <Button size="sm" disabled={busy || loading} onClick={() => void inspect()}>{t("Inspect remote evidence")}</Button>
      </div>
      {error ? <div className="px-4 pb-4"><ErrorNotice message={error}/></div> : null}
      {loading ? <p className="px-4 pb-4 text-sm text-muted-foreground">{t("Loading Sandbox tools…")}</p> : null}
      {inspection ? <>
        <div className="border-y border-border px-4 py-3 text-xs text-muted-foreground" role="status">
          {t("Verdict")}: {inspection.evidence.verdict} · {inspection.evidence.records.length} {t("records")}
        </div>
        {inspection.evidence.records.length ? <Table>
          <TableHeader><TableRow>
            <TableHead>{t("Task ID")}</TableHead><TableHead>{t("Evidence phase")}</TableHead><TableHead>{t("Activity")}</TableHead>
          </TableRow></TableHeader>
          <TableBody>{inspection.evidence.records.map(record => <TableRow key={record.taskId}>
            <TableCell className="font-mono text-xs" title={record.fingerprint}>{record.taskId}</TableCell>
            <TableCell>{record.phase}</TableCell><TableCell>{record.activity}</TableCell>
          </TableRow>)}</TableBody>
        </Table> : <EmptyPanelState><p className="text-sm text-muted-foreground">{t("No SSH Jobs to inspect.")}</p></EmptyPanelState>}
      </> : null}
    </DataPanel>
    <Dialog open={pending !== null} onOpenChange={open => { if (!open && !busy) setPending(null); }}>
      <DialogContent><DialogHeader>
        <DialogTitle>{t(pending === "pause" ? "Pause new SSH Jobs" : "Resume new SSH Jobs")}</DialogTitle>
        <DialogDescription>
          {t(pending === "pause"
            ? "Only future submissions will be blocked. Running or uncertain jobs remain unchanged; Stop/Destroy still requires proof."
            : "New submissions may proceed again, but unknown jobs will not be retried or cleared.")}
        </DialogDescription>
      </DialogHeader>
      <DialogFooter>
        <Button variant="outline" disabled={busy} onClick={() => setPending(null)}>{t("Cancel")}</Button>
        <Button disabled={busy} onClick={() => void changeAdmission()}>{t("Confirm")}</Button>
      </DialogFooter></DialogContent>
    </Dialog>
  </>;
}
