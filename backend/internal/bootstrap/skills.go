package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	skillapplication "github.com/orz-i/Mender/backend/internal/skill/application"
	skillbuiltin "github.com/orz-i/Mender/backend/internal/skill/builtin"
	skilldomain "github.com/orz-i/Mender/backend/internal/skill/domain"
	skillidentity "github.com/orz-i/Mender/backend/internal/skill/infrastructure/identity"
	skilllocalfs "github.com/orz-i/Mender/backend/internal/skill/infrastructure/localfs"
	skillsqlite "github.com/orz-i/Mender/backend/internal/skill/infrastructure/sqlite"
)

var ErrBuiltInSkillImmutable = errors.New("built-in skill installation is immutable")

const SkillSourceBuiltIn = "builtin"

// SkillInstallation is a management-safe projection of one exact built-in or
// managed Skill installation.
type SkillInstallation struct {
	SkillID         skilldomain.SkillID
	InstallationID  skilldomain.InstallationID
	Name            string
	Description     string
	SourceKind      string
	SourceReference string
	Digest          string
	VersionHint     string
	Trusted         bool
	Enabled         bool
	BuiltIn         bool
	InstalledAt     *time.Time
}

// SkillInspection includes exact validated resource metadata for one Skill
// installation. Reading it never executes Skill scripts.
type SkillInspection struct {
	Installation SkillInstallation
	Metadata     skilldomain.Metadata
	Resources    []skilldomain.Resource
	TotalBytes   int64
}

// Skills composes the built-in inventory with managed local-directory
// installations.
type Skills struct {
	store    *skillsqlite.Store
	packages *skilllocalfs.Store
	builtins []skillbuiltin.Entry
}

// NewSkills composes one Runtime-owned Skill Library.
func NewSkills(
	ctx context.Context,
	databasePath string,
	managedRoot string,
) (*Skills, error) {
	store, err := skillsqlite.Open(ctx, databasePath)
	if err != nil {
		return nil, fmt.Errorf("compose skills: metadata: %w", err)
	}
	packages, err := skilllocalfs.New(managedRoot)
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("compose skills: packages: %w", err)
	}
	builtins, err := skillbuiltin.All()
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("compose skills: built-ins: %w", err)
	}
	return &Skills{
		store:    store,
		packages: packages,
		builtins: builtins,
	}, nil
}

// InstallLocal ingests one local Agent Skill directory. Third-party content is
// untrusted and disabled by default.
func (s *Skills) InstallLocal(
	ctx context.Context,
	sourcePath string,
) (skilldomain.Installation, error) {
	if s == nil {
		return skilldomain.Installation{}, fmt.Errorf("install skill: library is nil")
	}
	return skillapplication.NewInstallLocal(
		s.store,
		s.packages,
		skillidentity.NewGenerator(),
		systemClock{},
	).Execute(ctx, sourcePath)
}

// List returns built-in and managed exact installations in deterministic order.
func (s *Skills) List(ctx context.Context) ([]SkillInstallation, error) {
	if s == nil {
		return nil, fmt.Errorf("list skills: library is nil")
	}
	managed, err := skillapplication.NewListInstallations(s.store).Execute(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]SkillInstallation, 0, len(s.builtins)+len(managed))
	for _, entry := range s.builtins {
		items = append(items, builtInSkillProjection(entry))
	}
	for _, item := range managed {
		items = append(items, managedSkillProjection(item))
	}
	slices.SortFunc(items, func(a, b SkillInstallation) int {
		if comparison := strings.Compare(a.Name, b.Name); comparison != 0 {
			return comparison
		}
		if a.BuiltIn != b.BuiltIn {
			if a.BuiltIn {
				return -1
			}
			return 1
		}
		return strings.Compare(a.InstallationID.String(), b.InstallationID.String())
	})
	return items, nil
}

// Inspect returns exact validated resources for one built-in or managed
// installation.
func (s *Skills) Inspect(
	ctx context.Context,
	id skilldomain.InstallationID,
) (SkillInspection, error) {
	if s == nil {
		return SkillInspection{}, fmt.Errorf("inspect skill: library is nil")
	}
	for _, entry := range s.builtins {
		if entry.InstallationID != id {
			continue
		}
		return SkillInspection{
			Installation: builtInSkillProjection(entry),
			Metadata:     entry.Snapshot.Metadata.Clone(),
			Resources:    append([]skilldomain.Resource(nil), entry.Snapshot.Resources...),
			TotalBytes:   entry.Snapshot.TotalBytes,
		}, nil
	}
	inspection, err := skillapplication.NewInspectInstallation(
		s.store,
		s.packages,
	).Execute(ctx, id)
	if err != nil {
		return SkillInspection{}, err
	}
	return SkillInspection{
		Installation: managedSkillProjection(inspection.Installation),
		Metadata:     inspection.Snapshot.Metadata.Clone(),
		Resources:    append([]skilldomain.Resource(nil), inspection.Snapshot.Resources...),
		TotalBytes:   inspection.Snapshot.TotalBytes,
	}, nil
}

// SetTrust changes trust for a managed installation. Built-ins are immutable.
func (s *Skills) SetTrust(
	ctx context.Context,
	id skilldomain.InstallationID,
	trusted bool,
) (skilldomain.Installation, error) {
	if s == nil {
		return skilldomain.Installation{}, fmt.Errorf("set skill trust: library is nil")
	}
	if s.isBuiltIn(id) {
		return skilldomain.Installation{}, ErrBuiltInSkillImmutable
	}
	return skillapplication.NewSetTrust(s.store).Execute(ctx, id, trusted)
}

// SetEnabled changes enablement for a managed installation. Built-ins are
// enabled by construction and immutable.
func (s *Skills) SetEnabled(
	ctx context.Context,
	id skilldomain.InstallationID,
	enabled bool,
) (skilldomain.Installation, error) {
	if s == nil {
		return skilldomain.Installation{}, fmt.Errorf("set skill enablement: library is nil")
	}
	if s.isBuiltIn(id) {
		return skilldomain.Installation{}, ErrBuiltInSkillImmutable
	}
	return skillapplication.NewSetEnabled(s.store).Execute(ctx, id, enabled)
}

// Remove deletes one managed installation. Built-ins cannot be removed.
func (s *Skills) Remove(
	ctx context.Context,
	id skilldomain.InstallationID,
) error {
	if s == nil {
		return fmt.Errorf("remove skill: library is nil")
	}
	if s.isBuiltIn(id) {
		return ErrBuiltInSkillImmutable
	}
	return skillapplication.NewRemoveInstallation(
		s.store,
		s.packages,
		nil,
	).Execute(ctx, id)
}

func (s *Skills) isBuiltIn(id skilldomain.InstallationID) bool {
	for _, entry := range s.builtins {
		if entry.InstallationID == id {
			return true
		}
	}
	return false
}

// Close releases the Skill metadata store.
func (s *Skills) Close() error {
	if s == nil || s.store == nil {
		return nil
	}
	return s.store.Close()
}

func builtInSkillProjection(entry skillbuiltin.Entry) SkillInstallation {
	return SkillInstallation{
		SkillID:         entry.SkillID,
		InstallationID:  entry.InstallationID,
		Name:            entry.Snapshot.Metadata.Name,
		Description:     entry.Snapshot.Metadata.Description,
		SourceKind:      SkillSourceBuiltIn,
		SourceReference: "skills/" + entry.Snapshot.Metadata.Name,
		Digest:          entry.Snapshot.Digest,
		VersionHint:     entry.Snapshot.Metadata.Values["version"],
		Trusted:         entry.Trusted,
		Enabled:         entry.Enabled,
		BuiltIn:         true,
	}
}

func managedSkillProjection(item skilldomain.Installation) SkillInstallation {
	installedAt := item.InstalledAt()
	return SkillInstallation{
		SkillID:         item.SkillID(),
		InstallationID:  item.ID(),
		Name:            item.Name(),
		Description:     item.Description(),
		SourceKind:      string(item.SourceKind()),
		SourceReference: item.SourceReference(),
		Digest:          item.Digest(),
		VersionHint:     item.VersionHint(),
		Trusted:         item.Trusted(),
		Enabled:         item.Enabled(),
		BuiltIn:         false,
		InstalledAt:     &installedAt,
	}
}
