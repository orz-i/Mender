package cli

import (
	"fmt"
	"text/tabwriter"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
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
						len(profile.Selections()),
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
						len(profile.Selections()),
					),
					profileProjection(profile),
				)
			})
		},
	}
}

func (a *app) newProfileAddToolCommand() *cobra.Command {
	var direct bool

	command := &cobra.Command{
		Use:   "add-tool <profile> <tool-id>",
		Short: "Add a Tool to a profile",
		Args:  exactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug, toolID, err := parseProfileToolArgs(args)
			if err != nil {
				return usageError{err: err}
			}

			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				profile, err := runtime.Profiles.AddTool.Execute(
					cmd.Context(),
					profileapplication.AddToolRequest{
						ProfileSlug: slug,
						ToolID:      toolID,
						Direct:      direct,
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
	selections := make([]map[string]any, 0, len(profile.Selections()))
	for _, selection := range profile.Selections() {
		selections = append(selections, map[string]any{
			"toolId": selection.ToolID.String(),
			"direct": selection.Direct,
		})
	}
	return map[string]any{
		"profileId":   profile.ID().String(),
		"slug":        profile.Slug().String(),
		"displayName": profile.DisplayName(),
		"mode":        string(profile.Mode()),
		"tools":       selections,
	}
}
