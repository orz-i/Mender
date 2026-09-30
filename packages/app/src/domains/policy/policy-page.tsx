import { useCallback, useEffect, useMemo, useState } from "react";
import {
  KeyRoundIcon,
  PencilIcon,
  PowerIcon,
  PowerOffIcon,
  ShieldCheckIcon,
  Trash2Icon,
} from "lucide-react";

import type {
  MenderClient,
  Policy,
  PolicyDecision,
  PolicyRule,
  PolicyRuleInput,
  PolicyStatus,
  Profile,
  Provider,
  ToolSummary,
} from "@mender/sdk";
import { Badge } from "@mender/ui/components/badge";
import { Button } from "@mender/ui/components/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@mender/ui/components/dialog";
import { Input } from "@mender/ui/components/input";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@mender/ui/components/table";

import { errorMessage } from "../../shared/errors";
import { IconButton } from "../../shared/icon-button";
import { EmptyState, ErrorNotice, PageHeader } from "../../shared/page";
import { usePagination } from "../../shared/pagination";
import { TablePagination } from "../../shared/table-pagination";
import {
  DataPanel,
  EmptyPanelState,
  MetricStrip,
  MetricTile,
} from "../../shared/workbench";

import { t, tCount } from "../../i18n/i18n";

type RuleStep = 0 | 1 | 2;

interface RuleEditorState {
  ruleId: string;
  step: RuleStep;
  form: PolicyRuleInput;
}

const originOptions: Array<{
  value: PolicyRuleInput["match"]["origins"][number];
  label: string;
}> = [
  { value: "cli", label: "CLI" },
  { value: "mcp", label: "MCP" },
  { value: "management_http", label: "Management HTTP" },
  { value: "desktop", label: "Desktop" },
  { value: "internal", label: "Internal" },
];

const scopeOptions: Array<{
  value: PolicyRuleInput["match"]["scopeKinds"][number];
  label: string;
}> = [
  { value: "catalog", label: "Catalog" },
  { value: "profile", label: "Profile" },
];

function emptyRuleForm(): PolicyRuleInput {
  return {
    displayName: "",
    enabled: true,
    decision: "deny",
    match: {
      origins: [],
      scopeKinds: [],
      profileRefs: [],
      providerIds: [],
      toolIds: [],
      factKnowledge: "any",
      effectsAny: [],
      resourcesAny: [],
    },
  };
}

export function PolicyPage({ client }: { client: MenderClient }) {
  const [policy, setPolicy] = useState<Policy | null>(null);
  const [status, setStatus] = useState<PolicyStatus | null>(null);
  const [profiles, setProfiles] = useState<Profile[]>([]);
  const [providers, setProviders] = useState<Provider[]>([]);
  const [tools, setTools] = useState<ToolSummary[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState("");

  const [defaultOpen, setDefaultOpen] = useState(false);
  const [defaultDecision, setDefaultDecision] =
    useState<PolicyDecision>("allow");
  const [editor, setEditor] = useState<RuleEditorState | null>(null);

  const refresh = useCallback(async () => {
    try {
      const [policyResult, statusResult, profileResult, providerResult, toolResult] =
        await Promise.all([
          client.getPolicy(),
          client.getPolicyStatus(),
          client.listProfiles(),
          client.listProviders(),
          client.browseTools({ limit: 100 }),
        ]);
      setPolicy(policyResult);
      setStatus(statusResult);
      setProfiles(profileResult.profiles);
      setProviders(providerResult.providers);
      setTools(toolResult.tools);
      setError("");
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setLoaded(true);
    }
  }, [client]);

  useEffect(() => {
    let active = true;
    void Promise.all([
      client.getPolicy(),
      client.getPolicyStatus(),
      client.listProfiles(),
      client.listProviders(),
      client.browseTools({ limit: 100 }),
    ])
      .then(([policyResult, statusResult, profileResult, providerResult, toolResult]) => {
        if (active) {
          setPolicy(policyResult);
          setStatus(statusResult);
          setProfiles(profileResult.profiles);
          setProviders(providerResult.providers);
          setTools(toolResult.tools);
          setError("");
          setLoaded(true);
        }
      })
      .catch((reason: unknown) => {
        if (active) {
          setError(errorMessage(reason));
          setLoaded(true);
        }
      });
    return () => {
      active = false;
    };
  }, [client]);

  const profileById = useMemo(
    () => new Map(profiles.map((profile) => [profile.profileId, profile])),
    [profiles],
  );
  const providerById = useMemo(
    () => new Map(providers.map((provider) => [provider.providerId, provider])),
    [providers],
  );
  const toolById = useMemo(
    () => new Map(tools.map((tool) => [tool.toolId, tool])),
    [tools],
  );

  function openDefaultEditor() {
    if (policy === null) {
      return;
    }
    setEditor(null);
    setDefaultDecision(policy.defaultDecision);
    setDefaultOpen(true);
  }

  async function saveDefault() {
    setBusy("default");
    setError("");
    try {
      await client.setPolicyDefault({ decision: defaultDecision });
      setDefaultOpen(false);
      await refresh();
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy("");
    }
  }

  function openCreateRule() {
    setDefaultOpen(false);
    setEditor({
      ruleId: "",
      step: 0,
      form: emptyRuleForm(),
    });
  }

  function openEditRule(rule: PolicyRule) {
    setDefaultOpen(false);
    setEditor({
      ruleId: rule.ruleId,
      step: 0,
      form: {
        displayName: rule.displayName,
        enabled: rule.enabled,
        decision: rule.decision,
        match: {
          origins: [...rule.match.origins],
          scopeKinds: [...rule.match.scopeKinds],
          profileRefs: [...rule.match.profileRefs],
          providerIds: [...rule.match.providerIds],
          toolIds: [...rule.match.toolIds],
          factKnowledge: rule.match.factKnowledge,
          effectsAny: [...rule.match.effectsAny],
          resourcesAny: rule.match.resourcesAny.map((resource) => ({
            ...resource,
          })),
        },
      },
    });
  }

  async function saveRule() {
    if (editor === null || !ruleDecisionValid(editor.form)) {
      return;
    }
    const key = editor.ruleId || "new-rule";
    setBusy(key);
    setError("");
    try {
      if (editor.ruleId) {
        await client.updatePolicyRule(editor.ruleId, editor.form);
      } else {
        await client.createPolicyRule(editor.form);
      }
      setEditor(null);
      await refresh();
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy("");
    }
  }

  async function toggleRule(rule: PolicyRule) {
    setBusy(rule.ruleId);
    setError("");
    try {
      await client.setPolicyRuleEnabled(rule.ruleId, !rule.enabled);
      await refresh();
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy("");
    }
  }

  async function removeRule(rule: PolicyRule) {
    if (
      !window.confirm(
        t('Delete Policy Rule "{{name}}"?', { name: rule.displayName }),
      )
    ) {
      return;
    }
    setBusy(rule.ruleId);
    setError("");
    try {
      await client.deletePolicyRule(rule.ruleId);
      await refresh();
    } catch (reason) {
      setError(errorMessage(reason));
    } finally {
      setBusy("");
    }
  }

  const rules = policy?.rules ?? [];
  const rulePagination = usePagination(rules);

  return (
    <>
      <PageHeader
        title="Policy"
        description="Define deterministic global execution rules. Rule order has no meaning; matching decisions combine by deny > require approval > allow."
        actions={<Button onClick={openCreateRule}>{t("Add rule")}</Button>}
      />

      {error ? <ErrorNotice message={error} /> : null}

      <MetricStrip>
        <MetricTile
          icon={ShieldCheckIcon}
          label="Default decision"
          value={<DecisionBadge decision={policy?.defaultDecision ?? "allow"} />}
          description="Used only when no enabled Rule matches."
          action={
            <Button
              size="sm"
              variant="outline"
              disabled={!loaded || policy === null}
              onClick={openDefaultEditor}
            >
              {t("Edit")}
            </Button>
          }
        />
        <MetricTile
          icon={PowerIcon}
          label="Enabled rules"
          value={
            status === null
              ? "—"
              : status.enabledRuleCount + " / " + status.ruleCount
          }
          description="Rules are unordered and independently managed."
        />
        <MetricTile
          icon={KeyRoundIcon}
          label="Approval behavior"
          value={t("Fail closed")}
          description="Approval requests reject when no interactive adapter is configured."
        />
      </MetricStrip>

      <DataPanel
        className="mt-5"
        title="Policy rules"
        description={tCount(
          rules.length,
          "{{count}} Policy Rule configured. Matching deny Rules cannot be overridden by allow Rules.",
          "{{count}} Policy Rules configured. Matching deny Rules cannot be overridden by allow Rules.",
        )}
      >
          {!loaded ? (
            <div className="px-4 py-12 text-center text-sm text-muted-foreground">
              {t("Loading Policy…")}
            </div>
          ) : rules.length ? (
            <>
              <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("Rule")}</TableHead>
                  <TableHead>{t("Decision")}</TableHead>
                  <TableHead>{t("State")}</TableHead>
                  <TableHead>{t("Scope / targets")}</TableHead>
                  <TableHead className="text-right">{t("Actions")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {rulePagination.items.map((rule) => (
                  <TableRow key={rule.ruleId}>
                    <TableCell>
                      <div className="font-medium">{rule.displayName}</div>
                      <div className="mt-1 text-xs text-muted-foreground">
                        {factsSummary(rule)}
                      </div>
                    </TableCell>
                    <TableCell>
                      <DecisionBadge decision={rule.decision} />
                    </TableCell>
                    <TableCell>
                      <Badge variant={rule.enabled ? "outline" : "secondary"}>
                        {t(rule.enabled ? "Enabled" : "Disabled")}
                      </Badge>
                    </TableCell>
                    <TableCell className="max-w-[340px]">
                      <p className="truncate text-sm text-muted-foreground">
                        {targetSummary(rule, profileById, providerById, toolById)}
                      </p>
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="flex justify-end gap-2">
                        <IconButton
                          variant="outline"
                          label={t("Edit")}
                          disabled={busy !== ""}
                          onClick={() => openEditRule(rule)}
                        >
                          <PencilIcon />
                        </IconButton>
                        <IconButton
                          variant="outline"
                          label={t(rule.enabled ? "Disable" : "Enable")}
                          disabled={busy !== ""}
                          onClick={() => void toggleRule(rule)}
                        >
                          {rule.enabled ? <PowerOffIcon /> : <PowerIcon />}
                        </IconButton>
                        <IconButton
                          variant="destructive"
                          label={t("Delete")}
                          disabled={busy !== ""}
                          onClick={() => void removeRule(rule)}
                        >
                          <Trash2Icon />
                        </IconButton>
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
              </Table>
              <div className="px-4 pb-3">
                <TablePagination
                  label="Policy rules"
                  pagination={rulePagination}
                />
              </div>
            </>
          ) : (
            <EmptyPanelState>
              <EmptyState
                title="No Policy Rules"
                description="The default decision currently governs every invocation. Add a Rule to target specific origins, scopes, Tools, Providers, or reviewed Governance Facts."
              />
            </EmptyPanelState>
          )}
      </DataPanel>

      <Dialog open={defaultOpen} onOpenChange={setDefaultOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{t("Edit default decision")}</DialogTitle>
            <DialogDescription>
              {t("This decision applies only when no enabled Policy Rule matches.")}
            </DialogDescription>
          </DialogHeader>
          <Field label="Default decision">
            <select
              aria-label={t("Default Policy decision")}
              value={defaultDecision}
              onChange={(event) =>
                setDefaultDecision(event.target.value as PolicyDecision)
              }
              className="h-9 w-full rounded-lg border border-input bg-background px-3 text-sm outline-none focus:ring-2 focus:ring-ring/40"
            >
              <option value="allow">{t("allow")}</option>
              <option value="require_approval">{t("require approval")}</option>
              <option value="deny">{t("deny")}</option>
            </select>
          </Field>
          {defaultDecision === "require_approval" ? (
            <ApprovalWarning />
          ) : null}
          <DialogFooter>
            <Button variant="outline" onClick={() => setDefaultOpen(false)}>
              {t("Cancel")}
            </Button>
            <Button disabled={busy !== ""} onClick={() => void saveDefault()}>
              {t(busy === "default" ? "Saving…" : "Save default")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {editor !== null ? (
        <RuleEditorDialog
          state={editor}
          profiles={profiles}
          providers={providers}
          tools={tools}
          busy={busy !== ""}
          onChange={setEditor}
          onClose={() => setEditor(null)}
          onSave={() => void saveRule()}
        />
      ) : null}
    </>
  );
}

function RuleEditorDialog({
  state,
  profiles,
  providers,
  tools,
  busy,
  onChange,
  onClose,
  onSave,
}: {
  state: RuleEditorState;
  profiles: Profile[];
  providers: Provider[];
  tools: ToolSummary[];
  busy: boolean;
  onChange: (value: RuleEditorState) => void;
  onClose: () => void;
  onSave: () => void;
}) {
  const isGlobal = isGlobalWildcard(state.form);

  function setForm(form: PolicyRuleInput) {
    onChange({ ...state, form });
  }

  function setStep(step: RuleStep) {
    onChange({ ...state, step });
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-h-[88vh] overflow-y-auto sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>
            {t(state.ruleId ? "Edit Policy rule" : "Add Policy rule")}
          </DialogTitle>
          <DialogDescription>
            {t("Step {{step}} of 3 · {{section}}", {
              step: state.step + 1,
              section: t(
                state.step === 0
                  ? "Decision"
                  : state.step === 1
                    ? "Scope and targets"
                    : "Governance Facts",
              ),
            })}
          </DialogDescription>
        </DialogHeader>

        <StepIndicator step={state.step} />

        {state.step === 0 ? (
          <DecisionStep form={state.form} onChange={setForm} />
        ) : null}

        {state.step === 1 ? (
          <ScopeStep
            form={state.form}
            profiles={profiles}
            providers={providers}
            tools={tools}
            onChange={setForm}
          />
        ) : null}

        {state.step === 2 ? (
          <FactsStep form={state.form} onChange={setForm} />
        ) : null}

        {state.step === 2 && isGlobal ? (
          <div className="rounded-lg border border-amber-500/30 bg-amber-500/5 px-3 py-2 text-xs leading-5 text-muted-foreground">
            {t("This Rule is global because every selector is a wildcard. It will match every valid invocation regardless of origin, scope, Provider, Tool, or Governance Facts.")}
          </div>
        ) : null}

        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t("Cancel")}
          </Button>
          {state.step > 0 ? (
            <Button
              variant="outline"
              onClick={() => setStep((state.step - 1) as RuleStep)}
            >
              {t("Back")}
            </Button>
          ) : null}
          {state.step < 2 ? (
            <Button
              disabled={state.step === 0 && !ruleDecisionValid(state.form)}
              onClick={() => setStep((state.step + 1) as RuleStep)}
            >
              {t("Next")}
            </Button>
          ) : (
            <Button
              disabled={busy || !ruleDecisionValid(state.form)}
              onClick={onSave}
            >
              {t(busy ? "Saving…" : state.ruleId ? "Save rule" : "Create rule")}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function DecisionStep({
  form,
  onChange,
}: {
  form: PolicyRuleInput;
  onChange: (value: PolicyRuleInput) => void;
}) {
  return (
    <div className="space-y-5">
      <Field label="Display name">
        <Input
          aria-label={t("Policy Rule display name")}
          placeholder={t("Require approval for MCP writes")}
          value={form.displayName}
          onChange={(event) =>
            onChange({ ...form, displayName: event.target.value })
          }
        />
      </Field>
      <Field label="Decision">
        <select
          aria-label={t("Policy Rule decision")}
          value={form.decision}
          onChange={(event) =>
            onChange({
              ...form,
              decision: event.target.value as PolicyDecision,
            })
          }
          className="h-9 w-full rounded-lg border border-input bg-background px-3 text-sm outline-none focus:ring-2 focus:ring-ring/40"
        >
          <option value="allow">{t("allow")}</option>
          <option value="require_approval">{t("require approval")}</option>
          <option value="deny">{t("deny")}</option>
        </select>
      </Field>
      <label className="flex items-center gap-2 text-sm">
        <input
          type="checkbox"
          checked={form.enabled}
          onChange={(event) =>
            onChange({ ...form, enabled: event.target.checked })
          }
        />
        {t("Enabled")}
      </label>
      {form.decision === "require_approval" ? <ApprovalWarning /> : null}
      <p className="text-xs leading-5 text-muted-foreground">
        {t("Rule order is not configurable. If multiple Rules match, Mender applies the most restrictive decision.")}
      </p>
    </div>
  );
}

function ScopeStep({
  form,
  profiles,
  providers,
  tools,
  onChange,
}: {
  form: PolicyRuleInput;
  profiles: Profile[];
  providers: Provider[];
  tools: ToolSummary[];
  onChange: (value: PolicyRuleInput) => void;
}) {
  return (
    <div className="space-y-6">
      <SelectorSection
        title="Origins"
        description="Empty means any origin."
        options={originOptions}
        selected={form.match.origins}
        onToggle={(value, checked) =>
          onChange({
            ...form,
            match: {
              ...form.match,
              origins: toggleValue(form.match.origins, value, checked),
            },
          })
        }
      />

      <SelectorSection
        title="Execution scope"
        description="Empty means Catalog or Profile scope."
        options={scopeOptions}
        selected={form.match.scopeKinds}
        onToggle={(value, checked) =>
          onChange({
            ...form,
            match: {
              ...form.match,
              scopeKinds: toggleValue(form.match.scopeKinds, value, checked),
              profileRefs:
                value === "profile" && !checked
                  ? []
                  : form.match.profileRefs,
            },
          })
        }
      />

      <InventorySelector
        title="Profiles"
        description="Selecting Profiles restricts this Rule to Profile scope."
        emptyLabel="No Profiles"
        items={profiles.map((profile) => ({
          value: profile.profileId,
          label: profile.displayName,
          detail: profile.slug,
        }))}
        selected={form.match.profileRefs}
        onToggle={(value, checked) =>
          onChange({
            ...form,
            match: {
              ...form.match,
              profileRefs: toggleValue(
                form.match.profileRefs,
                value,
                checked,
              ),
              scopeKinds:
                checked && !form.match.scopeKinds.includes("profile")
                  ? [...form.match.scopeKinds, "profile"]
                  : form.match.scopeKinds,
            },
          })
        }
      />

      <InventorySelector
        title="Providers"
        description="Empty means any Provider."
        emptyLabel="No Providers"
        items={providers.map((provider) => ({
          value: provider.providerId,
          label: provider.displayName,
          detail: provider.namespace,
        }))}
        selected={form.match.providerIds}
        onToggle={(value, checked) =>
          onChange({
            ...form,
            match: {
              ...form.match,
              providerIds: toggleValue(
                form.match.providerIds,
                value,
                checked,
              ),
            },
          })
        }
      />

      <InventorySelector
        title="Tools"
        description="Empty means any Tool."
        emptyLabel="No synchronized Tools"
        items={tools.map((tool) => ({
          value: tool.toolId,
          label: tool.title || tool.name,
          detail:
            providers.find((provider) => provider.providerId === tool.providerId)
              ?.namespace ?? tool.providerId,
        }))}
        selected={form.match.toolIds}
        onToggle={(value, checked) =>
          onChange({
            ...form,
            match: {
              ...form.match,
              toolIds: toggleValue(form.match.toolIds, value, checked),
            },
          })
        }
      />
    </div>
  );
}

function FactsStep({
  form,
  onChange,
}: {
  form: PolicyRuleInput;
  onChange: (value: PolicyRuleInput) => void;
}) {
  return (
    <div className="space-y-5">
      <Field label="Fact knowledge">
        <select
          aria-label={t("Policy fact knowledge")}
          value={form.match.factKnowledge}
          onChange={(event) => {
            const factKnowledge = event.target
              .value as PolicyRuleInput["match"]["factKnowledge"];
            onChange({
              ...form,
              match: {
                ...form.match,
                factKnowledge,
                ...(factKnowledge === "unknown"
                  ? { effectsAny: [], resourcesAny: [] }
                  : {}),
              },
            });
          }}
          className="h-9 w-full rounded-lg border border-input bg-background px-3 text-sm outline-none focus:ring-2 focus:ring-ring/40"
        >
          <option value="any">{t("any")}</option>
          <option value="known">{t("known")}</option>
          <option value="unknown">{t("unknown")}</option>
        </select>
      </Field>

      <p className="text-xs leading-5 text-muted-foreground">
        {t("Effect and resource predicates only match reviewed, known Governance Facts. They never treat unknown facts as an empty known set. Use fact-knowledge = unknown for an explicit fail-closed Rule.")}
      </p>

      {form.match.factKnowledge !== "unknown" ? (
        <>
          <Field label="Effects — one per line" optional>
            <textarea
              aria-label={t("Policy effects")}
              className="min-h-24 w-full rounded-lg border border-input bg-background px-3 py-2 font-mono text-sm outline-none focus:ring-2 focus:ring-ring/40"
              placeholder={"filesystem.write\nnetwork.write"}
              value={form.match.effectsAny.join("\n")}
              onChange={(event) =>
                onChange({
                  ...form,
                  match: {
                    ...form.match,
                    effectsAny: lines(event.target.value),
                  },
                })
              }
            />
          </Field>

          <div className="space-y-3">
            <div className="flex items-end justify-between gap-3">
              <div>
                <p className="text-sm font-medium">{t("Resource matchers")}</p>
                <p className="mt-1 text-xs text-muted-foreground">
                  {t("At least one matcher must match. Only exact and prefix are supported.")}
                </p>
              </div>
              <Button
                size="sm"
                variant="outline"
                onClick={() =>
                  onChange({
                    ...form,
                    match: {
                      ...form.match,
                      resourcesAny: [
                        ...form.match.resourcesAny,
                        { kind: "", operator: "exact", value: "" },
                      ],
                    },
                  })
                }
              >
                {t("Add resource")}
              </Button>
            </div>

            {form.match.resourcesAny.map((resource, index) => (
              <div
                key={index}
                className="grid gap-3 rounded-lg border border-border p-3 sm:grid-cols-[1fr_120px_1.5fr_auto]"
              >
                <Field label="Kind">
                  <Input
                    aria-label={"Policy resource kind " + String(index + 1)}
                    placeholder={t("path")}
                    value={resource.kind}
                    onChange={(event) =>
                      onChange({
                        ...form,
                        match: {
                          ...form.match,
                          resourcesAny: form.match.resourcesAny.map(
                            (item, itemIndex) =>
                              itemIndex === index
                                ? { ...item, kind: event.target.value }
                                : item,
                          ),
                        },
                      })
                    }
                  />
                </Field>
                <Field label="Operator">
                  <select
                    aria-label={
                      "Policy resource operator " + String(index + 1)
                    }
                    value={resource.operator}
                    onChange={(event) =>
                      onChange({
                        ...form,
                        match: {
                          ...form.match,
                          resourcesAny: form.match.resourcesAny.map(
                            (item, itemIndex) =>
                              itemIndex === index
                                ? {
                                    ...item,
                                    operator: event.target
                                      .value as PolicyRuleInput["match"]["resourcesAny"][number]["operator"],
                                  }
                                : item,
                          ),
                        },
                      })
                    }
                    className="h-9 w-full rounded-lg border border-input bg-background px-2 text-sm"
                  >
                    <option value="exact">{t("exact")}</option>
                    <option value="prefix">{t("prefix")}</option>
                  </select>
                </Field>
                <Field label="Value">
                  <Input
                    aria-label={"Policy resource value " + String(index + 1)}
                    placeholder={t("/workspace/release")}
                    value={resource.value}
                    onChange={(event) =>
                      onChange({
                        ...form,
                        match: {
                          ...form.match,
                          resourcesAny: form.match.resourcesAny.map(
                            (item, itemIndex) =>
                              itemIndex === index
                                ? { ...item, value: event.target.value }
                                : item,
                          ),
                        },
                      })
                    }
                  />
                </Field>
                <div className="flex items-end">
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() =>
                      onChange({
                        ...form,
                        match: {
                          ...form.match,
                          resourcesAny: form.match.resourcesAny.filter(
                            (_, itemIndex) => itemIndex !== index,
                          ),
                        },
                      })
                    }
                  >
                    {t("Remove")}
                  </Button>
                </div>
              </div>
            ))}
          </div>
        </>
      ) : null}
    </div>
  );
}

function SelectorSection<T extends string>({
  title,
  description,
  options,
  selected,
  onToggle,
}: {
  title: string;
  description: string;
  options: Array<{ value: T; label: string }>;
  selected: T[];
  onToggle: (value: T, checked: boolean) => void;
}) {
  return (
    <section>
      <p className="text-sm font-medium">{t(title)}</p>
      <p className="mt-1 text-xs text-muted-foreground">{t(description)}</p>
      <div className="mt-3 flex flex-wrap gap-2">
        {options.map((option) => (
          <label
            key={option.value}
            className="flex cursor-pointer items-center gap-2 rounded-lg border border-border px-3 py-2 text-sm"
          >
            <input
              type="checkbox"
              checked={selected.includes(option.value)}
              onChange={(event) =>
                onToggle(option.value, event.target.checked)
              }
            />
            {t(option.label)}
          </label>
        ))}
      </div>
    </section>
  );
}

function InventorySelector({
  title,
  description,
  emptyLabel,
  items,
  selected,
  onToggle,
}: {
  title: string;
  description: string;
  emptyLabel: string;
  items: Array<{ value: string; label: string; detail: string }>;
  selected: string[];
  onToggle: (value: string, checked: boolean) => void;
}) {
  return (
    <section>
      <p className="text-sm font-medium">{t(title)}</p>
      <p className="mt-1 text-xs text-muted-foreground">{t(description)}</p>
      {items.length ? (
        <div className="mt-3 max-h-52 space-y-2 overflow-y-auto pr-1">
          {items.map((item) => (
            <label
              key={item.value}
              className="flex cursor-pointer items-start gap-3 rounded-lg border border-border px-3 py-2"
            >
              <input
                type="checkbox"
                className="mt-1"
                checked={selected.includes(item.value)}
                onChange={(event) =>
                  onToggle(item.value, event.target.checked)
                }
              />
              <span className="min-w-0">
                <span className="block truncate text-sm font-medium">
                  {item.label}
                </span>
                <span className="mt-0.5 block truncate text-xs text-muted-foreground">
                  {item.detail}
                </span>
              </span>
            </label>
          ))}
        </div>
      ) : (
        <p className="mt-3 text-xs text-muted-foreground">{t(emptyLabel)}</p>
      )}
    </section>
  );
}

function StepIndicator({ step }: { step: RuleStep }) {
  const labels = ["Decision", "Scope / targets", "Governance Facts"];
  return (
    <div className="grid grid-cols-3 gap-2">
      {labels.map((label, index) => (
        <div
          key={label}
          className={
            "rounded-lg border px-3 py-2 text-center text-xs " +
            (index === step
              ? "border-foreground/30 bg-muted font-medium"
              : "border-border text-muted-foreground")
          }
        >
          {t(label)}
        </div>
      ))}
    </div>
  );
}

function DecisionBadge({ decision }: { decision: PolicyDecision }) {
  return (
    <Badge
      variant={
        decision === "deny"
          ? "destructive"
          : decision === "require_approval"
            ? "secondary"
            : "outline"
      }
    >
      {t(decision === "require_approval" ? "Require approval" : decision)}
    </Badge>
  );
}

function ApprovalWarning() {
  return (
    <div className="rounded-lg border border-amber-500/30 bg-amber-500/5 px-3 py-2 text-xs leading-5 text-muted-foreground">
      {t("The current Approval adapter is fail-closed. Invocations requiring approval are rejected until an interactive Approval workflow exists.")}
    </div>
  );
}

function Field({
  label,
  optional = false,
  children,
}: {
  label: string;
  optional?: boolean;
  children: React.ReactNode;
}) {
  return (
    <label className="block">
      <span className="mb-1.5 flex items-center gap-2 text-sm font-medium">
        {t(label)}
        {optional ? (
          <span className="text-xs font-normal text-muted-foreground">
            {t("Optional")}
          </span>
        ) : null}
      </span>
      {children}
    </label>
  );
}

function ruleDecisionValid(form: PolicyRuleInput) {
  return form.displayName.trim() !== "";
}

function isGlobalWildcard(form: PolicyRuleInput) {
  return (
    form.match.origins.length === 0 &&
    form.match.scopeKinds.length === 0 &&
    form.match.profileRefs.length === 0 &&
    form.match.providerIds.length === 0 &&
    form.match.toolIds.length === 0 &&
    form.match.factKnowledge === "any" &&
    form.match.effectsAny.length === 0 &&
    form.match.resourcesAny.length === 0
  );
}

function toggleValue<T extends string>(
  values: T[],
  value: T,
  checked: boolean,
) {
  if (checked) {
    return values.includes(value) ? values : [...values, value];
  }
  return values.filter((item) => item !== value);
}

function lines(value: string) {
  return value
    .split("\n")
    .map((item) => item.trim())
    .filter(Boolean);
}

function factsSummary(rule: PolicyRule) {
  const parts: string[] = [];
  if (rule.match.factKnowledge !== "any") {
    parts.push(
      t("facts: {{value}}", { value: t(rule.match.factKnowledge) }),
    );
  }
  if (rule.match.effectsAny.length) {
    parts.push(
      tCount(
        rule.match.effectsAny.length,
        "{{count}} effect",
        "{{count}} effects",
      ),
    );
  }
  if (rule.match.resourcesAny.length) {
    parts.push(
      tCount(
        rule.match.resourcesAny.length,
        "{{count}} resource",
        "{{count}} resources",
      ),
    );
  }
  return parts.length ? parts.join(" · ") : t("No Governance Fact predicates");
}

function targetSummary(
  rule: PolicyRule,
  profiles: Map<string, Profile>,
  providers: Map<string, Provider>,
  tools: Map<string, ToolSummary>,
) {
  const parts: string[] = [];
  if (rule.match.origins.length) {
    parts.push(
      t("origins: {{value}}", {
        value: rule.match.origins
          .map(
            (origin) =>
              t(originOptions.find((option) => option.value === origin)?.label ?? origin),
          )
          .join(", "),
      }),
    );
  }
  if (rule.match.scopeKinds.length) {
    parts.push(
      t("scope: {{value}}", {
        value: rule.match.scopeKinds
          .map(
            (scope) =>
              t(scopeOptions.find((option) => option.value === scope)?.label ?? scope),
          )
          .join(", "),
      }),
    );
  }
  if (rule.match.profileRefs.length) {
    parts.push(
      t("profiles: {{value}}", {
        value: rule.match.profileRefs
          .map((id) => profiles.get(id)?.slug ?? t("Missing Profile"))
          .join(", "),
      }),
    );
  }
  if (rule.match.providerIds.length) {
    parts.push(
      t("providers: {{value}}", {
        value: rule.match.providerIds
          .map((id) => providers.get(id)?.namespace ?? t("Missing Provider"))
          .join(", "),
      }),
    );
  }
  if (rule.match.toolIds.length) {
    parts.push(
      t("tools: {{value}}", {
        value: rule.match.toolIds
          .map(
            (id) =>
              tools.get(id)?.title || tools.get(id)?.name || t("Missing Tool"),
          )
          .join(", "),
      }),
    );
  }
  return parts.length ? parts.join(" · ") : t("Global wildcard");
}
