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

const maxPendingPermissions = 2

type permissionLocation struct {
	Path string `json:"path"`
}

type wirePermissionRequest struct {
	SessionID string `json:"sessionId"`
	ToolCall  struct {
		ToolCallID string               `json:"toolCallId"`
		Kind       string               `json:"kind"`
		Locations  []permissionLocation `json:"locations"`
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
	result.Target, result.CanAllow = reviewACPFileTarget(workspaceRoot, value.ToolCall.Kind, value.ToolCall.Locations)
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
	if s.pendingPermissions.Load() >= maxPendingPermissions {
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
				if !req.CanAllow {
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
		if writeErr := s.send(map[string]any{"jsonrpc": "2.0", "id": msg.ID,
			"result": map[string]any{"outcome": outcome}}); writeErr != nil {
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
func reviewACPFileTarget(root, kind string, locations []permissionLocation) (string, bool) {
	// The first acceptance gate only permits a previewable read. Editing requires
	// a bounded, reviewable patch diff AND enforced execution placement later.
	if kind != "read" || len(locations) != 1 || !filepath.IsAbs(root) {
		return "", false
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil || canonicalRoot != root {
		return "", false
	}
	proposed := locations[0].Path
	if proposed == "" || len(proposed) > 4096 || strings.ContainsAny(proposed, "\r\n\x00") {
		return "", false
	}
	if !filepath.IsAbs(proposed) {
		proposed = filepath.Join(root, proposed)
	}
	proposed, err = filepath.Abs(proposed)
	if err != nil {
		return "", false
	}
	if kind == "read" {
		proposed, err = filepath.EvalSymlinks(proposed)
		if err != nil {
			return "", false
		}
		info, err := os.Stat(proposed)
		if err != nil || !info.Mode().IsRegular() {
			return "", false
		}
	} else {
		parent, err := filepath.EvalSymlinks(filepath.Dir(proposed))
		if err != nil {
			return "", false
		}
		proposed = filepath.Join(parent, filepath.Base(proposed))
		if _, err := os.Lstat(proposed); err == nil {
			proposed, err = filepath.EvalSymlinks(proposed)
			if err != nil {
				return "", false
			}
		} else if !os.IsNotExist(err) {
			return "", false
		}
	}
	relative, err := filepath.Rel(root, proposed)
	if err != nil || relative == "." || relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) ||
		filepath.IsAbs(relative) || len(relative) > 240 ||
		strings.ContainsAny(relative, "\r\n\x00") {
		return "", false
	}
	return relative, true
}
