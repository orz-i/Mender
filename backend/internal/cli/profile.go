package cli

import (
	"fmt"
	"text/tabwriter"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	profileapplication "github.com/orz-i/Mender/backend/internal/profile/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	"github.com/spf13/cobra"
)

func (a *app) newProfileCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "profile",
		Short: "Manage tool exposure profiles",
	}
	command.AddCommand(a.newProfileCreateCommand())
	command.AddCommand(a.newProfileListCommand())
	command.AddCommand(a.newProfileShowCommand())
	command.AddCommand(a.newProfileAddToolCommand())
	command.AddCommand(a.newProfileRemoveToolCommand())
	return command
}

func (a *app) newProfileCreateCommand() *cobra.Command {
	var slugValue, displayName, modeValue string

	command := &cobra.Command{
		Use:   "create",
		Short: "Create a profile",
		Args:  exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if slugValue == "" || displayName == "" || modeValue == "" {
				return usageError{err: fmt.Errorf("--slug, --name, and --mode are required")}
			}
			mode, err := parseExposureMode(modeValue)
			if err != nil {
				return usageError{err: err}
			}

			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				profile, err := runtime.Profiles.Create.Execute(
					cmd.Context(),
					profileapplication.CreateProfileRequest{
						Slug:        slugValue,
						DisplayName: displayName,
						Mode:        mode,
					},
				)
				if err != nil {
					return err
				}
				return a.writeValue(
					fmt.Sprintf("created profile %s (%s)", profile.Slug(), profile.ID()),
					profileProjection(profile),
				)
			})
		},
	}
	command.Flags().StringVar(&slugValue, "slug", "", "stable profile slug")
	command.Flags().StringVar(&displayName, "name", "", "profile display name")
	command.Flags().StringVar(&modeValue, "mode", "", "exposure mode: direct, dynamic, or hybrid")
	return command
}

func (a *app) newProfileListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List profiles",
		Args:  exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				profiles, err := runtime.Profiles.List.Execute(cmd.Context())
				if err != nil {
					return err
				}

				if a.output == "json" {
					items := make([]map[string]any, 0, len(profiles))
					for _, profile := range profiles {
						items = append(items, profileProjection(profile))
					}
					return a.writeValue("", map[string]any{"profiles": items})
				}

				writer := tabwriter.NewWriter(a.options.Stdout, 0, 4, 2, ' ', 0)
				_, _ = fmt.Fprintln(writer, "SLUG\tID\tMODE\tTOOLS\tNAME")
				for _, profile := range profiles {
					_, _ = fmt.Fprintf(
						writer,
						"%s\t%s\t%s\t%d\t%s\n",
						profile.Slug(),
						profile.ID(),
						profile.Mode(),
						len(profile.Bindings()),
						profile.DisplayName(),
					)
				}
				return writer.Flush()
			})
		},
	}
}

func (a *app) newProfileShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show <profile>",
		Short: "Show one profile",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug, err := profiledomain.NewSlug(args[0])
			if err != nil {
				return usageError{err: err}
			}

			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				profile, err := runtime.Profiles.Get.Execute(cmd.Context(), slug)
				if err != nil {
					return err
				}
				return a.writeValue(
					fmt.Sprintf(
						"%s (%s) mode=%s tools=%d",
						profile.Slug(),
						profile.ID(),
						profile.Mode(),
						len(profile.Bindings()),
					),
					profileProjection(profile),
				)
			})
		},
	}
}

func (a *app) newProfileAddToolCommand() *cobra.Command {
	var direct bool
	var alias string
	var titleOverride string
	var descriptionOverride string
	var policyValue string
	var concurrencyValue string
	var credentialRefValues []string

	command := &cobra.Command{
		Use:   "add-tool <profile> <tool-id>",
		Short: "Add a Tool to a profile",
		Args:  exactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug, toolID, err := parseProfileToolArgs(args)
			if err != nil {
				return usageError{err: err}
			}
			policy, err := parseProfilePolicyConstraint(policyValue)
			if err != nil {
				return usageError{err: err}
			}
			concurrency, err := parseProfileConcurrencyConstraint(concurrencyValue)
			if err != nil {
				return usageError{err: err}
			}
			credentialRefs, err := parseCredentialRefValues(credentialRefValues)
			if err != nil {
				return usageError{err: err}
			}

			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				profile, err := runtime.Profiles.AddTool.Execute(
					cmd.Context(),
					profileapplication.AddToolRequest{
						ProfileSlug:           slug,
						ToolID:                toolID,
						Direct:                direct,
						Alias:                 alias,
						TitleOverride:         titleOverride,
						DescriptionOverride:   descriptionOverride,
						PolicyConstraint:      policy,
						ConcurrencyConstraint: concurrency,
						CredentialRefs:        credentialRefs,
					},
				)
				if err != nil {
					return err
				}
				return a.writeValue(
					fmt.Sprintf("added %s to profile %s", toolID, slug),
					profileProjection(profile),
				)
			})
		},
	}
	command.Flags().BoolVar(&direct, "direct", false, "export this Tool directly in hybrid mode")
	command.Flags().StringVar(&alias, "alias", "", "Profile-local machine/model Tool alias")
	command.Flags().StringVar(&titleOverride, "title", "", "Profile-local Tool title override")
	command.Flags().StringVar(&descriptionOverride, "description", "", "Profile-local Tool description override")
	command.Flags().StringVar(
		&policyValue,
		"policy",
		"inherit",
		"binding policy: inherit, require_approval, or deny",
	)
	command.Flags().StringVar(
		&concurrencyValue,
		"concurrency",
		"inherit",
		"binding concurrency: inherit or serial",
	)
	command.Flags().StringArrayVar(
		&credentialRefValues,
		"credential-ref",
		nil,
		"allow one inventory Credential Reference for this binding; may be repeated",
	)
	return command
}

func (a *app) newProfileRemoveToolCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "remove-tool <profile> <tool-id>",
		Short: "Remove a Tool from a profile",
		Args:  exactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug, toolID, err := parseProfileToolArgs(args)
			if err != nil {
				return usageError{err: err}
			}

			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				profile, err := runtime.Profiles.RemoveTool.Execute(
					cmd.Context(),
					profileapplication.RemoveToolRequest{
						ProfileSlug: slug,
						ToolID:      toolID,
					},
				)
				if err != nil {
					return err
				}
				return a.writeValue(
					fmt.Sprintf("removed %s from profile %s", toolID, slug),
					profileProjection(profile),
				)
			})
		},
	}
}

func parseExposureMode(value string) (profiledomain.ExposureMode, error) {
	mode := profiledomain.ExposureMode(value)
	switch mode {
	case profiledomain.ExposureDirect,
		profiledomain.ExposureDynamic,
		profiledomain.ExposureHybrid:
		return mode, nil
	default:
		return "", fmt.Errorf("mode must be direct, dynamic, or hybrid")
	}
}

func parseProfilePolicyConstraint(value string) (profiledomain.PolicyConstraint, error) {
	constraint := profiledomain.PolicyConstraint(value)
	switch constraint {
	case profiledomain.PolicyInherit,
		profiledomain.PolicyRequireApproval,
		profiledomain.PolicyDeny:
		return constraint, nil
	default:
		return "", fmt.Errorf("policy must be inherit, require_approval, or deny")
	}
}

func parseProfileConcurrencyConstraint(value string) (profiledomain.ConcurrencyConstraint, error) {
	constraint := profiledomain.ConcurrencyConstraint(value)
	switch constraint {
	case profiledomain.ConcurrencyInherit, profiledomain.ConcurrencySerial:
		return constraint, nil
	default:
		return "", fmt.Errorf("concurrency must be inherit or serial")
	}
}

func parseCredentialRefValues(values []string) ([]credentialdomain.Reference, error) {
	result := make([]credentialdomain.Reference, 0, len(values))
	for _, value := range values {
		ref, err := credentialdomain.ParseReference(value)
		if err != nil {
			return nil, err
		}
		result = append(result, ref)
	}
	return result, nil
}

func credentialRefStrings(values []credentialdomain.Reference) []string {
	result := make([]string, 0, len(values))
	for _, ref := range values {
		result = append(result, ref.String())
	}
	return result
}

func parseProfileToolArgs(
	args []string,
) (profiledomain.Slug, catalogdomain.ToolID, error) {
	slug, err := profiledomain.NewSlug(args[0])
	if err != nil {
		return "", "", err
	}
	toolID, err := catalogdomain.ParseToolID(args[1])
	if err != nil {
		return "", "", err
	}
	return slug, toolID, nil
}

func profileProjection(profile profiledomain.Profile) map[string]any {
	bindings := make([]map[string]any, 0, len(profile.Bindings()))
	for _, binding := range profile.Bindings() {
		bindings = append(bindings, map[string]any{
			"toolId":                binding.ToolID().String(),
			"direct":                binding.Direct(),
			"alias":                 binding.Alias(),
			"titleOverride":         binding.TitleOverride(),
			"descriptionOverride":   binding.DescriptionOverride(),
			"policyConstraint":      string(binding.PolicyConstraint()),
			"concurrencyConstraint": string(binding.ConcurrencyConstraint()),
			"credentialRefs":        credentialRefStrings(binding.CredentialRefs()),
		})
	}
	return map[string]any{
		"profileId":   profile.ID().String(),
		"slug":        profile.Slug().String(),
		"displayName": profile.DisplayName(),
		"mode":        string(profile.Mode()),
		"tools":       bindings,
	}
}
