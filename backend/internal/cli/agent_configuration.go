package cli

import (
	"fmt"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	"github.com/orz-i/Mender/backend/internal/bootstrap"
	"github.com/spf13/cobra"
)

// These commands administer existing Agent resources; no Agent is run by inspection.
func (a *app) newAgentModelsCommand() *cobra.Command {
	return &cobra.Command{Use: "models <agent-id>", Short: "Explicitly inspect authenticated ACP model choices (no prompt)",
		Args: exactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			id, err := agentdomain.ParseID(args[0])
			if err != nil {
				return usageError{err: err}
			}
			return a.withRuntime(cmd.Context(), func(r *bootstrap.Runtime) error {
				caller, err := agentCLIContext(r)
				if err != nil {
					return err
				}
				choices, err := r.Agents.InspectModels(cmd.Context(), id, caller)
				if err != nil {
					return err
				}
				return a.writeValue(fmt.Sprintf("%d advertised ACP models", len(choices.Models)), map[string]any{"models": choices})
			})
		}}
}
func (a *app) newAgentModelCommand() *cobra.Command {
	return &cobra.Command{Use: "model <agent-id> <model-id-or-empty>", Short: "Set an Agent's default ACP model identifier (empty uses backend default)",
		Args: exactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
			id, err := agentdomain.ParseID(args[0])
			if err != nil {
				return usageError{err: err}
			}
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				caller, err := agentCLIContext(runtime)
				if err != nil {
					return err
				}
				item, err := runtime.Agents.UpdateDefaultModel(cmd.Context(), id, args[1], caller)
				if err != nil {
					return err
				}
				return a.writeValue(fmt.Sprintf("Agent %s default model updated", id), map[string]any{"agent": agentCLIProjection(runtime, item)})
			})
		}}
}
func (a *app) newAgentWorkspaceCommand() *cobra.Command {
	root := &cobra.Command{Use: "workspace", Short: "Bind existing local Sandboxes to an Agent, and grant Profile selection"}
	root.AddCommand(&cobra.Command{Use: "list <agent-id>", Args: exactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		id, err := agentdomain.ParseID(args[0])
		if err != nil {
			return usageError{err: err}
		}
		return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
			caller, err := agentCLIContext(runtime)
			if err != nil {
				return err
			}
			items, err := runtime.Agents.Workspaces(cmd.Context(), id, caller)
			if err != nil {
				return err
			}
			return a.writeValue(fmt.Sprintf("%d registered Agent workspaces", len(items)), map[string]any{"workspaces": items})
		})
	}})
	root.AddCommand(&cobra.Command{Use: "bind <agent-id> <key> <local-sandbox-id>", Args: exactArgs(3), RunE: func(cmd *cobra.Command, args []string) error {
		id, err := agentdomain.ParseID(args[0])
		if err != nil {
			return usageError{err: err}
		}
		return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
			caller, err := agentCLIContext(runtime)
			if err != nil {
				return err
			}
			if err := runtime.Agents.BindWorkspace(cmd.Context(), id, args[1], args[2], caller); err != nil {
				return err
			}
			return a.writeValue("Agent workspace bound", map[string]string{"agentId": args[0], "workspace": args[1], "sandboxId": args[2]})
		})
	}})
	root.AddCommand(&cobra.Command{Use: "unbind <agent-id> <key>", Args: exactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		id, err := agentdomain.ParseID(args[0])
		if err != nil {
			return usageError{err: err}
		}
		return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
			caller, err := agentCLIContext(runtime)
			if err != nil {
				return err
			}
			if err := runtime.Agents.UnbindWorkspace(cmd.Context(), id, args[1], caller); err != nil {
				return err
			}
			return a.writeValue("Agent workspace unbound", map[string]string{"agentId": args[0], "workspace": args[1]})
		})
	}})
	for _, verb := range []string{"grant", "revoke"} {
		grant := verb == "grant"
		root.AddCommand(&cobra.Command{Use: verb + " <agent-id> <key> <profile-id>", Args: exactArgs(3), RunE: func(cmd *cobra.Command, args []string) error {
			id, err := agentdomain.ParseID(args[0])
			if err != nil {
				return usageError{err: err}
			}
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				caller, err := agentCLIContext(runtime)
				if err != nil {
					return err
				}
				if grant {
					err = runtime.Agents.GrantWorkspace(cmd.Context(), id, args[1], args[2], caller)
				} else {
					err = runtime.Agents.RevokeWorkspace(cmd.Context(), id, args[1], args[2], caller)
				}
				if err != nil {
					return err
				}
				return a.writeValue("Agent workspace access updated", map[string]any{"agentId": args[0], "workspace": args[1], "profileId": args[2], "granted": grant})
			})
		}})
	}
	return root
}
