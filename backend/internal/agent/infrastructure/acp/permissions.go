package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	agentapplication "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

const (
	maxPendingPermissions        = 2
	maxPermissionRequestsPerTask = 64
)

type permissionLocation struct {
	Path string `json:"path"`
}

type wirePermissionRequest struct {
	SessionID string `json:"sessionId"`
	ToolCall  struct {
		ToolCallID string               `json:"toolCallId"`
		Kind       string               `json:"kind"`
		Locations  []permissionLocation `json:"locations"`
		RawInput   struct {
			Path string `json:"path"`
		} `json:"rawInput"`
	} `json:"toolCall"`
	Options []struct {
		OptionID string `json:"optionId"`
		Kind     string `json:"kind"`
	} `json:"options"`
}

func parseACPApproval(raw json.RawMessage, sessionID, workspaceRoot string) (agentapplication.PermissionRequest, error) {
	var value wirePermissionRequest
	if len(raw) == 0 || len(raw) > 32*1024 || json.Unmarshal(raw, &value) != nil {
		return agentapplication.PermissionRequest{}, agentdomain.ErrInvalid
	}
	if value.SessionID != sessionID || sessionID == "" || len(value.SessionID) > 512 ||
		len(value.ToolCall.ToolCallID) < 1 || len(value.ToolCall.ToolCallID) > 128 ||
		len(value.Options) < 2 || len(value.Options) > 8 {
		return agentapplication.PermissionRequest{}, agentdomain.ErrForbidden
	}
	result := agentapplication.PermissionRequest{
		SessionID: value.SessionID, ToolCallID: value.ToolCall.ToolCallID, Kind: value.ToolCall.Kind,
	}
	result.Target, result.CanAllow = reviewACPFileTarget(workspaceRoot, value.ToolCall.Kind, value.ToolCall.Locations, value.ToolCall.RawInput.Path)
	seen := map[string]bool{}
	for _, option := range value.Options {
		if option.OptionID == "" || len(option.OptionID) > 128 ||
			strings.ContainsAny(option.OptionID, "\r\n\x00") || seen[option.OptionID] {
			return agentapplication.PermissionRequest{}, agentdomain.ErrInvalid
		}
		seen[option.OptionID] = true
		switch option.Kind {
		case "allow_once":
			if result.AllowOptionID != "" {
				return agentapplication.PermissionRequest{}, agentdomain.ErrInvalid
			}
			result.AllowOptionID = option.OptionID
		case "reject_once":
			if result.RejectOptionID != "" {
				return agentapplication.PermissionRequest{}, agentdomain.ErrInvalid
			}
			result.RejectOptionID = option.OptionID
			// "allow_always" and other kinds are NEVER considered an approval.
		}
	}
	if result.AllowOptionID == "" || result.RejectOptionID == "" {
		return agentapplication.PermissionRequest{}, agentdomain.ErrForbidden
	}
	return result, nil
}

// dispatchPermission never waits for a GUI response inside the receive loop.
// Multiple requests are bounded, replayed RPC IDs/toolCall IDs are rejected,
// and the response always echoes the *originally advertised* option ID.
func (s *stream) dispatchPermission(ctx context.Context, msg rpcMessage) error {
	if s.permissionRequester == nil {
		return errClientPermissionDenied
	}
	req, err := parseACPApproval(msg.Params, s.permissionSession, s.permissionRoot)
	if err != nil {
		return fmt.Errorf("%w: invalid ACP approval request", errClientPermissionDenied)
	}
	if len(msg.ID) > 64 || len(msg.ID) == 0 {
		return errClientPermissionDenied
	}
	requestID := string(msg.ID)
	if _, exists := s.seenRequests["rpc:"+requestID]; exists {
		return errClientPermissionDenied
	}
	if _, exists := s.seenRequests["tool:"+req.ToolCallID]; exists {
		return errClientPermissionDenied
	}
	if s.pendingPermissions.Load() >= maxPendingPermissions ||
		len(s.seenRequests) >= 2*maxPermissionRequestsPerTask {
		return errClientPermissionDenied
	}
	s.seenRequests["rpc:"+requestID] = struct{}{}
	s.seenRequests["tool:"+req.ToolCallID] = struct{}{}
	s.pendingPermissions.Add(1)
	go func() {
		defer s.pendingPermissions.Add(-1)
		decision, err := s.permissionRequester(ctx, req)
		if ctx.Err() != nil {
			return
		}
		outcome := map[string]any{"outcome": "cancelled"}
		if err == nil {
			optionID := ""
			switch decision.Choice {
			case agentdomain.ApprovalAllowOnce:
				if !req.CanAllow && !decision.AuthorizedByPolicy {
					err = agentdomain.ErrForbidden
					break
				}
				optionID = req.AllowOptionID
			case agentdomain.ApprovalReject:
				optionID = req.RejectOptionID
			default:
				err = agentdomain.ErrForbidden
			}
			if optionID != "" {
				outcome = map[string]any{"outcome": "selected", "optionId": optionID}
			}
		}
		if writeErr := s.sendPermission(ctx, map[string]any{"jsonrpc": "2.0", "id": msg.ID,
			"result": map[string]any{"outcome": outcome}}); writeErr != nil {
			// Cancellation while waiting for the ACP write lock is handled
			// by the Task context; don't turn it into an unrelated
			// client-permission failure after the Task was cancelled.
			if ctx.Err() != nil {
				return
			}
			err = errors.Join(err, fmt.Errorf("write ACP approval result: %w", writeErr))
		}
		if err != nil {
			select {
			case s.permissionErrors <- fmt.Errorf("%w: Agent permission was not authorized", errClientPermissionDenied):
			default:
			}
		}
	}()
	return nil
}

// reviewACPFileTarget is a bounded, conservative preview for a cooperative
// ACP decision. A reported location is NOT a trustworthy execution parameter:
// strong enforcement still requires an actual sandbox or Mender Tool route.
func reviewACPFileTarget(root, kind string, locations []permissionLocation, inputPath string) (string, bool) {
	// Only an explicitly reviewable single-file read may be approved. Edits
	// need an independently reviewed diff and an enforceable executor.
	// Agent-provided locations and rawInput are both UNTRUSTED; requiring
	// them to agree prevents an obvious contradictory preview. This is
	// cooperative host approval, NOT a mandatory execution sandbox.
	if kind != "read" || len(locations) != 1 || !filepath.IsAbs(root) ||
		inputPath == "" || len(inputPath) > 4096 || strings.ContainsAny(inputPath, "\r\n\x00") {
		return "", false
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil || canonicalRoot != root {
		return "", false
	}
	resolve := func(raw string) (string, string, bool) {
		if raw == "" || len(raw) > 4096 || strings.ContainsAny(raw, "\r\n\x00") {
			return "", "", false
		}
		if !filepath.IsAbs(raw) {
			raw = filepath.Join(root, raw)
		}
		raw = filepath.Clean(raw)
		relative, err := filepath.Rel(root, raw)
		// Do not stat a lexically external file just to draw a review preview.
		if err != nil || relative == "." || relative == ".." ||
			strings.HasPrefix(relative, ".."+string(filepath.Separator)) ||
			filepath.IsAbs(relative) || len(relative) > 240 {
			return "", "", false
		}
		canonical, err := filepath.EvalSymlinks(raw)
		if err != nil {
			return "", "", false
		}
		within, err := filepath.Rel(root, canonical)
		if err != nil || within == "." || within == ".." ||
			strings.HasPrefix(within, ".."+string(filepath.Separator)) ||
			filepath.IsAbs(within) {
			return "", "", false
		}
		info, err := os.Stat(canonical)
		if err != nil || !info.Mode().IsRegular() {
			return "", "", false
		}
		return relative, canonical, true
	}
	relative, canonical, ok := resolve(inputPath)
	if !ok {
		return "", false
	}
	_, reported, ok := resolve(locations[0].Path)
	if !ok || reported != canonical {
		return "", false
	}
	// Persist the lexical path: at decision time Mender revalidates this
	// exact target, including any symlinks that may have changed since ask.
	return relative, true
}
