package cli

import (
	"fmt"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	sandbox "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	"github.com/spf13/cobra"
)

func (a *app) newSandboxGrantsCommand() *cobra.Command {
	command := &cobra.Command{Use: "grants <sandbox-id>", Short: "Inspect explicitly granted Profile capabilities", Args: exactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		sid, e := sandbox.ParseID(args[0])
		if e != nil {
			return usageError{err: e}
		}
		return a.withRuntime(cmd.Context(), func(r *bootstrap.Runtime) error {
			owner, e := agentCLIContext(r)
			if e != nil {
				return e
			}
			items, e := r.Sandboxes.ListProfileGrants(cmd.Context(), sid, owner)
			if e != nil {
				return e
			}
			return a.writeValue(fmt.Sprintf("%d Sandbox Profile grants", len(items)), map[string]any{"grants": items})
		})
	}}
	return command
}
func (a *app) newSandboxGrantCommand(revoke bool) *cobra.Command {
	var raw []string
	action, short := "grant", "Set one Profile's exact Sandbox capabilities"
	if revoke {
		action, short = "revoke", "Remove all Sandbox capabilities granted to this Profile"
	}
	c := &cobra.Command{Use: action + " <sandbox-id> <profile-id>", Short: short, Args: exactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		sid, e := sandbox.ParseID(args[0])
		if e != nil {
			return usageError{err: e}
		}
		caps := []sandbox.Capability{}
		if !revoke {
			if len(raw) == 0 {
				return usageError{err: fmt.Errorf("at least one --capability required")}
			}
			seen := map[sandbox.Capability]bool{}
			for _, item := range raw {
				cap := sandbox.Capability(item)
				if e = cap.Validate(); e != nil {
					return usageError{err: e}
				}
				if seen[cap] {
					return usageError{err: fmt.Errorf("duplicate capability")}
				}
				seen[cap] = true
				caps = append(caps, cap)
			}
		}
		return a.withRuntime(cmd.Context(), func(r *bootstrap.Runtime) error {
			owner, e := agentCLIContext(r)
			if e != nil {
				return e
			}
			if e = r.Sandboxes.SetProfileGrants(cmd.Context(), sid, args[1], caps, owner); e != nil {
				return e
			}
			return a.writeValue("Sandbox grants updated", map[string]any{"sandboxId": sid, "profileId": args[1], "capabilities": caps})
		})
	}}
	if !revoke {
		c.Flags().StringArrayVar(&raw, "capability", nil, "reviewed file.read/file.search/file.edit/process.shell/job.submit/job.observe/job.cancel; repeat for each")
	}
	return c
}
