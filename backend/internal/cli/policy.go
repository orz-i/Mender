package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	policyapplication "github.com/orz-i/Mender/backend/internal/policy/application"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
	"github.com/spf13/cobra"
)

func (a *app) newPolicyCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "policy",
		Short: "Manage persistent Policy governance",
	}
	command.AddCommand(a.newPolicyShowCommand())
	command.AddCommand(a.newPolicySetDefaultCommand())
	command.AddCommand(a.newPolicyRuleCommand())
	return command
}

func (a *app) newPolicyShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show the persistent PolicySet",
		Args:  exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				policy, err := runtime.Policy.Get.Execute(cmd.Context())
				if err != nil {
					return err
				}
				if a.output == "json" {
					return a.writeValue("", policyProjection(policy))
				}

				writer := tabwriter.NewWriter(a.options.Stdout, 0, 4, 2, ' ', 0)
				_, _ = fmt.Fprintf(
					writer,
					"DEFAULT\t%s\nRULES\t%d\n\n",
					policy.DefaultDecision(),
					len(policy.Rules()),
				)
				if len(policy.Rules()) != 0 {
					_, _ = fmt.Fprintln(writer, "RULE ID\tENABLED\tDECISION\tNAME")
					for _, rule := range policy.Rules() {
						_, _ = fmt.Fprintf(
							writer,
							"%s\t%t\t%s\t%s\n",
							rule.ID(),
							rule.Enabled(),
							rule.Decision(),
							rule.DisplayName(),
						)
					}
				}
				return writer.Flush()
			})
		},
	}
}

func (a *app) newPolicySetDefaultCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "set-default <allow|require_approval|deny>",
		Short: "Set the decision used when no Policy Rule matches",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			decision, err := parsePolicyDecision(args[0])
			if err != nil {
				return usageError{err: err}
			}
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				policy, err := runtime.Policy.SetDefault.Execute(cmd.Context(), decision)
				if err != nil {
					return err
				}
				return a.writeValue(
					fmt.Sprintf("set Policy default to %s", decision),
					policyProjection(policy),
				)
			})
		},
	}
}

func (a *app) newPolicyRuleCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "rule",
		Short: "Manage Policy Rules",
	}
	command.AddCommand(a.newPolicyRuleAddCommand())
	command.AddCommand(a.newPolicyRuleRemoveCommand())
	command.AddCommand(a.newPolicyRuleEnableCommand(true))
	command.AddCommand(a.newPolicyRuleEnableCommand(false))
	return command
}

func (a *app) newPolicyRuleAddCommand() *cobra.Command {
	var displayName string
	var decisionValue string
	var disabled bool
	var origins []string
	var scopes []string
	var profileRefs []string
	var providerIDs []string
	var toolIDs []string
	var factKnowledge string
	var effects []string
	var resources []string

	command := &cobra.Command{
		Use:   "add",
		Short: "Add a persistent Policy Rule",
		Args:  exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(displayName) == "" {
				return usageError{err: fmt.Errorf("--name is required")}
			}
			decision, err := parsePolicyDecision(decisionValue)
			if err != nil {
				return usageError{err: err}
			}
			match, err := parsePolicyMatchFlags(
				origins,
				scopes,
				profileRefs,
				providerIDs,
				toolIDs,
				factKnowledge,
				effects,
				resources,
			)
			if err != nil {
				return usageError{err: err}
			}
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				rule, err := runtime.Policy.CreateRule.Execute(
					cmd.Context(),
					policyapplication.RuleInput{
						DisplayName: displayName,
						Enabled:     !disabled,
						Decision:    decision,
						Match:       match,
					},
				)
				if err != nil {
					return err
				}
				return a.writeValue(
					fmt.Sprintf("created Policy Rule %s", rule.ID()),
					policyRuleProjection(rule),
				)
			})
		},
	}

	command.Flags().StringVar(&displayName, "name", "", "human-readable Rule name")
	command.Flags().StringVar(
		&decisionValue,
		"decision",
		"",
		"Rule decision: allow, require_approval, or deny",
	)
	command.Flags().BoolVar(&disabled, "disabled", false, "create the Rule disabled")
	command.Flags().StringArrayVar(&origins, "origin", nil, "match origin; may be repeated")
	command.Flags().StringArrayVar(&scopes, "scope", nil, "match scope kind catalog/profile; may be repeated")
	command.Flags().StringArrayVar(&profileRefs, "profile-ref", nil, "match ProfileID; may be repeated")
	command.Flags().StringArrayVar(&providerIDs, "provider-id", nil, "match ProviderID; may be repeated")
	command.Flags().StringArrayVar(&toolIDs, "tool-id", nil, "match ToolID; may be repeated")
	command.Flags().StringVar(
		&factKnowledge,
		"fact-knowledge",
		"any",
		"match Governance Fact knowledge: any, known, or unknown",
	)
	command.Flags().StringArrayVar(&effects, "effect", nil, "match one effect; may be repeated")
	command.Flags().StringArrayVar(
		&resources,
		"resource",
		nil,
		"resource matcher OPERATOR:KIND:VALUE where OPERATOR is exact or prefix; may be repeated",
	)
	return command
}

func (a *app) newPolicyRuleRemoveCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <rule-id>",
		Short: "Remove a Policy Rule",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := policydomain.ParseRuleID(args[0])
			if err != nil {
				return usageError{err: err}
			}
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				if err := runtime.Policy.DeleteRule.Execute(cmd.Context(), id); err != nil {
					return err
				}
				return a.writeValue(
					fmt.Sprintf("removed Policy Rule %s", id),
					map[string]any{"ruleId": id.String()},
				)
			})
		},
	}
}

func (a *app) newPolicyRuleEnableCommand(enabled bool) *cobra.Command {
	action := "enable"
	short := "Enable a Policy Rule"
	if !enabled {
		action = "disable"
		short = "Disable a Policy Rule"
	}
	return &cobra.Command{
		Use:   action + " <rule-id>",
		Short: short,
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := policydomain.ParseRuleID(args[0])
			if err != nil {
				return usageError{err: err}
			}
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				rule, err := runtime.Policy.SetRuleEnabled.Execute(
					cmd.Context(),
					id,
					enabled,
				)
				if err != nil {
					return err
				}
				return a.writeValue(
					fmt.Sprintf("%sd Policy Rule %s", action, id),
					policyRuleProjection(rule),
				)
			})
		},
	}
}

func parsePolicyDecision(value string) (policydomain.DecisionKind, error) {
	decision := policydomain.DecisionKind(strings.TrimSpace(value))
	if err := policydomain.ValidateDecisionKind(decision); err != nil {
		return "", fmt.Errorf(
			"decision must be allow, require_approval, or deny",
		)
	}
	return decision, nil
}

func parsePolicyMatchFlags(
	originValues []string,
	scopeValues []string,
	profileRefs []string,
	providerIDs []string,
	toolIDs []string,
	factKnowledgeValue string,
	effects []string,
	resourceValues []string,
) (policydomain.MatchSpec, error) {
	origins := make([]policydomain.Origin, 0, len(originValues))
	for _, value := range originValues {
		origins = append(origins, policydomain.Origin(value))
	}
	scopeKinds := make([]policydomain.ScopeKind, 0, len(scopeValues))
	for _, value := range scopeValues {
		scopeKinds = append(scopeKinds, policydomain.ScopeKind(value))
	}
	resources := make([]policydomain.ResourceMatcher, 0, len(resourceValues))
	for _, value := range resourceValues {
		parts := strings.SplitN(value, ":", 3)
		if len(parts) != 3 {
			return policydomain.MatchSpec{}, fmt.Errorf(
				"resource must use OPERATOR:KIND:VALUE",
			)
		}
		resource, err := policydomain.NewResourceMatcher(
			parts[1],
			policydomain.ResourceOperator(parts[0]),
			parts[2],
		)
		if err != nil {
			return policydomain.MatchSpec{}, err
		}
		resources = append(resources, resource)
	}
	spec := policydomain.MatchSpec{
		Origins:       origins,
		ScopeKinds:    scopeKinds,
		ProfileRefs:   append([]string(nil), profileRefs...),
		ProviderIDs:   append([]string(nil), providerIDs...),
		ToolIDs:       append([]string(nil), toolIDs...),
		FactKnowledge: policydomain.FactKnowledge(factKnowledgeValue),
		EffectsAny:    append([]string(nil), effects...),
		ResourcesAny:  resources,
	}
	if _, err := policydomain.NewMatch(spec); err != nil {
		return policydomain.MatchSpec{}, err
	}
	return spec, nil
}

func policyProjection(policy policydomain.PolicySet) map[string]any {
	rules := make([]map[string]any, 0, len(policy.Rules()))
	for _, rule := range policy.Rules() {
		rules = append(rules, policyRuleProjection(rule))
	}
	return map[string]any{
		"defaultDecision": string(policy.DefaultDecision()),
		"rules":           rules,
	}
}

func policyRuleProjection(rule policydomain.Rule) map[string]any {
	match := rule.Match()
	resources := make([]map[string]any, 0, len(match.ResourcesAny()))
	for _, resource := range match.ResourcesAny() {
		resources = append(resources, map[string]any{
			"kind":     resource.Kind(),
			"operator": string(resource.Operator()),
			"value":    resource.Value(),
		})
	}
	return map[string]any{
		"ruleId":      rule.ID().String(),
		"displayName": rule.DisplayName(),
		"enabled":     rule.Enabled(),
		"decision":    string(rule.Decision()),
		"match": map[string]any{
			"origins":       policyOriginStringsCLI(match.Origins()),
			"scopeKinds":    policyScopeStringsCLI(match.ScopeKinds()),
			"profileRefs":   match.ProfileRefs(),
			"providerIds":   match.ProviderIDs(),
			"toolIds":       match.ToolIDs(),
			"factKnowledge": string(match.FactKnowledge()),
			"effectsAny":    match.EffectsAny(),
			"resourcesAny":  resources,
		},
		"createdAt": rule.CreatedAt(),
		"updatedAt": rule.UpdatedAt(),
	}
}

func policyOriginStringsCLI(values []policydomain.Origin) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, string(value))
	}
	return result
}

func policyScopeStringsCLI(values []policydomain.ScopeKind) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, string(value))
	}
	return result
}
