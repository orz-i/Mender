import { t } from "../../../i18n/i18n";
import { FormHeading } from "../../../shared/form-info";

export function Field({
  label,
  optional = false,
  help,
  children,
}: {
  label: string;
  optional?: boolean;
  help?: string;
  children: React.ReactNode;
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <div className="flex items-center gap-1">
        <FormHeading label={t(label)} help={help} />
        {optional ? <span className="text-xs text-muted-foreground">{t("(optional)")}</span> : null}
      </div>
      {children}
    </div>
  );
}

export function ReadOnlyField({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <div className="mb-1.5 text-sm font-medium">{t(label)}</div>
      <div className="min-w-0 break-all rounded-lg border border-border bg-muted/40 px-3 py-2 font-mono text-sm text-muted-foreground">
        {value}
      </div>
    </div>
  );
}
