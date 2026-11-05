import type { Credential } from "@mender/sdk";
import { Button } from "@mender/ui/components/button";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@mender/ui/components/dialog";
import { Input } from "@mender/ui/components/input";

import { t } from "../../../i18n/i18n";
import { EmptyState } from "../../../shared/page";
import { Field } from "./provider-fields";
import { MODEL_TOOLS_PLUGIN_ID, OPENAPI_PLUGIN_ID, type ProviderFormState } from "./provider-form-model";

export function ProviderAuthenticationDialog({
  open,
  form,
  credentials,
  busy,
  onOpenChange,
  onChange,
  onSubmit,
  submitLabel,
}: {
  open: boolean;
  form: ProviderFormState;
  credentials: Credential[];
  busy: boolean;
  onOpenChange: (open: boolean) => void;
  onChange: (value: ProviderFormState) => void;
  onSubmit: () => void;
  submitLabel: string;
}) {
  const staticCredentials = credentials.filter((item) => item.source !== "oauth");
  const oauthCredentials = credentials.filter((item) => item.source === "oauth");
  const firstCredential = staticCredentials[0]?.reference ?? "";
  const oauthAuthorizationConflict = Boolean(form.oauthCredentialRef) && (
    form.headers.split("\n").some((line) => /^\s*authorization\s*:/i.test(line)) ||
    form.credentialHeaders.some((binding) => binding.name.trim().toLowerCase() === "authorization")
  );

  function addHTTPBinding() {
    if (!firstCredential) {
      return;
    }
    onChange({
      ...form,
      credentialHeaders: [
        ...form.credentialHeaders,
        {
          name: "Authorization",
          scheme: "Bearer",
          credentialRef: firstCredential,
        },
      ],
    });
  }

  function addStdioBinding() {
    if (!firstCredential) {
      return;
    }
    onChange({
      ...form,
      credentialEnvironment: [
        ...form.credentialEnvironment,
        {
          name: "TOKEN",
          credentialRef: firstCredential,
        },
      ],
    });
  }

  function addOpenAPIBinding() {
    if (!firstCredential) {
      return;
    }
    onChange({
      ...form,
      openapiCredentials: [
        ...form.openapiCredentials,
        {
          schemeName: "",
          credentialRef: firstCredential,
        },
      ],
    });
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t("Provider authentication")}</DialogTitle>
          <DialogDescription className="sr-only">
            {t("Bind Provider authentication to inventory Credentials. Mender stores only Credential References and resolves secret values at connection and execution boundaries.")}
          </DialogDescription>
        </DialogHeader>

        <DialogBody>
        {credentials.length ? (
          <div className="space-y-4">
            {form.pluginId === MODEL_TOOLS_PLUGIN_ID ? (
              <>
                <Field label="Model API Credential" help={t("Select the inventory Credential used to authenticate the configured model API. The secret is resolved only during governed Tool execution and remains subject to Profile Credential scope.")}>
                  <select
                    aria-label={t("Model Tools API credential")}
                    value={form.modelToolsCredentialRef}
                    onChange={(event) =>
                      onChange({
                        ...form,
                        modelToolsCredentialRef: event.target.value,
                      })
                    }
                    className="h-8 w-full rounded-lg border border-input bg-background px-2.5 text-sm outline-none focus:ring-2 focus:ring-ring/40"
                  >
                    <option value="" disabled>
                      {t("Choose a Credential")}
                    </option>
                    {staticCredentials.map((item) => (
                      <option key={item.reference} value={item.reference}>
                        {item.displayName} · {item.reference}
                      </option>
                    ))}
                  </select>
                </Field>
              </>
            ) : form.pluginId === OPENAPI_PLUGIN_ID ? (
              <>
                <div className="space-y-3">
                  {form.openapiCredentials.map((binding, index) => (
                    <div
                      key={index}
                      className="grid gap-3 rounded-lg border border-border p-3 sm:grid-cols-[1fr_1.4fr_auto]"
                    >
                      <Field label="Security Scheme" help={t("Bind OpenAPI Security Scheme names to inventory Credentials. The scheme name must match the description exactly; Mender keeps the Credential Reference in Provider config and resolves the secret only at execution.")}>
                        <Input
                          aria-label={t("OpenAPI security scheme {{index}}", {
                            index: index + 1,
                          })}
                          placeholder={t("BearerAuth")}
                          value={binding.schemeName}
                          onChange={(event) =>
                            onChange({
                              ...form,
                              openapiCredentials: form.openapiCredentials.map(
                                (item, itemIndex) =>
                                  itemIndex === index
                                    ? {
                                        ...item,
                                        schemeName: event.target.value,
                                      }
                                    : item,
                              ),
                            })
                          }
                        />
                      </Field>
                      <Field label="Credential">
                        <select
                          aria-label={t("OpenAPI security credential {{index}}", {
                            index: index + 1,
                          })}
                          value={binding.credentialRef}
                          onChange={(event) =>
                            onChange({
                              ...form,
                              openapiCredentials: form.openapiCredentials.map(
                                (item, itemIndex) =>
                                  itemIndex === index
                                    ? {
                                        ...item,
                                        credentialRef: event.target.value,
                                      }
                                    : item,
                              ),
                            })
                          }
                          className="h-8 w-full rounded-lg border border-input bg-background px-2.5 text-sm outline-none focus:ring-2 focus:ring-ring/40"
                        >
                          {staticCredentials.map((item) => (
                            <option key={item.reference} value={item.reference}>
                              {item.displayName} · {item.reference}
                            </option>
                          ))}
                        </select>
                      </Field>
                      <div className="flex items-end">
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          onClick={() =>
                            onChange({
                              ...form,
                              openapiCredentials: form.openapiCredentials.filter(
                                (_, itemIndex) => itemIndex !== index,
                              ),
                            })
                          }
                        >
                          {t("Remove")}
                        </Button>
                      </div>
                    </div>
                  ))}
                </div>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={addOpenAPIBinding}
                >
                  {t("Add Security Scheme binding")}
                </Button>
              </>
            ) : form.transport === "http" ? (
              <>
                <Field label="OAuth Credential">
                  <select
                    aria-label={t("OAuth Credential")}
                    value={form.oauthCredentialRef}
                    onChange={(event) => onChange({ ...form, oauthCredentialRef: event.target.value })}
                    className="h-8 w-full rounded-lg border border-input bg-background px-2.5 text-sm outline-none focus:ring-2 focus:ring-ring/40"
                  >
                    <option value="">{t("No OAuth credential")}</option>
                    {oauthCredentials.map((item) => (
                      <option key={item.reference} value={item.reference}>
                        {item.displayName} · {item.oauth?.status === "connected" ? t("Connected") : item.oauth?.status === "expired" ? t("Expired") : t("Authorization required")}
                      </option>
                    ))}
                  </select>
                </Field>
                {oauthAuthorizationConflict ? (
                  <p role="alert" className="text-xs text-destructive">{t("OAuth cannot share an Authorization header. Remove the existing Authorization binding first.")}</p>
                ) : null}
                {form.headers.trim() ? (
                  <div className="rounded-lg border border-amber-500/30 bg-amber-500/5 px-3 py-2 text-xs leading-5 text-muted-foreground">
                    {t("Legacy HTTP header templates are still configured. They remain compatible but are not governed by Profile Credential scope. Prefer inventory-backed bindings below.")}
                  </div>
                ) : null}

                <div className="space-y-3">
                  {form.credentialHeaders.map((binding, index) => (
                    <div
                      key={index}
                      className="grid gap-3 rounded-lg border border-border p-3 sm:grid-cols-[1fr_120px_1.4fr_auto]"
                    >
                      <Field label="Header">
                        <Input
                          aria-label={t("Credential header name {{index}}", {
                            index: index + 1,
                          })}
                          value={binding.name}
                          onChange={(event) =>
                            onChange({
                              ...form,
                              credentialHeaders: form.credentialHeaders.map(
                                (item, itemIndex) =>
                                  itemIndex === index
                                    ? { ...item, name: event.target.value }
                                    : item,
                              ),
                            })
                          }
                        />
                      </Field>
                      <Field label="Scheme">
                        <Input
                          aria-label={t("Credential header scheme {{index}}", {
                            index: index + 1,
                          })}
                          placeholder={t("Bearer")}
                          value={binding.scheme}
                          onChange={(event) =>
                            onChange({
                              ...form,
                              credentialHeaders: form.credentialHeaders.map(
                                (item, itemIndex) =>
                                  itemIndex === index
                                    ? { ...item, scheme: event.target.value }
                                    : item,
                              ),
                            })
                          }
                        />
                      </Field>
                      <Field label="Credential">
                        <select
                          aria-label={t("Credential header credential {{index}}", {
                            index: index + 1,
                          })}
                          value={binding.credentialRef}
                          onChange={(event) =>
                            onChange({
                              ...form,
                              credentialHeaders: form.credentialHeaders.map(
                                (item, itemIndex) =>
                                  itemIndex === index
                                    ? {
                                        ...item,
                                        credentialRef: event.target.value,
                                      }
                                    : item,
                              ),
                            })
                          }
                          className="h-8 w-full rounded-lg border border-input bg-background px-2.5 text-sm outline-none focus:ring-2 focus:ring-ring/40"
                        >
                          {staticCredentials.map((item) => (
                            <option key={item.reference} value={item.reference}>
                              {item.displayName} · {item.reference}
                            </option>
                          ))}
                        </select>
                      </Field>
                      <div className="flex items-end">
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          onClick={() =>
                            onChange({
                              ...form,
                              credentialHeaders: form.credentialHeaders.filter(
                                (_, itemIndex) => itemIndex !== index,
                              ),
                            })
                          }
                        >
                          {t("Remove")}
                        </Button>
                      </div>
                    </div>
                  ))}
                </div>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={addHTTPBinding}
                >
                  {t("Add credential header")}
                </Button>
              </>
            ) : (
              <>
                <div className="space-y-3">
                  {form.credentialEnvironment.map((binding, index) => (
                    <div
                      key={index}
                      className="grid gap-3 rounded-lg border border-border p-3 sm:grid-cols-[1fr_1.4fr_auto]"
                    >
                      <Field label="Environment name">
                        <Input
                          aria-label={t("Credential environment name {{index}}", {
                            index: index + 1,
                          })}
                          value={binding.name}
                          onChange={(event) =>
                            onChange({
                              ...form,
                              credentialEnvironment:
                                form.credentialEnvironment.map(
                                  (item, itemIndex) =>
                                    itemIndex === index
                                      ? { ...item, name: event.target.value }
                                      : item,
                                ),
                            })
                          }
                        />
                      </Field>
                      <Field label="Credential">
                        <select
                          aria-label={t("Credential environment credential {{index}}", {
                            index: index + 1,
                          })}
                          value={binding.credentialRef}
                          onChange={(event) =>
                            onChange({
                              ...form,
                              credentialEnvironment:
                                form.credentialEnvironment.map(
                                  (item, itemIndex) =>
                                    itemIndex === index
                                      ? {
                                          ...item,
                                          credentialRef: event.target.value,
                                        }
                                      : item,
                                ),
                            })
                          }
                          className="h-8 w-full rounded-lg border border-input bg-background px-2.5 text-sm outline-none focus:ring-2 focus:ring-ring/40"
                        >
                          {staticCredentials.map((item) => (
                            <option key={item.reference} value={item.reference}>
                              {item.displayName} · {item.reference}
                            </option>
                          ))}
                        </select>
                      </Field>
                      <div className="flex items-end">
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          onClick={() =>
                            onChange({
                              ...form,
                              credentialEnvironment:
                                form.credentialEnvironment.filter(
                                  (_, itemIndex) => itemIndex !== index,
                                ),
                            })
                          }
                        >
                          {t("Remove")}
                        </Button>
                      </div>
                    </div>
                  ))}
                </div>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={addStdioBinding}
                >
                  {t("Add credential environment")}
                </Button>
              </>
            )}
          </div>
        ) : (
          <EmptyState
            title="No Credentials in inventory"
            description="Create a Credential first, then return here to configure Provider authentication."
          />
        )}
        </DialogBody>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("Cancel")}
          </Button>
          <Button disabled={busy || oauthAuthorizationConflict} onClick={onSubmit}>
            {busy ? t("Saving…") : t(submitLabel)}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
