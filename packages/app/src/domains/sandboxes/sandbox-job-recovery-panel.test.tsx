import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { MenderClient, Sandbox, SSHJobAdmissionState, SSHJobRecoveryView } from "@mender/sdk";
import { resetLocaleForTesting, setLocale } from "../../i18n/i18n";
import { SandboxJobRecoveryPanel } from "./sandbox-job-recovery-panel";

afterEach(() => { cleanup(); resetLocaleForTesting(); });

it("does not inspect SSH or change admissions until explicit owner actions", async () => {
  resetLocaleForTesting(); setLocale("en");
  const sandbox: Sandbox = {
    sandboxId: "sbx_00000000000000000000000000000099",
    name: "Controlled SSH", backend: "ssh_remote", isolation: "remote",
    state: "ready", ownerKind: "local_user", ownerRef: "fixture", localRoot: "",
    activeLeases: 0, backendAvailable: true,
  };
  const admission: SSHJobAdmissionState = { sandboxId: sandbox.sandboxId, generation: 1, paused: false, protocol: "legacy_v1" };
  const recovery: SSHJobRecoveryView = {
    sandboxId: sandbox.sandboxId, generation: 1, managed: true, paused: false,
    evidence: {
      inspectedAt: "2026-10-13T10:00:00Z", verdict: "uncertain",
      records: [{ taskId: "sjob_" + "a".repeat(32), phase: "manifest_only",
        state: "unknown", activity: "uncertain", fingerprint: "b".repeat(64), recoverable: false }],
    },
  };
  const getSSHJobAdmission = vi.fn().mockResolvedValue(admission);
  const getSSHJobRecovery = vi.fn().mockResolvedValue(recovery);
  const setSSHJobAdmission = vi.fn().mockResolvedValue({ ...admission, paused: true });
  render(<SandboxJobRecoveryPanel client={{ getSSHJobAdmission, getSSHJobRecovery, setSSHJobAdmission } as unknown as MenderClient} sandbox={sandbox} />);
  expect(await screen.findByText("Accepting")).toBeTruthy();
  expect(getSSHJobRecovery).not.toHaveBeenCalled();
  expect(setSSHJobAdmission).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "Inspect remote evidence" }));
  expect(await screen.findByText("manifest_only")).toBeTruthy();
  expect(getSSHJobRecovery).toHaveBeenCalledTimes(1);
  fireEvent.click(screen.getByRole("button", { name: "Pause submissions" }));
  expect(setSSHJobAdmission).not.toHaveBeenCalled();
  const confirmation = screen.getByRole("dialog", { name: "Pause new SSH Jobs" });
  fireEvent.click(within(confirmation).getByRole("button", { name: "Confirm" }));
  await waitFor(() => expect(setSSHJobAdmission).toHaveBeenCalledWith(sandbox.sandboxId, { paused: true }));
  expect(await screen.findByText("Paused")).toBeTruthy();
});

it("requires paused admissions and an explicit reviewed reason before v2 resolution", async () => {
  resetLocaleForTesting(); setLocale("en");
  const sandbox: Sandbox = {
    sandboxId: "sbx_" + "7".repeat(32), name: "R3b fixture SSH",
    backend: "ssh_remote", isolation: "remote", state: "ready", localRoot: "",
    ownerKind: "local_user", ownerRef: "fixture", activeLeases: 0, backendAvailable: true,
  };
  const record = {
    taskId: "sjob_" + "b".repeat(32), phase: "prelaunch_candidate" as const,
    state: "unknown", activity: "uncertain" as const, fingerprint: "a".repeat(64),
    recoverable: true,
  };
  const report: SSHJobRecoveryView = {
    sandboxId: sandbox.sandboxId, generation: 5, managed: true, paused: true,
    evidence: { inspectedAt: "2026-10-13T10:00:00Z", verdict: "uncertain", records: [record] },
  };
  const resolved: SSHJobRecoveryView = {
    ...report, evidence: { ...report.evidence, verdict: "quiescent_proved",
      records: [{ ...record, phase: "resolved_before_launch", activity: "quiescent_proved", recoverable: false }] },
  };
  const getSSHJobRecovery = vi.fn().mockResolvedValueOnce(report).mockResolvedValueOnce(resolved);
  const resolveSSHJobRecovery = vi.fn().mockResolvedValue({
    sandboxId: sandbox.sandboxId, generation: 5, evidence: resolved.evidence.records[0],
  });
  render(<SandboxJobRecoveryPanel client={{
    getSSHJobAdmission: vi.fn().mockResolvedValue({ sandboxId: sandbox.sandboxId, generation: 5, paused: true, protocol: "durable_v2" }),
    getSSHJobRecovery, resolveSSHJobRecovery,
  } as unknown as MenderClient} sandbox={sandbox} />);
  expect(await screen.findByText("Paused")).toBeTruthy();
  expect(resolveSSHJobRecovery).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "Inspect remote evidence" }));
  fireEvent.click(await screen.findByRole("button", { name: "Review prelaunch resolution" }));
  const modal = screen.getByRole("dialog", { name: "Resolve verified prelaunch job" });
  const confirm = within(modal).getByRole("button", { name: "Confirm prelaunch resolution" });
  expect(confirm.hasAttribute("disabled")).toBe(true);
  fireEvent.change(within(modal).getByLabelText("Review reason"), {
    target: { value: "Validated persisted prelaunch evidence" },
  });
  fireEvent.click(confirm);
  await waitFor(() => expect(resolveSSHJobRecovery).toHaveBeenCalledWith(
    sandbox.sandboxId, record.taskId, {
      fingerprint: record.fingerprint, generation: 5,
      reason: "Validated persisted prelaunch evidence",
    },
  ));
  expect(await screen.findByText("resolved_before_launch")).toBeTruthy();
  expect(getSSHJobRecovery).toHaveBeenCalledTimes(2);
});

it("permanently activates v2 only after pause and explicit owner confirmation", async () => {
  resetLocaleForTesting(); setLocale("en");
  const sandbox: Sandbox = {
    sandboxId: "sbx_" + "6".repeat(32), name: "Rocky v2 activation fixture",
    backend: "ssh_remote", isolation: "remote", state: "ready", localRoot: "",
    ownerKind: "local_user", ownerRef: "fixture", activeLeases: 0, backendAvailable: true,
  };
  const admission: SSHJobAdmissionState = {
    sandboxId: sandbox.sandboxId, generation: 1, paused: false, protocol: "legacy_v1",
  };
  const paused: SSHJobAdmissionState = {...admission, paused: true};
  const activated: SSHJobAdmissionState = {...paused, protocol: "durable_v2"};
  const getSSHJobAdmission = vi.fn().mockResolvedValue(admission);
  const setSSHJobAdmission = vi.fn().mockResolvedValue(paused);
  const activateSSHJobV2 = vi.fn().mockResolvedValue(activated);
  render(<SandboxJobRecoveryPanel client={{getSSHJobAdmission,setSSHJobAdmission,activateSSHJobV2} as unknown as MenderClient} sandbox={sandbox}/>);
  expect(await screen.findByText(/Job protocol: Legacy v1/)).toBeTruthy();
  expect(screen.queryByRole("button",{name:"Enable v2 permanently"})).toBeNull();
  fireEvent.click(screen.getByRole("button",{name:"Pause submissions"}));
  fireEvent.click(within(screen.getByRole("dialog",{name:"Pause new SSH Jobs"})).getByRole("button",{name:"Confirm"}));
  await waitFor(() => expect(setSSHJobAdmission).toHaveBeenCalledWith(sandbox.sandboxId,{paused:true}));
  fireEvent.click(screen.getByRole("button",{name:"Enable v2 permanently"}));
  expect(activateSSHJobV2).not.toHaveBeenCalled();
  fireEvent.click(within(screen.getByRole("dialog",{name:"Permanently enable SSH Jobs v2"})).getByRole("button",{name:"Confirm permanent v2 activation"}));
  await waitFor(() => expect(activateSSHJobV2).toHaveBeenCalledWith(sandbox.sandboxId,{confirm:true}));
  expect(await screen.findByText(/Job protocol: Durable v2 \(permanent\)/)).toBeTruthy();
  expect(screen.queryByRole("button",{name:"Enable v2 permanently"})).toBeNull();
  expect(screen.getByText("Paused")).toBeTruthy();
});
