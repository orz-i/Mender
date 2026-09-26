import { useEffect, useState } from "react";

import type {
  CredentialStatus,
  MenderClient,
  PolicyStatus,
} from "@mender/sdk";
import { Badge } from "@mender/ui/components/badge";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@mender/ui/components/card";

import { ErrorNotice, PageHeader } from "../../shared/page";

export function PolicyPage({ client }: { client: MenderClient }) {
  const [status, setStatus] = useState<PolicyStatus | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    let active = true;
    void client
      .getPolicyStatus()
      .then((value) => {
        if (active) {
          setStatus(value);
          setError("");
        }
      })
      .catch((reason: unknown) => {
        if (active) {
          setError(errorMessage(reason));
        }
      });
    return () => {
      active = false;
    };
  }, [client]);

  return (
    <>
      <PageHeader
        title="Policy"
        description="Current runtime policy behavior. Editing is intentionally unavailable until a real Policy application model exists."
      />
      {error ? <ErrorNotice message={error} /> : null}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between gap-3">
            <div>
              <CardTitle>Runtime policy</CardTitle>
              <CardDescription>
                Enforcement still occurs in the shared Invocation pipeline.
              </CardDescription>
            </div>
            <Badge variant="secondary">Read only</Badge>
          </div>
        </CardHeader>
        <CardContent className="grid gap-5 sm:grid-cols-2">
          <StatusField label="Evaluator" value={status?.mode ?? "Loading"} />
          <StatusField label="Approval" value={status?.approval ?? "Loading"} />
        </CardContent>
      </Card>
    </>
  );
}

export function CredentialsPage({ client }: { client: MenderClient }) {
  const [status, setStatus] = useState<CredentialStatus | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    let active = true;
    void client
      .getCredentialStatus()
      .then((value) => {
        if (active) {
          setStatus(value);
          setError("");
        }
      })
      .catch((reason: unknown) => {
        if (active) {
          setError(errorMessage(reason));
        }
      });
    return () => {
      active = false;
    };
  }, [client]);

  return (
    <>
      <PageHeader
        title="Credentials"
        description="Credential references are resolved only at the execution boundary. Persistent secret storage is not configured yet."
      />
      {error ? <ErrorNotice message={error} /> : null}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between gap-3">
            <div>
              <CardTitle>Credential capability</CardTitle>
              <CardDescription>
                No secret CRUD is exposed by this management surface.
              </CardDescription>
            </div>
            <Badge variant="secondary">Read only</Badge>
          </div>
        </CardHeader>
        <CardContent className="grid gap-5 sm:grid-cols-3">
          <StatusField label="Resolver" value={status?.resolver ?? "Loading"} />
          <StatusField
            label="Persistent store"
            value={status?.persistentStore ? "enabled" : "disabled"}
          />
          <StatusField
            label="Editable"
            value={status?.editable ? "yes" : "no"}
          />
        </CardContent>
      </Card>
    </>
  );
}

function StatusField({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg border border-border bg-muted/30 p-4">
      <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
        {label}
      </p>
      <p className="mt-2 font-mono text-sm">{value}</p>
    </div>
  );
}

function errorMessage(reason: unknown) {
  return reason instanceof Error ? reason.message : "Unexpected error";
}
