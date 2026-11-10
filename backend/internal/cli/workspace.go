package cli

import (
	"fmt"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandbox "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	workspaceapp "github.com/orz-i/Mender/backend/internal/workspace/application"
	workspace "github.com/orz-i/Mender/backend/internal/workspace/domain"
	"github.com/spf13/cobra"
)

func workspaceCLIContext(r *bootstrap.Runtime) (gov.Context, error) {
	return gov.NewContext(gov.OriginCLI, r.OwnerPrincipal(), gov.CatalogScope())
}
func (a *app) newWorkspaceCommand() *cobra.Command {
	c := &cobra.Command{Use: "workspace", Short: "Manage shared Workspaces, execution placements and explicit Profile targets"}
	c.AddCommand(&cobra.Command{Use: "list", Args: exactArgs(0), RunE: func(cmd *cobra.Command, _ []string) error {
		return a.withRuntime(cmd.Context(), func(r *bootstrap.Runtime) error {
			owner, e := workspaceCLIContext(r)
			if e != nil {
				return e
			}
			items, e := r.Workspaces.List(cmd.Context(), owner)
			if e != nil {
				return e
			}
			return a.writeValue(fmt.Sprintf("%d Workspaces", len(items)), map[string]any{"workspaces": items})
		})
	}})
	var name, root string
	add := &cobra.Command{Use: "add", Short: "Register an existing canonical local directory as a reusable Workspace", Args: exactArgs(0), RunE: func(cmd *cobra.Command, _ []string) error {
		if name == "" || root == "" {
			return usageError{err: fmt.Errorf("--name and --root are required")}
		}
		return a.withRuntime(cmd.Context(), func(r *bootstrap.Runtime) error {
			owner, e := workspaceCLIContext(r)
			if e != nil {
				return e
			}
			item, e := r.Workspaces.RegisterLocal(cmd.Context(), owner, name, root)
			if e != nil {
				return e
			}
			return a.writeValue("Workspace registered", map[string]any{"workspace": item})
		})
	}}
	add.Flags().StringVar(&name, "name", "", "display name")
	add.Flags().StringVar(&root, "root", "", "existing canonical absolute path")
	c.AddCommand(add)
	c.AddCommand(&cobra.Command{Use: "show <workspace-id>", Args: exactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return a.withRuntime(cmd.Context(), func(r *bootstrap.Runtime) error {
			owner, e := workspaceCLIContext(r)
			if e != nil {
				return e
			}
			id, e := workspace.ParseRef(args[0])
			if e != nil {
				return usageError{err: e}
			}
			item, e := r.Workspaces.Get(cmd.Context(), owner, id)
			if e != nil {
				return e
			}
			return a.writeValue("Workspace", map[string]any{"workspace": item})
		})
	}})
	c.AddCommand(&cobra.Command{Use: "placements <workspace-id>", Args: exactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return a.withRuntime(cmd.Context(), func(r *bootstrap.Runtime) error {
			owner, e := workspaceCLIContext(r)
			if e != nil {
				return e
			}
			id, e := workspace.ParseRef(args[0])
			if e != nil {
				return usageError{err: e}
			}
			items, e := r.Workspaces.Placements(cmd.Context(), owner, id)
			if e != nil {
				return e
			}
			return a.writeValue("Workspace placements", map[string]any{"placements": items})
		})
	}})
	for _, mode := range []struct {
		action string
		link   bool
	}{{"attach", true}, {"detach", false}} {
		selected := mode
		c.AddCommand(&cobra.Command{Use: selected.action + " <workspace-id> <sandbox-id>", Short: "Review or retire an exact local Sandbox placement (no automatic grants)", Args: exactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
			return a.withRuntime(cmd.Context(), func(r *bootstrap.Runtime) error {
				owner, e := workspaceCLIContext(r)
				if e != nil {
					return e
				}
				id, e := workspace.ParseRef(args[0])
				if e != nil {
					return usageError{err: e}
				}
				sid, e := sandbox.ParseID(args[1])
				if e != nil {
					return usageError{err: e}
				}
				if selected.link {
					e = r.Workspaces.LinkLocal(cmd.Context(), owner, id, sid.String())
				} else {
					e = r.Workspaces.Unlink(cmd.Context(), owner, id, sid.String())
				}
				if e != nil {
					return e
				}
				return a.writeValue("Workspace placement updated", map[string]any{"workspaceId": id, "sandboxId": sid})
			})
		}})
	}
	for _, mode := range []struct {
		action string
		grant  bool
	}{{"grant", true}, {"revoke", false}} {
		selected := mode
		c.AddCommand(&cobra.Command{Use: selected.action + " <workspace-id> <profile-id>", Short: "Explicitly grant or revoke one Profile's Workspace access", Args: exactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
			return a.withRuntime(cmd.Context(), func(r *bootstrap.Runtime) error {
				owner, e := workspaceCLIContext(r)
				if e != nil {
					return e
				}
				id, e := workspace.ParseRef(args[0])
				if e != nil {
					return usageError{err: e}
				}
				if selected.grant {
					e = r.Workspaces.Grant(cmd.Context(), owner, id, args[1])
				} else {
					e = r.Workspaces.Revoke(cmd.Context(), owner, id, args[1])
				}
				if e != nil {
					return e
				}
				return a.writeValue("Workspace Profile access updated", map[string]any{"workspaceId": id, "profileId": args[1]})
			})
		}})
	}
	c.AddCommand(&cobra.Command{Use: "remove <workspace-id>", Args: exactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return a.withRuntime(cmd.Context(), func(r *bootstrap.Runtime) error {
			owner, e := workspaceCLIContext(r)
			if e != nil {
				return e
			}
			id, e := workspace.ParseRef(args[0])
			if e != nil {
				return usageError{err: e}
			}
			if e = r.Workspaces.Delete(cmd.Context(), owner, id); e != nil {
				return e
			}
			return a.writeValue("Workspace retired", map[string]any{"workspaceId": id})
		})
	}})
	c.AddCommand(&cobra.Command{Use: "targets <profile-id>", Args: exactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return a.withRuntime(cmd.Context(), func(r *bootstrap.Runtime) error {
			owner, e := workspaceCLIContext(r)
			if e != nil {
				return e
			}
			targets, e := r.Workspaces.Targets(cmd.Context(), owner, args[0])
			if e != nil {
				return e
			}
			return a.writeValue("Profile execution targets", map[string]any{"targets": targets})
		})
	}})
	for _, mode := range []struct {
		action string
		add    bool
	}{{"target-add", true}, {"target-remove", false}} {
		selected := mode
		var profileID, toolID, workspaceID, sandboxID string
		cmd := &cobra.Command{Use: selected.action, Short: "Explicitly select or retire an authorized Tool execution target", Args: exactArgs(0), RunE: func(cmd *cobra.Command, _ []string) error {
			tid, e := catalog.ParseToolID(toolID)
			if e != nil {
				return usageError{err: e}
			}
			sid, e := sandbox.ParseID(sandboxID)
			if e != nil {
				return usageError{err: e}
			}
			target := workspaceapp.TargetRecord{ProfileID: profileID, ToolID: tid, WorkspaceID: workspaceID, SandboxID: sid.String()}
			return a.withRuntime(cmd.Context(), func(r *bootstrap.Runtime) error {
				owner, e := workspaceCLIContext(r)
				if e != nil {
					return e
				}
				if selected.add {
					e = r.Workspaces.PutTarget(cmd.Context(), owner, target)
				} else {
					e = r.Workspaces.DeleteTarget(cmd.Context(), owner, target)
				}
				if e != nil {
					return e
				}
				return a.writeValue("Tool target updated", map[string]any{"target": target})
			})
		}}
		cmd.Flags().StringVar(&profileID, "profile", "", "exact ProfileID")
		cmd.Flags().StringVar(&toolID, "tool", "", "canonical ToolID")
		cmd.Flags().StringVar(&sandboxID, "sandbox", "", "approved SandboxID")
		cmd.Flags().StringVar(&workspaceID, "workspace", "", "registered WorkspaceID; empty only for native:jobs")
		_ = cmd.MarkFlagRequired("profile")
		_ = cmd.MarkFlagRequired("tool")
		_ = cmd.MarkFlagRequired("sandbox")
		c.AddCommand(cmd)
	}
	return c
}
