package cli

import (
	"fmt"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	"github.com/spf13/cobra"
)

// A Core owner activates a protocol on the exact Sandbox, not a Task.
// Unlike an environment export, this survives all Mender/OS restarts and
// enforces one-way v2 routing across all processes sharing the SQLite DB.
func (a *app) newSandboxJobProtocolCommand() *cobra.Command {
	var activate, confirm bool
	command := &cobra.Command{
		Use:   "job-protocol <sandbox-id>",
		Short: "Inspect or permanently activate SSH Durable Jobs v2 on one Sandbox",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if confirm && !activate {
				return usageError{err: fmt.Errorf("--confirm requires --activate-v2")}
			}
			if activate && !confirm {
				return usageError{err: fmt.Errorf("permanent activation requires --confirm")}
			}
			id, err := sandboxdomain.ParseID(args[0])
			if err != nil {
				return usageError{err: err}
			}
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				caller, err := gov.NewContext(gov.OriginCLI, runtime.OwnerPrincipal(), gov.CatalogScope())
				if err != nil {
					return err
				}
				var state bootstrap.SSHJobAdmissionState
				if activate {
					state, err = runtime.Sandboxes.ActivateSSHJobV2(cmd.Context(), id, caller)
				} else {
					state, err = runtime.Sandboxes.SSHJobAdmission(cmd.Context(), id, caller)
				}
				if err != nil {
					return err
				}
				return a.writeValue(fmt.Sprintf("SSH Job protocol %s: %s", id, state.Protocol), map[string]any{"protocol": state})
			})
		},
	}
	command.Flags().BoolVar(&activate, "activate-v2", false, "verify pinned remote fsync/flock and permanently enable v2 (requires paused submissions)")
	command.Flags().BoolVar(&confirm, "confirm", false, "explicitly acknowledge irreversible v2 selection")
	return command
}
