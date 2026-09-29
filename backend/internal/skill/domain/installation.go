package domain

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxSourceReferenceBytes = 4096
	maxVersionHintRunes     = 256
)

var (
	digestPattern    = regexp.MustCompile("^sha256:[0-9a-f]{64}$")
	skillNamePattern = regexp.MustCompile("^[a-z0-9]+(?:-[a-z0-9]+)*$")
)

// SourceKind identifies where one managed Skill installation originated.
type SourceKind string

const (
	SourceLocalDirectory SourceKind = "local-directory"
)

// Installation is one exact immutable managed Skill snapshot.
//
// File contents live in the managed filesystem store. This record contains only
// bounded management metadata and administrative trust/enablement state.
type Installation struct {
	id              InstallationID
	skillID         SkillID
	name            string
	description     string
	sourceKind      SourceKind
	sourceReference string
	digest          string
	versionHint     string
	trusted         bool
	enabled         bool
	installedAt     time.Time
}

// NewInstallation constructs a validated managed Skill installation record.
func NewInstallation(
	id InstallationID,
	skillID SkillID,
	name string,
	description string,
	sourceKind SourceKind,
	sourceReference string,
	digest string,
	versionHint string,
	trusted bool,
	enabled bool,
	installedAt time.Time,
) (Installation, error) {
	if _, err := ParseInstallationID(id.String()); err != nil {
		return Installation{}, err
	}
	if _, err := ParseSkillID(skillID.String()); err != nil {
		return Installation{}, err
	}
	if name == "" || len(name) > 64 || !skillNamePattern.MatchString(name) {
		return Installation{}, fmt.Errorf(
			"skill installation name: must be 1-64 lowercase letters, numbers, or hyphen-separated segments",
		)
	}
	if strings.TrimSpace(description) == "" || utf8.RuneCountInString(description) > 1024 {
		return Installation{}, fmt.Errorf(
			"skill installation description: must contain between 1 and 1024 characters",
		)
	}
	if sourceKind != SourceLocalDirectory {
		return Installation{}, fmt.Errorf(
			"skill installation source kind: unsupported value %q",
			sourceKind,
		)
	}
	sourceReference = strings.TrimSpace(sourceReference)
	if sourceReference == "" {
		return Installation{}, fmt.Errorf("skill installation source reference: must not be empty")
	}
	if len(sourceReference) > maxSourceReferenceBytes {
		return Installation{}, fmt.Errorf(
			"skill installation source reference: must be at most %d bytes",
			maxSourceReferenceBytes,
		)
	}
	if !digestPattern.MatchString(digest) {
		return Installation{}, fmt.Errorf("skill installation digest: must be lowercase sha256")
	}
	if utf8.RuneCountInString(versionHint) > maxVersionHintRunes {
		return Installation{}, fmt.Errorf(
			"skill installation version hint: must be at most %d characters",
			maxVersionHintRunes,
		)
	}
	if enabled && !trusted {
		return Installation{}, fmt.Errorf("skill installation state: enabled installation must be trusted")
	}
	if installedAt.IsZero() {
		return Installation{}, fmt.Errorf("skill installation installed at: must not be zero")
	}

	return Installation{
		id:              id,
		skillID:         skillID,
		name:            name,
		description:     description,
		sourceKind:      sourceKind,
		sourceReference: sourceReference,
		digest:          digest,
		versionHint:     versionHint,
		trusted:         trusted,
		enabled:         enabled,
		installedAt:     installedAt.UTC(),
	}, nil
}

// Validate checks persisted installation invariants.
func (i Installation) Validate() error {
	_, err := NewInstallation(
		i.id,
		i.skillID,
		i.name,
		i.description,
		i.sourceKind,
		i.sourceReference,
		i.digest,
		i.versionHint,
		i.trusted,
		i.enabled,
		i.installedAt,
	)
	return err
}

// WithTrusted returns the installation with explicit trust state.
//
// Removing trust also disables the installation so deployment/export cannot
// continue to treat untrusted content as administratively enabled.
func (i Installation) WithTrusted(trusted bool) Installation {
	i.trusted = trusted
	if !trusted {
		i.enabled = false
	}
	return i
}

// WithEnabled returns the installation with explicit enablement state.
func (i Installation) WithEnabled(enabled bool) (Installation, error) {
	if enabled && !i.trusted {
		return Installation{}, fmt.Errorf("skill installation state: trust is required before enabling")
	}
	i.enabled = enabled
	return i, nil
}

func (i Installation) ID() InstallationID      { return i.id }
func (i Installation) SkillID() SkillID        { return i.skillID }
func (i Installation) Name() string            { return i.name }
func (i Installation) Description() string     { return i.description }
func (i Installation) SourceKind() SourceKind  { return i.sourceKind }
func (i Installation) SourceReference() string { return i.sourceReference }
func (i Installation) Digest() string          { return i.digest }
func (i Installation) VersionHint() string     { return i.versionHint }
func (i Installation) Trusted() bool           { return i.trusted }
func (i Installation) Enabled() bool           { return i.enabled }
func (i Installation) InstalledAt() time.Time  { return i.installedAt }
