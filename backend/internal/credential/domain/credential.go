package domain

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

const maxCredentialDisplayNameLength = 160

var environmentVariablePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Source identifies where one Credential's secret material is resolved.
type Source string

const (
	SourceNative      Source = "native"
	SourceEnvironment Source = "environment"
	SourceOAuth       Source = "oauth"
)

// Credential is persistent non-secret metadata for one secret Reference.
type Credential struct {
	reference           Reference
	displayName         string
	source              Source
	environmentVariable string
	oauth               *OAuthMetadata
	createdAt           time.Time
	updatedAt           time.Time
}

func NewCredential(
	reference Reference,
	displayName string,
	source Source,
	environmentVariable string,
	now time.Time,
) (Credential, error) {
	return restoreCredential(
		reference,
		displayName,
		source,
		environmentVariable,
		now,
		now,
	)
}

func RestoreCredential(
	reference Reference,
	displayName string,
	source Source,
	environmentVariable string,
	createdAt time.Time,
	updatedAt time.Time,
) (Credential, error) {
	return restoreCredential(
		reference,
		displayName,
		source,
		environmentVariable,
		createdAt,
		updatedAt,
	)
}

func restoreCredential(
	reference Reference,
	displayName string,
	source Source,
	environmentVariable string,
	createdAt time.Time,
	updatedAt time.Time,
) (Credential, error) {
	if _, err := ParseReference(reference.String()); err != nil {
		return Credential{}, err
	}
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		return Credential{}, fmt.Errorf("credential display_name: must not be empty")
	}
	if len(displayName) > maxCredentialDisplayNameLength {
		return Credential{}, fmt.Errorf("credential display_name: is too long")
	}
	environmentVariable = strings.TrimSpace(environmentVariable)
	switch source {
	case SourceNative:
		if environmentVariable != "" {
			return Credential{}, fmt.Errorf(
				"credential environment_variable: must be empty for native source",
			)
		}
	case SourceOAuth:
		if environmentVariable != "" {
			return Credential{}, fmt.Errorf("credential environment_variable: must be empty for OAuth source")
		}
	case SourceEnvironment:
		if !environmentVariablePattern.MatchString(environmentVariable) {
			return Credential{}, fmt.Errorf(
				"credential environment_variable: invalid variable name",
			)
		}
	default:
		return Credential{}, fmt.Errorf("credential source: unknown value %q", source)
	}
	if createdAt.IsZero() || updatedAt.IsZero() {
		return Credential{}, fmt.Errorf("credential timestamps: must not be zero")
	}
	if updatedAt.Before(createdAt) {
		return Credential{}, fmt.Errorf(
			"credential updated_at: must not be before created_at",
		)
	}

	return Credential{
		reference:           reference,
		displayName:         displayName,
		source:              source,
		environmentVariable: environmentVariable,
		createdAt:           createdAt.UTC(),
		updatedAt:           updatedAt.UTC(),
	}, nil
}

func (c Credential) Touch(now time.Time) (Credential, error) {
	updated, err := RestoreCredential(
		c.reference,
		c.displayName,
		c.source,
		c.environmentVariable,
		c.createdAt,
		now,
	)
	if err != nil {
		return Credential{}, err
	}
	if c.oauth != nil {
		return updated.WithOAuthMetadata(*c.oauth)
	}
	return updated, nil
}

func (c Credential) Reference() Reference {
	return c.reference
}

func (c Credential) DisplayName() string {
	return c.displayName
}

func (c Credential) Source() Source {
	return c.source
}

func (c Credential) EnvironmentVariable() string {
	return c.environmentVariable
}

// OAuthMetadata returns only public connection state; never OAuth bearer or refresh tokens.
func (c Credential) OAuthMetadata() *OAuthMetadata {
	if c.oauth == nil {
		return nil
	}
	clone := *c.oauth
	clone.Scopes = append([]string(nil), clone.Scopes...)
	return &clone
}

func (c Credential) WithOAuthMetadata(meta OAuthMetadata) (Credential, error) {
	if c.source != SourceOAuth {
		return Credential{}, fmt.Errorf("OAuth metadata requires an OAuth credential")
	}
	if err := meta.Validate(); err != nil {
		return Credential{}, err
	}
	copy := meta
	copy.Scopes = append([]string(nil), meta.Scopes...)
	c.oauth = &copy
	return c, nil
}

func (c Credential) CreatedAt() time.Time {
	return c.createdAt
}

func (c Credential) UpdatedAt() time.Time {
	return c.updatedAt
}
