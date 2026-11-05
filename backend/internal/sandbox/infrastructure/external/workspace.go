package external

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	workspaceapp "github.com/orz-i/Mender/backend/internal/workspace/application"
	workspace "github.com/orz-i/Mender/backend/internal/workspace/domain"
	sandboxv1 "github.com/orz-i/Mender/sdk/sandboxplugin/go/protocol/v1"
)

const maxWorkspaceArguments = 1024 * 1024

type Session struct {
	service      sandboxv1.SandboxPluginServiceClient
	id           string
	capabilities sandboxdomain.Capabilities
}

func (s *Session) Read(ctx context.Context, in workspace.ReadRequest) (workspace.ReadResult, error) {
	raw, err := s.executeWorkspace(ctx, sandboxdomain.CapabilityRead, in)
	if err != nil {
		return workspace.ReadResult{}, err
	}
	fields, err := resultObject(raw, "path", "content", "startLine", "endLine", "truncated")
	if err == nil {
		err = normalizeResultIntegers(fields, "startLine", "endLine")
	}
	var result workspace.ReadResult
	if err == nil {
		err = decodeResultObject(fields, &result)
	}
	if err != nil || result.StartLine < 0 || result.EndLine < 0 {
		return workspace.ReadResult{}, invalidWorkspaceResult(raw)
	}
	result.CanonicalBytes = len(bytes.TrimSpace(raw))
	return result, nil
}

func (s *Session) Search(ctx context.Context, in workspace.SearchRequest) (workspace.SearchResult, error) {
	raw, err := s.executeWorkspace(ctx, sandboxdomain.CapabilitySearch, in)
	if err != nil {
		return workspace.SearchResult{}, err
	}
	fields, err := resultObject(raw, "mode", "matches", "truncated")
	if err == nil {
		err = normalizeSearchMatches(fields)
	}
	var result workspace.SearchResult
	if err == nil {
		err = decodeResultObject(fields, &result)
	}
	if err != nil || (result.Mode != "files" && result.Mode != "content") {
		return workspace.SearchResult{}, invalidWorkspaceResult(raw)
	}
	result.CanonicalBytes = len(bytes.TrimSpace(raw))
	return result, nil
}

func (s *Session) Edit(ctx context.Context, in workspace.EditRequest) (workspace.EditResult, error) {
	raw, err := s.executeWorkspace(ctx, sandboxdomain.CapabilityEdit, in)
	if err != nil {
		return workspace.EditResult{}, err
	}
	fields, err := resultObject(raw, "path", "operation", "replacements", "bytesBefore", "bytesAfter")
	if err == nil {
		err = normalizeResultIntegers(fields, "replacements", "bytesBefore", "bytesAfter")
	}
	var result workspace.EditResult
	if err == nil {
		err = decodeResultObject(fields, &result)
	}
	if err != nil || (result.Operation != "create" && result.Operation != "replace" && result.Operation != "delete") ||
		result.Replacements < 0 || result.BytesBefore < 0 || result.BytesAfter < 0 {
		return workspace.EditResult{}, invalidWorkspaceResult(raw)
	}
	result.CanonicalBytes = len(bytes.TrimSpace(raw))
	return result, nil
}

func (s *Session) Shell(ctx context.Context, in workspace.ShellRequest) (workspace.ShellResult, error) {
	// The public v1 capability remains process.shell, independent of the
	// Workspace application's operation name.
	raw, err := s.executeWorkspace(ctx, sandboxdomain.CapabilityShell, in)
	if err != nil {
		return workspace.ShellResult{}, err
	}
	fields, err := resultObject(raw, "exitCode", "stdout", "stderr", "timedOut", "truncated")
	if err == nil {
		err = normalizeResultIntegers(fields, "exitCode")
	}
	var result workspace.ShellResult
	if err == nil {
		err = decodeResultObject(fields, &result)
	}
	if err != nil || result.ExitCode < -1 {
		return workspace.ShellResult{}, invalidWorkspaceResult(raw)
	}
	result.CanonicalBytes = len(bytes.TrimSpace(raw))
	return result, nil
}

func (s *Session) executeWorkspace(ctx context.Context, capability sandboxdomain.Capability, in any) ([]byte, error) {
	if s == nil || s.service == nil {
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	if !s.capabilities.Supports(capability) {
		return nil, sandboxdomain.ErrCapabilityUnavailable
	}
	// Do not normalize defaults here: omitted fields are part of the v1 wire
	// contract and remain the external backend's responsibility.
	arguments, err := marshalWorkspaceRequest(in)
	if err != nil {
		return nil, fmt.Errorf("sandbox plugin encode workspace arguments: %w", err)
	}
	if workspaceArgumentBytes(in, len(arguments)) > maxWorkspaceArguments {
		return nil, fmt.Errorf("sandbox plugin arguments exceed limit")
	}
	res, err := s.service.ExecuteWorkspace(ctx, &sandboxv1.ExecuteWorkspaceRequest{
		SessionId: s.id, Capability: string(capability), ArgumentsJson: arguments,
	})
	if err != nil {
		return nil, fmt.Errorf("sandbox plugin workspace: %w", err)
	}
	raw := res.GetContentJson()
	if len(raw) > maxResultJSON {
		return nil, fmt.Errorf("sandbox plugin result too large")
	}
	// Match the previous JSONValue boundary: the wire cap precedes trimming,
	// and parsing accepts the same Unicode whitespace at either edge.
	raw = bytes.TrimSpace(raw)
	// Keep malformed JSON (including numbers outside JSONValue's float64
	// parsing range) as the original execution error, not a schema failure.
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("sandbox plugin returned invalid JSON: %w", err)
	}
	return raw, nil
}

// resultObject enforces the existing Tool output schema before decoding can
// erase missing/null fields or unknown properties into typed zero values.
func resultObject(raw []byte, required ...string) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	if len(fields) != len(required) {
		return nil, invalidWorkspaceResult()
	}
	for _, name := range required {
		value, ok := fields[name]
		if !ok || string(value) == "null" {
			return nil, invalidWorkspaceResult()
		}
	}
	return fields, nil
}

func normalizeSearchMatches(fields map[string]json.RawMessage) error {
	var matches []json.RawMessage
	if err := json.Unmarshal(fields["matches"], &matches); err != nil {
		return err
	}
	for i, raw := range matches {
		var match map[string]json.RawMessage
		if err := json.Unmarshal(raw, &match); err != nil {
			return err
		}
		if _, ok := match["path"]; !ok {
			return invalidWorkspaceResult()
		}
		for key, value := range match {
			if (key != "path" && key != "line" && key != "text") || string(value) == "null" {
				return invalidWorkspaceResult()
			}
		}
		if _, ok := match["line"]; ok {
			if err := normalizeResultIntegers(match, "line"); err != nil {
				return err
			}
			var line int
			if err := json.Unmarshal(match["line"], &line); err != nil || line < 1 {
				return invalidWorkspaceResult()
			}
		}
		encoded, err := json.Marshal(match)
		if err != nil {
			return err
		}
		matches[i] = encoded
	}
	encoded, err := json.Marshal(matches)
	if err != nil {
		return err
	}
	fields["matches"] = encoded
	return nil
}

// JSON Schema integers include decimal/exponent representations with an
// integral value. Preserve those valid v1 results while checking typed bounds.
func normalizeResultIntegers(fields map[string]json.RawMessage, names ...string) error {
	for _, name := range names {
		number, ok := resultInteger(string(fields[name]))
		if !ok {
			return invalidWorkspaceResult()
		}
		fields[name] = json.RawMessage(strconv.FormatInt(number, 10))
	}
	return nil
}

// resultInteger retains the existing JSON Schema validator's float64 integer
// semantics, while preserving exact int64 counts when they are representable.
func resultInteger(raw string) (int64, bool) {
	if value, ok := exactResultInteger(raw); ok {
		return value, true
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.Trunc(value) != value || value <= -0x1p63 || value >= 0x1p63 {
		return 0, false
	}
	return int64(value), true
}

// exactResultInteger handles validated JSON numbers in linear space/time. Exponents
// never drive allocation: only a representable integer's final <=19 digits are
// assembled, so even zero with an enormous exponent remains cheap.
func exactResultInteger(raw string) (int64, bool) {
	negative := strings.HasPrefix(raw, "-")
	if negative {
		raw = raw[1:]
	}
	mantissa, exponentText := raw, ""
	if index := strings.IndexAny(raw, "eE"); index >= 0 {
		mantissa, exponentText = raw[:index], raw[index+1:]
	}
	fractionalDigits := 0
	if index := strings.IndexByte(mantissa, '.'); index >= 0 {
		fractionalDigits = len(mantissa) - index - 1
	}
	digits := strings.TrimLeft(strings.ReplaceAll(mantissa, ".", ""), "0")
	if digits == "" {
		return 0, true
	}
	significant := strings.TrimRight(digits, "0")
	if len(significant) > 19 {
		return 0, false
	}
	var exponent int64
	if exponentText != "" {
		var err error
		exponent, err = strconv.ParseInt(exponentText, 10, 64)
		if err != nil {
			return 0, false
		}
	}
	adjustment := int64(len(digits) - len(significant) - fractionalDigits)
	// Bounds precede addition, including when exponent is Min/MaxInt64.
	if exponent < -adjustment || exponent > 19-int64(len(significant))-adjustment {
		return 0, false
	}
	canonical := significant + strings.Repeat("0", int(exponent+adjustment))
	if negative {
		canonical = "-" + canonical
	}
	value, err := strconv.ParseInt(canonical, 10, 64)
	return value, err == nil
}

func decodeResultObject(fields map[string]json.RawMessage, result any) error {
	raw, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, result)
}

func invalidWorkspaceResult(raw ...[]byte) error {
	invalid := &workspace.InvalidResultError{Message: "sandbox plugin returned invalid workspace result"}
	if len(raw) != 0 {
		invalid.CanonicalBytes = len(bytes.TrimSpace(raw[0]))
	}
	return invalid
}

func (s *Session) Close() error {
	if s == nil || s.service == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), closeTimeout)
	defer cancel()
	_, err := s.service.CloseSession(ctx, &sandboxv1.CloseSessionRequest{SessionId: s.id})
	return err
}

var _ workspaceapp.Session = (*Session)(nil)
