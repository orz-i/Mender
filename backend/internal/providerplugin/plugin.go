package providerplugin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
)

const (
	maxIDLength          = 128
	maxVersionLength     = 128
	maxDisplayNameLength = 160
	maxDescriptionLength = 4096
)

var idPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*(?:\.[a-z][a-z0-9-]*)+$`)

// ID is the stable identity of one Provider plugin implementation.
//
// IDs are namespaced and intentionally are not normalized silently.
type ID string

// ParseID validates and returns a Provider plugin ID.
func ParseID(value string) (ID, error) {
	if value == "" {
		return "", fmt.Errorf("provider plugin id: must not be empty")
	}
	if len(value) > maxIDLength {
		return "", fmt.Errorf("provider plugin id: must be at most %d characters", maxIDLength)
	}
	if value != strings.TrimSpace(value) {
		return "", fmt.Errorf("provider plugin id: must not contain surrounding whitespace")
	}
	if !idPattern.MatchString(value) {
		return "", fmt.Errorf(
			"provider plugin id: must be a lowercase namespaced identifier such as io.mender.provider.mcp",
		)
	}
	return ID(value), nil
}

func (id ID) String() string {
	return string(id)
}

// Descriptor is safe Provider plugin metadata exposed by Runtime/management.
//
// Reading a Descriptor must not activate a Provider or perform network/process
// work.
type Descriptor struct {
	ID            ID
	Version       string
	DisplayName   string
	Description   string
	ConfigVersion int
}

// Validate checks descriptor invariants.
func (d Descriptor) Validate() error {
	if _, err := ParseID(d.ID.String()); err != nil {
		return err
	}
	if value := strings.TrimSpace(d.Version); value == "" {
		return fmt.Errorf("provider plugin version: must not be empty")
	} else if len(value) > maxVersionLength {
		return fmt.Errorf(
			"provider plugin version: must be at most %d characters",
			maxVersionLength,
		)
	}
	if value := strings.TrimSpace(d.DisplayName); value == "" {
		return fmt.Errorf("provider plugin display name: must not be empty")
	} else if len(value) > maxDisplayNameLength {
		return fmt.Errorf(
			"provider plugin display name: must be at most %d characters",
			maxDisplayNameLength,
		)
	}
	if len(strings.TrimSpace(d.Description)) > maxDescriptionLength {
		return fmt.Errorf(
			"provider plugin description: must be at most %d characters",
			maxDescriptionLength,
		)
	}
	if d.ConfigVersion < 1 {
		return fmt.Errorf("provider plugin config version: must be at least 1")
	}
	return nil
}

// OpenRequest contains the Mender-owned capabilities needed to open one
// configured Provider instance.
//
// Config is plugin-owned non-secret persisted configuration. Credentials is a
// capability, not resolved secret material.
type OpenRequest struct {
	ProviderID    catalogdomain.ProviderID
	ConfigVersion int
	Config        json.RawMessage
	Credentials   invocationapplication.CredentialResolver
}

// Plugin validates configuration and opens runtime Provider instances.
type Plugin interface {
	Descriptor() Descriptor
	ValidateConfig(context.Context, json.RawMessage) error
	Open(context.Context, OpenRequest) (Instance, error)
}

// Instance is one active Provider implementation behind canonical Mender ports.
type Instance interface {
	catalogapplication.ToolSource
	invocationapplication.ToolExecutor
	io.Closer
}
