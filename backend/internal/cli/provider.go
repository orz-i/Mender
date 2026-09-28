package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	mcpplugin "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/plugin"
	openapiplugin "github.com/orz-i/Mender/backend/internal/openapi/infrastructure/plugin"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	"github.com/spf13/cobra"
)

func (a *app) newProviderCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "provider",
		Short: "Manage tool providers",
	}
	command.AddCommand(a.newProviderAddCommand())
	command.AddCommand(a.newProviderPluginsCommand())
	command.AddCommand(a.newProviderListCommand())
	command.AddCommand(a.newProviderSyncCommand())
	return command
}

func (a *app) newProviderAddCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "add",
		Short: "Register a Provider through a built-in plugin",
	}
	command.AddCommand(a.newProviderAddMCPCommand())
	command.AddCommand(a.newProviderAddOpenAPICommand())

	// Phase 5 compatibility aliases. They are intentionally hidden so normal
	// CLI discovery teaches plugin-first Provider semantics.
	legacyHTTP := a.newProviderAddMCPHTTPCommand()
	legacyHTTP.Hidden = true
	command.AddCommand(legacyHTTP)

	legacyStdio := a.newProviderAddMCPStdioCommand()
	legacyStdio.Hidden = true
	command.AddCommand(legacyStdio)
	return command
}

func (a *app) newProviderAddMCPCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "mcp",
		Short: "Register an MCP Provider",
	}
	command.AddCommand(a.newProviderAddMCPHTTPCommand())
	command.AddCommand(a.newProviderAddMCPStdioCommand())
	return command
}

func (a *app) newProviderAddMCPHTTPCommand() *cobra.Command {
	var namespace, displayName, description, endpoint string

	command := &cobra.Command{
		Use:   "http",
		Short: "Register MCP over remote Streamable HTTP",
		Args:  exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if namespace == "" || displayName == "" || endpoint == "" {
				return usageError{err: fmt.Errorf("--namespace, --name, and --endpoint are required")}
			}
			raw, err := json.Marshal(mcpplugin.Config{
				Transport: mcpplugin.TransportConfig{
					Kind:     mcpplugin.TransportStreamableHTTP,
					Endpoint: endpoint,
				},
			})
			if err != nil {
				return fmt.Errorf("encode MCP provider config: %w", err)
			}
			return a.registerConfiguredProvider(
				cmd,
				namespace,
				displayName,
				description,
				mcpplugin.ID,
				raw,
			)
		},
	}
	command.Flags().StringVar(&namespace, "namespace", "", "stable provider namespace")
	command.Flags().StringVar(&displayName, "name", "", "provider display name")
	command.Flags().StringVar(&description, "description", "", "provider description")
	command.Flags().StringVar(&endpoint, "endpoint", "", "Streamable HTTP MCP endpoint")
	return command
}

func (a *app) newProviderAddMCPStdioCommand() *cobra.Command {
	var namespace, displayName, description, executable, directory string
	var args []string

	command := &cobra.Command{
		Use:   "stdio",
		Short: "Register MCP through a local stdio command",
		Args:  exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if namespace == "" || displayName == "" || executable == "" {
				return usageError{err: fmt.Errorf("--namespace, --name, and --exec are required")}
			}
			raw, err := json.Marshal(mcpplugin.Config{
				Transport: mcpplugin.TransportConfig{
					Kind:       mcpplugin.TransportStdio,
					Executable: executable,
					Args:       append([]string(nil), args...),
					Directory:  directory,
				},
			})
			if err != nil {
				return fmt.Errorf("encode MCP provider config: %w", err)
			}
			return a.registerConfiguredProvider(
				cmd,
				namespace,
				displayName,
				description,
				mcpplugin.ID,
				raw,
			)
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

func (a *app) newProviderAddOpenAPICommand() *cobra.Command {
	var namespace, displayName, description, specURL, specFile, serverURL string
	var include, exclude, credentialBindings []string

	command := &cobra.Command{
		Use:   "openapi",
		Short: "Register an OpenAPI Provider",
		Args:  exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if namespace == "" || displayName == "" {
				return usageError{err: fmt.Errorf("--namespace and --name are required")}
			}
			if (specURL == "") == (specFile == "") {
				return usageError{err: fmt.Errorf("exactly one of --spec-url or --spec-file is required")}
			}

			credentials, err := parseOpenAPICredentialBindings(credentialBindings)
			if err != nil {
				return usageError{err: err}
			}

			source := openapiplugin.SourceConfig{
				Kind: openapiplugin.SourceURL,
				URL:  specURL,
			}
			if specFile != "" {
				source = openapiplugin.SourceConfig{
					Kind: openapiplugin.SourceFile,
					Path: specFile,
				}
			}

			raw, err := json.Marshal(openapiplugin.Config{
				Source:    source,
				ServerURL: serverURL,
				Operations: openapiplugin.OperationFilter{
					Include: append([]string(nil), include...),
					Exclude: append([]string(nil), exclude...),
				},
				Credentials: credentials,
			})
			if err != nil {
				return fmt.Errorf("encode OpenAPI provider config: %w", err)
			}

			return a.registerConfiguredProvider(
				cmd,
				namespace,
				displayName,
				description,
				openapiplugin.ID,
				raw,
			)
		},
	}

	command.Flags().StringVar(&namespace, "namespace", "", "stable provider namespace")
	command.Flags().StringVar(&displayName, "name", "", "provider display name")
	command.Flags().StringVar(&description, "description", "", "provider description")
	command.Flags().StringVar(&specURL, "spec-url", "", "OpenAPI description URL")
	command.Flags().StringVar(&specFile, "spec-file", "", "absolute OpenAPI description file")
	command.Flags().StringVar(&serverURL, "server", "", "optional absolute API server override")
	command.Flags().StringArrayVar(&include, "include", nil, "operation SourceKey to include; may be repeated")
	command.Flags().StringArrayVar(&exclude, "exclude", nil, "operation SourceKey to exclude; may be repeated")
	command.Flags().StringArrayVar(
		&credentialBindings,
		"credential",
		nil,
		"SecurityScheme=credential://namespace/name binding; may be repeated",
	)
	return command
}

func parseOpenAPICredentialBindings(
	values []string,
) (map[string]credentialdomain.Reference, error) {
	if len(values) == 0 {
		return nil, nil
	}
	result := make(map[string]credentialdomain.Reference, len(values))
	for _, raw := range values {
		name, reference, ok := strings.Cut(raw, "=")
		name = strings.TrimSpace(name)
		reference = strings.TrimSpace(reference)
		if !ok || name == "" || reference == "" {
			return nil, fmt.Errorf(
				"invalid --credential %q; expected SecurityScheme=credential://namespace/name",
				raw,
			)
		}
		if _, exists := result[name]; exists {
			return nil, fmt.Errorf(
				"duplicate --credential binding for Security Scheme %q",
				name,
			)
		}
		ref, err := credentialdomain.ParseReference(reference)
		if err != nil {
			return nil, fmt.Errorf(
				"invalid --credential binding for Security Scheme %q: %w",
				name,
				err,
			)
		}
		result[name] = ref
	}
	return result, nil
}

func (a *app) registerConfiguredProvider(
	cmd *cobra.Command,
	namespace string,
	displayName string,
	description string,
	pluginID providerplugin.ID,
	raw json.RawMessage,
) error {
	return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
		provider, err := runtime.RegisterConfiguredProvider(
			cmd.Context(),
			catalogapplication.RegisterProviderRequest{
				Namespace:   namespace,
				DisplayName: displayName,
				Description: description,
			},
			pluginID,
			raw,
		)
		if err != nil {
			return err
		}

		syncResult, syncErr := runtime.SyncConfiguredProvider(
			cmd.Context(),
			provider.ID(),
		)
		if syncErr != nil {
			return fmt.Errorf(
				"provider %s registered but initial synchronization failed: %w",
				provider.ID(),
				syncErr,
			)
		}
		config, err := runtime.ProviderConfig(cmd.Context(), provider.ID())
		if err != nil {
			return err
		}

		item := providerProjection(provider)
		item["pluginId"] = config.PluginID.String()
		item["configVersion"] = config.ConfigVersion
		if transport, connection, _ := providerConnectionProjection(config); transport != "" {
			item["transport"] = transport
			item["connection"] = connection
		}

		return a.writeValue(
			fmt.Sprintf(
				"registered %s (%s) with %s; synchronized %d tool(s)",
				provider.ID(),
				provider.Namespace(),
				config.PluginID,
				syncResult.Total,
			),
			map[string]any{
				"provider": item,
				"sync":     syncProjection(syncResult),
			},
		)
	})
}

func (a *app) newProviderPluginsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "plugins",
		Short: "List available Provider plugins",
		Args:  exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				descriptors := runtime.ProviderPluginDescriptors()
				if a.output == "json" {
					items := make([]map[string]any, 0, len(descriptors))
					for _, descriptor := range descriptors {
						items = append(items, map[string]any{
							"pluginId":      descriptor.ID.String(),
							"version":       descriptor.Version,
							"displayName":   descriptor.DisplayName,
							"description":   descriptor.Description,
							"configVersion": descriptor.ConfigVersion,
						})
					}
					return a.writeValue("", map[string]any{"plugins": items})
				}

				writer := tabwriter.NewWriter(a.options.Stdout, 0, 4, 2, ' ', 0)
				_, _ = fmt.Fprintln(writer, "PLUGIN	VERSION	CONFIG	NAME")
				for _, descriptor := range descriptors {
					_, _ = fmt.Fprintf(
						writer,
						"%s\t%s\t%d\t%s\n",
						descriptor.ID,
						descriptor.Version,
						descriptor.ConfigVersion,
						descriptor.DisplayName,
					)
				}
				return writer.Flush()
			})
		},
	}
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
				configs, err := runtime.ListProviderConfigs(cmd.Context())
				if err != nil {
					return err
				}
				configByProvider := make(
					map[catalogdomain.ProviderID]providerplugin.Configuration,
					len(configs),
				)
				for _, config := range configs {
					configByProvider[config.ProviderID] = config
				}

				if a.output == "json" {
					items := make([]map[string]any, 0, len(providers))
					for _, provider := range providers {
						item := providerProjection(provider)
						if config, ok := configByProvider[provider.ID()]; ok {
							item["pluginId"] = config.PluginID.String()
							item["configVersion"] = config.ConfigVersion
							if transport, connection, _ := providerConnectionProjection(config); transport != "" {
								item["transport"] = transport
								item["connection"] = connection
							}
						}
						items = append(items, item)
					}
					return a.writeValue("", map[string]any{"providers": items})
				}

				writer := tabwriter.NewWriter(a.options.Stdout, 0, 4, 2, ' ', 0)
				_, _ = fmt.Fprintln(writer, "NAMESPACE	ID	PLUGIN	CONNECTION	NAME")
				for _, provider := range providers {
					pluginID := "-"
					connection := "-"
					if config, ok := configByProvider[provider.ID()]; ok {
						pluginID = config.PluginID.String()
						_, _, connection = providerConnectionProjection(config)
						if connection == "" {
							connection = "-"
						}
					}
					_, _ = fmt.Fprintf(
						writer,
						"%s\t%s\t%s\t%s\t%s\n",
						provider.Namespace(),
						provider.ID(),
						pluginID,
						connection,
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

				result, err := runtime.SyncConfiguredProvider(cmd.Context(), provider.ID())
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

func providerConnectionProjection(
	config providerplugin.Configuration,
) (transport string, connection map[string]any, summary string) {
	if config.PluginID != mcpplugin.ID {
		return "", nil, ""
	}
	mcpConfig, err := mcpplugin.DecodeConfig(config.Config)
	if err != nil {
		return "", nil, ""
	}
	switch mcpConfig.Transport.Kind {
	case mcpplugin.TransportStreamableHTTP:
		return "http", map[string]any{
			"kind":     string(mcpplugin.TransportStreamableHTTP),
			"endpoint": mcpConfig.Transport.Endpoint,
		}, "streamable-http " + mcpConfig.Transport.Endpoint
	case mcpplugin.TransportStdio:
		return "stdio", map[string]any{
			"kind":       string(mcpplugin.TransportStdio),
			"executable": mcpConfig.Transport.Executable,
			"args":       append([]string(nil), mcpConfig.Transport.Args...),
			"directory":  mcpConfig.Transport.Directory,
		}, "stdio " + mcpConfig.Transport.Executable
	default:
		return "", nil, ""
	}
}
