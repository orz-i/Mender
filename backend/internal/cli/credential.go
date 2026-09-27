package cli

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	credentialapplication "github.com/orz-i/Mender/backend/internal/credential/application"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	"github.com/spf13/cobra"
)

const maxCLISecretBytes = 64 * 1024

func (a *app) newCredentialCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "credential",
		Short: "Manage Credential inventory",
	}
	command.AddCommand(a.newCredentialListCommand())
	command.AddCommand(a.newCredentialAddCommand())
	command.AddCommand(a.newCredentialRotateCommand())
	command.AddCommand(a.newCredentialRemoveCommand())
	return command
}

func (a *app) newCredentialAddCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "add",
		Short: "Add a Credential",
	}
	command.AddCommand(a.newCredentialAddNativeCommand())
	command.AddCommand(a.newCredentialAddEnvironmentCommand())
	return command
}

func (a *app) newCredentialListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List non-secret Credential metadata",
		Args:  exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				items, err := runtime.Credentials.List.Execute(cmd.Context())
				if err != nil {
					return err
				}
				if a.output == "json" {
					projections := make([]map[string]any, 0, len(items))
					for _, item := range items {
						projections = append(projections, credentialProjection(item))
					}
					return a.writeValue("", map[string]any{"credentials": projections})
				}
				writer := tabwriter.NewWriter(a.options.Stdout, 0, 4, 2, ' ', 0)
				_, _ = fmt.Fprintln(writer, "REFERENCE	SOURCE	NAME	ENV")
				for _, item := range items {
					_, _ = fmt.Fprintf(
						writer,
						"%s\t%s\t%s\t%s\n",
						item.Reference(),
						item.Source(),
						item.DisplayName(),
						item.EnvironmentVariable(),
					)
				}
				return writer.Flush()
			})
		},
	}
}

func (a *app) newCredentialAddNativeCommand() *cobra.Command {
	var displayName string
	var secretStdin bool

	command := &cobra.Command{
		Use:   "native <namespace> <name>",
		Short: "Add a native-keyring Credential",
		Args:  exactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if displayName == "" {
				return usageError{err: fmt.Errorf("--display-name is required")}
			}
			if !secretStdin {
				return usageError{
					err: fmt.Errorf(
						"native secret must be supplied through stdin with --secret-stdin",
					),
				}
			}
			secret, err := readSecretFromStdin(a.options.Stdin)
			if err != nil {
				return usageError{err: err}
			}
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				item, err := runtime.Credentials.CreateNative.Execute(
					cmd.Context(),
					credentialapplication.CreateNativeRequest{
						Namespace:   args[0],
						Name:        args[1],
						DisplayName: displayName,
						Secret:      secret,
					},
				)
				if err != nil {
					return err
				}
				return a.writeValue(
					fmt.Sprintf("created Credential %s", item.Reference()),
					credentialProjection(item),
				)
			})
		},
	}
	command.Flags().StringVar(
		&displayName,
		"display-name",
		"",
		"Credential display name",
	)
	command.Flags().BoolVar(
		&secretStdin,
		"secret-stdin",
		false,
		"read secret bytes from stdin; no secret command-line flag exists",
	)
	return command
}

func (a *app) newCredentialAddEnvironmentCommand() *cobra.Command {
	var displayName string
	var environmentVariable string

	command := &cobra.Command{
		Use:   "environment <namespace> <name>",
		Short: "Add an environment-backed Credential",
		Args:  exactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if displayName == "" || environmentVariable == "" {
				return usageError{
					err: fmt.Errorf("--display-name and --env are required"),
				}
			}
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				item, err := runtime.Credentials.CreateEnvironment.Execute(
					cmd.Context(),
					credentialapplication.CreateEnvironmentRequest{
						Namespace:           args[0],
						Name:                args[1],
						DisplayName:         displayName,
						EnvironmentVariable: environmentVariable,
					},
				)
				if err != nil {
					return err
				}
				return a.writeValue(
					fmt.Sprintf("created Credential %s", item.Reference()),
					credentialProjection(item),
				)
			})
		},
	}
	command.Flags().StringVar(
		&displayName,
		"display-name",
		"",
		"Credential display name",
	)
	command.Flags().StringVar(
		&environmentVariable,
		"env",
		"",
		"environment variable name containing the secret",
	)
	return command
}

func (a *app) newCredentialRotateCommand() *cobra.Command {
	var secretStdin bool
	command := &cobra.Command{
		Use:   "rotate <namespace> <name>",
		Short: "Rotate a native Credential secret",
		Args:  exactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !secretStdin {
				return usageError{
					err: fmt.Errorf(
						"native secret must be supplied through stdin with --secret-stdin",
					),
				}
			}
			secret, err := readSecretFromStdin(a.options.Stdin)
			if err != nil {
				return usageError{err: err}
			}
			ref, err := credentialdomain.NewReference(args[0], args[1])
			if err != nil {
				return usageError{err: err}
			}
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				item, err := runtime.Credentials.RotateNative.Execute(
					cmd.Context(),
					ref,
					secret,
				)
				if err != nil {
					return err
				}
				return a.writeValue(
					fmt.Sprintf("rotated Credential %s", item.Reference()),
					credentialProjection(item),
				)
			})
		},
	}
	command.Flags().BoolVar(
		&secretStdin,
		"secret-stdin",
		false,
		"read secret bytes from stdin; no secret command-line flag exists",
	)
	return command
}

func (a *app) newCredentialRemoveCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <namespace> <name>",
		Short: "Remove a Credential that is not in use",
		Args:  exactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := credentialdomain.NewReference(args[0], args[1])
			if err != nil {
				return usageError{err: err}
			}
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				if err := runtime.Credentials.Delete.Execute(cmd.Context(), ref); err != nil {
					return err
				}
				return a.writeValue(
					fmt.Sprintf("removed Credential %s", ref),
					map[string]any{"reference": ref.String()},
				)
			})
		},
	}
}

func readSecretFromStdin(reader io.Reader) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(reader, maxCLISecretBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read Credential secret from stdin: %w", err)
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("Credential secret on stdin must not be empty")
	}
	if len(raw) > maxCLISecretBytes {
		return nil, fmt.Errorf("Credential secret on stdin is too large")
	}
	return raw, nil
}

func credentialProjection(item credentialdomain.Credential) map[string]any {
	return map[string]any{
		"reference":           item.Reference().String(),
		"namespace":           item.Reference().Namespace(),
		"name":                item.Reference().Name(),
		"displayName":         item.DisplayName(),
		"source":              string(item.Source()),
		"environmentVariable": item.EnvironmentVariable(),
		"createdAt":           item.CreatedAt(),
		"updatedAt":           item.UpdatedAt(),
	}
}
