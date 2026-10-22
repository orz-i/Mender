import { useState } from "react";
import { useNavigate } from "react-router";

import type { MenderClient, Profile } from "@mender/sdk";
import { Button } from "@mender/ui/components/button";
import { Input } from "@mender/ui/components/input";

import { t } from "../../i18n/i18n";
import { ErrorNotice } from "../../shared/page";
import { PageToolbar } from "../../shared/page-toolbar";
import { DataPanel } from "../../shared/workbench";
import { canonicalizeOwnedIdentifier, isCanonicalOwnedSlug, ownedIdentifierCreateError } from "../../shared/form-identifiers";

export function ProfileCreatePage({ client }: { client: MenderClient }) {
  const navigate = useNavigate();
  const [form, setForm] = useState<{ slug: string; displayName: string; mode: Profile["mode"] }>({
    slug: "", displayName: "", mode: "hybrid",
  });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const slug = canonicalizeOwnedIdentifier(form.slug);
  const slugValid = isCanonicalOwnedSlug(slug);
  const nameValid = form.displayName.trim() !== "" &&
    new TextEncoder().encode(form.displayName.trim()).length <= 160;
  const valid = slugValid && nameValid;

  async function create() {
    if (!valid || busy) return;
    setBusy(true);
    setError("");
    try {
      const created = await client.createProfile({
        slug, displayName: form.displayName.trim(), mode: form.mode,
      });
      // A new Profile has no grants. Continue directly to tool configuration.
      void navigate("/profiles/" + encodeURIComponent(created.slug) + "/edit", {
        state: { initialSection: "tools" },
      });
    } catch (reason) {
      setError(ownedIdentifierCreateError(reason, "profile"));
      setBusy(false);
    }
  }

  return (
    <>
      <PageToolbar actions={<Button variant="outline" disabled={busy}
        onClick={() => void navigate("/profiles")}>{t("Back to profiles")}</Button>} />
      <DataPanel title="Create profile" description="Configure the Profile identity first. Choose its tools after creation.">
        <form className="mx-auto flex w-full max-w-2xl flex-col gap-6 px-5 py-6 sm:px-8 sm:py-8"
          onSubmit={(event) => { event.preventDefault(); void create(); }}>
          {error ? <ErrorNotice message={error} /> : null}
          <div className="flex flex-col gap-2">
            <label htmlFor="profile-slug" className="text-sm font-medium">{t("Slug")}</label>
            <Input id="profile-slug" aria-label={t("Profile slug")} placeholder={t("default")}
              autoFocus value={form.slug} aria-invalid={form.slug.trim() !== "" && !slugValid}
              onChange={(event) => { setError(""); setForm((current) => ({ ...current, slug: event.target.value })); }} />
            <p className="text-xs text-muted-foreground">
              {t("Uppercase letters and spaces become a lowercase hyphenated identifier.")}
            </p>
            {form.slug.trim() !== "" ? <p role={slugValid ? "status" : "alert"}
              className={slugValid ? "break-all font-mono text-xs" : "text-xs text-destructive"}>
              {slugValid ? t("Result: {{value}}", { value: slug })
                : t("Use 1–63 characters, starting with a letter and ending with a letter or digit; only lowercase letters, digits and hyphens are allowed.")}
            </p> : null}
          </div>
          <div className="flex flex-col gap-2">
            <label htmlFor="profile-name" className="text-sm font-medium">{t("Display name")}</label>
            <Input id="profile-name" aria-label={t("Profile display name")} value={form.displayName}
              aria-invalid={form.displayName.trim() !== "" && !nameValid}
              onChange={(event) => { setError(""); setForm((current) => ({ ...current, displayName: event.target.value })); }} />
            {form.displayName.trim() !== "" && !nameValid ? <p role="alert" className="text-xs text-destructive">
              {t("Display name is too long (maximum 160 bytes).")}
            </p> : null}
          </div>
          <fieldset className="flex flex-col gap-2">
            <legend className="mb-2 text-sm font-medium">{t("Exposure mode")}</legend>
            <select aria-label={t("Exposure mode")} value={form.mode}
              onChange={(event) => setForm((current) => ({ ...current, mode: event.target.value as Profile["mode"] }))}
              className="h-10 rounded-lg border border-input bg-background px-3 text-sm outline-none focus:ring-2 focus:ring-ring/40">
              <option value="direct">{t("direct")}</option>
              <option value="dynamic">{t("dynamic")}</option>
              <option value="hybrid">{t("hybrid")}</option>
            </select>
            <p className="text-xs leading-5 text-muted-foreground">
              {t("Direct exports selected Tools. Dynamic exposes discovery meta-tools. Hybrid supports both and lets selected Tools opt into direct export.")}
            </p>
          </fieldset>
          <div className="flex flex-wrap items-center justify-end gap-2 border-t border-border pt-5">
            <Button type="button" variant="outline" disabled={busy} onClick={() => void navigate("/profiles")}>
              {t("Cancel")}
            </Button>
            <Button type="submit" disabled={busy || !valid}>
              {t(busy ? "Creating…" : "Create profile")}
            </Button>
          </div>
        </form>
      </DataPanel>
    </>
  );
}
