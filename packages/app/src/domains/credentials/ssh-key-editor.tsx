import { useEffect, useRef, useState } from "react";
import { ClipboardPasteIcon, FolderOpenIcon } from "lucide-react";

import { Button } from "@mender/ui/components/button";
import { t } from "../../i18n/i18n";
import { MAX_NATIVE_SECRET_BYTES } from "./credential-form";

// This control reads only a user-selected local file into the form's ephemeral
// value. File paths and key material are never added to metadata or diagnostics.
export function SSHKeyEditor({
  value,
  onChange,
}: {
  value: string;
  onChange: (value: string) => void;
}) {
  const fileInput = useRef<HTMLInputElement>(null);
  const importGeneration = useRef(0);
  useEffect(() => () => { importGeneration.current++; }, []);
  const [manual, setManual] = useState(false);
  const [fileName, setFileName] = useState("");
  const [error, setError] = useState("");

  async function importFile(file?: File) {
    if (!file) return;
    const generation = ++importGeneration.current;
    setError("");
    if (file.size === 0 || file.size > MAX_NATIVE_SECRET_BYTES) {
      setError(t("SSH key file must be between 1 byte and 64 KiB."));
      return;
    }
    try {
      const key = await file.text();
      if (generation !== importGeneration.current) return;
      if (!key.includes("PRIVATE KEY") || new TextEncoder().encode(key).length > MAX_NATIVE_SECRET_BYTES) {
        setError(t("Choose a PEM or OpenSSH private key file."));
        return;
      }
      onChange(key);
      setFileName(file.name);
      setManual(false);
    } catch {
      if (generation === importGeneration.current) setError(t("Could not read the selected SSH key file."));
    }
  }

  return (
    <div className="space-y-2">
      <input
        ref={fileInput}
        className="sr-only"
        type="file"
        aria-label={t("SSH private key file")}
        onChange={(event) => {
          const file = event.currentTarget.files?.[0];
          event.currentTarget.value = "";
          void importFile(file);
        }}
      />
      <div className="flex flex-wrap gap-2">
        <Button type="button" size="sm" variant="outline" onClick={() => fileInput.current?.click()}>
          <FolderOpenIcon className="size-4" aria-hidden="true" />
          {t("Choose key file")}
        </Button>
        <Button type="button" size="sm" variant="ghost" onClick={() => {
          importGeneration.current++;
          setManual((current) => !current);
          setError("");
        }}>
          <ClipboardPasteIcon className="size-4" aria-hidden="true" />
          {t(manual ? "Hide private key" : "Paste private key")}
        </Button>
      </div>
      {manual ? (
        <textarea
          aria-label={t("SSH private key")}
          autoComplete="off"
          spellCheck={false}
          rows={5}
          value={value}
          onChange={(event) => {
            setFileName("");
            onChange(event.target.value);
          }}
          placeholder={t("-----BEGIN OPENSSH PRIVATE KEY-----")}
          className="w-full resize-y rounded-lg border border-input bg-background p-3 font-mono text-xs outline-none focus-visible:ring-2 focus-visible:ring-ring"
        />
      ) : (
        <p role="status" className="text-xs text-muted-foreground">
          {value ? (
            fileName ? t("Key loaded from {{file}}; private key contents are hidden.", { file: fileName }) :
              t("Private key ready; contents are hidden.")
          ) : t("Select an existing private key file or paste its contents.")}
        </p>
      )}
      {error ? <p role="alert" className="text-xs text-destructive">{error}</p> : null}
    </div>
  );
}
