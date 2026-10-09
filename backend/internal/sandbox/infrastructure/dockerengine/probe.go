package dockerengine

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	mobyclient "github.com/moby/moby/client"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

const probeTimeout = 8 * time.Second
const minimumAPIMajor, minimumAPIMinor = 1, 51

// probeAPI deliberately offers only read-only daemon calls. An adapter with
// this dependency cannot create images/containers or perform side effects.
type probeAPI interface {
	Ping(context.Context, mobyclient.PingOptions) (mobyclient.PingResult, error)
	Info(context.Context, mobyclient.InfoOptions) (mobyclient.SystemInfoResult, error)
	ImageInspect(context.Context, string, ...mobyclient.ImageInspectOption) (mobyclient.ImageInspectResult, error)
}

// Inspector is a read-only SDK-backed host-owned Engine capability and local
// image observer; it is NOT an operational Sandbox Backend yet.
type Inspector struct {
	endpoint EngineEndpoint
	api      probeAPI
	close    func() error
}

func NewInspector(endpoint EngineEndpoint) (*Inspector, error) {
	if err := endpoint.PreflightSocket(); err != nil {
		return nil, fmt.Errorf("docker engine: preflight: %w", sandboxdomain.ErrBackendUnavailable)
	}
	// Intentionally omit FromEnv: Docker contexts, DOCKER_HOST, TLS, and remote
	// network endpoints can never override the reviewed local UNIX endpoint.
	cli, err := mobyclient.New(mobyclient.WithHost(endpoint.URL()))
	if err != nil {
		return nil, fmt.Errorf("docker engine: cannot initialize explicit local client: %w", sandboxdomain.ErrBackendUnavailable)
	}
	return &Inspector{endpoint: endpoint, api: cli, close: cli.Close}, nil
}

func (i *Inspector) Close() error {
	if i == nil || i.close == nil {
		return nil
	}
	return i.close()
}

type Probe struct {
	EngineVersion  string
	APIVersion     string
	OSType         string
	CgroupVersion  string
	SeccompBuiltin bool
	CPULimits      bool
	MemoryLimits   bool
	PIDsLimit      bool
	// Supported reflects observed daemon capabilities, not container isolation.
}

func (i *Inspector) Probe(ctx context.Context) (Probe, error) {
	if i == nil || i.api == nil {
		return Probe{}, sandboxdomain.ErrBackendUnavailable
	}
	if err := i.endpoint.PreflightSocket(); err != nil {
		return Probe{}, fmt.Errorf("docker engine: UNIX socket invalid: %w", sandboxdomain.ErrBackendUnavailable)
	}
	limited, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	ping, err := i.api.Ping(limited, mobyclient.PingOptions{NegotiateAPIVersion: true})
	if err != nil {
		return Probe{}, safeDaemonError(limited, err)
	}
	if !sufficientEngineAPI(ping.APIVersion) {
		return Probe{}, fmt.Errorf("docker engine: unsupported API version: %w", sandboxdomain.ErrBackendUnavailable)
	}
	info, err := i.api.Info(limited, mobyclient.InfoOptions{})
	if err != nil {
		return Probe{}, safeDaemonError(limited, err)
	}
	c := Probe{
		EngineVersion:  info.Info.ServerVersion,
		APIVersion:     ping.APIVersion,
		OSType:         info.Info.OSType,
		CgroupVersion:  info.Info.CgroupVersion,
		SeccompBuiltin: slices.Contains(info.Info.SecurityOptions, "name=seccomp,profile=builtin"),
		CPULimits:      info.Info.CPUCfsQuota && info.Info.CPUCfsPeriod,
		MemoryLimits:   info.Info.MemoryLimit,
		PIDsLimit:      info.Info.PidsLimit,
	}
	if c.OSType != "linux" || c.CgroupVersion != "2" || !c.SeccompBuiltin || !c.CPULimits || !c.MemoryLimits || !c.PIDsLimit || c.EngineVersion == "" {
		return Probe{}, fmt.Errorf("docker engine: required Linux seccomp/cgroup resource controls unavailable: %w", sandboxdomain.ErrBackendUnavailable)
	}
	return c, nil
}

func sufficientEngineAPI(raw string) bool {
	parts := strings.Split(raw, ".")
	if len(parts) != 2 {
		return false
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return false
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return false
	}
	return major > minimumAPIMajor || (major == minimumAPIMajor && minor >= minimumAPIMinor)
}

// LocalImage records the content-addressable locally available image identity
// after checking the exact manifest digest. No tag lookup, pull or build occurs.
type LocalImage struct {
	ImageID      string
	Digest       string
	OSType       string
	Architecture string
}

func (i *Inspector) InspectLocalImage(ctx context.Context, t Template) (LocalImage, error) {
	if err := t.Validate(); err != nil {
		return LocalImage{}, err
	}
	if _, err := i.Probe(ctx); err != nil {
		return LocalImage{}, err
	}
	limited, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	result, err := i.api.ImageInspect(limited, t.ImageDigest)
	if err != nil {
		return LocalImage{}, safeDaemonError(limited, err)
	}
	response := result.InspectResponse
	if response.Os != "linux" || !locallyVerifiedDigest(response.RepoDigests, t.ImageDigest) || !strings.HasPrefix(response.ID, "sha256:") || len(response.ID) != 71 || response.Architecture == "" {
		return LocalImage{}, fmt.Errorf("docker engine: requested immutable image not locally verified: %w", sandboxdomain.ErrBackendUnavailable)
	}
	return LocalImage{ImageID: response.ID, Digest: t.ImageDigest, OSType: response.Os, Architecture: response.Architecture}, nil
}

// Never expose raw daemon errors: remote error bodies can contain HTTP URLs,
// filesystem paths or registry/credential metadata supplied by the daemon.
func safeDaemonError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return sandboxdomain.ErrBackendUnavailable
}

// Docker Engine normalizes Docker Hub library repository names to short
// RepoDigests ("alpine@sha256:...") even when queried by its fully qualified
// form. This is the only permitted alias, and the content digest must match.
func locallyVerifiedDigest(known []string, expected string) bool {
	if slices.Contains(known, expected) {
		return true
	}
	const prefix = "docker.io/library/"
	if strings.HasPrefix(expected, prefix) {
		return slices.Contains(known, strings.TrimPrefix(expected, prefix))
	}
	return false
}
