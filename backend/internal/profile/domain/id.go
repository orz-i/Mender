package domain

import (
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

const (
	profileIDPrefix = "prof_"
	profileIDBytes  = 16
	profileIDHexLen = profileIDBytes * 2
	maxSlugLength   = 63
)

var slugPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*[a-z0-9]$|^[a-z]$`)

// ProfileID is Mender's stable opaque identity for a Profile.
type ProfileID string

func ParseProfileID(value string) (ProfileID, error) {
	if !strings.HasPrefix(value, profileIDPrefix) {
		return "", fmt.Errorf("profile_id: invalid prefix")
	}

	encoded := strings.TrimPrefix(value, profileIDPrefix)
	if len(encoded) != profileIDHexLen {
		return "", fmt.Errorf("profile_id: invalid length")
	}
	if encoded != strings.ToLower(encoded) {
		return "", fmt.Errorf("profile_id: must use lowercase hexadecimal encoding")
	}
	if _, err := hex.DecodeString(encoded); err != nil {
		return "", fmt.Errorf("profile_id: must use hexadecimal encoding")
	}

	return ProfileID(value), nil
}

func (id ProfileID) String() string {
	return string(id)
}

// Slug is a stable human-readable Profile identifier.
type Slug string

func NewSlug(value string) (Slug, error) {
	if value == "" {
		return "", fmt.Errorf("slug: must not be empty")
	}
	if len(value) > maxSlugLength {
		return "", fmt.Errorf("slug: must be at most 63 characters")
	}
	if !slugPattern.MatchString(value) {
		return "", fmt.Errorf("slug: must start with a lowercase letter and contain only lowercase letters, digits, or hyphens")
	}
	return Slug(value), nil
}

func (s Slug) String() string {
	return string(s)
}
