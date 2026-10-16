package cli

import (
	"fmt"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	"github.com/spf13/cobra"
)

func (a *app) newSandboxWorkspaceTargetsCommand() *cobra.Command {
	root := &cobra.Command{Use: "targets", Short: "Reuse one Sandbox Workspace Tool Suite across owner-authorized target Sandboxes"}
	root.AddCommand(&cobra.Command{Use: "list <tool-service-id>", Args: exactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return a.withRuntime(cmd.Context(), func(r *bootstrap.Runtime) error {
			caller, err := agentCLIContext(r)
			if err != nil {
				return err
			}
			targets, err := r.ListWorkspaceToolTargets(cmd.Context(), args[0], caller)
			if err != nil {
				return err
			}
			return a.writeValue(fmt.Sprintf("%d Workspace Tool targets", len(targets)), map[string]any{"workspaces": targets})
		})
	}})
	root.AddCommand(&cobra.Command{Use: "bind <tool-service-id> <key> <sandbox-id>", Args: exactArgs(3), RunE: func(cmd *cobra.Command, args []string) error {
		return a.withRuntime(cmd.Context(), func(r *bootstrap.Runtime) error {
			caller, err := agentCLIContext(r)
			if err != nil {
				return err
			}
			if err := r.BindWorkspaceToolTarget(cmd.Context(), args[0], args[1], args[2], caller); err != nil {
				return err
			}
			return a.writeValue("Workspace Tool target bound", map[string]string{"providerId": args[0], "workspace": args[1], "sandboxId": args[2]})
		})
	}})
	root.AddCommand(&cobra.Command{Use: "unbind <tool-service-id> <key>", Args: exactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		return a.withRuntime(cmd.Context(), func(r *bootstrap.Runtime) error {
			caller, err := agentCLIContext(r)
			if err != nil {
				return err
			}
			if err := r.UnbindWorkspaceToolTarget(cmd.Context(), args[0], args[1], caller); err != nil {
				return err
			}
			return a.writeValue("Workspace Tool target unbound", map[string]string{"providerId": args[0], "workspace": args[1]})
		})
	}})
	return root
}
