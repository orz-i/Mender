package cli

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sandboxdaytona "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/daytona"
	sandboxdocker "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/dockerengine"
	sandboxsqlite "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/sqlite"
	sandboxssh "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/sshremote"
	"github.com/spf13/cobra"
)

func (a *app) newSandboxCommand() *cobra.Command {
	command := &cobra.Command{Use: "sandbox", Short: "Manage governed local and remote Sandbox resources"}
	command.AddCommand(a.newSandboxListCommand())
	command.AddCommand(a.newSandboxToolsCommand())
	command.AddCommand(a.newSandboxJobToolsCommand())
	command.AddCommand(a.newSandboxJobRecoveryCommand())
	command.AddCommand(a.newSandboxJobResolutionCommand())
	command.AddCommand(a.newSandboxJobAdmissionCommand())
	command.AddCommand(a.newSandboxJobProtocolCommand())
	command.AddCommand(a.newSandboxSuitesCommand())
	command.AddCommand(a.newSandboxLifecycleCommand("start", governancedomain.ActionSandboxStart))
	command.AddCommand(a.newSandboxLifecycleCommand("stop", governancedomain.ActionSandboxStop))
	command.AddCommand(a.newSandboxLifecycleCommand("destroy", governancedomain.ActionSandboxDestroy))
	add := &cobra.Command{Use: "add", Short: "Provision a Sandbox"}
	add.AddCommand(a.newSandboxAddLocalCommand())
	add.AddCommand(a.newSandboxAddDockerCommand())
	add.AddCommand(a.newSandboxAddDaytonaCommand())
	add.AddCommand(a.newSandboxAddSSHRemoteCommand())
	add.AddCommand(a.newSandboxAddExternalCommand())
	command.AddCommand(a.newSandboxReconcileCommand())
	command.AddCommand(add)
	return command
}

func (a *app) newSandboxSuitesCommand() *cobra.Command {
	return &cobra.Command{Use: "suites", Short: "Inspect reviewed Tool Suite definitions without activating a backend",
		Args: exactArgs(0), RunE: func(cmd *cobra.Command, _ []string) error {
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				suites, err := runtime.ListToolSuiteDefinitions(cmd.Context())
				if err != nil {
					return err
				}
				return a.writeValue(fmt.Sprintf("%d Tool Suite definitions", len(suites)), map[string]any{"suites": suites})
			})
		},
	}
}
func (a *app) newSandboxToolsCommand() *cobra.Command {
	var prepare, remove, confirm bool
	command := &cobra.Command{Use: "tools <sandbox-id>", Short: "Inspect or explicitly manage one Sandbox Workspace Tool Suite",
		Args: exactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			if prepare && remove {
				return usageError{err: fmt.Errorf("--prepare and --remove are mutually exclusive")}
			}
			if remove && !confirm {
				return usageError{err: fmt.Errorf("--remove requires --confirm")}
			}
			id, err := sandboxdomain.ParseID(args[0])
			if err != nil {
				return usageError{err: err}
			}
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				caller, err := governancedomain.NewContext(governancedomain.OriginCLI, runtime.OwnerPrincipal(), governancedomain.CatalogScope())
				if err != nil {
					return err
				}
				if remove {
					if err := runtime.RemoveSandboxWorkspaceTools(cmd.Context(), id, caller); err != nil {
						return err
					}
					return a.writeValue("Sandbox Workspace Tool Suite retired", map[string]any{"sandboxId": id, "removed": true})
				}
				var state bootstrap.SandboxToolSuite
				if prepare {
					state, err = runtime.EnsureSandboxWorkspaceTools(cmd.Context(), id, caller)
				} else {
					state, err = runtime.SandboxWorkspaceTools(cmd.Context(), id, caller)
				}
				if err != nil {
					return err
				}
				return a.writeValue(fmt.Sprintf("Sandbox tools: %s (%d Tools)", state.Status, len(state.Tools)), map[string]any{"suite": state})
			})
		},
	}
	command.Flags().BoolVar(&prepare, "prepare", false, "explicitly create/adopt the governed Workspace suite; no Profile grants")
	command.Flags().BoolVar(&remove, "remove", false, "retire a suite only if no Profile or Policy references remain")
	command.Flags().BoolVar(&confirm, "confirm", false, "confirm removal of the Workspace suite")
	return command
}
func (a *app) newSandboxJobToolsCommand() *cobra.Command {
	var prepare, remove, confirm bool
	command := &cobra.Command{Use: "job-tools <sandbox-id>", Short: "Inspect or explicitly manage one SSH Jobs Tool Suite",
		Args: exactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			if prepare && remove {
				return usageError{err: fmt.Errorf("--prepare and --remove are mutually exclusive")}
			}
			if remove && !confirm {
				return usageError{err: fmt.Errorf("--remove requires --confirm")}
			}
			id, err := sandboxdomain.ParseID(args[0])
			if err != nil {
				return usageError{err: err}
			}
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				caller, err := governancedomain.NewContext(governancedomain.OriginCLI, runtime.OwnerPrincipal(), governancedomain.CatalogScope())
				if err != nil {
					return err
				}
				if remove {
					if err := runtime.RemoveSandboxJobTools(cmd.Context(), id, caller); err != nil {
						return err
					}
					return a.writeValue("SSH Jobs Tool Suite retired", map[string]any{"sandboxId": id, "removed": true})
				}
				var state bootstrap.SandboxToolSuite
				if prepare {
					state, err = runtime.EnsureSandboxJobTools(cmd.Context(), id, caller)
				} else {
					state, err = runtime.SandboxJobTools(cmd.Context(), id, caller)
				}
				if err != nil {
					return err
				}
				return a.writeValue(fmt.Sprintf("Sandbox tools: %s (%d Tools)", state.Status, len(state.Tools)), map[string]any{"suite": state})
			})
		},
	}
	command.Flags().BoolVar(&prepare, "prepare", false, "explicitly create/adopt the governed Jobs suite; no Profile grants")
	command.Flags().BoolVar(&remove, "remove", false, "retire a suite only if no Profile or Policy references remain")
	command.Flags().BoolVar(&confirm, "confirm", false, "confirm removal of the Jobs suite")
	return command
}
func (a *app) newSandboxAddLocalCommand() *cobra.Command {
	var name, root, isolation string
	command := &cobra.Command{
		Use: "local", Short: "Provision a local Workspace Sandbox with explicit isolation",
		Args: exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if name == "" || root == "" || isolation == "" {
				return usageError{err: fmt.Errorf("--name, --root, and --isolation are required")}
			}
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				caller, err := governancedomain.NewContext(governancedomain.OriginCLI, runtime.OwnerPrincipal(), governancedomain.CatalogScope())
				if err != nil {
					return err
				}
				item, err := runtime.Sandboxes.CreateLocalSandbox(cmd.Context(), bootstrap.CreateLocalSandboxRequest{
					Name: name, Root: root, Isolation: sandboxdomain.IsolationClass(isolation),
				}, caller)
				if err != nil {
					return err
				}
				return a.writeValue(fmt.Sprintf("created %s (%s, %s)", item.ID(), item.Backend(), item.Isolation()), map[string]any{"sandbox": sandboxProjection(item)})
			})
		},
	}
	command.Flags().StringVar(&name, "name", "", "Sandbox display name")
	command.Flags().StringVar(&root, "root", "", "absolute local directory")
	command.Flags().StringVar(&isolation, "isolation", "", "host (unisolated) or restricted_host (fail closed)")
	return command
}
func (a *app) newSandboxListCommand() *cobra.Command {
	return &cobra.Command{Use: "list", Short: "List configured Sandboxes", Args: exactArgs(0), RunE: func(cmd *cobra.Command, _ []string) error {
		return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
			items, err := runtime.Sandboxes.List(cmd.Context())
			if err != nil {
				return err
			}
			projected := make([]map[string]any, 0, len(items))
			for _, item := range items {
				projected = append(projected, sandboxProjection(item))
			}
			if a.output == "json" {
				return a.writeValue("", map[string]any{"sandboxes": projected})
			}
			writer := tabwriter.NewWriter(a.options.Stdout, 0, 4, 2, ' ', 0)
			_, _ = fmt.Fprintln(writer, "SANDBOX ID\tBACKEND\tISOLATION\tNAME")
			for _, item := range items {
				_, _ = fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", item.ID(), item.Backend(), item.Isolation(), item.Spec().Name)
			}
			return writer.Flush()
		})
	}}
}
func sandboxProjection(item sandboxdomain.Sandbox) map[string]any {
	spec := item.Spec()
	return map[string]any{"sandboxId": spec.ID.String(), "name": spec.Name, "backend": string(spec.Backend), "isolation": string(spec.Isolation), "localRoot": spec.LocalRoot, "ownerKind": spec.OwnerKind, "ownerRef": spec.OwnerRef,
		"pluginId": spec.Binding.PluginID.String(), "extensionId": spec.Binding.ExtensionID.String(),
		"installationId": spec.Binding.InstallationID.String()}
}

func (a *app) newSandboxLifecycleCommand(name string, action governancedomain.Action) *cobra.Command {
	var confirm bool
	command := &cobra.Command{
		Use:   name + " <sandbox-id>",
		Short: "Govern a local Sandbox lifecycle transition. Destroy never deletes host files.",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := sandboxdomain.ParseID(args[0])
			if err != nil {
				return usageError{err: err}
			}
			if action == governancedomain.ActionSandboxDestroy && !confirm {
				return usageError{err: fmt.Errorf("sandbox destroy requires --confirm; host files remain untouched")}
			}
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				caller, err := governancedomain.NewContext(governancedomain.OriginCLI, runtime.OwnerPrincipal(), governancedomain.CatalogScope())
				if err != nil {
					return err
				}
				run, err := runtime.Sandboxes.ChangeLifecycle(cmd.Context(), id, action, caller)
				if err != nil {
					return err
				}
				item, err := runtime.Sandboxes.Get(cmd.Context(), id)
				if err != nil {
					return err
				}
				return a.writeValue(fmt.Sprintf("Sandbox %s: %s", id, run.State), map[string]any{"sandbox": sandboxProjection(item), "state": string(run.State), "generation": run.Generation})
			})
		},
	}
	if action == governancedomain.ActionSandboxDestroy {
		command.Flags().BoolVar(&confirm, "confirm", false, "acknowledge irreversible resource tombstone (local directory is NOT removed)")
	}
	return command
}

func (a *app) newSandboxAddDockerCommand() *cobra.Command {
	var name, image string
	var uid, gid, cpu, ram, workspace, pids uint32
	command := &cobra.Command{Use: "docker", Short: "Provision a Docker Sandbox from a locally cached immutable image digest", Args: exactArgs(0), RunE: func(cmd *cobra.Command, _ []string) error {
		if name == "" || image == "" {
			return usageError{err: fmt.Errorf("docker Sandbox requires --name and --image-digest")}
		}
		template := sandboxdocker.Template{ImageDigest: image, UserID: uid, GroupID: gid, CPUmilli: cpu, MemoryMiB: ram, WorkspaceMiB: workspace, PIDs: pids}
		if err := template.Validate(); err != nil {
			return usageError{err: err}
		}
		return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
			caller, err := governancedomain.NewContext(governancedomain.OriginCLI, runtime.OwnerPrincipal(), governancedomain.CatalogScope())
			if err != nil {
				return err
			}
			item, err := runtime.Sandboxes.CreateDockerSandbox(cmd.Context(), bootstrap.CreateDockerSandboxRequest{Name: name, Template: template}, caller)
			if err != nil {
				return err
			}
			return a.writeValue(fmt.Sprintf("Docker Sandbox %s provisioned", item.ID()), map[string]any{"sandbox": sandboxProjection(item)})
		})
	}}
	command.Flags().StringVar(&name, "name", "", "Sandbox display name")
	command.Flags().StringVar(&image, "image-digest", "", "locally cached immutable repository@sha256 digest; no pulls")
	command.Flags().Uint32Var(&uid, "user-id", 10001, "non-root numeric UID")
	command.Flags().Uint32Var(&gid, "group-id", 10001, "non-root numeric GID")
	command.Flags().Uint32Var(&cpu, "cpu-milli", 1000, "bounded CPU millicores")
	command.Flags().Uint32Var(&ram, "memory-mib", 512, "bounded memory in MiB")
	command.Flags().Uint32Var(&workspace, "workspace-mib", 64, "ephemeral in-container tmpfs size MiB")
	command.Flags().Uint32Var(&pids, "pids", 64, "bounded PID count")
	return command
}

func (a *app) newSandboxReconcileCommand() *cobra.Command {
	return &cobra.Command{Use: "reconcile <sandbox-id>", Short: "Inspect an unknown Docker Sandbox without recreating it", Args: exactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		id, err := sandboxdomain.ParseID(args[0])
		if err != nil {
			return usageError{err: err}
		}
		return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
			caller, err := governancedomain.NewContext(governancedomain.OriginCLI, runtime.OwnerPrincipal(), governancedomain.CatalogScope())
			if err != nil {
				return err
			}
			item, err := runtime.Sandboxes.Get(cmd.Context(), id)
			if err != nil {
				return err
			}
			if item.Backend() == sandboxdomain.BackendLocalWorkspace {
				return sandboxdomain.ErrBackendUnavailable
			}
			run, err := runtime.Sandboxes.Reconcile(cmd.Context(), id, caller)
			if err != nil {
				return err
			}
			item, err = runtime.Sandboxes.Get(cmd.Context(), id)
			if err != nil {
				return err
			}
			return a.writeValue(fmt.Sprintf("Sandbox %s reconciled: %s", id, run.State), map[string]any{"sandbox": sandboxProjection(item), "state": string(run.State), "generation": run.Generation})
		})
	}}
}

func (a *app) newSandboxAddDaytonaCommand() *cobra.Command {
	var name, credential, image, target string
	var cpu, memory, disk, ttl int
	cmd := &cobra.Command{Use: "daytona", Short: "Governed Daytona remote Sandbox creation (may incur provider charges)", Args: exactArgs(0), RunE: func(cmd *cobra.Command, _ []string) error {
		ref, err := credentialdomain.ParseReference(credential)
		if err != nil {
			return usageError{err: err}
		}
		template := sandboxdaytona.Template{CredentialRef: ref, ImageDigest: image, Target: target, CPU: cpu, MemoryGiB: memory, DiskGiB: disk, TTLMinutes: ttl}
		if err := template.Validate(); err != nil {
			return usageError{err: err}
		}
		if name == "" {
			return usageError{err: fmt.Errorf("daytona requires --name")}
		}
		return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
			caller, err := governancedomain.NewContext(governancedomain.OriginCLI, runtime.OwnerPrincipal(), governancedomain.CatalogScope())
			if err != nil {
				return err
			}
			item, err := runtime.Sandboxes.CreateDaytonaSandbox(cmd.Context(), bootstrap.CreateDaytonaSandboxRequest{Name: name, Template: template}, caller)
			if err != nil && item.ID() == "" {
				return err
			}
			run, lookupErr := runtime.Sandboxes.GetRun(cmd.Context(), item.ID())
			if lookupErr != nil {
				return lookupErr
			}
			return a.writeValue(fmt.Sprintf("Daytona Sandbox %s: %s (verify remote charges)", item.ID(), run.State), map[string]any{"sandbox": sandboxProjection(item), "state": string(run.State), "generation": run.Generation})
		})
	}}
	cmd.Flags().StringVar(&name, "name", "", "Sandbox display name")
	cmd.Flags().StringVar(&credential, "credential-ref", "", "Mender Credential Reference, e.g. credential://daytona/api-key")
	cmd.Flags().StringVar(&image, "image-digest", "", "immutable repository@sha256 image reference")
	cmd.Flags().StringVar(&target, "target", sandboxdaytona.TargetUS, "allowlisted Daytona target")
	cmd.Flags().IntVar(&cpu, "cpu", 1, "vCPU allocation (1..2)")
	cmd.Flags().IntVar(&memory, "memory-gib", 2, "memory allocation GiB (1..4)")
	cmd.Flags().IntVar(&disk, "disk-gib", 3, "disk allocation GiB (3..10)")
	cmd.Flags().IntVar(&ttl, "ttl-minutes", 30, "provider-enforced lifetime (15..120min), not a USD spending limit")
	return cmd
}

func (a *app) newSandboxAddSSHRemoteCommand() *cobra.Command {
	var name, host, user, root, publicKey, credential string
	var port uint16
	command := &cobra.Command{Use: "ssh-remote", Short: "Bind a trusted SSH login to a governed remote Workspace (NOT OS isolation)", Args: exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			ref, err := credentialdomain.ParseReference(credential)
			if err != nil {
				return usageError{err: err}
			}
			t := sandboxssh.Template{Host: host, Port: port, User: user, Root: root, HostPublicKey: publicKey, CredentialRef: ref}
			if err := t.Validate(); err != nil {
				return usageError{err: err}
			}
			if name == "" {
				return usageError{err: fmt.Errorf("SSH remote requires --name")}
			}
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				caller, err := governancedomain.NewContext(governancedomain.OriginCLI,
					runtime.OwnerPrincipal(), governancedomain.CatalogScope())
				if err != nil {
					return err
				}
				item, err := runtime.Sandboxes.CreateSSHRemoteSandbox(cmd.Context(),
					bootstrap.CreateSSHRemoteSandboxRequest{Name: name, Template: t}, caller)
				if err != nil {
					return err
				}
				return a.writeValue(fmt.Sprintf("SSH Remote Sandbox %s verified (remote host unchanged)", item.ID()),
					map[string]any{"sandbox": sandboxProjection(item)})
			})
		},
	}
	command.Flags().StringVar(&name, "name", "", "Sandbox display name")
	command.Flags().StringVar(&host, "host", "", "explicit SSH host name or IP")
	command.Flags().Uint16Var(&port, "port", 22, "SSH port (1..65535)")
	command.Flags().StringVar(&user, "user", "", "SSH login user; root permitted but has privileged remote access")
	command.Flags().StringVar(&root, "root", "", "absolute POSIX remote workspace directory")
	command.Flags().StringVar(&publicKey, "host-public-key", "", "explicit pinned Ed25519, ECDSA or RSA >=2048 host public key in OpenSSH format")
	command.Flags().StringVar(&credential, "credential-ref", "", "native OS keychain reference, credential://ssh/name; never a private key")
	return command
}

// Add a remote Sandbox only by an installed, verified, pinned extension package.
// The driver config is non-secret; no model Tool can select this operation.
func (a *app) newSandboxAddExternalCommand() *cobra.Command {
	var name, installation, configJSON string
	command := &cobra.Command{Use: "external", Short: "Provision a governed Sandbox from one exact installed backend plugin", Args: exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if name == "" || installation == "" || configJSON == "" {
				return usageError{err: fmt.Errorf("--name, --installation and --config-json are required")}
			}
			id, err := platformplugin.ParseInstallationID(installation)
			if err != nil {
				return usageError{err: err}
			}
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				installation, err := runtime.GetPluginInstallation(cmd.Context(), id)
				if err != nil {
					return err
				}
				descriptor, verified := installation.VerifiedDescriptor()
				if !verified {
					return fmt.Errorf("sandbox plugin installation descriptor is unverified")
				}
				config := sandboxsqlite.ExternalConfig{ConfigVersion: descriptor.ConfigVersion, JSON: json.RawMessage(configJSON)}
				if err := config.Validate(); err != nil {
					return usageError{err: err}
				}
				caller, err := governancedomain.NewContext(governancedomain.OriginCLI, runtime.OwnerPrincipal(), governancedomain.CatalogScope())
				if err != nil {
					return err
				}
				item, createErr := runtime.Sandboxes.CreateExternalSandbox(cmd.Context(), bootstrap.CreateExternalSandboxRequest{
					Name: name, InstallationID: id, Config: config,
				}, caller)
				if item.ID() == "" {
					return createErr
				}
				run, err := runtime.Sandboxes.GetRun(cmd.Context(), item.ID())
				if err != nil {
					return err
				}
				// A remote uncertain result must be visible; never silently
				// auto-retry or mark a running Sandbox as ready.
				return a.writeValue(fmt.Sprintf("external Sandbox %s: %s (review/reconcile if unknown)", item.ID(), run.State),
					map[string]any{"sandbox": sandboxProjection(item), "state": string(run.State), "generation": run.Generation,
						"needsReconciliation": createErr != nil})
			})
		},
	}
	command.Flags().StringVar(&name, "name", "", "Sandbox display name")
	command.Flags().StringVar(&installation, "installation", "", "exact installed sandbox.backend InstallationID")
	command.Flags().StringVar(&configJSON, "config-json", "", "bounded non-secret plugin configuration JSON (credential fields are refused)")
	return command
}
