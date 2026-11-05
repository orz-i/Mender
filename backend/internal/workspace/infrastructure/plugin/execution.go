package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	invocation "github.com/orz-i/Mender/backend/internal/invocation/domain"
	workspaceapp "github.com/orz-i/Mender/backend/internal/workspace/application"
	workspace "github.com/orz-i/Mender/backend/internal/workspace/domain"
)

func executeTyped(ctx context.Context, lease *workspaceapp.Lease, operation workspace.Operation, arguments invocation.JSONValue, maxResultBytes int) (invocation.Outcome, error) {
	if err := ctx.Err(); err != nil {
		return mapResult(nil, lease.Abort(err), maxResultBytes)
	}
	if err := lease.CheckArgumentSize(len(arguments.Bytes())); err != nil {
		return mapResult(nil, lease.Abort(err), maxResultBytes)
	}
	var result any
	var err error
	switch operation {
	case workspace.Read:
		result, err = decodeAndRun(ctx, lease, arguments, sourceKeyRead, lease.Read)
	case workspace.Search:
		result, err = decodeAndRun(ctx, lease, arguments, sourceKeySearch, lease.Search)
	case workspace.Edit:
		result, err = decodeAndRun(ctx, lease, arguments, sourceKeyEdit, lease.Edit)
	case workspace.Shell:
		result, err = decodeAndRun(ctx, lease, arguments, sourceKeyShell, lease.Shell)
	default:
		return mapResult(nil, lease.Abort(fmt.Errorf("unknown Workspace operation %q", operation)), maxResultBytes)
	}
	return mapResult(result, err, maxResultBytes)
}

func decodeAndRun[I, O any](ctx context.Context, lease *workspaceapp.Lease, arguments invocation.JSONValue, label string, run func(context.Context, I) (O, error)) (O, error) {
	var input I
	if err := decodeStrict(arguments.Bytes(), &input, "decode "+label+" arguments"); err != nil {
		var zero O
		return zero, lease.Abort(&workspace.ToolError{Message: err.Error()})
	}
	// Record optional field presence here, where Tool JSON is translated. The
	// application and backend receive only typed values and explicitness bits.
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(arguments.Bytes(), &fields); err != nil {
		var zero O
		return zero, lease.Abort(err)
	}
	var present workspace.Fields
	for name, field := range map[string]workspace.Fields{
		"offset": workspace.FieldOffset, "limit": workspace.FieldLimit,
		"path": workspace.FieldPath, "query": workspace.FieldQuery,
		"pattern": workspace.FieldPattern, "regex": workspace.FieldRegex,
		"caseSensitive": workspace.FieldCaseSensitive, "maxResults": workspace.FieldMaxResults,
		"content": workspace.FieldContent, "edits": workspace.FieldEdits,
		"cwd": workspace.FieldCwd, "timeoutMs": workspace.FieldTimeoutMS,
	} {
		if _, ok := fields[name]; ok {
			present |= field
		}
	}
	switch request := any(&input).(type) {
	case *workspace.ReadRequest:
		request.Present = present
		request.CanonicalBytes = len(arguments.Bytes())
	case *workspace.SearchRequest:
		request.Present = present
		request.CanonicalBytes = len(arguments.Bytes())
	case *workspace.EditRequest:
		request.Present = present
		request.CanonicalBytes = len(arguments.Bytes())
		var edits []map[string]json.RawMessage
		if err := json.Unmarshal(fields["edits"], &edits); err == nil {
			for index, edit := range edits {
				if _, ok := edit["expectedReplacements"]; ok {
					request.Edits[index].Present |= workspace.FieldExpectedReplacements
				}
			}
		}
	case *workspace.ShellRequest:
		request.Present = present
		request.CanonicalBytes = len(arguments.Bytes())
	}
	return run(ctx, input)
}

func mapResult(result any, err error, maxResultBytes int) (invocation.Outcome, error) {
	if err != nil {
		var cleanup *workspaceapp.CleanupError
		var tool *workspace.ToolError
		var invalid *workspace.InvalidResultError
		switch {
		case errors.As(err, &cleanup):
			return invocation.Outcome{}, err
		case errors.As(err, &tool):
			return invocation.Failed(invocation.FailureSourceTool, invocation.FailureToolError, tool.Message)
		case errors.As(err, &invalid):
			if maxResultBytes > 0 && invalid.CanonicalBytes > maxResultBytes {
				return invocation.Failed(invocation.FailureSourceResultGuard, invocation.FailureResultTooLarge, "tool result exceeds the configured canonical result-size limit")
			}
			return invocation.Failed(invocation.FailureSourceResultGuard, invocation.FailureOutputSchemaMismatch, "tool result does not match the declared output schema")
		default:
			return invocation.Outcome{}, err
		}
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return invocation.Outcome{}, fmt.Errorf("encode Workspace result: %w", err)
	}
	content, err := invocation.NewJSONValue(raw)
	if err != nil {
		return invocation.Outcome{}, fmt.Errorf("normalize Workspace result: %w", err)
	}
	var originalBytes int
	switch value := result.(type) {
	case workspace.ReadResult:
		originalBytes = value.CanonicalBytes
	case workspace.SearchResult:
		originalBytes = value.CanonicalBytes
	case workspace.EditResult:
		originalBytes = value.CanonicalBytes
	case workspace.ShellResult:
		originalBytes = value.CanonicalBytes
	}
	if originalBytes > 0 {
		return invocation.CompleteWithResultSize(content, originalBytes)
	}
	return invocation.Complete(content)
}
