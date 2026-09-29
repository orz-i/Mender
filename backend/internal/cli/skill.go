package cli

import (
	"context"
	"fmt"
	"text/tabwriter"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	skillapplication "github.com/orz-i/Mender/backend/internal/skill/application"
	skilldomain "github.com/orz-i/Mender/backend/internal/skill/domain"
	"github.com/orz-i/Mender/backend/internal/skill/infrastructure/agentskill"
	"github.com/spf13/cobra"
)

func (a *app) newSkillCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "skill",
		Short: "Manage Agent Skill resources",
	}
	command.AddCommand(a.newSkillListCommand())
	command.AddCommand(a.newSkillInspectCommand())
	command.AddCommand(a.newSkillValidateCommand())
	command.AddCommand(a.newSkillInstallCommand())
	command.AddCommand(a.newSkillTrustCommand(true))
	command.AddCommand(a.newSkillTrustCommand(false))
	command.AddCommand(a.newSkillEnabledCommand(true))
	command.AddCommand(a.newSkillEnabledCommand(false))
	command.AddCommand(a.newSkillRemoveCommand())
	return command
}

func (a *app) newSkillListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List built-in and managed Skill installations",
		Args:  exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				items, err := runtime.Skills.List(cmd.Context())
				if err != nil {
					return err
				}
				if a.output == "json" {
					projected := make([]map[string]any, 0, len(items))
					for _, item := range items {
						projected = append(projected, skillInstallationProjection(item))
					}
					return a.writeValue("", map[string]any{"installations": projected})
				}
				writer := tabwriter.NewWriter(a.options.Stdout, 0, 4, 2, ' ', 0)
				_, _ = fmt.Fprintln(writer, "INSTALLATION	SKILL	SOURCE	VERSION	TRUSTED	ENABLED")
				for _, item := range items {
					version := item.VersionHint
					if version == "" {
						version = "-"
					}
					_, _ = fmt.Fprintf(
						writer,
						"%s\t%s\t%s\t%s\t%t\t%t\n",
						item.InstallationID,
						item.Name,
						item.SourceKind,
						version,
						item.Trusted,
						item.Enabled,
					)
				}
				return writer.Flush()
			})
		},
	}
}

func (a *app) newSkillInspectCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "inspect <skill-or-installation>",
		Short: "Inspect one exact Skill installation",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				item, err := resolveSkillReference(cmd.Context(), runtime, args[0])
				if err != nil {
					return err
				}
				inspection, err := runtime.Skills.Inspect(cmd.Context(), item.InstallationID)
				if err != nil {
					return err
				}
				projected := skillInspectionProjection(inspection)
				if a.output == "json" {
					return a.writeValue("", map[string]any{"skill": projected})
				}
				_, err = fmt.Fprintf(
					a.options.Stdout,
					"Installation: %s\nSkill: %s\nName: %s\nSource: %s\nDigest: %s\nTrusted: %t\nEnabled: %t\nResources: %d\n",
					item.InstallationID,
					item.SkillID,
					item.Name,
					item.SourceKind,
					item.Digest,
					item.Trusted,
					item.Enabled,
					len(inspection.Resources),
				)
				return err
			})
		},
	}
}

func (a *app) newSkillValidateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "validate <skill-directory>",
		Short: "Validate an Agent Skill directory without installing or executing it",
		Args:  exactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			snapshot, err := agentskill.LoadDirectory(args[0])
			if err != nil {
				return usageError{err: err}
			}
			projected := skillSnapshotProjection(snapshot)
			if a.output == "json" {
				return a.writeValue("", map[string]any{"skill": projected})
			}
			return a.writeValue(
				fmt.Sprintf(
					"valid Skill %s (%d resources, %s)",
					snapshot.Metadata.Name,
					len(snapshot.Resources),
					snapshot.Digest,
				),
				projected,
			)
		},
	}
}

func (a *app) newSkillInstallCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "install <skill-directory>",
		Short: "Install a local Agent Skill into Mender-managed storage",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				item, err := runtime.Skills.InstallLocal(cmd.Context(), args[0])
				if err != nil {
					return err
				}
				projected := managedSkillInstallationProjection(item)
				return a.writeValue(
					fmt.Sprintf(
						"installed Skill %s as %s (untrusted, disabled)",
						item.Name(),
						item.ID(),
					),
					map[string]any{"installation": projected},
				)
			})
		},
	}
}

func (a *app) newSkillTrustCommand(trusted bool) *cobra.Command {
	name := "untrust"
	short := "Remove trust from a managed Skill installation and disable it"
	if trusted {
		name = "trust"
		short = "Trust a managed Skill installation"
	}
	return &cobra.Command{
		Use:   name + " <installation>",
		Short: short,
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := skilldomain.ParseInstallationID(args[0])
			if err != nil {
				return usageError{err: err}
			}
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				item, err := runtime.Skills.SetTrust(cmd.Context(), id, trusted)
				if err != nil {
					return err
				}
				return a.writeValue(
					fmt.Sprintf("%sed %s", name, id),
					map[string]any{"installation": managedSkillInstallationProjection(item)},
				)
			})
		},
	}
}

func (a *app) newSkillEnabledCommand(enabled bool) *cobra.Command {
	name := "disable"
	short := "Disable a managed Skill installation"
	if enabled {
		name = "enable"
		short = "Enable a trusted managed Skill installation"
	}
	return &cobra.Command{
		Use:   name + " <installation>",
		Short: short,
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := skilldomain.ParseInstallationID(args[0])
			if err != nil {
				return usageError{err: err}
			}
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				item, err := runtime.Skills.SetEnabled(cmd.Context(), id, enabled)
				if err != nil {
					return err
				}
				return a.writeValue(
					fmt.Sprintf("%sd %s", name, id),
					map[string]any{"installation": managedSkillInstallationProjection(item)},
				)
			})
		},
	}
}

func (a *app) newSkillRemoveCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <installation>",
		Short: "Remove an unreferenced managed Skill installation",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := skilldomain.ParseInstallationID(args[0])
			if err != nil {
				return usageError{err: err}
			}
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				if err := runtime.Skills.Remove(cmd.Context(), id); err != nil {
					return err
				}
				return a.writeValue(
					fmt.Sprintf("removed %s", id),
					map[string]any{"installationId": id.String()},
				)
			})
		},
	}
}

func resolveSkillReference(
	ctx context.Context,
	runtime *bootstrap.Runtime,
	reference string,
) (bootstrap.SkillInstallation, error) {
	items, err := runtime.Skills.List(ctx)
	if err != nil {
		return bootstrap.SkillInstallation{}, err
	}
	matches := make([]bootstrap.SkillInstallation, 0, 1)
	for _, item := range items {
		if reference == item.InstallationID.String() ||
			reference == item.SkillID.String() ||
			reference == item.Name {
			matches = append(matches, item)
		}
	}
	if len(matches) == 0 {
		return bootstrap.SkillInstallation{}, fmt.Errorf(
			"%w: %s",
			skillapplication.ErrInstallationNotFound,
			reference,
		)
	}
	if len(matches) > 1 {
		return bootstrap.SkillInstallation{}, usageError{err: fmt.Errorf(
			"skill reference %q matches %d installations; use an exact InstallationID",
			reference,
			len(matches),
		)}
	}
	return matches[0], nil
}

func skillInstallationProjection(item bootstrap.SkillInstallation) map[string]any {
	result := map[string]any{
		"skillId":         item.SkillID.String(),
		"installationId":  item.InstallationID.String(),
		"name":            item.Name,
		"description":     item.Description,
		"sourceKind":      item.SourceKind,
		"sourceReference": item.SourceReference,
		"digest":          item.Digest,
		"versionHint":     item.VersionHint,
		"trusted":         item.Trusted,
		"enabled":         item.Enabled,
		"builtIn":         item.BuiltIn,
	}
	if item.InstalledAt != nil {
		result["installedAt"] = *item.InstalledAt
	}
	return result
}

func managedSkillInstallationProjection(
	item skilldomain.Installation,
) map[string]any {
	return map[string]any{
		"skillId":         item.SkillID().String(),
		"installationId":  item.ID().String(),
		"name":            item.Name(),
		"description":     item.Description(),
		"sourceKind":      string(item.SourceKind()),
		"sourceReference": item.SourceReference(),
		"digest":          item.Digest(),
		"versionHint":     item.VersionHint(),
		"trusted":         item.Trusted(),
		"enabled":         item.Enabled(),
		"builtIn":         false,
		"installedAt":     item.InstalledAt(),
	}
}

func skillInspectionProjection(
	inspection bootstrap.SkillInspection,
) map[string]any {
	result := skillInstallationProjection(inspection.Installation)
	result["metadata"] = map[string]any{
		"name":          inspection.Metadata.Name,
		"description":   inspection.Metadata.Description,
		"license":       inspection.Metadata.License,
		"compatibility": inspection.Metadata.Compatibility,
		"values":        inspection.Metadata.Values,
		"allowedTools":  inspection.Metadata.AllowedTools,
	}
	resources := make([]map[string]any, 0, len(inspection.Resources))
	for _, resource := range inspection.Resources {
		resources = append(resources, map[string]any{
			"path":       resource.Path,
			"kind":       string(resource.Kind),
			"size":       resource.Size,
			"sha256":     resource.SHA256,
			"executable": resource.Executable,
		})
	}
	result["resources"] = resources
	result["totalBytes"] = inspection.TotalBytes
	return result
}

func skillSnapshotProjection(snapshot skilldomain.Snapshot) map[string]any {
	resources := make([]map[string]any, 0, len(snapshot.Resources))
	for _, resource := range snapshot.Resources {
		resources = append(resources, map[string]any{
			"path":       resource.Path,
			"kind":       string(resource.Kind),
			"size":       resource.Size,
			"sha256":     resource.SHA256,
			"executable": resource.Executable,
		})
	}
	return map[string]any{
		"name":          snapshot.Metadata.Name,
		"description":   snapshot.Metadata.Description,
		"license":       snapshot.Metadata.License,
		"compatibility": snapshot.Metadata.Compatibility,
		"metadata":      snapshot.Metadata.Values,
		"allowedTools":  snapshot.Metadata.AllowedTools,
		"digest":        snapshot.Digest,
		"totalBytes":    snapshot.TotalBytes,
		"resources":     resources,
	}
}
