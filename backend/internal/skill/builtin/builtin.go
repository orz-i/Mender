package builtin

import (
	"embed"
	"fmt"
	"strings"

	skilldomain "github.com/orz-i/Mender/backend/internal/skill/domain"
	"github.com/orz-i/Mender/backend/internal/skill/infrastructure/agentskill"
)

const (
	// MenderToolsSkillID is the stable logical identity of Mender's built-in
	// progressive-disclosure CLI Skill.
	MenderToolsSkillID skilldomain.SkillID = "skill_c3e37b8fb5b18a1c1f97cb2f8fb4cdb1"

	SourceKind = "builtin"
)

// content is a release-safe copy of repository Skill sources. Tests require
// parity with /skills/mender-tools so the repository copy remains authoritative.
//
//go:embed data/mender-tools
var content embed.FS

// Entry is one exact statically linked built-in Skill snapshot.
type Entry struct {
	SkillID        skilldomain.SkillID
	InstallationID skilldomain.InstallationID
	SourceKind     string
	Trusted        bool
	Enabled        bool
	Snapshot       skilldomain.Snapshot
}

// Clone returns a deep copy safe for management/application callers.
func (e Entry) Clone() Entry {
	e.Snapshot = e.Snapshot.Clone()
	return e
}

// All returns the complete statically linked built-in Skill set.
func All() ([]Entry, error) {
	menderTools, err := loadMenderTools()
	if err != nil {
		return nil, err
	}
	return []Entry{menderTools}, nil
}

func loadMenderTools() (Entry, error) {
	snapshot, err := agentskill.LoadFS(content, "data/mender-tools")
	if err != nil {
		return Entry{}, fmt.Errorf("load built-in mender-tools skill: %w", err)
	}
	installationID, err := installationIDForDigest(snapshot.Digest)
	if err != nil {
		return Entry{}, fmt.Errorf("load built-in mender-tools skill: %w", err)
	}
	return Entry{
		SkillID:        MenderToolsSkillID,
		InstallationID: installationID,
		SourceKind:     SourceKind,
		Trusted:        true,
		Enabled:        true,
		Snapshot:       snapshot,
	}, nil
}

func installationIDForDigest(digest string) (skilldomain.InstallationID, error) {
	const prefix = "sha256:"
	if !strings.HasPrefix(digest, prefix) {
		return "", fmt.Errorf("skill digest must use sha256")
	}
	encoded := strings.TrimPrefix(digest, prefix)
	if len(encoded) < 32 {
		return "", fmt.Errorf("skill digest is too short")
	}
	return skilldomain.ParseInstallationID("sinst_" + encoded[:32])
}
