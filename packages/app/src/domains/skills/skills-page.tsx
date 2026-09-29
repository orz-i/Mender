import { useCallback, useEffect, useMemo, useState } from "react";
import {
  EyeIcon,
  PowerIcon,
  PowerOffIcon,
  ShieldCheckIcon,
  ShieldOffIcon,
  Trash2Icon,
  UploadIcon,
} from "lucide-react";

import type {
  MenderClient,
  SkillDeployment,
  SkillInspection,
  SkillInstallation,
} from "@mender/sdk";
import { Badge } from "@mender/ui/components/badge";
import { Button } from "@mender/ui/components/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@mender/ui/components/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@mender/ui/components/dialog";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@mender/ui/components/table";

import { formatDateTime, formatNumber, t } from "../../i18n/i18n";
import { errorMessage } from "../../shared/errors";
import { IconButton } from "../../shared/icon-button";
import { EmptyState, ErrorNotice, PageHeader } from "../../shared/page";
import { usePagination } from "../../shared/pagination";
import { TablePagination } from "../../shared/table-pagination";

type DeploySkill = (
  installationId: string,
  target: SkillDeployment["target"],
) => Promise<SkillDeployment | null>;

const deploymentTargets: Array<{
  value: SkillDeployment["target"];
  label: string;
}> = [
  { value: "agents-project", label: "Agents project" },
  { value: "github-project", label: "GitHub project" },
  { value: "claude-project", label: "Claude project" },
  { value: "agents-user", label: "Agents user" },
  { value: "copilot-user", label: "Copilot user" },
];

export function SkillsPage({
  client,
  installSkill,
  deploySkill,
}: {
  client: MenderClient;
  installSkill?: () => Promise<SkillInstallation | null>;
  deploySkill?: DeploySkill;
}) {
  const [installations, setInstallations] = useState<SkillInstallation[]>([]);
  const [deployments, setDeployments] = useState<SkillDeployment[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");

  const [inspection, setInspection] = useState<SkillInspection | null>(null);
  const [deploying, setDeploying] = useState<SkillInstallation | null>(null);
  const [deploymentTarget, setDeploymentTarget] =
    useState<SkillDeployment["target"]>("agents-project");

  const refresh = useCallback(async () => {
    try {
      const [skillResult, deploymentResult] = await Promise.all([
        client.listSkills(),
        client.listSkillDeployments(),
      ]);
      setInstallations(skillResult.installations);
      setDeployments(deploymentResult.deployments);
      setError("");
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setLoaded(true);
    }
  }, [client]);

  useEffect(() => {
    let active = true;
    void Promise.all([client.listSkills(), client.listSkillDeployments()])
      .then(([skillResult, deploymentResult]) => {
        if (!active) {
          return;
        }
        setInstallations(skillResult.installations);
        setDeployments(deploymentResult.deployments);
        setError("");
        setLoaded(true);
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

  const installationByID = useMemo(
    () =>
      new Map(
        installations.map((item) => [item.installationId, item] as const),
      ),
    [installations],
  );
  const skillPagination = usePagination(installations);
  const deploymentPagination = usePagination(deployments);

  async function install() {
    if (installSkill === undefined) {
      return;
    }
    setBusy("install");
    setError("");
    try {
      const installed = await installSkill();
      if (installed !== null) {
        await refresh();
      }
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy("");
    }
  }

  async function view(item: SkillInstallation) {
    setBusy("view:" + item.installationId);
    setError("");
    try {
      setInspection(await client.getSkillInstallation(item.installationId));
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy("");
    }
  }

  async function toggleTrust(item: SkillInstallation) {
    setBusy("trust:" + item.installationId);
    setError("");
    try {
      if (item.trusted) {
        await client.untrustSkillInstallation(item.installationId);
      } else {
        await client.trustSkillInstallation(item.installationId);
      }
      await refresh();
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy("");
    }
  }

  async function toggleEnabled(item: SkillInstallation) {
    setBusy("enabled:" + item.installationId);
    setError("");
    try {
      if (item.enabled) {
        await client.disableSkillInstallation(item.installationId);
      } else {
        await client.enableSkillInstallation(item.installationId);
      }
      await refresh();
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy("");
    }
  }

  async function remove(item: SkillInstallation) {
    setBusy("remove:" + item.installationId);
    setError("");
    try {
      await client.deleteSkillInstallation(item.installationId);
      await refresh();
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy("");
    }
  }

  async function deploy() {
    if (deploying === null || deploySkill === undefined) {
      return;
    }
    setBusy("deploy:" + deploying.installationId);
    setError("");
    try {
      const deployed = await deploySkill(
        deploying.installationId,
        deploymentTarget,
      );
      if (deployed !== null) {
        setDeploying(null);
        await refresh();
      }
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy("");
    }
  }

  async function undeploy(item: SkillDeployment) {
    setBusy("undeploy:" + item.deploymentId);
    setError("");
    try {
      await client.undeploySkill(item.deploymentId);
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
        title="Skills"
        description="Manage portable Agent Skills, exact installations, trust, and deployments."
        actions={
          <div className="flex gap-2">
            <Button
              type="button"
              variant="outline"
              onClick={() => void refresh()}
              disabled={busy !== ""}
            >
              {t("Refresh")}
            </Button>
            {installSkill !== undefined ? (
              <Button
                type="button"
                onClick={() => void install()}
                disabled={busy !== ""}
              >
                {busy === "install" ? t("Installing…") : t("Install Skill")}
              </Button>
            ) : null}
          </div>
        }
      />

      {error ? <ErrorNotice message={error} /> : null}

      <Card>
        <CardHeader className="border-b">
          <CardTitle>{t("Skill Library")}</CardTitle>
          <CardDescription>
            {t(
              "Built-in Skills are immutable. Local installations must be trusted before they can be enabled or deployed.",
            )}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {!loaded ? (
            <p className="text-sm text-muted-foreground">{t("Loading…")}</p>
          ) : installations.length === 0 ? (
            <EmptyState
              title="No Skills"
              description="Install a local Agent Skill or use a built-in Skill."
            />
          ) : (
            <>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t("Skill")}</TableHead>
                    <TableHead>{t("Source")}</TableHead>
                    <TableHead>{t("Version")}</TableHead>
                    <TableHead>{t("Trust")}</TableHead>
                    <TableHead>{t("Enabled")}</TableHead>
                    <TableHead className="text-right">
                      {t("Actions")}
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {skillPagination.items.map((item) => (
                    <TableRow key={item.installationId}>
                      <TableCell>
                        <div className="min-w-0">
                          <p className="font-medium">{item.name}</p>
                          <p className="max-w-[28rem] truncate text-xs text-muted-foreground">
                            {item.description}
                          </p>
                        </div>
                      </TableCell>
                      <TableCell>
                        <Badge variant="outline">
                          {item.builtIn ? t("Built-in") : t("Local")}
                        </Badge>
                      </TableCell>
                      <TableCell>
                        {item.versionHint || t("Unversioned")}
                      </TableCell>
                      <TableCell>
                        <Badge
                          variant={item.trusted ? "outline" : "secondary"}
                        >
                          {item.trusted ? t("Trusted") : t("Untrusted")}
                        </Badge>
                      </TableCell>
                      <TableCell>
                        <Badge
                          variant={item.enabled ? "outline" : "secondary"}
                        >
                          {item.enabled ? t("Enabled") : t("Disabled")}
                        </Badge>
                      </TableCell>
                      <TableCell>
                        <div className="flex justify-end gap-1">
                          <IconButton
                            type="button"
                            variant="ghost"
                            label={t("View {{skill}}", { skill: item.name })}
                            disabled={busy !== ""}
                            onClick={() => void view(item)}
                          >
                            <EyeIcon />
                          </IconButton>
                          {deploySkill !== undefined && item.enabled ? (
                            <IconButton
                              type="button"
                              variant="ghost"
                              label={t("Deploy {{skill}}", {
                                skill: item.name,
                              })}
                              disabled={busy !== ""}
                              onClick={() => setDeploying(item)}
                            >
                              <UploadIcon />
                            </IconButton>
                          ) : null}
                          {!item.builtIn ? (
                            <>
                              <IconButton
                                type="button"
                                variant="ghost"
                                label={
                                  item.trusted
                                    ? t("Untrust {{skill}}", {
                                        skill: item.name,
                                      })
                                    : t("Trust {{skill}}", {
                                        skill: item.name,
                                      })
                                }
                                disabled={busy !== ""}
                                onClick={() => void toggleTrust(item)}
                              >
                                {item.trusted ? (
                                  <ShieldOffIcon />
                                ) : (
                                  <ShieldCheckIcon />
                                )}
                              </IconButton>
                              <IconButton
                                type="button"
                                variant="ghost"
                                label={
                                  item.enabled
                                    ? t("Disable {{skill}}", {
                                        skill: item.name,
                                      })
                                    : t("Enable {{skill}}", {
                                        skill: item.name,
                                      })
                                }
                                disabled={busy !== "" || !item.trusted}
                                onClick={() => void toggleEnabled(item)}
                              >
                                {item.enabled ? (
                                  <PowerOffIcon />
                                ) : (
                                  <PowerIcon />
                                )}
                              </IconButton>
                              <IconButton
                                type="button"
                                variant="ghost"
                                label={t("Delete {{skill}}", {
                                  skill: item.name,
                                })}
                                disabled={busy !== ""}
                                onClick={() => void remove(item)}
                              >
                                <Trash2Icon />
                              </IconButton>
                            </>
                          ) : null}
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
              <TablePagination label="Skills" pagination={skillPagination} />
            </>
          )}
        </CardContent>
      </Card>

      <Card className="mt-6">
        <CardHeader className="border-b">
          <CardTitle>{t("Deployments")}</CardTitle>
          <CardDescription>
            {t(
              "Deployments bind one exact Skill installation to an Agent host directory and report local drift.",
            )}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {deployments.length === 0 ? (
            <EmptyState
              title="No Skill deployments"
              description="Deploy a trusted Skill from Desktop or the CLI."
            />
          ) : (
            <>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t("Skill")}</TableHead>
                    <TableHead>{t("Target")}</TableHead>
                    <TableHead>{t("Drift")}</TableHead>
                    <TableHead>{t("Destination")}</TableHead>
                    <TableHead>{t("Deployed")}</TableHead>
                    <TableHead className="text-right">
                      {t("Actions")}
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {deploymentPagination.items.map((item) => (
                    <TableRow key={item.deploymentId}>
                      <TableCell>
                        {installationByID.get(item.installationId)?.name ??
                          item.installationId}
                      </TableCell>
                      <TableCell>{deploymentTargetLabel(item.target)}</TableCell>
                      <TableCell>
                        <Badge
                          variant={
                            item.drift === "current"
                              ? "outline"
                              : "secondary"
                          }
                        >
                          {deploymentDriftLabel(item.drift)}
                        </Badge>
                      </TableCell>
                      <TableCell>
                        <span
                          className="block max-w-[28rem] truncate font-mono text-xs"
                          title={item.destination}
                        >
                          {item.destination}
                        </span>
                      </TableCell>
                      <TableCell>{formatDateTime(item.deployedAt)}</TableCell>
                      <TableCell>
                        <div className="flex justify-end">
                          <IconButton
                            type="button"
                            variant="ghost"
                            label={t("Undeploy {{skill}}", {
                              skill:
                                installationByID.get(item.installationId)
                                  ?.name ?? item.installationId,
                            })}
                            disabled={busy !== ""}
                            onClick={() => void undeploy(item)}
                          >
                            <Trash2Icon />
                          </IconButton>
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
              <TablePagination
                label="Skill deployments"
                pagination={deploymentPagination}
              />
            </>
          )}
        </CardContent>
      </Card>

      <Dialog
        open={inspection !== null}
        onOpenChange={(open) => {
          if (!open) {
            setInspection(null);
          }
        }}
      >
        <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-3xl">
          {inspection !== null ? (
            <>
              <DialogHeader>
                <DialogTitle>{inspection.installation.name}</DialogTitle>
                <DialogDescription>
                  {t("Exact Skill installation and validated resource inventory.")}
                </DialogDescription>
              </DialogHeader>

              <div className="grid gap-3 sm:grid-cols-2">
                <DetailRow
                  label="Installation"
                  value={inspection.installation.installationId}
                  mono
                />
                <DetailRow
                  label="Skill ID"
                  value={inspection.installation.skillId}
                  mono
                />
                <DetailRow
                  label="Digest"
                  value={inspection.installation.digest}
                  mono
                />
                <DetailRow
                  label="Total size"
                  value={t("{{count}} bytes", {
                    count: formatNumber(inspection.totalBytes),
                  })}
                />
                <DetailRow
                  label="License"
                  value={inspection.metadata.license || t("Not declared")}
                />
                <DetailRow
                  label="Compatibility"
                  value={
                    inspection.metadata.compatibility || t("Not declared")
                  }
                />
              </div>

              {inspection.metadata.allowedTools ? (
                <div className="rounded-lg border bg-muted/30 p-3">
                  <p className="text-xs font-medium text-muted-foreground">
                    {t("Allowed tools metadata")}
                  </p>
                  <p className="mt-1 font-mono text-xs">
                    {inspection.metadata.allowedTools}
                  </p>
                  <p className="mt-2 text-xs text-muted-foreground">
                    {t(
                      "This metadata is informational only. Mender Policy and Profile governance remain authoritative.",
                    )}
                  </p>
                </div>
              ) : null}

              <div>
                <h3 className="mb-2 text-sm font-medium">
                  {t("Resources")}
                </h3>
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>{t("Path")}</TableHead>
                      <TableHead>{t("Kind")}</TableHead>
                      <TableHead>{t("Size")}</TableHead>
                      <TableHead>{t("Executable")}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {inspection.resources.map((resource) => (
                      <TableRow key={resource.path}>
                        <TableCell className="font-mono text-xs">
                          {resource.path}
                        </TableCell>
                        <TableCell>{resourceKindLabel(resource.kind)}</TableCell>
                        <TableCell>
                          {t("{{count}} bytes", {
                            count: formatNumber(resource.size),
                          })}
                        </TableCell>
                        <TableCell>
                          {resource.executable ? t("Yes") : t("No")}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            </>
          ) : null}
        </DialogContent>
      </Dialog>

      <Dialog
        open={deploying !== null}
        onOpenChange={(open) => {
          if (!open) {
            setDeploying(null);
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("Deploy Skill")}</DialogTitle>
            <DialogDescription>
              {deploying === null
                ? ""
                : t("Deploy exact installation {{installation}} of {{skill}}.", {
                    installation: deploying.installationId,
                    skill: deploying.name,
                  })}
            </DialogDescription>
          </DialogHeader>

          <label className="grid gap-2">
            <span className="text-sm font-medium">{t("Target")}</span>
            <select
              aria-label={t("Skill deployment target")}
              value={deploymentTarget}
              onChange={(event) =>
                setDeploymentTarget(
                  event.target.value as SkillDeployment["target"],
                )
              }
              className="h-9 rounded-lg border border-input bg-background px-2 text-sm text-foreground outline-none focus:ring-2 focus:ring-ring/40"
            >
              {deploymentTargets.map((item) => (
                <option key={item.value} value={item.value}>
                  {t(item.label)}
                </option>
              ))}
            </select>
          </label>

          <DialogFooter showCloseButton>
            <Button
              type="button"
              onClick={() => void deploy()}
              disabled={
                deploying === null ||
                busy === "deploy:" + deploying.installationId
              }
            >
              {deploying !== null &&
              busy === "deploy:" + deploying.installationId
                ? t("Deploying…")
                : t("Deploy")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

function DetailRow({
  label,
  value,
  mono = false,
}: {
  label: string;
  value: string;
  mono?: boolean;
}) {
  return (
    <div className="rounded-lg border bg-muted/20 p-3">
      <p className="text-xs font-medium text-muted-foreground">{t(label)}</p>
      <p
        className={[
          "mt-1 break-all text-sm",
          mono ? "font-mono text-xs" : "",
        ].join(" ")}
      >
        {value}
      </p>
    </div>
  );
}

function deploymentTargetLabel(target: SkillDeployment["target"]) {
  switch (target) {
    case "agents-project":
      return t("Agents project");
    case "github-project":
      return t("GitHub project");
    case "claude-project":
      return t("Claude project");
    case "agents-user":
      return t("Agents user");
    case "copilot-user":
      return t("Copilot user");
  }
}

function deploymentDriftLabel(drift: SkillDeployment["drift"]) {
  switch (drift) {
    case "current":
      return t("Current");
    case "drifted":
      return t("Drifted");
    case "missing":
      return t("Missing");
  }
}

function resourceKindLabel(
  kind: SkillInspection["resources"][number]["kind"],
) {
  switch (kind) {
    case "manifest":
      return t("Manifest");
    case "script":
      return t("Script");
    case "reference":
      return t("Reference");
    case "asset":
      return t("Asset");
    case "other":
      return t("Other");
  }
}
