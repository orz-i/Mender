package cli

import (
	"fmt"
	"text/tabwriter"

	agentapplication "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	"github.com/orz-i/Mender/backend/internal/bootstrap"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	"github.com/spf13/cobra"
)

func (a *app) newAgentCommand() *cobra.Command {
	command := &cobra.Command{Use: "agent", Short: "Configure governed Agent CLI resources and review delegated tasks"}
	command.AddCommand(a.newAgentListCommand())
	command.AddCommand(a.newAgentAddCommand())
	command.AddCommand(a.newAgentShowCommand())
	command.AddCommand(a.newAgentToolsCommand())
	command.AddCommand(a.newAgentRemoveCommand())
	command.AddCommand(a.newAgentTasksCommand())
	command.AddCommand(a.newAgentAdmissionsCommand())
	command.AddCommand(a.newAgentTaskCommand())
	command.AddCommand(a.newAgentCancelCommand())
	command.AddCommand(a.newAgentGrantCommand(true))
	command.AddCommand(a.newAgentGrantCommand(false))
	command.AddCommand(a.newAgentGrantsCommand())
	return command
}
func agentCLIContext(runtime *bootstrap.Runtime) (gov.Context, error) {
	return gov.NewContext(gov.OriginCLI, runtime.OwnerPrincipal(), gov.CatalogScope())
}
func agentCLIProjection(runtime *bootstrap.Runtime, item agentdomain.Agent) map[string]any {
	spec := item.Spec()
	out := map[string]any{
		"agentId": spec.ID, "name": spec.Name, "backend": spec.Backend, "pluginId": spec.PluginID,
		"workspaceRoot": spec.WorkspaceRoot, "isolation": spec.Isolation, "createdAt": spec.CreatedAt,
		"backendAvailable": false,
	}
	if _, err := runtime.AgentAvailability(spec.Backend); err != nil {
		out["issue"] = err.Error()
	} else {
		out["backendAvailable"] = true
	}
	return out
}
func (a *app) newAgentListCommand() *cobra.Command {
	return &cobra.Command{Use: "list", Short: "List configured Agents without starting any CLI",
		Args: exactArgs(0), RunE: func(cmd *cobra.Command, _ []string) error {
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				caller, err := agentCLIContext(runtime)
				if err != nil {
					return err
				}
				agents, err := runtime.Agents.List(cmd.Context(), caller)
				if err != nil {
					return err
				}
				if a.output == "json" {
					items := make([]map[string]any, 0, len(agents))
					for _, agent := range agents {
						items = append(items, agentCLIProjection(runtime, agent))
					}
					return a.writeValue("", map[string]any{"agents": items})
				}
				w := tabwriter.NewWriter(a.options.Stdout, 0, 4, 2, ' ', 0)
				_, _ = fmt.Fprintln(w, "AGENT ID\tBACKEND\tNAME\tSTATUS")
				for _, agent := range agents {
					_, availableErr := runtime.AgentAvailability(agent.Spec().Backend)
					status := "detected"
					if availableErr != nil {
						status = "unavailable"
					}
					_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", agent.ID(), agent.Spec().Backend, agent.Spec().Name, status)
				}
				return w.Flush()
			})
		},
	}
}
func (a *app) newAgentAddCommand() *cobra.Command {
	var name, backend, root, isolation string
	command := &cobra.Command{Use: "add", Short: "Create a local Agent resource; never executes an Agent",
		Args: exactArgs(0), RunE: func(cmd *cobra.Command, _ []string) error {
			if name == "" || backend == "" || root == "" {
				return usageError{err: fmt.Errorf("--name, --backend and --root are required")}
			}
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				caller, err := agentCLIContext(runtime)
				if err != nil {
					return err
				}
				agent, err := runtime.CreateAgentWithTools(cmd.Context(), agentapplication.CreateRequest{
					Name: name, Backend: agentdomain.Backend(backend), WorkspaceRoot: root, Isolation: isolation,
				}, caller)
				if err != nil {
					return err
				}
				return a.writeValue(fmt.Sprintf("created Agent %s", agent.ID()), map[string]any{"agent": agentCLIProjection(runtime, agent)})
			})
		},
	}
	command.Flags().StringVar(&name, "name", "", "Agent display name")
	command.Flags().StringVar(&backend, "backend", "", "codex or grok")
	command.Flags().StringVar(&root, "root", "", "absolute existing local workspace directory")
	command.Flags().StringVar(&isolation, "isolation", "restricted_host", "restricted_host or explicit unisolated host")
	return command
}
func (a *app) newAgentShowCommand() *cobra.Command {
	return &cobra.Command{Use: "show <agent-id>", Short: "Inspect one Agent and CLI availability without launching it",
		Args: exactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			id, err := agentdomain.ParseID(args[0])
			if err != nil {
				return usageError{err: err}
			}
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				caller, err := agentCLIContext(runtime)
				if err != nil {
					return err
				}
				agent, err := runtime.Agents.Get(cmd.Context(), id, caller)
				if err != nil {
					return err
				}
				return a.writeValue(fmt.Sprintf("%s (%s)", agent.Spec().Name, agent.ID()), map[string]any{"agent": agentCLIProjection(runtime, agent)})
			})
		},
	}
}
func (a *app) newAgentToolsCommand() *cobra.Command {
	var prepare bool
	command := &cobra.Command{Use: "tools <agent-id>", Short: "Inspect or explicitly repair an Agent-owned Tool suite",
		Args: exactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			id, err := agentdomain.ParseID(args[0])
			if err != nil {
				return usageError{err: err}
			}
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				caller, err := agentCLIContext(runtime)
				if err != nil {
					return err
				}
				var state bootstrap.AgentTools
				if prepare {
					state, err = runtime.EnsureAgentTools(cmd.Context(), id, caller)
				} else {
					state, err = runtime.AgentTools(cmd.Context(), id, caller)
				}
				if err != nil {
					return err
				}
				return a.writeValue(fmt.Sprintf("Agent tools: %s (%d Tools)", state.Status, len(state.Tools)), map[string]any{"suite": state})
			})
		},
	}
	command.Flags().BoolVar(&prepare, "prepare", false, "owner-authorized create/adopt and sync; does not grant Profile access")
	return command
}
func (a *app) newAgentRemoveCommand() *cobra.Command {
	var confirm bool
	command := &cobra.Command{Use: "remove <agent-id>", Short: "Delete only an Agent with no task or Provider references",
		Args: exactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			if !confirm {
				return usageError{err: fmt.Errorf("agent removal requires --confirm")}
			}
			id, err := agentdomain.ParseID(args[0])
			if err != nil {
				return usageError{err: err}
			}
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				caller, err := agentCLIContext(runtime)
				if err != nil {
					return err
				}
				if err := runtime.DeleteAgentWithTools(cmd.Context(), id, caller); err != nil {
					return err
				}
				return a.writeValue(fmt.Sprintf("removed Agent %s", id), map[string]string{"agentId": string(id)})
			})
		},
	}
	command.Flags().BoolVar(&confirm, "confirm", false, "confirm removal of an unreferenced Agent")
	return command
}
func (a *app) newAgentTasksCommand() *cobra.Command {
	return &cobra.Command{Use: "tasks <agent-id>", Short: "List the last 100 persisted delegated tasks",
		Args: exactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			id, err := agentdomain.ParseID(args[0])
			if err != nil {
				return usageError{err: err}
			}
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				caller, err := agentCLIContext(runtime)
				if err != nil {
					return err
				}
				tasks, err := runtime.Agents.Tasks(cmd.Context(), id, caller)
				if err != nil {
					return err
				}
				if a.output == "json" {
					return a.writeValue("", map[string]any{"tasks": tasks})
				}
				w := tabwriter.NewWriter(a.options.Stdout, 0, 4, 2, ' ', 0)
				_, _ = fmt.Fprintln(w, "TASK ID\tSTATUS\tCREATED")
				for _, task := range tasks {
					_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", task.ID, task.Status, task.CreatedAt.Format("2006-01-02 15:04:05"))
				}
				return w.Flush()
			})
		},
	}
}
func (a *app) newAgentAdmissionsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "admissions <agent-id>",
		Short: "Review bounded Agent activation/submission/cancellation Policy decisions",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := agentdomain.ParseID(args[0])
			if err != nil {
				return usageError{err: err}
			}
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				caller, err := agentCLIContext(runtime)
				if err != nil {
					return err
				}
				events, err := runtime.Agents.Admissions(cmd.Context(), id, caller, 100)
				if err != nil {
					return err
				}
				if a.output == "json" {
					return a.writeValue("", map[string]any{"events": events})
				}
				writer := tabwriter.NewWriter(a.options.Stdout, 0, 4, 2, ' ', 0)
				_, _ = fmt.Fprintln(writer, "ACTION\tDECISION\tSTATUS\tTIME")
				for _, event := range events {
					_, _ = fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n",
						event.Action, event.PolicyKind, event.Status, event.OccurredAt.Format("2006-01-02 15:04:05"))
				}
				return writer.Flush()
			})
		},
	}
}

func (a *app) newAgentTaskCommand() *cobra.Command {
	return &cobra.Command{Use: "task <task-id>", Short: "Inspect an authorized Task and bounded result",
		Args: exactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			id, err := agentdomain.ParseTaskID(args[0])
			if err != nil {
				return usageError{err: err}
			}
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				caller, err := agentCLIContext(runtime)
				if err != nil {
					return err
				}
				task, err := runtime.Agents.Task(cmd.Context(), id, caller)
				if err != nil {
					return err
				}
				return a.writeValue(fmt.Sprintf("Task %s: %s", task.ID, task.Status), map[string]any{"task": task})
			})
		},
	}
}
func (a *app) newAgentCancelCommand() *cobra.Command {
	return &cobra.Command{Use: "cancel <task-id>", Short: "Request cancellation; Task state is not immediately terminal",
		Args: exactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			id, err := agentdomain.ParseTaskID(args[0])
			if err != nil {
				return usageError{err: err}
			}
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				caller, err := agentCLIContext(runtime)
				if err != nil {
					return err
				}
				if err := runtime.Agents.Cancel(cmd.Context(), id, caller); err != nil {
					return err
				}
				return a.writeValue(fmt.Sprintf("requested cancellation of %s", id), map[string]any{"taskId": id, "cancelRequested": true})
			})
		},
	}
}
func (a *app) newAgentGrantCommand(grant bool) *cobra.Command {
	name := "grant"
	short := "Grant a specific Profile access to one Agent"
	if !grant {
		name = "revoke"
		short = "Revoke a Profile's access to one Agent"
	}
	return &cobra.Command{Use: name + " <agent-id> <profile-id>", Short: short,
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
				if grant {
					err = runtime.Agents.Grant(cmd.Context(), id, args[1], caller)
				} else {
					err = runtime.Agents.Revoke(cmd.Context(), id, args[1], caller)
				}
				if err != nil {
					return err
				}
				return a.writeValue(fmt.Sprintf("Agent Profile access updated: %s", name),
					map[string]any{"agentId": id, "profileId": args[1], "granted": grant})
			})
		},
	}
}
func (a *app) newAgentGrantsCommand() *cobra.Command {
	return &cobra.Command{Use: "grants <agent-id>", Short: "Inspect current exact Profile grants",
		Args: exactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			id, err := agentdomain.ParseID(args[0])
			if err != nil {
				return usageError{err: err}
			}
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				caller, err := agentCLIContext(runtime)
				if err != nil {
					return err
				}
				ids, err := runtime.Agents.Grants(cmd.Context(), id, caller)
				if err != nil {
					return err
				}
				if a.output == "json" {
					return a.writeValue("", map[string]any{"profileIds": ids})
				}
				for _, profileID := range ids {
					if _, err := fmt.Fprintln(a.options.Stdout, profileID); err != nil {
						return err
					}
				}
				return nil
			})
		},
	}
}
