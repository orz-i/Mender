import { useEffect, useState } from "react";
import {
  ArrowRightIcon,
  BoxIcon,
  CloudIcon,
  ContainerIcon,
  FolderIcon,
  FolderOpenIcon,
  LockKeyholeIcon,
  ServerIcon,
} from "lucide-react";
import { Link, useNavigate, useSearchParams } from "react-router";

import type { CachedDockerImage, Credential, MenderClient } from "@mender/sdk";
import { Button } from "@mender/ui/components/button";
import { Input } from "@mender/ui/components/input";

import { t } from "../../i18n/i18n";
import { errorMessage } from "../../shared/errors";
import { dockerStatusText } from "../../shared/docker-status";
import { ErrorNotice } from "../../shared/page";
import { IconButton } from "../../shared/icon-button";
import { PageToolbar } from "../../shared/page-toolbar";
import { DataPanel } from "../../shared/workbench";

type Backend = "local_workspace" | "docker" | "daytona" | "ssh_remote";
type CreationStep = "backend" | "configure" | "limits" | "review";

const IMAGE_DIGEST = /^[a-z0-9]+(?:[.-][a-z0-9]+)+(?:\/[a-z0-9]+(?:[._-][a-z0-9]+)*){2,}@sha256:[0-9a-f]{64}$/;

function ResourceField({
  label,
  value,
  onChange,
  min,
  max,
}: {
  label: string;
  value: number;
  onChange: (next: number) => void;
  min: number;
  max: number;
}) {
  return (
    <label className="grid gap-2 text-sm">
      <span className="font-medium">{t(label)}</span>
      <Input
        type="number"
        min={min}
        max={max}
        value={value}
        onChange={(event) => onChange(Number(event.target.value))}
        aria-label={t(label)}
      />
    </label>
  );
}

function ReviewField({ label, value }: { label: string; value: string | number }) {
  return (
    <div className="min-w-0 border-b border-border/65 pb-3">
      <dt className="text-xs font-medium text-muted-foreground">{t(label)}</dt>
      <dd className="mt-1 break-all text-sm font-medium text-foreground">{value}</dd>
    </div>
  );
}

function BackendChoice({
  label,
  description,
  icon: Icon,
  onClick,
  disabledReason,
}: {
  label: string;
  description: string;
  icon: typeof BoxIcon;
  onClick?: () => void;
  disabledReason?: string;
}) {
  const inner = (
    <>
      <span className="flex size-11 items-center justify-center rounded-xl bg-foreground text-background dark:bg-muted dark:text-foreground">
        <Icon className="size-5" aria-hidden="true" />
      </span>
      <span className="mt-4 flex items-center justify-between gap-3 font-semibold">
        <span>{label}</span>
        {onClick ? <ArrowRightIcon className="size-4 shrink-0" aria-hidden="true" /> : null}
      </span>
      <span className="mt-2 text-xs leading-5 text-muted-foreground">{description}</span>
      {disabledReason ? (
        <span className="mt-2 break-words text-xs leading-5 text-muted-foreground">{disabledReason}</span>
      ) : null}
    </>
  );
  const classes =
    "flex min-h-44 min-w-0 flex-col items-start rounded-[20px] border border-border bg-background/60 p-5 text-left text-sm transition-[border-color,box-shadow,background-color]";

  if (!onClick) {
    return (
      <div aria-disabled="true" className={classes + " opacity-60"}>
        {inner}
      </div>
    );
  }
  return (
    <button
      type="button"
      onClick={onClick}
      className={classes + " hover:border-primary hover:bg-card hover:shadow-sm focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"}
    >
      {inner}
    </button>
  );
}

function CreationProgress({
  step,
  selected,
}: {
  step: CreationStep;
  selected: Backend;
}) {
  const steps: Array<{ value: CreationStep; label: string }> = [
    { value: "backend", label: "Backend" },
    { value: "configure", label: "Configuration" },
    ...(selected === "local_workspace" || selected === "ssh_remote" ? [] : [{ value: "limits" as const, label: "Resource limits" }]),
    { value: "review", label: "Review" },
  ];

  return (
    <ol aria-label={t("Creation steps")} className="flex flex-wrap gap-x-5 gap-y-3 border-b border-border/70 pb-5">
      {steps.map((entry, index) => {
        const active = entry.value === step;
        const complete = steps.findIndex((item) => item.value === step) > index;
        return (
          <li
            key={entry.value}
            aria-current={active ? "step" : undefined}
            className={[
              "inline-flex items-center gap-2 text-xs font-medium",
              active ? "text-foreground" : "text-muted-foreground",
            ].join(" ")}
          >
            <span
              className={[
                "flex size-7 items-center justify-center rounded-full border text-xs tabular-nums",
                active ? "border-primary bg-primary text-primary-foreground" :
                  complete ? "border-accent bg-accent text-accent-foreground" : "border-border bg-background",
              ].join(" ")}
            >
              {index + 1}
            </span>
            {t(entry.label)}
          </li>
        );
      })}
    </ol>
  );
}

function daytonaStatus(value: string): string {
  switch (value) {
    case "Experimental Daytona disabled; local operator must set MENDER_DAYTONA_EXPERIMENTAL=enabled before Runtime startup":
      return t("Experimental Daytona disabled; local operator must set MENDER_DAYTONA_EXPERIMENTAL=enabled before Runtime startup");
    case "Experimental Daytona enabled by operator; credential and live provider validation required; may incur charges":
      return t("Experimental Daytona enabled by operator; credential and live provider validation required; may incur charges");
    default:
      return value;
  }
}

/** Sandbox creation is explicit and progressive: selection → identity → limits → authority review. */
export function CreateSandboxPage({
  client,
  chooseLocalDirectory,
}: {
  client: MenderClient;
  chooseLocalDirectory?: () => Promise<string | null>;
}) {
  const navigate=useNavigate();
  const [searchParams]=useSearchParams();
  const chosenBuiltin=searchParams.get("pluginId");
  const fixedBackend:Backend|undefined=
    chosenBuiltin==="io.mender.sandbox.local-workspace"?"local_workspace":
    chosenBuiltin==="io.mender.sandbox.docker"?"docker":
    chosenBuiltin==="io.mender.sandbox.ssh-remote"?"ssh_remote":
    chosenBuiltin==="io.mender.sandbox.daytona"?"daytona":undefined;
  const [step,setStep]=useState<CreationStep>(fixedBackend?"configure":"backend");
  const [selected,setSelected]=useState<Backend>(fixedBackend??"local_workspace");
  const [name, setName] = useState("");
  const [root, setRoot] = useState("");
  const [pickingDirectory, setPickingDirectory] = useState(false);
  const [isolation, setIsolation] = useState<"" | "host" | "restricted_host">("");
  const [restrictedAvailable, setRestrictedAvailable] = useState(false);
  const [dockerAvailable, setDockerAvailable] = useState(false);
  const [dockerStatus, setDockerStatus] = useState("");
  const [daytonaConfigured, setDaytonaConfigured] = useState(false);
  const [daytonaMessage, setDaytonaMessage] = useState("");
  const [credentials, setCredentials] = useState<Credential[]>([]);
  const [daytonaCredential, setDaytonaCredential] = useState("");
  const [sshHost, setSSHHost] = useState("");
  const [sshPort, setSSHPort] = useState(22);
  const [sshUser, setSSHUser] = useState("");
  const [sshRoot, setSSHRoot] = useState("");
  const [sshHostPublicKey, setSSHHostPublicKey] = useState("");
  const [sshCredentialRef, setSSHCredentialRef] = useState("");
  const [daytonaCPU, setDaytonaCPU] = useState(1);
  const [daytonaMemory, setDaytonaMemory] = useState(2);
  const [daytonaDisk, setDaytonaDisk] = useState(3);
  const [daytonaTTL, setDaytonaTTL] = useState(30);
  const [imageDigest, setImageDigest] = useState("");
  const [dockerImages, setDockerImages] = useState<CachedDockerImage[]>([]);
  const [dockerImagesLoading, setDockerImagesLoading] = useState(true);
  const [dockerImagesIssue, setDockerImagesIssue] = useState("");
  const [userId, setUserId] = useState(10001);
  const [groupId, setGroupId] = useState(10001);
  const [cpuMilli, setCpuMilli] = useState(1000);
  const [memoryMiB, setMemoryMiB] = useState(512);
  const [workspaceMiB, setWorkspaceMiB] = useState(64);
  const [pids, setPids] = useState(64);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    let active = true;
    void client.getRuntimeGovernance()
      .then((info) => { if (active) setRestrictedAvailable(info.backend.restrictedHostAvailable); })
      .catch((reason: unknown) => { if (active) setError(errorMessage(reason)); });
    void client.getSandboxBackends()
      .then((info) => {
        if (!active) return;
        setDockerAvailable(info.dockerAvailable);
        setDockerStatus(info.dockerStatus);
        setDaytonaConfigured(info.daytonaConfigured);
        setDaytonaMessage(info.daytonaStatus);
      })
      .catch((reason: unknown) => { if (active) setDockerStatus(errorMessage(reason)); });
    void client.listCredentials()
      .then((result) => { if (active) setCredentials(result.credentials); })
      .catch((reason: unknown) => { if (active) setError(errorMessage(reason)); });
    return () => { active = false; };
  }, [client]);

  // Observe loss/recovery as well as initial discovery; polling is read-only.
  // A daemon outage disables provisioning even if the form is already open.
  useEffect(() => {
    let active = true;
    let pending = false;
    const timer = window.setInterval(() => {
      if (pending) return;
      pending = true;
      void client.getSandboxBackends()
        .then((info) => {
          if (!active) return;
          setDockerAvailable(info.dockerAvailable);
          setDockerStatus(info.dockerStatus);
        })
        .catch((reason: unknown) => {
          if (active) {
            setDockerAvailable(false);
            setDockerStatus(errorMessage(reason));
          }
        })
        .finally(() => { pending = false; });
    }, 5000);
    return () => { active = false; window.clearInterval(timer); };
  }, [client]);

  useEffect(() => {
    if (!dockerAvailable) return;
    let active = true;
    void client.listDockerImages()
      .then((result) => {
        if (!active) return;
        setDockerImages(result.images);
        // If there is exactly one trusted digest, select it rather than
        // asking the user to copy a content hash from their Docker CLI.
        if (result.images.length === 1) {
          setImageDigest((current) => current || result.images[0]!.imageDigest);
        }
      })
      .catch((reason: unknown) => {
        if (active) setDockerImagesIssue(errorMessage(reason));
      })
      .finally(() => {
        if (active) setDockerImagesLoading(false);
      });
    return () => { active = false; };
  }, [client, dockerAvailable]);

  const digestOK = IMAGE_DIGEST.test(imageDigest);
  const credentialOK = credentials.some((item) => item.reference === daytonaCredential);
  const sshCredentials = credentials.filter((item) => item.namespace === "ssh" && item.source === "native");
  const sshValid = sshHost.length > 0 && sshHost.length <= 253 &&
    /^[A-Za-z0-9.:-]+$/.test(sshHost) &&
    /^[a-z_][a-z0-9_-]{0,62}$/.test(sshUser) && sshUser !== "root" &&
    Number.isInteger(sshPort) && sshPort >= 1 && sshPort <= 65535 &&
    sshRoot.startsWith("/") && sshRoot.length > 1 && sshRoot.length <= 1024 &&
    !sshRoot.split("/").some((part) => part === ".." || part === ".") &&
    sshHostPublicKey.trim().startsWith("ssh-ed25519 ") &&
    !sshHostPublicKey.includes("\n") &&
    sshCredentials.some((item) => item.reference === sshCredentialRef);
  const identityValid = Boolean(name.trim()) && (
    selected === "local_workspace" ? Boolean(root.trim() && isolation) :
      selected === "docker" ? dockerAvailable && digestOK :
        selected === "ssh_remote" ? sshValid :
          daytonaConfigured && digestOK && credentialOK
  );
  const dockerValid = dockerAvailable && digestOK &&
    userId >= 1000 && userId <= 65534 && groupId >= 1000 && groupId <= 65534 &&
    cpuMilli >= 100 && cpuMilli <= 8000 && memoryMiB >= 256 && memoryMiB <= 16384 &&
    workspaceMiB >= 64 && workspaceMiB <= 2048 && workspaceMiB <= memoryMiB &&
    pids >= 16 && pids <= 512;
  const daytonaValid = daytonaConfigured && digestOK && credentialOK &&
    daytonaCPU >= 1 && daytonaCPU <= 2 && daytonaMemory >= 1 && daytonaMemory <= 4 &&
    daytonaDisk >= 3 && daytonaDisk <= 10 && daytonaTTL >= 15 && daytonaTTL <= 120;
  const formValid = identityValid &&
    (selected === "local_workspace" || selected === "ssh_remote" ? true :
      selected === "docker" ? dockerValid : daytonaValid);

  function chooseBackend(next: Backend) {
    setSelected(next);
    setStep("configure");
    setError("");
  }

  async function pickLocalDirectory() {
    if (!chooseLocalDirectory || pickingDirectory) return;
    setPickingDirectory(true);
    setError("");
    try {
      const directory = await chooseLocalDirectory();
      if (directory) setRoot(directory);
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setPickingDirectory(false);
    }
  }

  async function create() {
    if (!formValid || step !== "review" || busy) return;
    setBusy(true);
    setError("");
    try {
      const sandbox = selected === "docker"
        ? await client.createDockerSandbox({ name: name.trim(), imageDigest, userId, groupId, cpuMilli, memoryMiB, workspaceMiB, pids })
        : selected === "ssh_remote"
          ? await client.createSSHRemoteSandbox({
              name: name.trim(), host: sshHost.trim(), port: sshPort, user: sshUser,
              root: sshRoot.trim(), hostPublicKey: sshHostPublicKey.trim(), credentialRef: sshCredentialRef,
            })
        : selected === "daytona"
          ? await client.createDaytonaSandbox({ name: name.trim(), credentialRef: daytonaCredential, imageDigest, target: "us", cpu: daytonaCPU, memoryGiB: daytonaMemory, diskGiB: daytonaDisk, ttlMinutes: daytonaTTL })
          : await client.createLocalSandbox({ name: name.trim(), root: root.trim(), isolation: isolation as "host" | "restricted_host" });
      void navigate(`/sandboxes/${sandbox.sandboxId}`);
    } catch (reason) {
      setError(errorMessage(reason));
      setBusy(false);
    }
  }

  return (
    <>
      <PageToolbar actions={
        <Button variant="outline" onClick={() => void navigate("/sandboxes")}>{t("Back to sandboxes")}</Button>
      } />
      {error ? <ErrorNotice message={error} /> : null}
      <DataPanel
        title="Create sandbox"
        description="Choose a backend, configure isolation, then review the Sandbox authority before provisioning."
      >
        <div className="mx-auto flex max-w-4xl flex-col gap-6 p-5 sm:p-7">
          <CreationProgress step={step} selected={selected} />

          {step === "backend" ? (
            <section aria-label={t("Choose backend")} className="grid gap-4 sm:grid-cols-2">
              <BackendChoice label={t("Local workspace")}
                icon={FolderIcon}
                description={t("Host and restricted-host execution. Not a container or VM.")}
                onClick={() => chooseBackend("local_workspace")} />
              <BackendChoice label={t("Docker")} icon={ContainerIcon}
                description={t("Isolated, locally cached immutable image; no network, no host mounts.")}
                {...(dockerAvailable ? { onClick: () => chooseBackend("docker") } : {
                  disabledReason: dockerStatus ? dockerStatusText(dockerStatus) : t("Configure MENDER_DOCKER_SOCKET on the Runtime host."),
                })} />
              <BackendChoice label={t("SSH Remote")} icon={ServerIcon}
                description={t("Connect to an SSH account with a pinned host key. SSH does not isolate remote processes or files.")}
                onClick={() => chooseBackend("ssh_remote")} />
              <BackendChoice label={t("Daytona") + " · " + t("Experimental")} icon={CloudIcon}
                description={t("Remote Daytona Sandbox uses a Mender credential and incurs provider charges. Provider availability is verified on creation.")}
                {...(daytonaConfigured ? { onClick: () => chooseBackend("daytona") } : {
                  disabledReason: daytonaStatus(daytonaMessage),
                })} />
              <BackendChoice label={t("E2B") + " · " + t("Planned")} icon={BoxIcon}
                description={t("Backend not yet implemented")} />
            </section>
          ) : null}

          {step === "configure" ? (
            <section aria-label={t("Configuration")} className="flex max-w-2xl flex-col gap-5">
              {selected === "docker" && !dockerAvailable ? (
                <p role="status" className="text-sm text-muted-foreground">
                  {dockerStatusText(dockerStatus || "Docker Engine not detected")}
                </p>
              ) : null}
              <label className="grid gap-2 text-sm">
                <span className="font-medium">{t("Sandbox name")}</span>
                <Input aria-label={t("Sandbox name")} value={name} maxLength={160}
                  onChange={(event) => setName(event.target.value)} />
              </label>

              {selected === "local_workspace" ? (
                <>
                  <div className="grid gap-2 text-sm">
                    <label htmlFor="local-sandbox-root" className="font-medium">{t("Absolute local directory")}</label>
                    <div className="relative">
                      <Input id="local-sandbox-root" value={root}
                        className={chooseLocalDirectory ? "pr-12" : undefined}
                        onChange={(event) => setRoot(event.target.value)}
                        placeholder={t("/Users/example/project")} />
                      {chooseLocalDirectory ? (
                        <div className="absolute inset-y-0 right-1 flex items-center">
                          <IconButton type="button" variant="ghost" size="icon-sm"
                            label={t("Choose local directory")}
                            disabled={pickingDirectory || busy}
                            onClick={() => void pickLocalDirectory()}>
                            <FolderOpenIcon aria-hidden="true" />
                          </IconButton>
                        </div>
                      ) : null}
                    </div>
                  </div>
                  <label className="grid gap-2 text-sm">
                    <span className="font-medium">{t("Isolation")}</span>
                    <select aria-label={t("Isolation")} value={isolation}
                      onChange={(event) => setIsolation(event.target.value as typeof isolation)}
                      className="h-10 w-full rounded-xl border border-input bg-card px-3 text-sm">
                      <option value="">{t("Choose isolation")}</option>
                      <option value="restricted_host" disabled={!restrictedAvailable}>
                        {t("Restricted host")}{!restrictedAvailable ? " · " + t("Unavailable") : ""}
                      </option>
                      <option value="host">{t("Host (not isolated)")}</option>
                    </select>
                  </label>
                  <p className="text-xs leading-5 text-muted-foreground">
                    {t("Isolation choice is persisted per Sandbox; changing global Runtime settings will not weaken it.")}
                  </p>
                </>
              ) : selected === "ssh_remote" ? (
                <>
                  <div className="grid gap-4 sm:grid-cols-2">
                    <label className="grid gap-2 text-sm">
                      <span className="font-medium">{t("SSH host")}</span>
                      <Input aria-label={t("SSH host")} value={sshHost} maxLength={253}
                        placeholder={t("server.example.com")}
                        onChange={(event) => setSSHHost(event.target.value.trim())}/>
                    </label>
                    <ResourceField label="SSH port" value={sshPort} onChange={setSSHPort} min={1} max={65535} />
                    <label className="grid gap-2 text-sm">
                      <span className="font-medium">{t("SSH login user")}</span>
                      <Input aria-label={t("SSH login user")} value={sshUser}
                        onChange={(event) => setSSHUser(event.target.value.trim())}/>
                    </label>
                    <label className="grid gap-2 text-sm">
                      <span className="font-medium">{t("Remote workspace root")}</span>
                      <Input aria-label={t("Remote workspace root")} value={sshRoot} placeholder={t("/home/agent/project")}
                        onChange={(event) => setSSHRoot(event.target.value)}/>
                    </label>
                  </div>
                  <label className="grid gap-2 text-sm">
                    <span className="font-medium">{t("Pinned SSH host public key")}</span>
                    <textarea aria-label={t("Pinned SSH host public key")} rows={3} spellCheck={false}
                      value={sshHostPublicKey}
                      onChange={(event) => setSSHHostPublicKey(event.target.value)}
                      placeholder={t("ssh-ed25519 AAAA...")}
                      className="w-full resize-y rounded-xl border border-input bg-card p-3 font-mono text-xs"/>
                  </label>
                  <label className="grid gap-2 text-sm">
                    <span className="font-medium">{t("SSH private key credential")}</span>
                    <select aria-label={t("SSH private key credential")} value={sshCredentialRef}
                      onChange={(event) => setSSHCredentialRef(event.target.value)}
                      className="h-10 w-full rounded-xl border border-input bg-card px-3 text-sm">
                      <option value="">{t("Choose a credential")}</option>
                      {sshCredentials.map((item) => (
                        <option key={item.reference} value={item.reference}>
                          {item.displayName}{" · "}{item.reference}
                        </option>
                      ))}
                    </select>
                  </label>
                  {sshCredentials.length === 0 ? (
                    <p className="text-xs leading-5 text-muted-foreground">
                      {t("No SSH key credentials. Add an SSH private key credential in Credentials first.")}{" "}
                      <Link to="/credentials" className="font-medium text-primary-ink underline-offset-4 hover:underline">
                        {t("Credentials")}
                      </Link>
                    </p>
                  ) : null}
                  <p className="text-xs leading-5 text-muted-foreground">
                    {t("SSH Remote is not container or VM isolation. The remote shell can access everything permitted to its login account. Only connect to a trusted host and unprivileged user.")}
                  </p>
                </>
              ) : (
                <>
                  {selected === "daytona" ? (
                    <>
                      <label className="grid gap-2 text-sm">
                        <span className="font-medium">{t("Mender Credential Reference")}</span>
                        <select aria-label={t("Mender Credential Reference")} value={daytonaCredential}
                          onChange={(event) => setDaytonaCredential(event.target.value)}
                          className="h-10 w-full rounded-xl border border-input bg-card px-3 text-sm">
                          <option value="">{t("Choose a credential")}</option>
                          {credentials.map((credential) => (
                            <option key={credential.reference} value={credential.reference}>
                              {credential.displayName} · {credential.reference}
                            </option>
                          ))}
                        </select>
                      </label>
                      {credentials.length === 0 ? (
                        <p className="text-xs leading-5 text-muted-foreground">
                          {t("No credentials available. Add a Daytona API credential first.")}{" "}
                          <Link to="/credentials" className="font-medium text-primary-ink underline-offset-4 hover:underline">
                            {t("Credentials")}
                          </Link>
                        </p>
                      ) : null}
                    </>
                  ) : null}
                  {selected === "docker" ? (
                    <>
                      <label className="grid gap-2 text-sm">
                        <span className="font-medium">{t("Cached Docker image")}</span>
                        <select aria-label={t("Cached Docker image")} value={
                          dockerImages.some((item) => item.imageDigest === imageDigest) ? imageDigest : ""
                        } onChange={(event) => setImageDigest(event.target.value)}
                          className="h-10 w-full rounded-xl border border-input bg-card px-3 text-sm">
                          <option value="">{t("Select a locally cached image")}</option>
                          {dockerImages.map((item) => (
                            <option value={item.imageDigest} key={item.imageDigest}>
                              {item.displayName}{" · sha256:"}{item.imageDigest.split("@sha256:")[1]?.slice(0, 12)}
                            </option>
                          ))}
                        </select>
                      </label>
                      <p className="text-xs leading-5 text-muted-foreground">
                        {dockerImagesLoading ? t("Checking cached Docker images…")
                          : dockerImagesIssue || (dockerImages.length === 0
                            ? t("No locally cached images with verified digests. Pull a suitable image in Docker first, then reopen this form.")
                            : t("Only locally verified immutable images can be used. Mender does not pull images."))}
                      </p>
                      <details className="rounded-xl border border-border/70 px-4 py-3 text-sm">
                        <summary className="cursor-pointer font-medium">{t("Enter image digest manually")}</summary>
                        <label className="mt-3 grid gap-2 text-sm">
                          <span className="font-medium">{t("Immutable image digest")}</span>
                          <Input aria-label={t("Immutable image digest")} value={imageDigest}
                            onChange={(event) => setImageDigest(event.target.value.trim())}
                            placeholder={t("docker.io/library/alpine@sha256:…")}
                            autoComplete="off" />
                        </label>
                      </details>
                    </>
                  ) : (
                    <>
                      <label className="grid gap-2 text-sm">
                        <span className="font-medium">{t("Immutable image digest")}</span>
                        <Input aria-label={t("Immutable image digest")} value={imageDigest}
                          onChange={(event) => setImageDigest(event.target.value.trim())}
                          placeholder={t("docker.io/library/python@sha256:…")}
                          autoComplete="off" />
                      </label>
                      <p className="text-xs leading-5 text-muted-foreground">
                        {t("Remote Daytona Sandbox uses a Mender credential and incurs provider charges. Provider availability is verified on creation.")}
                      </p>
                    </>
                  )}
                </>
              )}

              <div className="flex flex-wrap gap-2 border-t border-border/65 pt-5">
                <Button variant="outline" onClick={() => {
                  if(fixedBackend && chosenBuiltin){
                    void navigate("/plugins/"+encodeURIComponent(chosenBuiltin+"@builtin"));
                    return;
                  }
                  setStep("backend");
                }}>{t("Back")}</Button>
                <Button disabled={!identityValid} onClick={() => setStep(selected === "local_workspace" || selected === "ssh_remote" ? "review" : "limits")}>
                  {t(selected === "local_workspace" || selected === "ssh_remote" ? "Review" : "Next")}
                  <ArrowRightIcon data-icon="inline-end" />
                </Button>
              </div>
            </section>
          ) : null}

          {step === "limits" && selected !== "local_workspace" && selected !== "ssh_remote" ? (
            <section aria-label={t("Resource limits")} className="flex max-w-2xl flex-col gap-5">
              <div className="grid gap-4 sm:grid-cols-2">
                {selected === "docker" ? (
                  <>
                    <ResourceField label="User ID (non-root)" value={userId} onChange={setUserId} min={1000} max={65534} />
                    <ResourceField label="Group ID (non-root)" value={groupId} onChange={setGroupId} min={1000} max={65534} />
                    <ResourceField label="CPU (millicores)" value={cpuMilli} onChange={setCpuMilli} min={100} max={8000} />
                    <ResourceField label="Memory (MiB)" value={memoryMiB} onChange={setMemoryMiB} min={256} max={16384} />
                    <ResourceField label="Workspace tmpfs (MiB)" value={workspaceMiB} onChange={setWorkspaceMiB} min={64} max={2048} />
                    <ResourceField label="PID limit" value={pids} onChange={setPids} min={16} max={512} />
                  </>
                ) : (
                  <>
                    <ResourceField label="Remote CPUs" value={daytonaCPU} onChange={setDaytonaCPU} min={1} max={2} />
                    <ResourceField label="Remote memory (GiB)" value={daytonaMemory} onChange={setDaytonaMemory} min={1} max={4} />
                    <ResourceField label="Remote disk (GiB)" value={daytonaDisk} onChange={setDaytonaDisk} min={3} max={10} />
                    <ResourceField label="TTL (minutes)" value={daytonaTTL} onChange={setDaytonaTTL} min={15} max={120} />
                  </>
                )}
              </div>
              <p className="text-xs leading-5 text-muted-foreground">
                {selected === "docker"
                  ? t("Docker execution uses read-only rootfs, non-root user, no network or host mounts. Workspace files are ephemeral across restarts.")
                  : t("Daytona network egress is blocked. Remote sandbox TTL limits wall-clock lifetime, NOT monetary charges. Creating a sandbox may incur provider fees.")}
              </p>
              <div className="flex flex-wrap gap-2 border-t border-border/65 pt-5">
                <Button variant="outline" onClick={() => setStep("configure")}>{t("Back")}</Button>
                <Button disabled={!formValid} onClick={() => setStep("review")}>{t("Review")}</Button>
              </div>
            </section>
          ) : null}

          {step === "review" ? (
            <section aria-label={t("Review Sandbox authority")} className="flex max-w-3xl flex-col gap-5">
              <p className="text-sm font-semibold">{t("Review Sandbox authority")}</p>
              <dl className="grid gap-x-6 gap-y-4 rounded-2xl border border-border/70 bg-background/60 p-5 sm:grid-cols-2">
                <ReviewField label="Name" value={name.trim()} />
                <ReviewField label="Backend" value={selected === "docker" ? t("Docker") : selected === "ssh_remote" ? t("SSH Remote") : selected === "daytona" ? t("Daytona") : t("Local workspace")} />
                {selected === "local_workspace" ? (
                  <>
                    <ReviewField label="Workspace" value={root.trim()} />
                    <ReviewField label="Isolation" value={isolation === "host" ? t("Host (not isolated)") : t("Restricted host")} />
                  </>
                ) : selected === "ssh_remote" ? (
                  <>
                    <ReviewField label="SSH host" value={sshHost}/>
                    <ReviewField label="SSH port" value={sshPort}/>
                    <ReviewField label="SSH login user" value={sshUser}/>
                    <ReviewField label="Remote workspace root" value={sshRoot}/>
                    <ReviewField label="SSH private key credential" value={sshCredentialRef}/>
                    <ReviewField label="Pinned SSH host public key" value={sshHostPublicKey}/>
                    <ReviewField label="Isolation" value={t("Remote account (not isolated)")}/>
                  </>
                ) : (
                  <>
                    <ReviewField label="Immutable image digest" value={imageDigest} />
                    {selected === "docker" ? (
                      <>
                        <ReviewField label="Isolation" value={t("Container")} />
                        <ReviewField label="CPU (millicores)" value={cpuMilli} />
                        <ReviewField label="Memory (MiB)" value={memoryMiB} />
                        <ReviewField label="Workspace tmpfs (MiB)" value={workspaceMiB} />
                        <ReviewField label="PID limit" value={pids} />
                        <ReviewField label="User ID (non-root)" value={userId} />
                        <ReviewField label="Group ID (non-root)" value={groupId} />
                      </>
                    ) : (
                      <>
                        <ReviewField label="Mender Credential Reference" value={daytonaCredential} />
                        <ReviewField label="Remote CPUs" value={daytonaCPU} />
                        <ReviewField label="Remote memory (GiB)" value={daytonaMemory} />
                        <ReviewField label="Remote disk (GiB)" value={daytonaDisk} />
                        <ReviewField label="TTL (minutes)" value={daytonaTTL} />
                      </>
                    )}
                  </>
                )}
              </dl>
              <div role="note" className="flex items-start gap-3 rounded-xl border border-border bg-background p-4 text-xs leading-5">
                <LockKeyholeIcon className="mt-0.5 size-4 shrink-0 text-primary-ink" aria-hidden="true" />
                <p>
                  {selected === "local_workspace"
                    ? t(isolation === "host"
                        ? "Host mode is not isolated and uses the Runtime user's local file and process permissions."
                        : "Isolation choice is persisted per Sandbox; changing global Runtime settings will not weaken it.")
                    : selected === "docker"
                      ? t("Docker Workspace files are ephemeral; stopping and starting recreates the container and erases scratch files.")
                      : selected === "ssh_remote"
                        ? t("SSH Remote is not container or VM isolation. The remote shell can access everything permitted to its login account. Only connect to a trusted host and unprivileged user.")
                        : t("Creating a Daytona Sandbox can incur provider charges. TTL is not a USD spending cap. Maximum 2 active remote sandboxes per Principal.")}
                </p>
              </div>
              <div className="flex flex-wrap gap-2 border-t border-border/65 pt-5">
                <Button variant="outline" disabled={busy} onClick={() => setStep(selected === "local_workspace" || selected === "ssh_remote" ? "configure" : "limits")}>
                  {t("Back")}
                </Button>
                <Button disabled={busy || !formValid} onClick={() => void create()}>
                  {busy ? t("Creating…") : t("Create sandbox")}
                </Button>
              </div>
            </section>
          ) : null}
        </div>
      </DataPanel>
    </>
  );
}
