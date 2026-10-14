package cli

import (
	"fmt"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	"github.com/spf13/cobra"
)

func (a *app) newSandboxJobAdmissionCommand() *cobra.Command {
	var pause, resume bool
	command := &cobra.Command{
		Use:   "job-admission <sandbox-id>",
		Short: "Inspect or explicitly pause/resume new SSH Jobs; existing jobs are unchanged",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if pause && resume {
				return usageError{err: fmt.Errorf("--pause and --resume are mutually exclusive")}
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
				if pause || resume {
					state, err = runtime.Sandboxes.SetSSHJobSubmissionsPaused(cmd.Context(), id, pause, caller)
				} else {
					state, err = runtime.Sandboxes.SSHJobAdmission(cmd.Context(), id, caller)
				}
				if err != nil {
					return err
				}
				status := "accepting new submissions"
				if state.Paused {
					status = "paused (existing jobs not cancelled)"
				}
				return a.writeValue(fmt.Sprintf("SSH Job admission %s: %s", id, status), map[string]any{"admission": state})
			})
		},
	}
	command.Flags().BoolVar(&pause, "pause", false, "deny new submissions without contacting remote host")
	command.Flags().BoolVar(&resume, "resume", false, "explicitly resume new submissions on Ready Sandbox")
	return command
}
