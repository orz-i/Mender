package domain

import (
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

const (
	sandboxIDPrefix     = "sbx_"
	sandboxIDHexLength  = 32
	maxDisplayNameBytes = 160
	maxOwnerRefBytes    = 255
	maxBackendRefBytes  = 256
)

var (
	ErrNotFound              = errors.New("sandbox not found")
	ErrConflict              = errors.New("sandbox state conflict")
	ErrNotAuthorized         = errors.New("sandbox principal not authorized")
	ErrCapabilityUnavailable = errors.New("sandbox capability unavailable")
	ErrBackendUnavailable    = errors.New("sandbox backend unavailable")
)

// ID identifies a persistent Sandbox resource, independent of its backend run ID.
type ID string

func ParseID(raw string) (ID, error) {
	if !strings.HasPrefix(raw, sandboxIDPrefix) || len(raw) != len(sandboxIDPrefix)+sandboxIDHexLength {
		return "", fmt.Errorf("sandbox id: expected sbx_ and 32 hexadecimal characters")
	}
	encoded := strings.TrimPrefix(raw, sandboxIDPrefix)
	if encoded != strings.ToLower(encoded) {
		return "", fmt.Errorf("sandbox id: encoding must be lowercase")
	}
	if _, err := hex.DecodeString(encoded); err != nil {
		return "", fmt.Errorf("sandbox id: invalid hex: %w", err)
	}
	return ID(raw), nil
}
func (id ID) String() string { return string(id) }

// BackendKind is a host-recognized driver identity, not a Provider plugin ID.
type BackendKind string

const (
	BackendLocalWorkspace BackendKind = "local_workspace"
	BackendDocker         BackendKind = "docker"
	BackendDaytona        BackendKind = "daytona"
	BackendE2B            BackendKind = "e2b"
	BackendExternal       BackendKind = "external"
)

func (k BackendKind) Validate() error {
	switch k {
	case BackendLocalWorkspace, BackendDocker, BackendDaytona, BackendE2B, BackendExternal:
		return nil
	}
	return fmt.Errorf("sandbox backend: unsupported kind %q", k)
}

// IsolationClass is a truthful coarse label; it is not proof of security.
type IsolationClass string

const (
	IsolationHost           IsolationClass = "host"
	IsolationRestrictedHost IsolationClass = "restricted_host"
	IsolationContainer      IsolationClass = "container"
	IsolationRemote         IsolationClass = "remote"
)

func (i IsolationClass) Validate() error {
	switch i {
	case IsolationHost, IsolationRestrictedHost, IsolationContainer, IsolationRemote:
		return nil
	}
	return fmt.Errorf("sandbox isolation: unsupported class %q", i)
}

// Capability is an operation a backend actually implements without host fallbacks.
type Capability string

const (
	CapabilityRead   Capability = "file.read"
	CapabilitySearch Capability = "file.search"
	CapabilityEdit   Capability = "file.edit"
	CapabilityShell  Capability = "process.shell"
)

func (c Capability) Validate() error {
	switch c {
	case CapabilityRead, CapabilitySearch, CapabilityEdit, CapabilityShell:
		return nil
	}
	return fmt.Errorf("sandbox capability: unknown %q", c)
}

type Capabilities struct {
	Read   bool
	Search bool
	Edit   bool
	Shell  bool
}

func (c Capabilities) Supports(op Capability) bool {
	switch op {
	case CapabilityRead:
		return c.Read
	case CapabilitySearch:
		return c.Search
	case CapabilityEdit:
		return c.Edit
	case CapabilityShell:
		return c.Shell
	}
	return false
}

// Spec is a desired-state configuration containing no secrets. A local root is
// only valid for local_workspace, and cannot be supplied as a remote host mount.
type Spec struct {
	ID        ID
	Name      string
	Backend   BackendKind
	Binding   BackendBinding
	Isolation IsolationClass
	LocalRoot string
	OwnerKind string
	OwnerRef  string
}

type Sandbox struct{ spec Spec }

func NewSandbox(spec Spec) (Sandbox, error) {
	if _, err := ParseID(spec.ID.String()); err != nil {
		return Sandbox{}, err
	}
	if spec.Name == "" || spec.Name != strings.TrimSpace(spec.Name) || len(spec.Name) > maxDisplayNameBytes {
		return Sandbox{}, fmt.Errorf("sandbox name: invalid or too long")
	}
	if err := spec.Backend.Validate(); err != nil {
		return Sandbox{}, err
	}
	if err := spec.Binding.Validate(); err != nil {
		return Sandbox{}, err
	}
	if spec.Backend == BackendExternal {
		if spec.Binding.InstallationID == "" || spec.Binding.ExtensionID != "main" ||
			strings.HasPrefix(spec.Binding.PluginID.String(), "io.mender.") {
			return Sandbox{}, fmt.Errorf("external sandbox requires an exact third-party plugin installation")
		}
	} else if err := RequireBuiltinBinding(spec.Backend, spec.Binding); err != nil {
		return Sandbox{}, err
	}
	if err := spec.Isolation.Validate(); err != nil {
		return Sandbox{}, err
	}
	if spec.OwnerKind != "local_user" && spec.OwnerKind != "service" {
		return Sandbox{}, fmt.Errorf("sandbox owner kind: invalid")
	}
	if spec.OwnerRef == "" || spec.OwnerRef != strings.TrimSpace(spec.OwnerRef) || len(spec.OwnerRef) > maxOwnerRefBytes {
		return Sandbox{}, fmt.Errorf("sandbox owner reference: invalid")
	}
	switch spec.Backend {
	case BackendLocalWorkspace:
		if spec.Isolation != IsolationHost && spec.Isolation != IsolationRestrictedHost {
			return Sandbox{}, fmt.Errorf("local workspace cannot claim non-host isolation")
		}
		if !filepath.IsAbs(spec.LocalRoot) || filepath.Clean(spec.LocalRoot) != spec.LocalRoot || len(spec.LocalRoot) > 4096 {
			return Sandbox{}, fmt.Errorf("local workspace root: must be a canonical absolute path")
		}
	default:
		if spec.LocalRoot != "" {
			return Sandbox{}, fmt.Errorf("remote or container sandbox cannot receive host workspace root")
		}
		if spec.Backend == BackendDocker && spec.Isolation != IsolationContainer {
			return Sandbox{}, fmt.Errorf("docker sandbox must declare container isolation")
		}
		if (spec.Backend == BackendDaytona || spec.Backend == BackendE2B || spec.Backend == BackendExternal) && spec.Isolation != IsolationRemote {
			return Sandbox{}, fmt.Errorf("remote sandbox must declare remote isolation")
		}
	}
	return Sandbox{spec: spec}, nil
}
func (s Sandbox) Validate() error           { _, err := NewSandbox(s.spec); return err }
func (s Sandbox) Spec() Spec                { return s.spec }
func (s Sandbox) ID() ID                    { return s.spec.ID }
func (s Sandbox) Backend() BackendKind      { return s.spec.Backend }
func (s Sandbox) Isolation() IsolationClass { return s.spec.Isolation }
func (s Sandbox) OwnerKind() string         { return s.spec.OwnerKind }
func (s Sandbox) OwnerRef() string          { return s.spec.OwnerRef }

// State is a typed observed lifecycle state; not all drivers implement stop/start.
type State string

const (
	StateDefined      State = "defined"
	StateProvisioning State = "provisioning"
	StateReady        State = "ready"
	StateStopping     State = "stopping"
	StateStopped      State = "stopped"
	StateDestroying   State = "destroying"
	StateDestroyed    State = "destroyed"
	StateFailed       State = "failed"
	StateUnknown      State = "unknown"
	StateOrphaned     State = "orphaned"
)

var allowedTransitions = map[State]map[State]bool{
	StateDefined:      {StateProvisioning: true, StateDestroying: true},
	StateProvisioning: {StateReady: true, StateFailed: true, StateUnknown: true},
	StateReady:        {StateStopping: true, StateDestroying: true, StateFailed: true, StateUnknown: true},
	StateStopping:     {StateStopped: true, StateFailed: true, StateUnknown: true},
	StateStopped:      {StateProvisioning: true, StateDestroying: true, StateUnknown: true},
	StateDestroying:   {StateDestroyed: true, StateFailed: true, StateUnknown: true},
	StateFailed:       {StateProvisioning: true, StateDestroying: true, StateUnknown: true},
	StateUnknown:      {StateReady: true, StateStopped: true, StateFailed: true, StateOrphaned: true, StateDestroying: true, StateDestroyed: true},
	StateOrphaned:     {StateDestroying: true, StateUnknown: true, StateDestroyed: true},
}

func (s State) Validate() error {
	if _, ok := allowedTransitions[s]; ok {
		return nil
	}
	if s == StateDestroyed {
		return nil
	}
	return fmt.Errorf("sandbox state: unknown %q", s)
}
func (s State) CanTransition(to State) bool { return allowedTransitions[s][to] }

// Run captures one observed backend instance and a monotonic generation used
// to fence stale updates. ExternalID is backend-owned and never a Tool input.
type Run struct {
	SandboxID  ID
	Generation uint64
	State      State
	ExternalID string
}

func NewRun(id ID, generation uint64, state State, externalID string) (Run, error) {
	if _, err := ParseID(id.String()); err != nil {
		return Run{}, err
	}
	if generation == 0 {
		return Run{}, fmt.Errorf("sandbox run generation: must be positive")
	}
	if err := state.Validate(); err != nil {
		return Run{}, err
	}
	if len(externalID) > maxBackendRefBytes || strings.TrimSpace(externalID) != externalID {
		return Run{}, fmt.Errorf("sandbox external reference: invalid")
	}
	if state == StateReady && externalID == "" {
		return Run{}, fmt.Errorf("ready sandbox run requires backend reference")
	}
	return Run{SandboxID: id, Generation: generation, State: state, ExternalID: externalID}, nil
}
func (r Run) Transition(to State, externalID string) (Run, error) {
	if !r.State.CanTransition(to) {
		return Run{}, fmt.Errorf("sandbox transition %s -> %s: %w", r.State, to, ErrConflict)
	}
	generation := r.Generation
	if to == StateProvisioning && r.State != StateDefined {
		generation++
	}
	if to == StateProvisioning {
		externalID = ""
	}
	return NewRun(r.SandboxID, generation, to, externalID)
}
