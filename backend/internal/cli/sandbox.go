package cli

import (
	"fmt"
	"text/tabwriter"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	"github.com/spf13/cobra"
)

func (a *app) newSandboxCommand() *cobra.Command {
	command := &cobra.Command{Use: "sandbox", Short: "Manage governed local and remote Sandbox resources"}
	command.AddCommand(a.newSandboxListCommand())
	command.AddCommand(a.newSandboxLifecycleCommand("start", governancedomain.ActionSandboxStart))
	command.AddCommand(a.newSandboxLifecycleCommand("stop", governancedomain.ActionSandboxStop))
	command.AddCommand(a.newSandboxLifecycleCommand("destroy", governancedomain.ActionSandboxDestroy))
	add := &cobra.Command{Use: "add", Short: "Provision a Sandbox"}
	add.AddCommand(a.newSandboxAddLocalCommand())
	command.AddCommand(add)
	return command
}

func (a *app) newSandboxAddLocalCommand() *cobra.Command {
	var name, root, isolation string
	command := &cobra.Command{
		Use: "local", Short: "Provision a local Workspace Sandbox with explicit isolation",
		Args: exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if name == "" || root == "" || isolation == "" {
				return usageError{err: fmt.Errorf("--name, --root, and --isolation are required")}
			}
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				caller, err := governancedomain.NewContext(governancedomain.OriginCLI, runtime.OwnerPrincipal(), governancedomain.CatalogScope())
				if err != nil {
					return err
				}
				item, err := runtime.Sandboxes.CreateLocalSandbox(cmd.Context(), bootstrap.CreateLocalSandboxRequest{
					Name: name, Root: root, Isolation: sandboxdomain.IsolationClass(isolation),
				}, caller)
				if err != nil {
					return err
				}
				return a.writeValue(fmt.Sprintf("created %s (%s, %s)", item.ID(), item.Backend(), item.Isolation()), map[string]any{"sandbox": sandboxProjection(item)})
			})
		},
	}
	command.Flags().StringVar(&name, "name", "", "Sandbox display name")
	command.Flags().StringVar(&root, "root", "", "absolute local directory")
	command.Flags().StringVar(&isolation, "isolation", "", "host (unisolated) or restricted_host (fail closed)")
	return command
}
func (a *app) newSandboxListCommand() *cobra.Command {
	return &cobra.Command{Use: "list", Short: "List configured Sandboxes", Args: exactArgs(0), RunE: func(cmd *cobra.Command, _ []string) error {
		return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
			items, err := runtime.Sandboxes.List(cmd.Context())
			if err != nil {
				return err
			}
			projected := make([]map[string]any, 0, len(items))
			for _, item := range items {
				projected = append(projected, sandboxProjection(item))
			}
			if a.output == "json" {
				return a.writeValue("", map[string]any{"sandboxes": projected})
			}
			writer := tabwriter.NewWriter(a.options.Stdout, 0, 4, 2, ' ', 0)
			_, _ = fmt.Fprintln(writer, "SANDBOX ID\tBACKEND\tISOLATION\tNAME")
			for _, item := range items {
				_, _ = fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", item.ID(), item.Backend(), item.Isolation(), item.Spec().Name)
			}
			return writer.Flush()
		})
	}}
}
func sandboxProjection(item sandboxdomain.Sandbox) map[string]any {
	spec := item.Spec()
	return map[string]any{"sandboxId": spec.ID.String(), "name": spec.Name, "backend": string(spec.Backend), "isolation": string(spec.Isolation), "localRoot": spec.LocalRoot, "ownerKind": spec.OwnerKind, "ownerRef": spec.OwnerRef}
}

func (a *app) newSandboxLifecycleCommand(name string, action governancedomain.Action) *cobra.Command {
	var confirm bool
	command := &cobra.Command{
		Use:   name + " <sandbox-id>",
		Short: "Govern a local Sandbox lifecycle transition. Destroy never deletes host files.",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := sandboxdomain.ParseID(args[0])
			if err != nil {
				return usageError{err: err}
			}
			if action == governancedomain.ActionSandboxDestroy && !confirm {
				return usageError{err: fmt.Errorf("sandbox destroy requires --confirm; host files remain untouched")}
			}
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				caller, err := governancedomain.NewContext(governancedomain.OriginCLI, runtime.OwnerPrincipal(), governancedomain.CatalogScope())
				if err != nil {
					return err
				}
				run, err := runtime.Sandboxes.ChangeLocalLifecycle(cmd.Context(), id, action, caller)
				if err != nil {
					return err
				}
				item, err := runtime.Sandboxes.Get(cmd.Context(), id)
				if err != nil {
					return err
				}
				return a.writeValue(fmt.Sprintf("Sandbox %s: %s", id, run.State), map[string]any{"sandbox": sandboxProjection(item), "state": string(run.State), "generation": run.Generation})
			})
		},
	}
	if action == governancedomain.ActionSandboxDestroy {
		command.Flags().BoolVar(&confirm, "confirm", false, "acknowledge irreversible resource tombstone (local directory is NOT removed)")
	}
	return command
}
