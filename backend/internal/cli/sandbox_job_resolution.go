package cli

import (
	"fmt"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	"github.com/spf13/cobra"
)

func (a *app) newSandboxJobResolutionCommand() *cobra.Command {
	var fingerprint, reason string
	var generation uint64
	var confirm bool
	cmd := &cobra.Command{
		Use:   "job-resolve <sandbox-id> <task-id>",
		Short: "Explicit owner-only v2 prelaunch recovery under SSH execution fence",
		Args:  exactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !confirm {
				return usageError{err: fmt.Errorf("SSH Job recovery requires --confirm and reviewed evidence")}
			}
			id, err := sandboxdomain.ParseID(args[0])
			if err != nil {
				return usageError{err: err}
			}
			return a.withRuntime(cmd.Context(), func(r *bootstrap.Runtime) error {
				owner, err := gov.NewContext(gov.OriginCLI, r.OwnerPrincipal(), gov.CatalogScope())
				if err != nil {
					return err
				}
				result, err := r.Sandboxes.ResolveSSHJobRecovery(cmd.Context(), id, bootstrap.SSHJobResolutionRequest{
					TaskID: args[1], Fingerprint: fingerprint, Generation: generation, Reason: reason,
				}, owner)
				if err != nil {
					return err
				}
				return a.writeValue("SSH v2 task operationally resolved before launch; command exit remains unknown",
					map[string]any{"resolution": result})
			})
		},
	}
	cmd.Flags().StringVar(&fingerprint, "fingerprint", "", "exact inspected SHA-256 evidence fingerprint")
	cmd.Flags().StringVar(&reason, "reason", "", "required operator review reason; audit stores only its digest")
	cmd.Flags().Uint64Var(&generation, "generation", 0, "exact inspected Sandbox Run generation")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "confirm evidence-based non-execution resolution")
	_ = cmd.MarkFlagRequired("fingerprint")
	_ = cmd.MarkFlagRequired("reason")
	_ = cmd.MarkFlagRequired("generation")
	return cmd
}
