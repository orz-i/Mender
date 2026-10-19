import { useCallback, useEffect, useMemo, useState } from "react";
import { RefreshCwIcon, Trash2Icon, UnplugIcon } from "lucide-react";

import type { Credential, MenderClient } from "@mender/sdk";
import { Badge } from "@mender/ui/components/badge";
import { Button } from "@mender/ui/components/button";
import {
  Dialog,
  DialogContent,
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

import { errorMessage } from "../../shared/errors";
import { IconButton } from "../../shared/icon-button";
import { EmptyState, ErrorNotice } from "../../shared/page";
import { PageToolbar, ResultPanel } from "../../shared/page-toolbar";
import { usePagination } from "../../shared/pagination";
import { TablePagination } from "../../shared/table-pagination";
import { EmptyPanelState, SearchField } from "../../shared/workbench";

import { t } from "../../i18n/i18n";
import {
  canonicalCredentialPart,
  credentialCreateError,
  credentialReferencePreview,
  ENVIRONMENT_VARIABLE_PATTERN,
  MAX_CREDENTIAL_DISPLAY_NAME_BYTES,
  MAX_NATIVE_SECRET_BYTES,
  validCredentialPart,
} from "./credential-form";
import { SSHKeyEditor } from "./ssh-key-editor";

type CredentialSource = Credential["source"] | "ssh-key";

const emptyCreateForm = {
  namespace: "",
  name: "",
  displayName: "",
  source: "native" as CredentialSource,
  secret: "",
  sshKeyPassphrase: "",
  environmentVariable: "",
};

export function CredentialsPage({ client }: { client: MenderClient }) {
  const [credentials, setCredentials] = useState<Credential[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState("");
  const [inventoryQuery, setInventoryQuery] = useState("");

  const [createOpen, setCreateOpen] = useState(false);
  const [createForm, setCreateForm] = useState(emptyCreateForm);
  const [createError, setCreateError] = useState("");

  const [rotateCredential, setRotateCredential] =
    useState<Credential | null>(null);
  const [rotateSecret, setRotateSecret] = useState("");
  const [rotatePassphrase, setRotatePassphrase] = useState("");
  const [rotateError, setRotateError] = useState("");

  const refresh = useCallback(async () => {
    try {
      const result = await client.listCredentials();
      setCredentials(result.credentials);
      setError("");
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setLoaded(true);
    }
  }, [client]);

  useEffect(() => {
    let active = true;
    void client
      .listCredentials()
      .then((result) => {
        if (active) {
          setCredentials(result.credentials);
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
  }, [client]);

  const visibleCredentials = useMemo(() => {
    const needle = inventoryQuery.trim().toLocaleLowerCase();
    return needle ? credentials.filter((item) =>
      [item.displayName, item.namespace, item.name, item.source, item.environmentVariable ?? "", item.oauth?.resource ?? "", item.oauth?.status ?? ""]
        .some((value) => value.toLocaleLowerCase().includes(needle))
    ) : credentials;
  }, [credentials, inventoryQuery]);
  const credentialPagination = usePagination(visibleCredentials);

  const namespace = canonicalCredentialPart(createForm.namespace);
  const name = canonicalCredentialPart(createForm.name);
  const namespaceInvalid = createForm.namespace.trim() !== "" && !validCredentialPart(namespace);
  const nameInvalid = createForm.name.trim() !== "" && !validCredentialPart(name);
  const canonicalReference = credentialReferencePreview(namespace, name);
  const referencePreview = canonicalReference ?? (
    namespace && name ? t("Invalid namespace or name") : t("Derived after namespace and name")
  );
  const displayNameValid = createForm.displayName.trim() !== "" &&
    new TextEncoder().encode(createForm.displayName.trim()).length <= MAX_CREDENTIAL_DISPLAY_NAME_BYTES;
  const secretValid = createForm.secret !== "" &&
    new TextEncoder().encode(createForm.secret).length <= MAX_NATIVE_SECRET_BYTES;
  const environmentVariableValid = ENVIRONMENT_VARIABLE_PATTERN.test(createForm.environmentVariable.trim());
  const sshKeyMode = createForm.source === "ssh-key";
  const createValid = canonicalReference !== null && displayNameValid && (
    createForm.source !== "environment" ? secretValid : environmentVariableValid
  ) && (!sshKeyMode || (namespace === "ssh" && createForm.secret.includes("PRIVATE KEY")));

  function updateCreateForm(changes: Partial<typeof emptyCreateForm>) {
    setCreateError("");
    setCreateForm((current) => ({ ...current, ...changes }));
  }

  async function createCredential() {
    if (!createValid || busy !== "") return;
    setBusy("create");
    setCreateError("");
    try {
      if (createForm.source !== "environment") {
        await client.createNativeCredential({
          namespace,
          name,
          displayName: createForm.displayName.trim(),
          secret: createForm.secret,
          ...(sshKeyMode && createForm.sshKeyPassphrase ? { sshKeyPassphrase: createForm.sshKeyPassphrase } : {}),
        });
      } else {
        await client.createEnvironmentCredential({
          namespace,
          name,
          displayName: createForm.displayName.trim(),
          environmentVariable: createForm.environmentVariable.trim(),
        });
      }
      setCreateOpen(false);
      setCreateForm(emptyCreateForm);
      await refresh();
    } catch (reason) {
      setCreateError(credentialCreateError(reason));
    } finally {
      setBusy("");
    }
  }

  async function rotate() {
    if (rotateCredential === null) {
      return;
    }
    setBusy(rotateCredential.reference);
    setError("");
    try {
      await client.rotateCredential(
        rotateCredential.namespace,
        rotateCredential.name,
        {
          secret: rotateSecret,
          ...(rotateCredential.namespace === "ssh" && rotatePassphrase ? { sshKeyPassphrase: rotatePassphrase } : {}),
        },
      );
      setRotateCredential(null);
      setRotateSecret("");
      setRotatePassphrase("");
      setRotateError("");
      await refresh();
    } catch (reason) {
      setRotateError(rotateCredential.namespace === "ssh" ? credentialCreateError(reason) : errorMessage(reason));
    } finally {
      setBusy("");
    }
  }

  async function disconnectOAuth(item: Credential) {
    if (item.source !== "oauth" || item.oauth?.status !== "connected" || busy !== "") return;
    if (!window.confirm(t('Disconnect OAuth Credential "{{name}}"? Reauthorization will be required.', { name: item.displayName }))) return;
    setBusy(item.reference);
    setError("");
    try {
      await client.disconnectOAuthCredential(item.namespace, item.name);
      await refresh();
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy("");
    }
  }

  async function remove(item: Credential) {
    if (
      !window.confirm(
        t('Delete Credential "{{name}}"? Profile or Provider references will block deletion.', {
          name: item.displayName,
        }),
      )
    ) {
      return;
    }
    setBusy(item.reference);
    setError("");
    try {
      await client.deleteCredential(item.namespace, item.name);
      await refresh();
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy("");
    }
  }

  return (
    <>
      <PageToolbar
        filters={
          <div className="w-full min-w-0 sm:w-56 sm:shrink-0">
            <SearchField label="Search credentials" placeholder="Search credentials…" value={inventoryQuery} onChange={setInventoryQuery} />
          </div>
        }
        actions={
          <>
            <Button variant="outline" size="sm" disabled={busy !== ""} onClick={() => void refresh()}>{t("Refresh")}</Button>
            <Button size="sm" onClick={() => { setCreateError(""); setCreateOpen(true); }}>{t("Add credential")}</Button>
          </>
        }
      />
      {error ? <ErrorNotice message={error} /> : null}
      <ResultPanel title="Credential inventory">
          {!loaded ? (
            <div className="px-4 py-12 text-center text-sm text-muted-foreground">
              {t("Loading Credentials…")}
            </div>
          ) : visibleCredentials.length ? (
            <>
              <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("Credential")}</TableHead>
                  <TableHead>{t("Source")}</TableHead>
                  <TableHead>{t("Source metadata")}</TableHead>
                  <TableHead className="text-right">{t("Actions")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {credentialPagination.items.map((item) => (
                  <TableRow key={item.reference}>
                    <TableCell className="font-medium">
                      {item.displayName}
                    </TableCell>
                    <TableCell>
                      <Badge variant="outline">
                        {item.source === "oauth"
                          ? `OAuth · ${item.oauth?.status === "connected" ? t("Connected") : item.oauth?.status === "expired" ? t("Expired") : t("Authorization required")}`
                          : item.namespace === "ssh" ? t("SSH private key") : t(item.source)}
                      </Badge>
                    </TableCell>
                    <TableCell className="font-mono text-xs text-muted-foreground">
                      {item.source === "oauth"
                        ? <span className="block max-w-64 truncate" title={item.oauth?.resource}>{item.oauth?.resource || "—"}</span>
                        : item.source === "environment"
                          ? item.environmentVariable
                          : t("OS credential store")}
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="flex justify-end gap-2">
                        {item.source === "oauth" && item.oauth?.status === "connected" ? (
                          <IconButton
                            variant="outline"
                            label={t("Disconnect OAuth")}
                            disabled={busy !== ""}
                            onClick={() => void disconnectOAuth(item)}
                          >
                            <UnplugIcon />
                          </IconButton>
                        ) : null}
                        {item.source === "native" ? (
                          <IconButton
                            variant="outline"
                            label={t("Rotate")}
                            disabled={busy !== ""}
                            onClick={() => {
                              setRotateSecret("");
                              setRotatePassphrase("");
                              setRotateError("");
                              setRotateCredential(item);
                            }}
                          >
                            <RefreshCwIcon />
                          </IconButton>
                        ) : null}
                        <IconButton
                          variant="destructive"
                          label={t("Delete")}
                          disabled={busy !== ""}
                          onClick={() => void remove(item)}
                        >
                          <Trash2Icon />
                        </IconButton>
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
              </Table>
            </>
          ) : (
            <EmptyPanelState>
              <EmptyState
                title={credentials.length ? t("No matching credentials") : t("No Credentials")}
                description={credentials.length ? t("Try a different search or filter.") : t("Add a native or environment-backed Credential before configuring Provider authentication or Profile credential scope.")}
              />
            </EmptyPanelState>
          )}
      </ResultPanel>

      <TablePagination
        label="Credentials"
        pagination={credentialPagination}
      />

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
        <DialogContent className="grid-rows-[auto_minmax(0,1fr)_auto] max-h-[calc(100dvh-2rem)] sm:max-w-xl">
          <div className="grid gap-3">
            <DialogHeader>
              <DialogTitle>{t("Add credential")}</DialogTitle>
              <DialogDescription>
                {t("Inventory metadata is stored in Mender. Native secret values go directly to the OS credential store; environment-backed Credentials store only the variable name.")}
              </DialogDescription>
            </DialogHeader>
            {createError ? (
              <div role="alert" className="rounded-xl border border-destructive/20 bg-destructive/10 px-3 py-2 text-sm text-destructive">
                {createError}
              </div>
            ) : null}
          </div>

          <div className="min-h-0 space-y-4 overflow-y-auto pr-1">
            <Field label="Source">
              <select
                aria-label={t("Credential source")}
                value={createForm.source}
                onChange={(event) => updateCreateForm({
                  source: event.target.value as CredentialSource,
                  namespace: event.target.value === "ssh-key" ? "ssh" : "",
                  name: "",
                  displayName: "",
                  secret: "",
                  sshKeyPassphrase: "",
                  environmentVariable: "",
                })}
                className="h-9 w-full rounded-lg border border-input bg-background px-3 text-sm outline-none focus:ring-2 focus:ring-ring/40"
              >
                <option value="native">{t("native keyring")}</option>
                <option value="environment">{t("environment variable")}</option>
                <option value="ssh-key">{t("SSH private key")}</option>
              </select>
            </Field>

            {sshKeyMode ? (
              <div className="space-y-2">
                <Field label="Credential name">
                  <Input
                    aria-label={t("SSH credential name")}
                    placeholder={t("My server")}
                    value={createForm.displayName}
                    aria-invalid={nameInvalid || (createForm.displayName !== "" && !displayNameValid)}
                    onChange={(event) => updateCreateForm({
                      name: event.target.value,
                      displayName: event.target.value,
                    })}
                  />
                </Field>
                <p className="text-xs text-muted-foreground">{t("SSH credential identifier is generated automatically.")}</p>
                {nameInvalid ? <p className="text-xs text-destructive">{t("Use a name starting with a letter, up to 63 letters, digits or hyphens.")}</p> : null}
                {createForm.displayName.trim() !== "" && !displayNameValid ? (
                  <p className="text-xs text-destructive">{t("Display name is too long (maximum 160 bytes).")}</p>
                ) : null}
              </div>
            ) : (
              <>
            <div className="grid gap-4 sm:grid-cols-2">
              <Field label="Namespace">
                <Input
                  aria-label={t("Credential namespace")}
                  placeholder={t("github")}
                  value={createForm.namespace}
                  disabled={sshKeyMode}
                  aria-invalid={namespaceInvalid}
                  onChange={(event) => updateCreateForm({ namespace: event.target.value })}
                />
              </Field>
              <Field label="Name">
                <Input
                  aria-label={t("Credential name")}
                  placeholder={t("work")}
                  value={createForm.name}
                  aria-invalid={nameInvalid}
                  onChange={(event) => updateCreateForm({ name: event.target.value })}
                />
              </Field>
            </div>

            <p className="text-xs leading-5 text-muted-foreground">
              {t("Namespace and name are lowercased and spaces become hyphens. Use 1–63 letters, digits or hyphens, starting with a letter.")}
              {namespaceInvalid || nameInvalid ? (
                <span className="block text-destructive">{t("Use valid namespace and name identifiers before creating a credential.")}</span>
              ) : null}
            </p>

            <Field label="Display name">
              <Input
                aria-label={t("Credential display name")}
                placeholder={t("GitHub work")}
                value={createForm.displayName}
                aria-invalid={createForm.displayName.trim() !== "" && !displayNameValid}
                onChange={(event) => updateCreateForm({ displayName: event.target.value })}
              />
            </Field>

            {createForm.displayName.trim() !== "" && !displayNameValid ? (
              <p className="text-xs text-destructive">{t("Display name is too long (maximum 160 bytes).")}</p>
            ) : null}
            <ReadOnlyField label="Reference" value={referencePreview} />
              </>
            )}

            {createForm.source !== "environment" ? (
              sshKeyMode ? (
                <div className="space-y-3">
                  <div className="space-y-2">
                    <p className="text-sm font-medium">{t("SSH private key")}</p>
                    <SSHKeyEditor value={createForm.secret} onChange={(secret) => updateCreateForm({ secret })} />
                  </div>
                  <Field label="SSH key passphrase (optional)">
                    <Input
                      type="password"
                      autoComplete="new-password"
                      aria-label={t("SSH key passphrase (optional)")}
                      value={createForm.sshKeyPassphrase}
                      onChange={(event) => updateCreateForm({ sshKeyPassphrase: event.target.value })}
                    />
                  </Field>
                  <p className="text-xs leading-5 text-muted-foreground">
                    {t("Supports Ed25519, ECDSA and RSA 2048+ private keys. Encrypted keys are decrypted once on import; the OS keychain protects the stored key.")}
                  </p>
                  {createForm.secret !== "" && !secretValid ? (
                    <p className="text-xs text-destructive">{t("Secret exceeds the 64 KiB limit.")}</p>
                  ) : null}
                </div>
              ) : (
                <Field label="Secret">
                  <Input
                    type="password"
                    autoComplete="new-password"
                    aria-label={t("Credential secret")}
                    value={createForm.secret}
                    aria-invalid={createForm.secret !== "" && !secretValid}
                    onChange={(event) => updateCreateForm({ secret: event.target.value })}
                  />
                  <p className="mt-1 text-xs leading-5 text-muted-foreground">
                    {t("The value is submitted once and is never returned by Mender.")}
                  </p>
                  {createForm.secret !== "" && !secretValid ? (
                    <p className="mt-1 text-xs text-destructive">{t("Secret exceeds the 64 KiB limit.")}</p>
                  ) : null}
                </Field>
              )
            ) : (
              <Field label="Environment variable">
                <Input
                  aria-label={t("Credential environment variable")}
                  placeholder={t("GITHUB_TOKEN")}
                  value={createForm.environmentVariable}
                  aria-invalid={createForm.environmentVariable.trim() !== "" && !environmentVariableValid}
                  onChange={(event) => updateCreateForm({ environmentVariable: event.target.value })}
                />
                {createForm.environmentVariable.trim() !== "" && !environmentVariableValid ? (
                  <p className="mt-1 text-xs text-destructive">{t("Enter a valid environment variable name.")}</p>
                ) : null}
              </Field>
            )}
          </div>

          <DialogFooter>
            <Button variant="outline" onClick={() => {
              setCreateOpen(false);
              setCreateForm(emptyCreateForm);
              setCreateError("");
            }}>
              {t("Cancel")}
            </Button>
            <Button
              disabled={!createValid || busy !== ""}
              onClick={() => void createCredential()}
            >
              {t(busy === "create" ? "Creating…" : "Create credential")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog
        open={rotateCredential !== null}
        onOpenChange={(open) => {
          if (!open) {
            setRotateCredential(null);
            setRotateSecret("");
            setRotatePassphrase("");
            setRotateError("");
          }
        }}
      >
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{t("Rotate native secret")}</DialogTitle>
            <DialogDescription>
              {t("Replace the secret stored for {{name}}. The current value is never revealed.", {
                name: rotateCredential?.displayName ?? t("this Credential"),
              })}
            </DialogDescription>
          </DialogHeader>
          {rotateError ? <p role="alert" className="text-sm text-destructive">{rotateError}</p> : null}
          {rotateCredential?.namespace === "ssh" ? (
            <div className="space-y-3">
              <p className="text-sm font-medium">{t("SSH private key")}</p>
              <SSHKeyEditor value={rotateSecret} onChange={(secret) => {
                setRotateSecret(secret);
                setRotateError("");
              }} />
              <Field label="SSH key passphrase (optional)">
                <Input
                  type="password"
                  autoComplete="new-password"
                  aria-label={t("SSH key passphrase (optional)")}
                  value={rotatePassphrase}
                  onChange={(event) => {
                    setRotatePassphrase(event.target.value);
                    setRotateError("");
                  }}
                />
              </Field>
            </div>
          ) : (
            <Field label="New secret">
              <Input
                type="password"
                autoComplete="new-password"
                aria-label={t("New Credential secret")}
                value={rotateSecret}
                onChange={(event) => setRotateSecret(event.target.value)}
              />
            </Field>
          )}
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => {
                setRotateCredential(null);
                setRotateSecret("");
                setRotatePassphrase("");
                setRotateError("");
              }}
            >
              {t("Cancel")}
            </Button>
            <Button
              disabled={rotateSecret === "" || busy !== ""}
              onClick={() => void rotate()}
            >
              {t(busy ? "Rotating…" : "Rotate secret")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

function Field({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <label className="block">
      <span className="mb-1.5 block text-sm font-medium">{t(label)}</span>
      {children}
    </label>
  );
}

function ReadOnlyField({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <div className="mb-1.5 text-sm font-medium">{t(label)}</div>
      <div className="rounded-lg border border-border bg-muted/30 px-3 py-2 font-mono text-xs">
        {value}
      </div>
    </div>
  );
}
