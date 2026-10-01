import type { ReactNode } from "react";

import { t } from "../i18n/i18n";

export function PageHeader({
  actions,
}: {
  actions?: ReactNode;
}) {
  if (!actions) {
    return null;
  }

  return (
    <div className="mb-4 flex justify-end">
      <div className="shrink-0">{actions}</div>
    </div>
  );
}

export function ErrorNotice({ message }: { message: string }) {
  return (
    <div className="mb-5 rounded-xl border border-destructive/20 bg-destructive/6 px-4 py-3 text-sm text-destructive">
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
    <div className="rounded-2xl border border-dashed border-border/90 bg-muted/15 px-6 py-12 text-center">
      <p className="font-medium tracking-tight">{t(title)}</p>
      <p className="mx-auto mt-1 max-w-lg text-sm leading-5 text-muted-foreground">
        {t(description)}
      </p>
    </div>
  );
}
