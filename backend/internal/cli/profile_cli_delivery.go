package cli

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	"github.com/spf13/cobra"
)

// profile cli emits local owner command recipes. Commands are supplied as
// structured argv arrays in JSON; the textual form is for interactive shells.
func (a *app) newProfileCLIInstructionsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "cli <profile>",
		Short: "Show local Profile-scoped CLI and MCP stdio command recipes",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug, err := profiledomain.NewSlug(args[0])
			if err != nil {
				return usageError{err: err}
			}
			return a.withRuntime(cmd.Context(), func(r *bootstrap.Runtime) error {
				config, err := r.LocalProfileCLI(cmd.Context(), slug)
				if err != nil {
					return err
				}
				if a.output == "json" {
					return a.writeValue("", config)
				}
				for _, row := range []struct {
					label  string
					tokens []string
				}{
					{"Search", config.Search},
					{"Describe", config.Describe},
					{"Call", config.Call},
					{"MCP stdio", config.MCPStdio},
				} {
					_, err := fmt.Fprintf(a.options.Stdout, "%s: %s\n", row.label, quoteCLIArgs(row.tokens))
					if err != nil {
						return err
					}
				}
				_, err = fmt.Fprintln(a.options.Stdout,
					"Local host owner commands only. Profile scope governs Mender Tool invocations; it does not sandbox arbitrary shell commands.")
				return err
			})
		},
	}
}

func quoteCLIArgs(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		if runtime.GOOS == "windows" {
			quoted = append(quoted, strconv.Quote(arg))
		} else {
			quoted = append(quoted, "'"+strings.ReplaceAll(arg, "'", "'\"'\"'")+"'")
		}
	}
	return strings.Join(quoted, " ")
}
