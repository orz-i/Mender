import { useCallback, useEffect, useState } from "react";

import type { AuditRecord, MenderClient } from "@mender/sdk";
import { Badge } from "@mender/ui/components/badge";
import { Button } from "@mender/ui/components/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@mender/ui/components/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@mender/ui/components/table";

import { EmptyState, ErrorNotice, PageHeader } from "../../shared/page";

export function AuditPage({ client }: { client: MenderClient }) {
  const [records, setRecords] = useState<AuditRecord[]>([]);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const refresh = useCallback(async () => {
    setBusy(true);
    try {
      const result = await client.listAudit(100);
      setRecords(result.records);
      setError("");
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy(false);
    }
  }, [client]);

  useEffect(() => {
    let active = true;
    void client
      .listAudit(100)
      .then((result) => {
        if (active) {
          setRecords(result.records);
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
        title="Audit"
        description="Data-minimized terminal invocation records. Arguments, result payloads, credential references, and secrets are not retained here."
        actions={
          <Button variant="outline" onClick={() => void refresh()} disabled={busy}>
            Refresh
          </Button>
        }
      />
      {error ? <ErrorNotice message={error} /> : null}

      <Card>
        <CardHeader>
          <CardTitle>Invocation history</CardTitle>
        </CardHeader>
        <CardContent>
          {records.length ? (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Finished</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Caller</TableHead>
                  <TableHead>Tool</TableHead>
                  <TableHead>Policy</TableHead>
                  <TableHead>Error</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {records.map((record) => (
                  <TableRow key={record.invocationId}>
                    <TableCell className="whitespace-nowrap text-xs text-muted-foreground">
                      {new Date(record.finishedAt).toLocaleString()}
                    </TableCell>
                    <TableCell>
                      <Badge
                        variant={
                          record.status === "completed" ? "outline" : "secondary"
                        }
                      >
                        {record.status}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-xs">{record.caller || "—"}</TableCell>
                    <TableCell className="max-w-60 truncate font-mono text-[11px]">
                      {record.toolId}
                    </TableCell>
                    <TableCell className="text-xs">
                      {record.policyKind || "—"}
                    </TableCell>
                    <TableCell className="text-xs text-muted-foreground">
                      {record.errorClass || "—"}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          ) : (
            <EmptyState
              title="No audit records"
              description="Invocation outcomes will appear here after Tools are called."
            />
          )}
        </CardContent>
      </Card>
    </>
  );
}

function errorMessage(reason: unknown) {
  return reason instanceof Error ? reason.message : "Unexpected error";
}
