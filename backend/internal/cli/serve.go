package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sort"
	"time"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	mcpserver "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/server"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	"github.com/spf13/cobra"
)

func (a *app) newServeCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "serve",
		Short: "Serve Mender through supported protocols",
	}
	command.AddCommand(a.newServeMCPCommand())
	return command
}

func (a *app) newServeMCPCommand() *cobra.Command {
	var profileValue, httpAddress string
	var stdio bool

	command := &cobra.Command{
		Use:   "mcp",
		Short: "Serve one Profile over MCP",
		Args:  exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if profileValue == "" {
				return usageError{err: fmt.Errorf("--profile is required")}
			}
			if stdio == (httpAddress != "") {
				return usageError{err: fmt.Errorf("exactly one of --stdio or --http is required")}
			}

			slug, err := profiledomain.NewSlug(profileValue)
			if err != nil {
				return usageError{err: err}
			}

			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				profile, err := runtime.Profiles.Get.Execute(cmd.Context(), slug)
				if err != nil {
					return err
				}
				if err := activateProfileProviders(cmd.Context(), runtime, profile); err != nil {
					return err
				}

				adapter, err := runtime.NewMCPServer(cmd.Context(), profile)
				if err != nil {
					return err
				}

				if stdio {
					_, _ = fmt.Fprintf(
						a.options.Stderr,
						"serving MCP profile %s over stdio\n",
						profile.Slug(),
					)
					return a.options.ServeStdio(cmd.Context(), adapter)
				}

				_, _ = fmt.Fprintf(
					a.options.Stderr,
					"serving MCP profile %s on %s\n",
					profile.Slug(),
					httpAddress,
				)
				return a.options.ServeHTTP(cmd.Context(), httpAddress, adapter)
			})
		},
	}

	command.Flags().StringVar(&profileValue, "profile", "", "profile slug to expose")
	command.Flags().BoolVar(&stdio, "stdio", false, "serve MCP over stdin/stdout")
	command.Flags().StringVar(&httpAddress, "http", "", "serve stateless Streamable HTTP on address")
	return command
}

func activateProfileProviders(
	ctx context.Context,
	runtime *bootstrap.Runtime,
	profile profiledomain.Profile,
) error {
	providerSet := make(map[catalogdomain.ProviderID]struct{})
	for _, toolID := range profile.ToolIDs() {
		tool, err := runtime.Catalog.DescribeTool.Execute(ctx, toolID)
		if err != nil {
			return fmt.Errorf("activate profile provider: describe %s: %w", toolID, err)
		}
		providerSet[tool.ProviderID()] = struct{}{}
	}

	providerIDs := make([]catalogdomain.ProviderID, 0, len(providerSet))
	for providerID := range providerSet {
		providerIDs = append(providerIDs, providerID)
	}
	sort.Slice(providerIDs, func(i, j int) bool {
		return providerIDs[i].String() < providerIDs[j].String()
	})

	for _, providerID := range providerIDs {
		if err := runtime.ActivateConfiguredMCPProvider(ctx, providerID); err != nil {
			return fmt.Errorf("activate profile provider %s: %w", providerID, err)
		}
	}
	return nil
}

func serveHTTP(
	ctx context.Context,
	address string,
	adapter *mcpserver.Adapter,
) error {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", address, err)
	}
	defer func() { _ = listener.Close() }()

	server := &http.Server{
		Handler:           adapter.StreamableHTTPHandler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Serve(listener)
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve MCP HTTP: %w", err)

	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown MCP HTTP: %w", err)
		}
		err := <-errCh
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve MCP HTTP: %w", err)
		}
		return nil
	}
}
