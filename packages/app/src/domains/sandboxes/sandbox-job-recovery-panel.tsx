import { useEffect, useState } from "react";
import type { MenderClient, Sandbox, SSHJobAdmissionState, SSHJobRecoveryView, SSHJobRecoveryRecord } from "@mender/sdk";
import { Badge } from "@mender/ui/components/badge";
import { Button } from "@mender/ui/components/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@mender/ui/components/dialog";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@mender/ui/components/table";
import { Textarea } from "@mender/ui/components/textarea";
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
  const [activateV2Open, setActivateV2Open] = useState(false);
  const [resolveTarget, setResolveTarget] = useState<SSHJobRecoveryRecord | null>(null);
  const [reviewReason, setReviewReason] = useState("");

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
      setAdmission(previous => ({ sandboxId: report.sandboxId, generation: report.generation, paused: report.paused, protocol: previous?.protocol ?? "legacy_v1" }));
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

  async function activateV2() {
    if (busy || !admission?.paused || admission.protocol !== "legacy_v1") return;
    setBusy(true); setError("");
    try {
      setAdmission(await client.activateSSHJobV2(sandbox.sandboxId, { confirm: true }));
      setActivateV2Open(false);
    } catch (reason) {setError(errorMessage(reason));}
    finally {setBusy(false);}
  }

  async function resolvePrelaunch() {
    if (busy || !inspection || !admission?.paused || !resolveTarget || reviewReason.trim().length < 5) return;
    setBusy(true); setError("");
    try {
      await client.resolveSSHJobRecovery(sandbox.sandboxId, resolveTarget.taskId, {
        fingerprint: resolveTarget.fingerprint, generation: inspection.generation, reason: reviewReason,
      });
      // A fresh inspection is authoritative. Never present a success based
      // solely on a remote marker write or an uncertain acknowledgment.
      setInspection(await client.getSSHJobRecovery(sandbox.sandboxId));
      setResolveTarget(null); setReviewReason("");
    } catch (reason) { setError(errorMessage(reason)); }
    finally { setBusy(false); }
  }

  return <>
    <DataPanel title={t("SSH Job Recovery")} description={sandbox.name}>
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border p-4">
        <div className="space-y-1">
          <p className="text-sm font-medium">{t("New submissions")}</p>
          {admission ? <Badge variant="outline">{t(admission.paused ? "Paused" : "Accepting")}</Badge> : null}
          {admission ? <p className="text-xs text-muted-foreground">{t("Job protocol")}: {t(admission.protocol === "durable_v2" ? "Durable v2 (permanent)" : "Legacy v1")}</p> : null}
          <p className="text-xs text-muted-foreground">
            {t("Pausing only blocks new submissions. Existing tasks, logs, and cancellation remain independently governed.")}
          </p>
        </div>
        <div className="flex items-center gap-2">
          {admission?.paused && admission.protocol === "legacy_v1" ? <Button size="sm" variant="outline" disabled={busy} onClick={() => setActivateV2Open(true)}>{t("Enable v2 permanently")}</Button> : null}
          {admission ? <Button size="sm" variant="outline" disabled={busy} onClick={() => setPending(admission.paused ? "resume" : "pause")}>
            {t(admission.paused ? "Resume submissions" : "Pause submissions")}
          </Button> : null}
        </div>
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
            <TableHead>{t("Task ID")}</TableHead><TableHead>{t("Evidence phase")}</TableHead><TableHead>{t("Activity")}</TableHead><TableHead>{t("Recovery action")}</TableHead>
          </TableRow></TableHeader>
          <TableBody>{inspection.evidence.records.map(record => <TableRow key={record.taskId}>
            <TableCell className="font-mono text-xs" title={record.fingerprint}>{record.taskId}</TableCell>
            <TableCell>{record.phase}</TableCell><TableCell>{record.activity}</TableCell>
            <TableCell>{record.recoverable && admission?.paused ?
              <Button size="sm" variant="outline" disabled={busy} onClick={() => {setResolveTarget(record);setReviewReason("");}}>{t("Review prelaunch resolution")}</Button>:
              <span className="text-xs text-muted-foreground">{t("Not eligible for automatic resolution")}</span>}</TableCell>
          </TableRow>)}</TableBody>
        </Table> : <EmptyPanelState><p className="text-sm text-muted-foreground">{t("No SSH Jobs to inspect.")}</p></EmptyPanelState>}
      </> : null}
    </DataPanel>
    <Dialog open={activateV2Open} onOpenChange={open => {if (!busy) setActivateV2Open(open);}}>
      <DialogContent><DialogHeader>
        <DialogTitle>{t("Permanently enable SSH Jobs v2")}</DialogTitle>
        <DialogDescription>{t("The pinned SSH host must support durable fsync and execution fencing. Once enabled, future jobs will use v2 across restarts. Legacy records remain unchanged and there is no downgrade. Submissions stay paused until you resume them.")}</DialogDescription>
      </DialogHeader>
      <DialogFooter>
        <Button variant="outline" disabled={busy} onClick={() => setActivateV2Open(false)}>{t("Cancel")}</Button>
        <Button disabled={busy || !admission?.paused || admission.protocol !== "legacy_v1"} onClick={() => void activateV2()}>{t("Confirm permanent v2 activation")}</Button>
      </DialogFooter></DialogContent>
    </Dialog>
    <Dialog open={resolveTarget !== null} onOpenChange={open => {if (!open && !busy) setResolveTarget(null);}}>
      <DialogContent><DialogHeader>
        <DialogTitle>{t("Resolve verified prelaunch job")}</DialogTitle>
        <DialogDescription>{t("Only a verified v2 prelaunch candidate may be resolved. This does not mark the command as completed or failed. Unknown v1 tasks remain blocked.")}</DialogDescription>
      </DialogHeader>
      {resolveTarget ? <div className="space-y-3">
        <p className="font-mono text-xs break-all">{resolveTarget.taskId}</p>
        <p className="font-mono text-xs text-muted-foreground break-all">{resolveTarget.fingerprint}</p>
        <label htmlFor="ssh-job-recovery-reason" className="text-sm font-medium">{t("Review reason")}</label>
        <Textarea id="ssh-job-recovery-reason" value={reviewReason} maxLength={512}
          onChange={event => setReviewReason(event.target.value)} placeholder={t("Explain the verified prelaunch evidence")} />
      </div> : null}
      <DialogFooter><Button variant="outline" disabled={busy} onClick={() => setResolveTarget(null)}>{t("Cancel")}</Button>
        <Button disabled={busy || reviewReason.trim().length < 5} onClick={() => void resolvePrelaunch()}>{t("Confirm prelaunch resolution")}</Button>
      </DialogFooter></DialogContent>
    </Dialog>
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
