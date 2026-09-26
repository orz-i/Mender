package cli

import (
	"context"
	"fmt"
	"text/tabwriter"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	mcpconfigstore "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/configstore"
	"github.com/spf13/cobra"
)

func (a *app) newProviderCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "provider",
		Short: "Manage tool providers",
	}
	command.AddCommand(a.newProviderAddCommand())
	command.AddCommand(a.newProviderListCommand())
	command.AddCommand(a.newProviderSyncCommand())
	return command
}

func (a *app) newProviderAddCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "add",
		Short: "Register an MCP provider",
	}
	command.AddCommand(a.newProviderAddHTTPCommand())
	command.AddCommand(a.newProviderAddStdioCommand())
	return command
}

func (a *app) newProviderAddHTTPCommand() *cobra.Command {
	var namespace, displayName, description, endpoint string

	command := &cobra.Command{
		Use:   "http",
		Short: "Register a remote Streamable HTTP MCP provider",
		Args:  exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if namespace == "" || displayName == "" || endpoint == "" {
				return usageError{err: fmt.Errorf("--namespace, --name, and --endpoint are required")}
			}

			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				provider, err := runtime.RegisterMCPProvider(
					cmd.Context(),
					catalogapplication.RegisterProviderRequest{
						Namespace:   namespace,
						DisplayName: displayName,
						Description: description,
					},
					mcpconfigstore.Config{
						Transport: mcpconfigstore.TransportHTTP,
						Endpoint:  endpoint,
					},
				)
				if err != nil {
					return err
				}

				syncResult, syncErr := runtime.SyncConfiguredMCPProvider(cmd.Context(), provider.ID())
				if syncErr != nil {
					return fmt.Errorf(
						"provider %s registered but initial synchronization failed: %w",
						provider.ID(),
						syncErr,
					)
				}

				return a.writeValue(
					fmt.Sprintf(
						"registered %s (%s); synchronized %d tool(s)",
						provider.ID(),
						provider.Namespace(),
						syncResult.Total,
					),
					map[string]any{
						"provider": providerProjection(provider),
						"sync":     syncProjection(syncResult),
					},
				)
			})
		},
	}
	command.Flags().StringVar(&namespace, "namespace", "", "stable provider namespace")
	command.Flags().StringVar(&displayName, "name", "", "provider display name")
	command.Flags().StringVar(&description, "description", "", "provider description")
	command.Flags().StringVar(&endpoint, "endpoint", "", "Streamable HTTP MCP endpoint")
	return command
}

func (a *app) newProviderAddStdioCommand() *cobra.Command {
	var namespace, displayName, description, executable, directory string
	var args []string

	command := &cobra.Command{
		Use:   "stdio",
		Short: "Register a local stdio MCP provider",
		Args:  exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if namespace == "" || displayName == "" || executable == "" {
				return usageError{err: fmt.Errorf("--namespace, --name, and --exec are required")}
			}

			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				provider, err := runtime.RegisterMCPProvider(
					cmd.Context(),
					catalogapplication.RegisterProviderRequest{
						Namespace:   namespace,
						DisplayName: displayName,
						Description: description,
					},
					mcpconfigstore.Config{
						Transport:  mcpconfigstore.TransportStdio,
						Executable: executable,
						Args:       append([]string(nil), args...),
						Directory:  directory,
					},
				)
				if err != nil {
					return err
				}

				syncResult, syncErr := runtime.SyncConfiguredMCPProvider(cmd.Context(), provider.ID())
				if syncErr != nil {
					return fmt.Errorf(
						"provider %s registered but initial synchronization failed: %w",
						provider.ID(),
						syncErr,
					)
				}

				return a.writeValue(
					fmt.Sprintf(
						"registered %s (%s); synchronized %d tool(s)",
						provider.ID(),
						provider.Namespace(),
						syncResult.Total,
					),
					map[string]any{
						"provider": providerProjection(provider),
						"sync":     syncProjection(syncResult),
					},
				)
			})
		},
	}
	command.Flags().StringVar(&namespace, "namespace", "", "stable provider namespace")
	command.Flags().StringVar(&displayName, "name", "", "provider display name")
	command.Flags().StringVar(&description, "description", "", "provider description")
	command.Flags().StringVar(&executable, "exec", "", "MCP server executable")
	command.Flags().StringArrayVar(&args, "arg", nil, "argument passed to the executable; may be repeated")
	command.Flags().StringVar(&directory, "dir", "", "working directory")
	return command
}

func (a *app) newProviderListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List registered providers",
		Args:  exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				providers, err := runtime.Catalog.ListProviders.Execute(cmd.Context())
				if err != nil {
					return err
				}
				configs, err := runtime.ListMCPProviderConfigs(cmd.Context())
				if err != nil {
					return err
				}

				configByProvider := make(map[catalogdomain.ProviderID]mcpconfigstore.Config, len(configs))
				for _, config := range configs {
					configByProvider[config.ProviderID] = config
				}

				if a.output == "json" {
					items := make([]map[string]any, 0, len(providers))
					for _, provider := range providers {
						item := providerProjection(provider)
						if config, ok := configByProvider[provider.ID()]; ok {
							item["transport"] = string(config.Transport)
							item["connection"] = connectionProjection(config)
						}
						items = append(items, item)
					}
					return a.writeValue("", map[string]any{"providers": items})
				}

				writer := tabwriter.NewWriter(a.options.Stdout, 0, 4, 2, ' ', 0)
				_, _ = fmt.Fprintln(writer, "NAMESPACE\tID\tTRANSPORT\tNAME")
				for _, provider := range providers {
					transport := "-"
					if config, ok := configByProvider[provider.ID()]; ok {
						transport = string(config.Transport)
					}
					_, _ = fmt.Fprintf(
						writer,
						"%s\t%s\t%s\t%s\n",
						provider.Namespace(),
						provider.ID(),
						transport,
						provider.DisplayName(),
					)
				}
				return writer.Flush()
			})
		},
	}
}

func (a *app) newProviderSyncCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "sync <provider>",
		Short: "Synchronize a provider tool catalog",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				provider, err := resolveProvider(cmd.Context(), runtime, args[0])
				if err != nil {
					return err
				}

				result, err := runtime.SyncConfiguredMCPProvider(cmd.Context(), provider.ID())
				if err != nil {
					return err
				}
				return a.writeValue(
					fmt.Sprintf(
						"synchronized %s: %d tool(s), %d created, %d updated, %d removed",
						provider.Namespace(),
						result.Total,
						result.Created,
						result.Updated,
						result.Removed,
					),
					map[string]any{
						"provider": providerProjection(provider),
						"sync":     syncProjection(result),
					},
				)
			})
		},
	}
}

func resolveProvider(
	ctx context.Context,
	runtime *bootstrap.Runtime,
	value string,
) (catalogdomain.Provider, error) {
	if id, err := catalogdomain.ParseProviderID(value); err == nil {
		return runtime.Catalog.GetProvider.Execute(ctx, id)
	}

	namespace, err := catalogdomain.NewNamespace(value)
	if err != nil {
		return catalogdomain.Provider{}, fmt.Errorf("provider must be a valid ProviderID or namespace: %w", err)
	}
	return runtime.Catalog.FindProviderByNamespace.Execute(ctx, namespace)
}

func providerProjection(provider catalogdomain.Provider) map[string]any {
	return map[string]any{
		"providerId":  provider.ID().String(),
		"namespace":   provider.Namespace().String(),
		"displayName": provider.DisplayName(),
		"description": provider.Description(),
		"createdAt":   provider.CreatedAt(),
		"updatedAt":   provider.UpdatedAt(),
	}
}

func syncProjection(result catalogapplication.SyncProviderToolsResult) map[string]any {
	return map[string]any{
		"created": result.Created,
		"updated": result.Updated,
		"removed": result.Removed,
		"total":   result.Total,
	}
}

func connectionProjection(config mcpconfigstore.Config) map[string]any {
	switch config.Transport {
	case mcpconfigstore.TransportHTTP:
		return map[string]any{
			"endpoint": config.Endpoint,
		}
	case mcpconfigstore.TransportStdio:
		return map[string]any{
			"executable": config.Executable,
			"args":       append([]string(nil), config.Args...),
			"directory":  config.Directory,
		}
	default:
		return map[string]any{}
	}
}
