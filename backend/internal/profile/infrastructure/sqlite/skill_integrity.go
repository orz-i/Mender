package sqlite

import (
	"context"
	"fmt"
)

// EnsureSkillReferenceGuards is installed after Profile and Skill schemas exist
// in the shared Runtime database. It closes a cross-connection race between
// Skill trust/removal and Profile membership mutations. Built-in Skills are
// immutable and are not rows in skill_installations.
func (s *Store) EnsureSkillReferenceGuards(ctx context.Context) error {
	statements := []string{
		`CREATE TRIGGER IF NOT EXISTS profile_skill_restrict_installation_delete
		BEFORE DELETE ON skill_installations
		WHEN EXISTS(SELECT 1 FROM profile_skills WHERE installation_id = OLD.installation_id)
		BEGIN SELECT RAISE(ABORT, 'skill installation in use by Profile'); END`,
		`CREATE TRIGGER IF NOT EXISTS profile_skill_restrict_installation_disable
		BEFORE UPDATE OF trusted, enabled ON skill_installations
		WHEN (NEW.trusted = 0 OR NEW.enabled = 0)
			AND EXISTS(SELECT 1 FROM profile_skills WHERE installation_id = OLD.installation_id)
		BEGIN SELECT RAISE(ABORT, 'skill installation in use by Profile'); END`,
		`CREATE TRIGGER IF NOT EXISTS profile_skill_restrict_disabled_selection
		BEFORE INSERT ON profile_skills
		WHEN EXISTS(SELECT 1 FROM skill_installations
			WHERE installation_id = NEW.installation_id AND (trusted = 0 OR enabled = 0))
		BEGIN SELECT RAISE(ABORT, 'profile skill installation not deployable'); END`,
	}
	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("install Profile-Skill reference guard: %w", err)
		}
	}
	return nil
}
