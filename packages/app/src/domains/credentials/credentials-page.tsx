import { useCallback, useEffect, useMemo, useState } from "react";

import type { Credential, MenderClient } from "@mender/sdk";
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
import { Input } from "@mender/ui/components/input";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@mender/ui/components/table";

import { EmptyState, ErrorNotice, PageHeader } from "../../shared/page";
import { usePagination } from "../../shared/pagination";
import { TablePagination } from "../../shared/table-pagination";

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
      : "Derived after namespace and name";
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
        `Delete Credential "${item.displayName}"? Profile or Provider references will block deletion.`,
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
        title="Credentials"
        description="Manage non-secret Credential metadata and the secret source used at execution time."
        actions={
          <Button
            onClick={() => {
              setError("");
              setCreateOpen(true);
            }}
          >
            Add credential
          </Button>
        }
      />

      {error ? <ErrorNotice message={error} /> : null}

      <Card>
        <CardHeader className="border-b">
          <CardTitle>Credential inventory</CardTitle>
          <CardDescription>
            {credentials.length} configured{" "}
            {credentials.length === 1 ? "Credential" : "Credentials"}. Secret
            values are never displayed here.
          </CardDescription>
        </CardHeader>
        <CardContent>
          {!loaded ? (
            <div className="py-10 text-center text-sm text-muted-foreground">
              Loading Credentials…
            </div>
          ) : credentials.length ? (
            <>
              <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Credential</TableHead>
                  <TableHead>Source</TableHead>
                  <TableHead>Source metadata</TableHead>
                  <TableHead className="text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {credentialPagination.items.map((item) => (
                  <TableRow key={item.reference}>
                    <TableCell>
                      <div className="font-medium">{item.displayName}</div>
                      <div className="mt-1 font-mono text-xs text-muted-foreground">
                        {item.reference}
                      </div>
                    </TableCell>
                    <TableCell>
                      <Badge variant="outline">{item.source}</Badge>
                    </TableCell>
                    <TableCell className="font-mono text-xs text-muted-foreground">
                      {item.source === "environment"
                        ? item.environmentVariable
                        : "OS credential store"}
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="flex justify-end gap-2">
                        {item.source === "native" ? (
                          <Button
                            variant="outline"
                            size="sm"
                            disabled={busy !== ""}
                            onClick={() => {
                              setRotateSecret("");
                              setRotateCredential(item);
                            }}
                          >
                            Rotate
                          </Button>
                        ) : null}
                        <Button
                          variant="destructive"
                          size="sm"
                          disabled={busy !== ""}
                          onClick={() => void remove(item)}
                        >
                          Delete
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
              </Table>
              <TablePagination
                label="Credentials"
                pagination={credentialPagination}
              />
            </>
          ) : (
            <EmptyState
              title="No Credentials"
              description="Add a native or environment-backed Credential before configuring Provider authentication or Profile credential scope."
            />
          )}
        </CardContent>
      </Card>

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
            <DialogTitle>Add credential</DialogTitle>
            <DialogDescription>
              Inventory metadata is stored in Mender. Native secret values go
              directly to the OS credential store; environment-backed
              Credentials store only the variable name.
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-4">
            <Field label="Source">
              <select
                aria-label="Credential source"
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
                <option value="native">native keyring</option>
                <option value="environment">environment variable</option>
              </select>
            </Field>

            <div className="grid gap-4 sm:grid-cols-2">
              <Field label="Namespace">
                <Input
                  aria-label="Credential namespace"
                  placeholder="github"
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
                  aria-label="Credential name"
                  placeholder="work"
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
                aria-label="Credential display name"
                placeholder="GitHub work"
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
                  aria-label="Credential secret"
                  value={createForm.secret}
                  onChange={(event) =>
                    setCreateForm((current) => ({
                      ...current,
                      secret: event.target.value,
                    }))
                  }
                />
                <p className="mt-1 text-xs leading-5 text-muted-foreground">
                  The value is submitted once and is never returned by Mender.
                </p>
              </Field>
            ) : (
              <Field label="Environment variable">
                <Input
                  aria-label="Credential environment variable"
                  placeholder="GITHUB_TOKEN"
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
              Cancel
            </Button>
            <Button
              disabled={!createValid || busy !== ""}
              onClick={() => void createCredential()}
            >
              {busy === "create" ? "Creating…" : "Create credential"}
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
            <DialogTitle>Rotate native secret</DialogTitle>
            <DialogDescription>
              Replace the secret stored for{" "}
              {rotateCredential?.displayName ?? "this Credential"}. The current
              value is never revealed.
            </DialogDescription>
          </DialogHeader>
          <Field label="New secret">
            <Input
              type="password"
              autoComplete="new-password"
              aria-label="New Credential secret"
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
              Cancel
            </Button>
            <Button
              disabled={rotateSecret === "" || busy !== ""}
              onClick={() => void rotate()}
            >
              {busy ? "Rotating…" : "Rotate secret"}
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
      <span className="mb-1.5 block text-sm font-medium">{label}</span>
      {children}
    </label>
  );
}

function ReadOnlyField({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <div className="mb-1.5 text-sm font-medium">{label}</div>
      <div className="rounded-lg border border-border bg-muted/30 px-3 py-2 font-mono text-xs">
        {value}
      </div>
    </div>
  );
}

function errorMessage(reason: unknown) {
  return reason instanceof Error ? reason.message : "Unexpected error";
}
