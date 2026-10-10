export function SandboxMetadata({ name, value }: { name: string; value: string }) {
  return (
    <div className="min-w-0 border-b border-border/70 pb-3">
      <dt className="text-xs font-medium text-muted-foreground">{name}</dt>
      <dd className="mt-1 break-all text-sm font-medium text-foreground">{value}</dd>
    </div>
  );
}
