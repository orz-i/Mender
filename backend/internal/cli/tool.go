package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	"github.com/spf13/cobra"
)

func (a *app) newToolCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "tool",
		Short: "Discover and invoke tools",
	}
	command.AddCommand(a.newToolSearchCommand())
	command.AddCommand(a.newToolDescribeCommand())
	command.AddCommand(a.newToolCallCommand())
	return command
}

func (a *app) newToolSearchCommand() *cobra.Command {
	var limit int
	var profileValue string

	command := &cobra.Command{
		Use:   "search <query>",
		Short: "Search the canonical tool catalog",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if limit < 1 || limit > 100 {
				return usageError{err: fmt.Errorf("--limit must be between 1 and 100")}
			}

			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				request := catalogapplication.SearchToolsRequest{
					Query: args[0],
					Limit: limit,
				}

				if profileValue != "" {
					slug, err := profiledomain.NewSlug(profileValue)
					if err != nil {
						return usageError{err: err}
					}
					profile, err := runtime.Profiles.Get.Execute(cmd.Context(), slug)
					if err != nil {
						return err
					}
					request.AllowedToolIDs = profile.ToolIDs()
				}

				results, err := runtime.Catalog.SearchTools.Execute(cmd.Context(), request)
				if err != nil {
					return err
				}

				if a.output == "json" {
					items := make([]map[string]any, 0, len(results))
					for _, result := range results {
						items = append(items, searchResultProjection(result))
					}
					return a.writeValue("", map[string]any{"tools": items})
				}

				writer := tabwriter.NewWriter(a.options.Stdout, 0, 4, 2, ' ', 0)
				_, _ = fmt.Fprintln(writer, "RANK\tTOOL ID\tNAME\tTITLE")
				for _, result := range results {
					_, _ = fmt.Fprintf(
						writer,
						"%d\t%s\t%s\t%s\n",
						result.Rank,
						result.ToolID,
						result.Name,
						result.Title,
					)
				}
				return writer.Flush()
			})
		},
	}
	command.Flags().IntVar(&limit, "limit", 10, "maximum number of search results")
	command.Flags().StringVar(&profileValue, "profile", "", "restrict search to one profile slug")
	return command
}

func (a *app) newToolDescribeCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "describe <tool-id>",
		Short: "Describe one canonical Tool",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			toolID, err := catalogdomain.ParseToolID(args[0])
			if err != nil {
				return usageError{err: err}
			}

			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				tool, err := runtime.Catalog.DescribeTool.Execute(cmd.Context(), toolID)
				if err != nil {
					return err
				}
				projection, err := toolProjection(tool)
				if err != nil {
					return err
				}

				if a.output == "json" {
					return a.writeValue("", projection)
				}
				_, err = fmt.Fprintf(
					a.options.Stdout,
					"%s (%s)\nProvider: %s\n%s\nInput schema: %s\n",
					tool.Title(),
					tool.ID(),
					tool.ProviderID(),
					tool.Description(),
					string(tool.InputSchema().Bytes()),
				)
				return err
			})
		},
	}
}

func (a *app) newToolCallCommand() *cobra.Command {
	var rawArguments string
	var argumentsFile string

	command := &cobra.Command{
		Use:   "call <tool-id>",
		Short: "Invoke one canonical Tool",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			toolID, err := catalogdomain.ParseToolID(args[0])
			if err != nil {
				return usageError{err: err}
			}

			arguments, err := readArguments(rawArguments, argumentsFile)
			if err != nil {
				return usageError{err: err}
			}

			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				tool, err := runtime.Catalog.DescribeTool.Execute(cmd.Context(), toolID)
				if err != nil {
					return err
				}
				if err := runtime.ActivateConfiguredMCPProvider(cmd.Context(), tool.ProviderID()); err != nil {
					return fmt.Errorf("activate tool provider: %w", err)
				}

				request, err := invokeRequest(toolID, arguments)
				if err != nil {
					return err
				}
				response, err := runtime.InvokeTool.Execute(cmd.Context(), request)
				if err != nil {
					return err
				}

				projection, err := invocationProjection(response.InvocationID.String(), response.Result)
				if err != nil {
					return err
				}
				if a.output == "json" {
					return a.writeValue("", projection)
				}

				switch response.Result.Kind() {
				case invocationdomain.ResultKindComplete:
					_, err = fmt.Fprintln(a.options.Stdout, string(response.Result.Content().Bytes()))
				case invocationdomain.ResultKindInputRequired:
					_, err = fmt.Fprintf(
						a.options.Stdout,
						"input required: %s (%s)\n",
						response.Result.Message(),
						response.Result.RequestID(),
					)
				case invocationdomain.ResultKindTask:
					_, err = fmt.Fprintf(
						a.options.Stdout,
						"task %s: %s\n",
						response.Result.TaskID(),
						response.Result.TaskStatus(),
					)
				}
				return err
			})
		},
	}
	command.Flags().StringVar(&rawArguments, "args", "", "tool arguments as a JSON object")
	command.Flags().StringVar(&argumentsFile, "args-file", "", "read tool arguments from a JSON file")
	command.MarkFlagsMutuallyExclusive("args", "args-file")
	return command
}

func readArguments(raw, path string) ([]byte, error) {
	if raw == "" && path == "" {
		raw = "{}"
	}
	if path != "" {
		//nolint:gosec // args-file is an explicit user-provided CLI input path.
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read arguments file: %w", err)
		}
		raw = string(data)
	}

	var value map[string]any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return nil, fmt.Errorf("arguments must be a JSON object: %w", err)
	}
	return json.Marshal(value)
}

func invokeRequest(
	toolID catalogdomain.ToolID,
	arguments []byte,
) (invocationapplication.InvokeToolRequest, error) {
	executionContext, err := invocationdomain.NewInvocationContext(
		invocationdomain.OriginCLI,
		"cli",
		invocationdomain.CatalogExecutionScope(),
	)
	if err != nil {
		return invocationapplication.InvokeToolRequest{}, fmt.Errorf("construct CLI invocation context: %w", err)
	}
	return invocationapplication.InvokeToolRequest{
		ToolID:    toolID,
		Arguments: arguments,
		Context:   executionContext,
	}, nil
}

func searchResultProjection(result catalogapplication.ToolSearchResult) map[string]any {
	return map[string]any{
		"rank":        result.Rank,
		"toolId":      result.ToolID.String(),
		"providerId":  result.ProviderID.String(),
		"name":        result.Name,
		"title":       result.Title,
		"description": result.Description,
	}
}

func toolProjection(tool catalogdomain.Tool) (map[string]any, error) {
	var inputSchema any
	if err := json.Unmarshal(tool.InputSchema().Bytes(), &inputSchema); err != nil {
		return nil, fmt.Errorf("decode input schema: %w", err)
	}

	var outputSchema any
	if schema := tool.OutputSchema(); schema != nil {
		if err := json.Unmarshal(schema.Bytes(), &outputSchema); err != nil {
			return nil, fmt.Errorf("decode output schema: %w", err)
		}
	}

	return map[string]any{
		"toolId":       tool.ID().String(),
		"providerId":   tool.ProviderID().String(),
		"sourceKey":    tool.SourceKey(),
		"name":         tool.Name(),
		"title":        tool.Title(),
		"description":  tool.Description(),
		"inputSchema":  inputSchema,
		"outputSchema": outputSchema,
		"tags":         tool.Tags(),
	}, nil
}

func invocationProjection(
	invocationID string,
	result invocationdomain.Result,
) (map[string]any, error) {
	projection := map[string]any{
		"invocationId": invocationID,
		"kind":         string(result.Kind()),
	}

	switch result.Kind() {
	case invocationdomain.ResultKindComplete:
		var content any
		if err := json.Unmarshal(result.Content().Bytes(), &content); err != nil {
			return nil, fmt.Errorf("decode invocation content: %w", err)
		}
		projection["content"] = content

	case invocationdomain.ResultKindInputRequired:
		projection["requestId"] = result.RequestID()
		projection["message"] = result.Message()
		if schema := result.InputSchema(); len(schema) != 0 {
			var inputSchema any
			if err := json.Unmarshal(schema, &inputSchema); err != nil {
				return nil, fmt.Errorf("decode input-required schema: %w", err)
			}
			projection["inputSchema"] = inputSchema
		}

	case invocationdomain.ResultKindTask:
		projection["taskId"] = result.TaskID()
		projection["status"] = result.TaskStatus()
		projection["message"] = result.Message()
	}

	return projection, nil
}
