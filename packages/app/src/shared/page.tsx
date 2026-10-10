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
    <div className="mb-5 flex flex-wrap justify-end gap-2">
      <div className="shrink-0">{actions}</div>
    </div>
  );
}

export function ErrorNotice({ message }: { message: string }) {
  return (
    <div className="mb-5 rounded-2xl border border-destructive/20 bg-destructive/10 px-5 py-4 text-sm text-destructive">
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
    <div className="rounded-[20px] border border-dashed border-border bg-background/70 px-6 py-12 text-center">
      <p className="font-medium tracking-tight">{t(title)}</p>
      <p className="mx-auto mt-1 max-w-lg text-sm leading-5 text-muted-foreground">
        {t(description)}
      </p>
    </div>
  );
}
