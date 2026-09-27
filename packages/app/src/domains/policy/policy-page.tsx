import { useCallback, useEffect, useMemo, useState } from "react";

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
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@mender/ui/components/card";
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

import { EmptyState, ErrorNotice, PageHeader } from "../../shared/page";

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
    if (!window.confirm(`Delete Policy Rule "${rule.displayName}"?`)) {
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

  return (
    <>
      <PageHeader
        title="Policy"
        description="Define deterministic global execution rules. Rule order has no meaning; matching decisions combine by deny > require approval > allow."
        actions={<Button onClick={openCreateRule}>Add rule</Button>}
      />

      {error ? <ErrorNotice message={error} /> : null}

      <div className="mb-6 grid gap-4 md:grid-cols-3">
        <Card>
          <CardHeader className="pb-2">
            <CardDescription>Default decision</CardDescription>
            <CardTitle className="flex items-center justify-between gap-3 text-lg">
              <DecisionBadge decision={policy?.defaultDecision ?? "allow"} />
              <Button
                size="sm"
                variant="outline"
                disabled={!loaded || policy === null}
                onClick={openDefaultEditor}
              >
                Edit
              </Button>
            </CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-sm text-muted-foreground">
              Used only when no enabled Rule matches.
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardDescription>Enabled rules</CardDescription>
            <CardTitle className="text-lg">
              {status?.enabledRuleCount ?? "—"} / {status?.ruleCount ?? "—"}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-sm text-muted-foreground">
              Rules are unordered and independently managed.
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardDescription>Approval behavior</CardDescription>
            <CardTitle className="text-lg">Fail closed</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-sm text-muted-foreground">
              require approval currently rejects when no interactive Approval
              adapter is configured.
            </p>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader className="border-b">
          <CardTitle>Policy rules</CardTitle>
          <CardDescription>
            {rules.length} configured {rules.length === 1 ? "Rule" : "Rules"}.
            Matching deny Rules cannot be overridden by allow Rules.
          </CardDescription>
        </CardHeader>
        <CardContent>
          {!loaded ? (
            <div className="py-10 text-center text-sm text-muted-foreground">
              Loading Policy…
            </div>
          ) : rules.length ? (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Rule</TableHead>
                  <TableHead>Decision</TableHead>
                  <TableHead>State</TableHead>
                  <TableHead>Scope / targets</TableHead>
                  <TableHead className="text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {rules.map((rule) => (
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
                        {rule.enabled ? "Enabled" : "Disabled"}
                      </Badge>
                    </TableCell>
                    <TableCell className="max-w-[340px]">
                      <p className="truncate text-sm text-muted-foreground">
                        {targetSummary(rule, profileById, providerById, toolById)}
                      </p>
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="flex justify-end gap-2">
                        <Button
                          size="sm"
                          variant="outline"
                          disabled={busy !== ""}
                          onClick={() => openEditRule(rule)}
                        >
                          Edit
                        </Button>
                        <Button
                          size="sm"
                          variant="outline"
                          disabled={busy !== ""}
                          onClick={() => void toggleRule(rule)}
                        >
                          {rule.enabled ? "Disable" : "Enable"}
                        </Button>
                        <Button
                          size="sm"
                          variant="destructive"
                          disabled={busy !== ""}
                          onClick={() => void removeRule(rule)}
                        >
                          Delete
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          ) : (
            <EmptyState
              title="No Policy Rules"
              description="The default decision currently governs every invocation. Add a Rule to target specific origins, scopes, Tools, Providers, or reviewed Governance Facts."
            />
          )}
        </CardContent>
      </Card>

      <Dialog open={defaultOpen} onOpenChange={setDefaultOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>Edit default decision</DialogTitle>
            <DialogDescription>
              This decision applies only when no enabled Policy Rule matches.
            </DialogDescription>
          </DialogHeader>
          <Field label="Default decision">
            <select
              aria-label="Default Policy decision"
              value={defaultDecision}
              onChange={(event) =>
                setDefaultDecision(event.target.value as PolicyDecision)
              }
              className="h-9 w-full rounded-lg border border-input bg-background px-3 text-sm outline-none focus:ring-2 focus:ring-ring/40"
            >
              <option value="allow">allow</option>
              <option value="require_approval">require approval</option>
              <option value="deny">deny</option>
            </select>
          </Field>
          {defaultDecision === "require_approval" ? (
            <ApprovalWarning />
          ) : null}
          <DialogFooter>
            <Button variant="outline" onClick={() => setDefaultOpen(false)}>
              Cancel
            </Button>
            <Button disabled={busy !== ""} onClick={() => void saveDefault()}>
              {busy === "default" ? "Saving…" : "Save default"}
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
            {state.ruleId ? "Edit Policy rule" : "Add Policy rule"}
          </DialogTitle>
          <DialogDescription>
            Step {state.step + 1} of 3 ·{" "}
            {state.step === 0
              ? "Decision"
              : state.step === 1
                ? "Scope and targets"
                : "Governance Facts"}
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
            This Rule is global because every selector is a wildcard. It will
            match every valid invocation regardless of origin, scope, Provider,
            Tool, or Governance Facts.
          </div>
        ) : null}

        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            Cancel
          </Button>
          {state.step > 0 ? (
            <Button
              variant="outline"
              onClick={() => setStep((state.step - 1) as RuleStep)}
            >
              Back
            </Button>
          ) : null}
          {state.step < 2 ? (
            <Button
              disabled={state.step === 0 && !ruleDecisionValid(state.form)}
              onClick={() => setStep((state.step + 1) as RuleStep)}
            >
              Next
            </Button>
          ) : (
            <Button
              disabled={busy || !ruleDecisionValid(state.form)}
              onClick={onSave}
            >
              {busy ? "Saving…" : state.ruleId ? "Save rule" : "Create rule"}
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
          aria-label="Policy Rule display name"
          placeholder="Require approval for MCP writes"
          value={form.displayName}
          onChange={(event) =>
            onChange({ ...form, displayName: event.target.value })
          }
        />
      </Field>
      <Field label="Decision">
        <select
          aria-label="Policy Rule decision"
          value={form.decision}
          onChange={(event) =>
            onChange({
              ...form,
              decision: event.target.value as PolicyDecision,
            })
          }
          className="h-9 w-full rounded-lg border border-input bg-background px-3 text-sm outline-none focus:ring-2 focus:ring-ring/40"
        >
          <option value="allow">allow</option>
          <option value="require_approval">require approval</option>
          <option value="deny">deny</option>
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
        Enabled
      </label>
      {form.decision === "require_approval" ? <ApprovalWarning /> : null}
      <p className="text-xs leading-5 text-muted-foreground">
        Rule order is not configurable. If multiple Rules match, Mender applies
        the most restrictive decision.
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
                value === "catalog" &&
                checked &&
                form.match.scopeKinds.length === 0
                  ? form.match.profileRefs
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
          aria-label="Policy fact knowledge"
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
          <option value="any">any</option>
          <option value="known">known</option>
          <option value="unknown">unknown</option>
        </select>
      </Field>

      <p className="text-xs leading-5 text-muted-foreground">
        Effect and resource predicates only match reviewed, known Governance
        Facts. They never treat unknown facts as an empty known set. Use
        fact-knowledge = unknown for an explicit fail-closed Rule.
      </p>

      {form.match.factKnowledge !== "unknown" ? (
        <>
          <Field label="Effects — one per line" optional>
            <textarea
              aria-label="Policy effects"
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
                <p className="text-sm font-medium">Resource matchers</p>
                <p className="mt-1 text-xs text-muted-foreground">
                  At least one matcher must match. Only exact and prefix are
                  supported.
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
                Add resource
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
                    placeholder="path"
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
                    <option value="exact">exact</option>
                    <option value="prefix">prefix</option>
                  </select>
                </Field>
                <Field label="Value">
                  <Input
                    aria-label={"Policy resource value " + String(index + 1)}
                    placeholder="/workspace/release"
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
                    Remove
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
      <p className="text-sm font-medium">{title}</p>
      <p className="mt-1 text-xs text-muted-foreground">{description}</p>
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
            {option.label}
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
      <p className="text-sm font-medium">{title}</p>
      <p className="mt-1 text-xs text-muted-foreground">{description}</p>
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
        <p className="mt-3 text-xs text-muted-foreground">{emptyLabel}</p>
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
          {label}
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
      {decision === "require_approval" ? "Require approval" : decision}
    </Badge>
  );
}

function ApprovalWarning() {
  return (
    <div className="rounded-lg border border-amber-500/30 bg-amber-500/5 px-3 py-2 text-xs leading-5 text-muted-foreground">
      The current Approval adapter is fail-closed. Invocations requiring
      approval are rejected until an interactive Approval workflow exists.
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
        {label}
        {optional ? (
          <span className="text-xs font-normal text-muted-foreground">
            Optional
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
    parts.push("facts: " + rule.match.factKnowledge);
  }
  if (rule.match.effectsAny.length) {
    parts.push(rule.match.effectsAny.length + " effect");
  }
  if (rule.match.resourcesAny.length) {
    parts.push(rule.match.resourcesAny.length + " resource");
  }
  return parts.length ? parts.join(" · ") : "No Governance Fact predicates";
}

function targetSummary(
  rule: PolicyRule,
  profiles: Map<string, Profile>,
  providers: Map<string, Provider>,
  tools: Map<string, ToolSummary>,
) {
  const parts: string[] = [];
  if (rule.match.origins.length) {
    parts.push("origins: " + rule.match.origins.join(", "));
  }
  if (rule.match.scopeKinds.length) {
    parts.push("scope: " + rule.match.scopeKinds.join(", "));
  }
  if (rule.match.profileRefs.length) {
    parts.push(
      "profiles: " +
        rule.match.profileRefs
          .map((id) => profiles.get(id)?.slug ?? "Missing Profile")
          .join(", "),
    );
  }
  if (rule.match.providerIds.length) {
    parts.push(
      "providers: " +
        rule.match.providerIds
          .map((id) => providers.get(id)?.namespace ?? "Missing Provider")
          .join(", "),
    );
  }
  if (rule.match.toolIds.length) {
    parts.push(
      "tools: " +
        rule.match.toolIds
          .map((id) => tools.get(id)?.title || tools.get(id)?.name || "Missing Tool")
          .join(", "),
    );
  }
  return parts.length ? parts.join(" · ") : "Global wildcard";
}

function errorMessage(reason: unknown) {
  return reason instanceof Error ? reason.message : "Unexpected error";
}
