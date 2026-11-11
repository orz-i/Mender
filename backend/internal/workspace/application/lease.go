package application

import (
	"context"
	"errors"
	"fmt"
	"sync"

	workspace "github.com/orz-i/Mender/backend/internal/workspace/domain"
)

// Lease retains the admitted backend's cleanup authority. Close failure is
// never downgraded into a Tool failure or a successful operation.
type Lease struct {
	session      Session
	closeBackend func() error
	constraints  ExecutionConstraints
	once         sync.Once
	closeErr     error
}

func NewLease(session Session, closeBackend func() error, constraints ExecutionConstraints) *Lease {
	return &Lease{session: session, closeBackend: closeBackend, constraints: constraints}
}
func (l *Lease) Close() error {
	if l == nil || l.closeBackend == nil {
		return nil
	}
	l.once.Do(func() { l.closeErr = l.closeBackend() })
	return l.closeErr
}

type CleanupError struct{ Cause error }

func (e *CleanupError) Error() string { return e.Cause.Error() }
func (e *CleanupError) Unwrap() error { return e.Cause }

func finish[T any](l *Lease, result T, runErr error) (T, error) {
	if closeErr := l.Close(); closeErr != nil {
		var tool *workspace.ToolError
		var invalid *workspace.InvalidResultError
		if errors.As(runErr, &tool) || errors.As(runErr, &invalid) {
			runErr = nil
		}
		var zero T
		return zero, &CleanupError{Cause: errors.Join(runErr, fmt.Errorf("release Sandbox Workspace lease: %w", closeErr))}
	}
	return result, runErr
}
func (l *Lease) Abort(err error) error { _, result := finish(l, struct{}{}, err); return result }
func (l *Lease) CheckArgumentSize(size int) error {
	if l.constraints.MaxArgumentBytes == 0 || size <= l.constraints.MaxArgumentBytes {
		return nil
	}
	return &workspace.ToolError{Message: fmt.Sprintf("workspace arguments exceed limit of %d bytes", l.constraints.MaxArgumentBytes)}
}
func (l *Lease) Read(ctx context.Context, in workspace.ReadRequest) (workspace.ReadResult, error) {
	result, err := l.session.Read(ctx, in)
	return finish(l, result, err)
}
func (l *Lease) Search(ctx context.Context, in workspace.SearchRequest) (workspace.SearchResult, error) {
	result, err := l.session.Search(ctx, in)
	return finish(l, result, err)
}
func (l *Lease) Edit(ctx context.Context, in workspace.EditRequest) (workspace.EditResult, error) {
	result, err := l.session.Edit(ctx, in)
	return finish(l, result, err)
}
func (l *Lease) Shell(ctx context.Context, in workspace.ShellRequest) (workspace.ShellResult, error) {
	result, err := l.session.Shell(ctx, in)
	return finish(l, result, err)
}
