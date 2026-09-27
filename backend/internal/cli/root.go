package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	credentialapplication "github.com/orz-i/Mender/backend/internal/credential/application"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	mcpconfigstore "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/configstore"
	mcpserver "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/server"
	profileapplication "github.com/orz-i/Mender/backend/internal/profile/application"
	"github.com/spf13/cobra"
)

const (
	ExitSuccess   = 0
	ExitInternal  = 1
	ExitUsage     = 2
	ExitConfig    = 3
	ExitPolicy    = 4
	ExitExecution = 5
)

type Options struct {
	Stdin       io.Reader
	Stdout      io.Writer
	Stderr      io.Writer
	OpenRuntime func(context.Context, string) (*bootstrap.Runtime, error)
	Getenv      func(string) string
	HomeDir     func() (string, error)
	ServeStdio  func(context.Context, *mcpserver.Adapter) error
	ServeHTTP   func(context.Context, string, *mcpserver.Adapter) error
}

type app struct {
	options Options
	dbPath  string
	output  string
}

type usageError struct {
	err error
}

func (e usageError) Error() string { return e.err.Error() }
func (e usageError) Unwrap() error { return e.err }

type internalError struct {
	err error
}

func (e internalError) Error() string { return e.err.Error() }
func (e internalError) Unwrap() error { return e.err }

type invocationOutcomeError struct {
	outcome invocationdomain.Outcome
}

func (e invocationOutcomeError) Error() string {
	return e.outcome.Message()
}

func NewRootCommand(options Options) *cobra.Command {
	options = normalizeOptions(options)
	a := &app{options: options}

	root := &cobra.Command{
		Use:           "mender",
		Short:         "Local-first tool runtime and gateway for LLMs and agents",
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRunE: func(*cobra.Command, []string) error {
			if a.output != "text" && a.output != "json" {
				return usageError{err: fmt.Errorf("output must be text or json")}
			}
			return nil
		},
	}
	root.SetIn(options.Stdin)
	root.SetOut(options.Stdout)
	root.SetErr(options.Stderr)
	root.PersistentFlags().StringVar(&a.dbPath, "db", "", "path to Mender SQLite database")
	root.PersistentFlags().StringVar(&a.output, "output", "text", "output format: text or json")
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return usageError{err: err}
	})

	root.AddCommand(a.newCredentialCommand())
	root.AddCommand(a.newProviderCommand())
	root.AddCommand(a.newProfileCommand())
	root.AddCommand(a.newToolCommand())
	root.AddCommand(a.newServeCommand())
	root.AddCommand(a.newVersionCommand())
	return root
}

func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	root := NewRootCommand(Options{Stdout: stdout, Stderr: stderr})
	root.SetArgs(args)
	if err := root.ExecuteContext(ctx); err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
		return exitCode(err)
	}
	return ExitSuccess
}

func normalizeOptions(options Options) Options {
	if options.Stdin == nil {
		options.Stdin = os.Stdin
	}
	if options.Stdout == nil {
		options.Stdout = io.Discard
	}
	if options.Stderr == nil {
		options.Stderr = io.Discard
	}
	if options.OpenRuntime == nil {
		options.OpenRuntime = func(ctx context.Context, path string) (*bootstrap.Runtime, error) {
			return bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{DatabasePath: path})
		}
	}
	if options.Getenv == nil {
		options.Getenv = os.Getenv
	}
	if options.HomeDir == nil {
		options.HomeDir = os.UserHomeDir
	}
	if options.ServeStdio == nil {
		options.ServeStdio = func(ctx context.Context, adapter *mcpserver.Adapter) error {
			return adapter.RunStdio(ctx)
		}
	}
	if options.ServeHTTP == nil {
		options.ServeHTTP = serveHTTP
	}
	return options
}

func (a *app) withRuntime(
	ctx context.Context,
	fn func(*bootstrap.Runtime) error,
) error {
	path, err := a.resolveDatabasePath()
	if err != nil {
		return err
	}
	runtime, err := a.options.OpenRuntime(ctx, path)
	if err != nil {
		return internalError{err: fmt.Errorf("open Mender runtime: %w", err)}
	}
	runErr := fn(runtime)
	closeErr := runtime.Close()
	if runErr != nil {
		if closeErr != nil {
			return errors.Join(
				runErr,
				internalError{err: fmt.Errorf("close Mender runtime: %w", closeErr)},
			)
		}
		return runErr
	}
	if closeErr != nil {
		return internalError{err: fmt.Errorf("close Mender runtime: %w", closeErr)}
	}
	return nil
}

func (a *app) resolveDatabasePath() (string, error) {
	path := a.dbPath
	if path == "" {
		path = a.options.Getenv("MENDER_DB")
	}
	if path == "" {
		home, err := a.options.HomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve Mender home: %w", err)
		}
		path = filepath.Join(home, ".mender", "mender.db")
	}

	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve database path: %w", err)
	}
	parent := filepath.Dir(absolute)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return "", fmt.Errorf("create database directory: %w", err)
	}
	return absolute, nil
}

func exactArgs(count int) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) != count {
			return usageError{err: fmt.Errorf("expected %d argument(s), got %d", count, len(args))}
		}
		return nil
	}
}

func exitCode(err error) int {
	var usage usageError
	var internal internalError
	var outcomeErr invocationOutcomeError
	switch {
	case errors.As(err, &usage),
		errors.Is(err, credentialapplication.ErrInvalidSecret),
		errors.Is(err, credentialapplication.ErrInvalidSource),
		errors.Is(err, profileapplication.ErrInvalidBinding),
		errors.Is(err, profileapplication.ErrBindingNameConflict):
		return ExitUsage
	case errors.As(err, &internal):
		return ExitInternal
	case errors.As(err, &outcomeErr):
		return invocationOutcomeExitCode(outcomeErr.outcome)
	case errors.Is(err, catalogapplication.ErrProviderNotFound),
		errors.Is(err, catalogapplication.ErrToolNotFound),
		errors.Is(err, credentialapplication.ErrCredentialNotFound),
		errors.Is(err, credentialapplication.ErrReferenceConflict),
		errors.Is(err, credentialapplication.ErrCredentialInUse),
		errors.Is(err, profileapplication.ErrProfileNotFound),
		errors.Is(err, mcpconfigstore.ErrNotFound):
		return ExitConfig
	case errors.Is(err, invocationapplication.ErrPolicyDenied),
		errors.Is(err, invocationapplication.ErrApprovalDenied):
		return ExitPolicy
	}
	return ExitExecution
}

func invocationOutcomeExitCode(outcome invocationdomain.Outcome) int {
	if outcome.Kind() == invocationdomain.OutcomeKindFailed {
		return ExitExecution
	}
	if outcome.Kind() != invocationdomain.OutcomeKindRejected {
		return ExitExecution
	}
	switch outcome.RejectionCode() {
	case invocationdomain.RejectionToolNotFound,
		invocationdomain.RejectionToolOutsideScope:
		return ExitConfig
	case invocationdomain.RejectionInvalidArguments:
		return ExitUsage
	case invocationdomain.RejectionPolicyDenied,
		invocationdomain.RejectionApprovalDenied:
		return ExitPolicy
	default:
		return ExitExecution
	}
}
