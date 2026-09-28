import type { ReactNode } from "react";

import { Badge } from "@mender/ui/components/badge";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@mender/ui/components/card";

import { t } from "../i18n/i18n";

export function PageHeader({
  title,
  description,
  actions,
}: {
  title: string;
  description: string;
  actions?: ReactNode;
}) {
  return (
    <header className="mb-6 flex items-start justify-between gap-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">{t(title)}</h1>
        <p className="mt-1 max-w-3xl text-sm leading-6 text-muted-foreground">
          {t(description)}
        </p>
      </div>
      {actions}
    </header>
  );
}

export function ErrorNotice({ message }: { message: string }) {
  return (
    <div className="mb-4 rounded-lg border border-destructive/30 bg-destructive/10 px-4 py-3 text-sm text-destructive">
      {message}
    </div>
  );
}

export function EmptyState({
  title,
  description,
}: {
  title: string;
  description: string;
}) {
  return (
    <div className="rounded-lg border border-dashed border-border px-6 py-10 text-center">
      <p className="font-medium">{t(title)}</p>
      <p className="mt-1 text-sm text-muted-foreground">{t(description)}</p>
    </div>
  );
}

export function StatusCard({
  title,
  value,
  description,
  status = "neutral",
}: {
  title: string;
  value: string;
  description: string;
  status?: "neutral" | "good" | "warning";
}) {
  return (
    <Card>
      <CardHeader className="pb-2">
        <div className="flex items-center justify-between gap-3">
          <CardDescription>{t(title)}</CardDescription>
          <Badge variant={status === "warning" ? "secondary" : "outline"}>
            {t(status)}
          </Badge>
        </div>
        <CardTitle className="text-lg">{value}</CardTitle>
      </CardHeader>
      <CardContent>
        <p className="text-sm text-muted-foreground">{t(description)}</p>
      </CardContent>
    </Card>
  );
}
