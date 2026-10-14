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
  const admission: SSHJobAdmissionState = { sandboxId: sandbox.sandboxId, generation: 1, paused: false };
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
