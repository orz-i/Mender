package cli

import (
	"context"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	catalogapp "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	"github.com/spf13/cobra"
)

// The local Profile delivery path is always scoped. Unlike administrative
// mender tool commands, the profile is mandatory, and no Catalog fallback
// exists even when the selected Profile has zero Tools.
func (a *app) newProfileToolDeliveryCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "tool",
		Short: "Invoke and inspect only Tools selected in a Profile",
	}
	command.AddCommand(a.newProfileToolSearchCommand())
	command.AddCommand(a.newProfileToolDescribeCommand())
	command.AddCommand(a.newProfileToolCallCommand())
	return command
}

func resolveProfileCLI(ctx context.Context, runtime *bootstrap.Runtime, raw string) (profiledomain.Profile, error) {
	slug, err := profiledomain.NewSlug(raw)
	if err != nil {
		return profiledomain.Profile{}, usageError{err: err}
	}
	return runtime.Profiles.Get.Execute(ctx, slug)
}

func (a *app) newProfileToolSearchCommand() *cobra.Command {
	var limit int
	command := &cobra.Command{
		Use:   "search <profile> <query>",
		Short: "Search only Tools selected in this Profile",
		Args:  exactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if limit < 1 || limit > 100 {
				return usageError{err: fmt.Errorf("--limit must be between 1 and 100")}
			}
			if strings.TrimSpace(args[1]) == "" {
				return usageError{err: fmt.Errorf("search query must not be empty")}
			}
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				profile, err := resolveProfileCLI(cmd.Context(), runtime, args[0])
				if err != nil {
					return err
				}
				allowed := profile.ToolIDs()
				// Never let an empty Profile become an unrestricted Catalog search.
				if len(allowed) == 0 {
					if a.output == "json" {
						return a.writeValue("", map[string]any{"tools": []map[string]any{}})
					}
					return nil
				}
				tools, err := runtime.Catalog.SearchTools.Execute(cmd.Context(), catalogapp.SearchToolsRequest{
					Query: args[1], Limit: limit, AllowedToolIDs: allowed,
				})
				if err != nil {
					return err
				}
				if a.output == "json" {
					items := make([]map[string]any, 0, len(tools))
					for _, item := range tools {
						items = append(items, searchResultProjection(item))
					}
					return a.writeValue("", map[string]any{"tools": items})
				}
				writer := tabwriter.NewWriter(a.options.Stdout, 0, 4, 2, ' ', 0)
				_, _ = fmt.Fprintln(writer, "RANK\tTOOL ID\tNAME\tTITLE")
				for _, item := range tools {
					_, _ = fmt.Fprintf(writer, "%d\t%s\t%s\t%s\n", item.Rank, item.ToolID, item.Name, item.Title)
				}
				return writer.Flush()
			})
		},
	}
	command.Flags().IntVar(&limit, "limit", 10, "maximum search results")
	return command
}

func (a *app) newProfileToolDescribeCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "describe <profile> <tool-id>",
		Short: "Describe one Tool selected in this Profile",
		Args:  exactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := catalogdomain.ParseToolID(args[1])
			if err != nil {
				return usageError{err: err}
			}
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				profile, err := resolveProfileCLI(cmd.Context(), runtime, args[0])
				if err != nil {
					return err
				}
				if !profile.Contains(id) {
					return fmt.Errorf("tool %s is outside Profile %s", id, profile.Slug())
				}
				tool, err := runtime.Catalog.DescribeTool.Execute(cmd.Context(), id)
				if err != nil {
					return err
				}
				result, err := toolProjection(tool)
				if err != nil {
					return err
				}
				if a.output == "json" {
					return a.writeValue("", result)
				}
				_, err = fmt.Fprintf(a.options.Stdout, "%s (%s)\nProvider: %s\n%s\nInput schema: %s\n",
					tool.Title(), tool.ID(), tool.ProviderID(), tool.Description(), string(tool.InputSchema().Bytes()))
				return err
			})
		},
	}
}

func (a *app) newProfileToolCallCommand() *cobra.Command {
	var rawArguments, argumentsFile string
	command := &cobra.Command{
		Use:   "call <profile> <tool-id>",
		Short: "Invoke a Tool with mandatory Profile governance and Audit",
		Args:  exactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			toolID, err := catalogdomain.ParseToolID(args[1])
			if err != nil {
				return usageError{err: err}
			}
			arguments, err := readArguments(rawArguments, argumentsFile)
			if err != nil {
				return usageError{err: err}
			}
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				// Resolving the exact Profile before calling avoids a missing
				// Profile silently falling through to the owner Catalog scope.
				profile, err := resolveProfileCLI(cmd.Context(), runtime, args[0])
				if err != nil {
					return err
				}
				if !profile.Contains(toolID) {
					return fmt.Errorf("tool %s is outside Profile %s", toolID, profile.Slug())
				}
				execution, err := cliInvocationContext(cmd.Context(), runtime, profile.Slug().String())
				if err != nil {
					return err
				}
				response, err := runtime.CallConfiguredTool(cmd.Context(), toolID, arguments, execution)
				if err != nil {
					return err
				}
				projection, err := invocationProjection(response.InvocationID.String(), response.Outcome)
				if err != nil {
					return err
				}
				if a.output == "json" {
					if err := a.writeValue("", projection); err != nil {
						return err
					}
					if isNonSuccessOutcome(response.Outcome) {
						return invocationOutcomeError{outcome: response.Outcome}
					}
					return nil
				}
				switch response.Outcome.Kind() {
				case invocationdomain.OutcomeKindComplete:
					_, err = fmt.Fprintln(a.options.Stdout, string(response.Outcome.Content().Bytes()))
				case invocationdomain.OutcomeKindInputRequired:
					_, err = fmt.Fprintf(a.options.Stdout, "input required: %s (%s)\n",
						response.Outcome.Message(), response.Outcome.RequestID())
				case invocationdomain.OutcomeKindTask:
					_, err = fmt.Fprintf(a.options.Stdout, "task %s: %s\n",
						response.Outcome.TaskID(), response.Outcome.TaskStatus())
				case invocationdomain.OutcomeKindRejected, invocationdomain.OutcomeKindFailed:
					return invocationOutcomeError{outcome: response.Outcome}
				}
				return err
			})
		},
	}
	command.Flags().StringVar(&rawArguments, "args", "", "Tool arguments as a JSON object")
	command.Flags().StringVar(&argumentsFile, "args-file", "", "read Tool arguments from a JSON file")
	command.MarkFlagsMutuallyExclusive("args", "args-file")
	return command
}
