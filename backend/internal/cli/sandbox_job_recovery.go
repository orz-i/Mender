package cli

import (
	"fmt"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	"github.com/spf13/cobra"
)

func (a *app) newSandboxJobRecoveryCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "job-recovery <sandbox-id>",
		Short: "Inspect SSH Job recovery evidence without modifying remote tasks",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := sandboxdomain.ParseID(args[0])
			if err != nil {
				return usageError{err: err}
			}
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				caller, err := gov.NewContext(gov.OriginCLI, runtime.OwnerPrincipal(), gov.CatalogScope())
				if err != nil {
					return err
				}
				view, err := runtime.Sandboxes.InspectSSHJobRecovery(cmd.Context(), id, caller)
				if err != nil {
					return err
				}
				return a.writeValue(fmt.Sprintf("SSH Job recovery: %s (%d records; %s)", id, len(view.Evidence.Records), view.Evidence.Verdict), map[string]any{"recovery": view})
			})
		},
	}
}
