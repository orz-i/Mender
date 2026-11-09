package managed

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
)

// profileToolMetadataFingerprint describes the effective immutable Tool
// metadata consumed by a Profile projection. The Profile revision separately
// fences aliases and ToolBinding membership; do not hash mutable timestamps,
// so a no-op synchronization cannot restart unrelated MCP Provider adapters.
//
// CatalogRevision is checked before and after this bounded read by the caller.
func (r *Manager) fingerprint(ctx context.Context, profile Snapshot) (string, error) {
	ids := slices.Clone(profile.ToolIDs())
	slices.SortFunc(ids, func(a, b catalogdomain.ToolID) int {
		if a.String() < b.String() {
			return -1
		}
		if a.String() > b.String() {
			return 1
		}
		return 0
	})
	hasher := sha256.New()
	for _, id := range ids {
		tool, err := r.deps.DescribeTool(ctx, id)
		if err != nil {
			return "", fmt.Errorf("fingerprint Profile Tool %s: %w", id, err)
		}
		if tool.ID() != id || tool.SourceRef().Validate() != nil {
			return "", fmt.Errorf("fingerprint Profile Tool %s: invalid canonical source identity", id)
		}
		var output []byte
		if schema := tool.OutputSchema(); schema != nil {
			output = schema.Bytes()
		}
		// JSON encoding provides unambiguous field framing and stable map-free
		// ordering; tool input/output schemas are immutable canonical byte slices.
		fields := struct {
			ToolID      string   `json:"toolId"`
			SourceRef   string   `json:"sourceId"`
			ProviderID  string   `json:"providerId,omitempty"`
			SourceKey   string   `json:"sourceKey"`
			Name        string   `json:"name"`
			Title       string   `json:"title"`
			Description string   `json:"description"`
			Input       []byte   `json:"input"`
			Output      []byte   `json:"output"`
			Tags        []string `json:"tags"`
		}{
			ToolID: id.String(), SourceRef: tool.SourceRef().String(),
			SourceKey: tool.SourceKey(), Name: tool.Name(), Title: tool.Title(),
			Description: tool.Description(), Input: tool.InputSchema().Bytes(),
			Output: output, Tags: tool.Tags(),
		}
		if tool.SourceRef().Kind() == catalogdomain.SourceProvider {
			fields.ProviderID = tool.ProviderID().String()
		}
		blob, err := json.Marshal(fields)
		if err != nil {
			return "", fmt.Errorf("fingerprint Profile Tool metadata %s: %w", id, err)
		}
		if _, err = hasher.Write(blob); err != nil {
			return "", err
		}
		if _, err = hasher.Write([]byte{0}); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}
