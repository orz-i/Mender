import { t } from "../i18n/i18n";

/** Single-topic chooser. Native buttons retain ordinary keyboard tab order. */
export function FocusSwitch<Value extends string>({
  value,
  items,
  label,
  onChange,
}: {
  value: Value;
  items: ReadonlyArray<{ value: Value; label: string }>;
  label: string;
  onChange: (value: Value) => void;
}) {
  return (
    <div
      role="group"
      aria-label={t(label)}
      className="inline-flex max-w-full flex-wrap items-center gap-1 rounded-2xl bg-secondary p-1"
    >
      {items.map((item) => (
        <button
          key={item.value}
          type="button"
          aria-pressed={value === item.value}
          onClick={() => onChange(item.value)}
          className={[
            "min-h-9 rounded-xl px-4 py-2 text-sm font-medium transition-[background-color,box-shadow,color]",
            "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring",
            value === item.value
              ? "bg-card text-foreground shadow-sm"
              : "text-muted-foreground hover:bg-card/65 hover:text-foreground",
          ].join(" ")}
        >
          {t(item.label)}
        </button>
      ))}
    </div>
  );
}
