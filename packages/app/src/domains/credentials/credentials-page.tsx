import { useCallback, useEffect, useMemo, useState } from "react";
import { RefreshCwIcon, Trash2Icon } from "lucide-react";

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
import { EmptyState, ErrorNotice, PageHeader } from "../../shared/page";
import { usePagination } from "../../shared/pagination";
import { TablePagination } from "../../shared/table-pagination";
import { DataPanel, EmptyPanelState } from "../../shared/workbench";

import { t, tCount } from "../../i18n/i18n";

type CredentialSource = Credential["source"];

const emptyCreateForm = {
  namespace: "",
  name: "",
  displayName: "",
  source: "native" as CredentialSource,
  secret: "",
  environmentVariable: "",
};

export function CredentialsPage({ client }: { client: MenderClient }) {
  const [credentials, setCredentials] = useState<Credential[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState("");

  const [createOpen, setCreateOpen] = useState(false);
  const [createForm, setCreateForm] = useState(emptyCreateForm);

  const [rotateCredential, setRotateCredential] =
    useState<Credential | null>(null);
  const [rotateSecret, setRotateSecret] = useState("");

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

  const credentialPagination = usePagination(credentials);

  const referencePreview = useMemo(() => {
    const namespace = createForm.namespace.trim();
    const name = createForm.name.trim();
    return namespace && name
      ? "credential://" + namespace + "/" + name
      : t("Derived after namespace and name");
  }, [createForm.name, createForm.namespace]);

  async function createCredential() {
    setBusy("create");
    setError("");
    try {
      if (createForm.source === "native") {
        await client.createNativeCredential({
          namespace: createForm.namespace,
          name: createForm.name,
          displayName: createForm.displayName,
          secret: createForm.secret,
        });
      } else {
        await client.createEnvironmentCredential({
          namespace: createForm.namespace,
          name: createForm.name,
          displayName: createForm.displayName,
          environmentVariable: createForm.environmentVariable,
        });
      }
      setCreateOpen(false);
      setCreateForm(emptyCreateForm);
      await refresh();
    } catch (reason) {
      setError(errorMessage(reason));
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
        { secret: rotateSecret },
      );
      setRotateCredential(null);
      setRotateSecret("");
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

  const createValid =
    createForm.namespace.trim() !== "" &&
    createForm.name.trim() !== "" &&
    createForm.displayName.trim() !== "" &&
    (createForm.source === "native"
      ? createForm.secret !== ""
      : createForm.environmentVariable.trim() !== "");

  return (
    <>
      <PageHeader
        actions={
          <Button
            onClick={() => {
              setError("");
              setCreateOpen(true);
            }}
          >
            {t("Add credential")}
          </Button>
        }
      />

      {error ? <ErrorNotice message={error} /> : null}

      <DataPanel
        title="Credential inventory"
        description={tCount(
          credentials.length,
          "{{count}} Credential configured. Secret values are never displayed here.",
          "{{count}} Credentials configured. Secret values are never displayed here.",
        )}
      >
          {!loaded ? (
            <div className="px-4 py-12 text-center text-sm text-muted-foreground">
              {t("Loading Credentials…")}
            </div>
          ) : credentials.length ? (
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
                      <Badge variant="outline">{t(item.source)}</Badge>
                    </TableCell>
                    <TableCell className="font-mono text-xs text-muted-foreground">
                      {item.source === "environment"
                        ? item.environmentVariable
                        : t("OS credential store")}
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="flex justify-end gap-2">
                        {item.source === "native" ? (
                          <IconButton
                            variant="outline"
                            label={t("Rotate")}
                            disabled={busy !== ""}
                            onClick={() => {
                              setRotateSecret("");
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
              <div className="px-4 pb-3">
                <TablePagination
                  label="Credentials"
                  pagination={credentialPagination}
                />
              </div>
            </>
          ) : (
            <EmptyPanelState>
              <EmptyState
                title="No Credentials"
                description="Add a native or environment-backed Credential before configuring Provider authentication or Profile credential scope."
              />
            </EmptyPanelState>
          )}
      </DataPanel>

      <Dialog
        open={createOpen}
        onOpenChange={(open) => {
          setCreateOpen(open);
          if (!open) {
            setCreateForm(emptyCreateForm);
          }
        }}
      >
        <DialogContent className="sm:max-w-xl">
          <DialogHeader>
            <DialogTitle>{t("Add credential")}</DialogTitle>
            <DialogDescription>
              {t("Inventory metadata is stored in Mender. Native secret values go directly to the OS credential store; environment-backed Credentials store only the variable name.")}
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-4">
            <Field label="Source">
              <select
                aria-label={t("Credential source")}
                value={createForm.source}
                onChange={(event) =>
                  setCreateForm((current) => ({
                    ...current,
                    source: event.target.value as CredentialSource,
                    secret: "",
                    environmentVariable: "",
                  }))
                }
                className="h-9 w-full rounded-lg border border-input bg-background px-3 text-sm outline-none focus:ring-2 focus:ring-ring/40"
              >
                <option value="native">{t("native keyring")}</option>
                <option value="environment">{t("environment variable")}</option>
              </select>
            </Field>

            <div className="grid gap-4 sm:grid-cols-2">
              <Field label="Namespace">
                <Input
                  aria-label={t("Credential namespace")}
                  placeholder={t("github")}
                  value={createForm.namespace}
                  onChange={(event) =>
                    setCreateForm((current) => ({
                      ...current,
                      namespace: event.target.value,
                    }))
                  }
                />
              </Field>
              <Field label="Name">
                <Input
                  aria-label={t("Credential name")}
                  placeholder={t("work")}
                  value={createForm.name}
                  onChange={(event) =>
                    setCreateForm((current) => ({
                      ...current,
                      name: event.target.value,
                    }))
                  }
                />
              </Field>
            </div>

            <Field label="Display name">
              <Input
                aria-label={t("Credential display name")}
                placeholder={t("GitHub work")}
                value={createForm.displayName}
                onChange={(event) =>
                  setCreateForm((current) => ({
                    ...current,
                    displayName: event.target.value,
                  }))
                }
              />
            </Field>

            <ReadOnlyField label="Reference" value={referencePreview} />

            {createForm.source === "native" ? (
              <Field label="Secret">
                <Input
                  type="password"
                  autoComplete="new-password"
                  aria-label={t("Credential secret")}
                  value={createForm.secret}
                  onChange={(event) =>
                    setCreateForm((current) => ({
                      ...current,
                      secret: event.target.value,
                    }))
                  }
                />
                <p className="mt-1 text-xs leading-5 text-muted-foreground">
                  {t("The value is submitted once and is never returned by Mender.")}
                </p>
              </Field>
            ) : (
              <Field label="Environment variable">
                <Input
                  aria-label={t("Credential environment variable")}
                  placeholder={t("GITHUB_TOKEN")}
                  value={createForm.environmentVariable}
                  onChange={(event) =>
                    setCreateForm((current) => ({
                      ...current,
                      environmentVariable: event.target.value,
                    }))
                  }
                />
              </Field>
            )}
          </div>

          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)}>
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
          <Field label="New secret">
            <Input
              type="password"
              autoComplete="new-password"
              aria-label={t("New Credential secret")}
              value={rotateSecret}
              onChange={(event) => setRotateSecret(event.target.value)}
            />
          </Field>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => {
                setRotateCredential(null);
                setRotateSecret("");
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
