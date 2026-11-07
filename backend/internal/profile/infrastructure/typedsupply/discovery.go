package typedsupply

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"

	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
)

const maxCursorBytes = 4096
const maxBrowseOffset = 10000000

var ErrStaleCursor = errors.New("typed Profile discovery cursor is stale")

// SourceHeader reveals only identities that survived the exact Profile
// ToolBinding and authority filter in Project(). No Provider/Agent inventory
// query or fallback to a broader Catalog is permitted here.
type SourceHeader struct {
	SourceID   string `json:"sourceId"`
	SourceKind string `json:"sourceKind"`
	SourceName string `json:"sourceName"`
	ToolCount  int    `json:"toolCount"`
}
type BrowseResult struct {
	Sources    []SourceHeader `json:"sources"`
	Tools      []Tool         `json:"tools"`
	NextCursor string         `json:"nextCursor"`
}
type browseCursor struct {
	Version     int    `json:"v"`
	View        string `json:"view"`
	Query       string `json:"query"`
	SourceID    string `json:"sourceId"`
	Limit       int    `json:"limit"`
	Offset      int    `json:"offset"`
	Fingerprint string `json:"fingerprint"`
}

// Fingerprint is the exact Profile-filtered supply revision. Profile and
// delivery revisions alone are not enough: a concurrent Agent/Sandbox grant
// revocation must invalidate an old Cursor even without a Profile write.
func (s Snapshot) Fingerprint() (string, error) {
	data, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

// Source returns only a Profile-visible exact typed source. An unbound but
// syntactically valid SourceRef is indistinguishable from a missing source.
func (s Snapshot) Source(sourceID string) (Source, bool) {
	ref, err := catalog.ParseSourceRef(sourceID)
	if err != nil {
		return Source{}, false
	}
	for _, source := range s.Sources {
		if source.SourceID == ref.String() {
			return cloneSource(source), true
		}
	}
	return Source{}, false
}

// Browse is Profile-scoped, deterministic Source-first discovery, suitable
// for later MCP/HTTP adapters. It has no database access and never widens
// access beyond this immutable, already filtered Snapshot.
func (s Snapshot) Browse(view, sourceID, query, cursor string, limit int) (BrowseResult, error) {
	if view != "sources" && view != "tools" {
		return BrowseResult{}, ErrInvalidSupply
	}
	if limit < 1 || limit > 100 {
		return BrowseResult{}, ErrInvalidSupply
	}
	if len(query) > 1024 {
		return BrowseResult{}, ErrInvalidSupply
	}
	normalized := strings.ToLower(strings.TrimSpace(query))
	if view == "tools" && sourceID == "" && normalized == "" {
		return BrowseResult{}, ErrInvalidSupply
	}
	if sourceID != "" {
		if _, err := catalog.ParseSourceRef(sourceID); err != nil {
			return BrowseResult{}, ErrInvalidSupply
		}
	}
	fingerprint, err := s.Fingerprint()
	if err != nil {
		return BrowseResult{}, err
	}
	offset := 0
	if cursor != "" {
		decoded, err := decodeBrowseCursor(cursor)
		if err != nil {
			return BrowseResult{}, err
		}
		if decoded.View != view || decoded.Query != normalized ||
			decoded.SourceID != sourceID || decoded.Limit != limit ||
			decoded.Fingerprint != fingerprint {
			return BrowseResult{}, ErrStaleCursor
		}
		offset = decoded.Offset
	}
	out := BrowseResult{Sources: []SourceHeader{}, Tools: []Tool{}}
	if view == "sources" {
		for _, source := range s.Sources {
			if sourceID != "" && sourceID != source.SourceID {
				continue
			}
			if normalized != "" && !strings.Contains(strings.ToLower(source.SourceName+" "+source.SourceID), normalized) {
				continue
			}
			out.Sources = append(out.Sources, SourceHeader{
				SourceID: source.SourceID, SourceKind: source.SourceKind,
				SourceName: source.SourceName, ToolCount: len(source.Tools),
			})
		}
	} else {
		for _, source := range s.Sources {
			if sourceID != "" && sourceID != source.SourceID {
				continue
			}
			for _, tool := range source.Tools {
				if normalized != "" && !strings.Contains(strings.ToLower(tool.Name+" "+tool.Title+" "+tool.Description+" "+tool.SourceKey), normalized) {
					continue
				}
				out.Tools = append(out.Tools, tool)
			}
		}
	}
	total := len(out.Sources)
	if view == "tools" {
		total = len(out.Tools)
	}
	if offset > total {
		return BrowseResult{}, ErrStaleCursor
	}
	end := offset + limit
	if end > total {
		end = total
	}
	if view == "sources" {
		out.Sources = append([]SourceHeader{}, out.Sources[offset:end]...)
	} else {
		selected := make([]Tool, 0, end-offset)
		for _, tool := range out.Tools[offset:end] {
			selected = append(selected, cloneTool(tool))
		}
		out.Tools = selected
	}
	if end < total {
		next := browseCursor{Version: 2, View: view, Query: normalized, SourceID: sourceID,
			Limit: limit, Offset: end, Fingerprint: fingerprint}
		raw, err := json.Marshal(next)
		if err != nil {
			return BrowseResult{}, err
		}
		out.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
	}
	return out, nil
}
func cloneTool(tool Tool) Tool {
	tool.Targets = slices.Clone(tool.Targets)
	return tool
}
func cloneSource(source Source) Source {
	tools := make([]Tool, 0, len(source.Tools))
	for _, tool := range source.Tools {
		tools = append(tools, cloneTool(tool))
	}
	source.Tools = tools
	return source
}

func decodeBrowseCursor(raw string) (browseCursor, error) {
	if len(raw) > maxCursorBytes {
		return browseCursor{}, ErrStaleCursor
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return browseCursor{}, ErrStaleCursor
	}
	decoder := json.NewDecoder(strings.NewReader(string(decoded)))
	decoder.DisallowUnknownFields()
	var value browseCursor
	if err := decoder.Decode(&value); err != nil {
		return browseCursor{}, ErrStaleCursor
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return browseCursor{}, ErrStaleCursor
	}
	if value.Version != 2 || value.Limit < 1 || value.Limit > 100 ||
		value.Offset < 1 || value.Offset > maxBrowseOffset || len(value.Fingerprint) != sha256.Size*2 {
		return browseCursor{}, ErrStaleCursor
	}
	if _, err := hex.DecodeString(value.Fingerprint); err != nil {
		return browseCursor{}, ErrStaleCursor
	}
	if value.View != "sources" && value.View != "tools" {
		return browseCursor{}, ErrStaleCursor
	}
	if value.SourceID != "" {
		if _, err := catalog.ParseSourceRef(value.SourceID); err != nil {
			return browseCursor{}, ErrStaleCursor
		}
	}
	if len(value.Query) > 1024 {
		return browseCursor{}, ErrStaleCursor
	}
	return value, nil
}
