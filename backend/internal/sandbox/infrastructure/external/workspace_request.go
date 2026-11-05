package external

import (
	"encoding/json"
	"fmt"

	workspace "github.com/orz-i/Mender/backend/internal/workspace/domain"
)

type presentRequestField struct {
	flag  workspace.Fields
	name  string
	value any
}

// The host records source size after alias removal and before typed decoding.
// Re-encoding HTML/Unicode may expand the RPC representation; that must not
// introduce a second, stricter limit than the original v1 argument boundary.
// Direct typed callers without a source measurement use the encoded size.
func workspaceArgumentBytes(in any, encodedBytes int) int {
	var originalBytes int
	switch request := in.(type) {
	case workspace.ReadRequest:
		originalBytes = request.CanonicalBytes
	case workspace.SearchRequest:
		originalBytes = request.CanonicalBytes
	case workspace.EditRequest:
		originalBytes = request.CanonicalBytes
	case workspace.ShellRequest:
		originalBytes = request.CanonicalBytes
	}
	if originalBytes > 0 {
		return originalBytes
	}
	return encodedBytes
}

// marshalWorkspaceRequest preserves both omitted defaults and explicit zero
// values from v1 callers. Presence crosses the application port as typed bits,
// never as raw Tool arguments.
func marshalWorkspaceRequest(in any) ([]byte, error) {
	var fields map[string]json.RawMessage
	var err error
	switch request := in.(type) {
	case workspace.ReadRequest:
		fields, err = workspaceRequestFields(request, request.Present,
			presentRequestField{workspace.FieldOffset, "offset", request.Offset},
			presentRequestField{workspace.FieldLimit, "limit", request.Limit})
	case workspace.SearchRequest:
		fields, err = workspaceRequestFields(request, request.Present,
			presentRequestField{workspace.FieldPath, "path", request.Path},
			presentRequestField{workspace.FieldQuery, "query", request.Query},
			presentRequestField{workspace.FieldPattern, "pattern", request.Pattern},
			presentRequestField{workspace.FieldRegex, "regex", request.Regex},
			presentRequestField{workspace.FieldCaseSensitive, "caseSensitive", request.CaseSensitive},
			presentRequestField{workspace.FieldMaxResults, "maxResults", request.MaxResults})
	case workspace.EditRequest:
		fields, err = workspaceRequestFields(request, request.Present,
			presentRequestField{workspace.FieldContent, "content", request.Content})
		if err == nil && (len(request.Edits) != 0 || request.Present.Has(workspace.FieldEdits)) {
			edits := make([]map[string]json.RawMessage, len(request.Edits))
			for i, edit := range request.Edits {
				edits[i], err = workspaceRequestFields(edit, edit.Present,
					presentRequestField{workspace.FieldExpectedReplacements, "expectedReplacements", edit.ExpectedReplacements})
				if err != nil {
					return nil, err
				}
			}
			fields["edits"], err = json.Marshal(edits)
		}
	case workspace.ShellRequest:
		fields, err = workspaceRequestFields(request, request.Present,
			presentRequestField{workspace.FieldCwd, "cwd", request.Cwd},
			presentRequestField{workspace.FieldTimeoutMS, "timeoutMs", request.TimeoutMS})
	default:
		return nil, fmt.Errorf("unsupported typed workspace request")
	}
	if err != nil {
		return nil, err
	}
	return json.Marshal(fields)
}

func workspaceRequestFields(in any, present workspace.Fields, optional ...presentRequestField) (map[string]json.RawMessage, error) {
	raw, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	for _, field := range optional {
		if !present.Has(field.flag) {
			continue
		}
		fields[field.name], err = json.Marshal(field.value)
		if err != nil {
			return nil, err
		}
	}
	return fields, nil
}
