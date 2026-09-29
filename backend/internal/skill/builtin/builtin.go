package builtin

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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

// Materialize copies one exact built-in Skill into a deployment
// destination. The copy is user-writable and never mutates embedded content.
func Materialize(
	ctx context.Context,
	id skilldomain.InstallationID,
	destination string,
) error {
	entries, err := All()
	if err != nil {
		return err
	}
	var entry *Entry
	for index := range entries {
		if entries[index].InstallationID == id {
			entry = &entries[index]
			break
		}
	}
	if entry == nil {
		return fmt.Errorf("materialize built-in skill: installation %s not found", id)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	destination = filepath.Clean(strings.TrimSpace(destination))
	if destination == "." || !filepath.IsAbs(destination) {
		return fmt.Errorf("materialize built-in skill: destination must be absolute")
	}
	if err := os.Mkdir(destination, 0o700); err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("materialize built-in skill: destination already exists: %s", destination)
		}
		return fmt.Errorf("materialize built-in skill: create destination: %w", err)
	}
	complete := false
	defer func() {
		if !complete {
			_ = os.RemoveAll(destination)
		}
	}()

	root, err := os.OpenRoot(destination)
	if err != nil {
		return fmt.Errorf("materialize built-in skill: open destination: %w", err)
	}
	defer func() { _ = root.Close() }()

	sourceRoot := filepath.ToSlash(filepath.Join("data", entry.Snapshot.Metadata.Name))
	for _, resource := range entry.Snapshot.Resources {
		if err := ctx.Err(); err != nil {
			return err
		}
		raw, err := fs.ReadFile(
			content,
			sourceRoot+"/"+filepath.ToSlash(resource.Path),
		)
		if err != nil {
			return fmt.Errorf(
				"materialize built-in skill: read resource %q: %w",
				resource.Path,
				err,
			)
		}
		relative := filepath.FromSlash(resource.Path)
		parent := filepath.Dir(relative)
		if parent != "." {
			//nolint:gosec // Deployment directories are user-private and need owner execute.
			if err := root.MkdirAll(parent, 0o700); err != nil {
				return fmt.Errorf(
					"materialize built-in skill: create parent %q: %w",
					parent,
					err,
				)
			}
		}
		mode := os.FileMode(0o600)
		if resource.Executable {
			mode = 0o700
		}
		file, err := root.OpenFile(
			relative,
			os.O_WRONLY|os.O_CREATE|os.O_EXCL,
			mode,
		)
		if err != nil {
			return fmt.Errorf(
				"materialize built-in skill: create resource %q: %w",
				resource.Path,
				err,
			)
		}
		if _, err := file.Write(raw); err != nil {
			_ = file.Close()
			return fmt.Errorf(
				"materialize built-in skill: write resource %q: %w",
				resource.Path,
				err,
			)
		}
		if err := file.Close(); err != nil {
			return fmt.Errorf(
				"materialize built-in skill: close resource %q: %w",
				resource.Path,
				err,
			)
		}
	}

	deployed, err := agentskill.LoadDirectory(destination)
	if err != nil {
		return fmt.Errorf("materialize built-in skill: verify destination: %w", err)
	}
	if deployed.Digest != entry.Snapshot.Digest {
		return fmt.Errorf("materialize built-in skill: destination digest mismatch")
	}
	complete = true
	return nil
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
