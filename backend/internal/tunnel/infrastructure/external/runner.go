package external

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	invocationjsonschema "github.com/orz-i/Mender/backend/internal/invocation/infrastructure/jsonschema"
	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
	installationdomain "github.com/orz-i/Mender/backend/internal/plugin/installation/domain"
	pluginprocess "github.com/orz-i/Mender/backend/internal/plugin/process"
	tunnelapplication "github.com/orz-i/Mender/backend/internal/tunnel/application"
	tunneldomain "github.com/orz-i/Mender/backend/internal/tunnel/domain"
	tunnelv1 "github.com/orz-i/Mender/sdk/tunnelplugin/go/protocol/v1"
)

const rpcConnectTimeout = 8 * time.Second

type InstallationLookup interface {
	Get(context.Context, platformplugin.InstallationID) (installationdomain.Installation, error)
}
type Runner struct {
	installations InstallationLookup
	host          *pluginprocess.Supervisor
}

func NewRunner(installations InstallationLookup, host *pluginprocess.Supervisor) (*Runner, error) {
	if installations == nil || host == nil {
		return nil, fmt.Errorf("external Tunnel plugin dependencies unavailable")
	}
	return &Runner{installations: installations, host: host}, nil
}

// CheckInstallation uses ONLY persisted verified descriptors. Neither
// discovery nor Preflight is permitted to launch a native process or resolve
// a Credential Reference.
func (r *Runner) CheckInstallation(ctx context.Context, tunnel tunneldomain.Tunnel) (installationdomain.Installation, error) {
	if r == nil || r.installations == nil {
		return installationdomain.Installation{}, fmt.Errorf("external Tunnel runner unavailable")
	}
	binding := tunnel.Binding()
	if err := binding.ValidateProvider(tunneldomain.ProviderExternal); err != nil {
		return installationdomain.Installation{}, err
	}
	item, err := r.installations.Get(ctx, binding.InstallationID)
	if err != nil {
		return installationdomain.Installation{}, err
	}
	if item.Status() != installationdomain.StatusEnabled ||
		item.Extension().Kind != platformplugin.TunnelTransport ||
		item.Extension().ProtocolVersion != 1 || item.PluginID() != binding.PluginID ||
		item.Extension().ID != binding.ExtensionID {
		return installationdomain.Installation{}, fmt.Errorf("tunnel transport package unavailable or wrong kind")
	}
	pinned, ok := item.VerifiedDescriptor()
	if !ok || pinned.Extension != item.Extension() ||
		!slices.Equal(pinned.Capabilities, []string{"loopback-mcp-proxy"}) {
		return installationdomain.Installation{}, fmt.Errorf("tunnel transport package lacks verified capability")
	}
	if err := r.host.VerifyPackage(ctx, item); err != nil {
		return installationdomain.Installation{}, err
	}
	return item, nil
}
func (r *Runner) Preflight(ctx context.Context, tunnel tunneldomain.Tunnel) error {
	_, err := r.CheckInstallation(ctx, tunnel)
	return err
}
func encodeConfig(externalID string) ([]byte, error) {
	if len(externalID) == 0 || len(externalID) > 256 {
		return nil, fmt.Errorf("tunnel external ID invalid")
	}
	return json.Marshal(map[string]string{"externalTunnelId": externalID})
}

func (r *Runner) native(ctx context.Context, item installationdomain.Installation) (*Process, error) {
	raw, err := r.host.Ensure(ctx, item)
	if err != nil {
		return nil, err
	}
	instance, ok := raw.(*Process)
	if !ok {
		return nil, fmt.Errorf("tunnel process host returned wrong extension")
	}
	return instance, nil
}

// VerifyConfiguration starts a verified native transport only in the
// explicit operator create path. It never connects to the network or receives
// a Credential secret; metadata reads never call this function.
func (r *Runner) VerifyConfiguration(ctx context.Context, item installationdomain.Installation, externalID string) error {
	if item.Status() != installationdomain.StatusEnabled || item.Extension().Kind != platformplugin.TunnelTransport {
		return fmt.Errorf("external Tunnel package not enabled")
	}
	raw, err := encodeConfig(externalID)
	if err != nil {
		return err
	}
	pinned, ok := item.VerifiedDescriptor()
	if !ok {
		return fmt.Errorf("tunnel package is unverified")
	}
	if len(pinned.ConfigSchema) > 0 {
		schema, err := catalogdomain.NewSchema(pinned.ConfigSchema)
		if err != nil {
			return err
		}
		value, err := invocationdomain.NewJSONValue(raw)
		if err != nil {
			return err
		}
		accepted, err := invocationjsonschema.Validator{}.Validate(ctx, schema, value)
		if err != nil {
			return err
		}
		if !accepted {
			return fmt.Errorf("tunnel config rejected by host schema")
		}
	}
	process, err := r.native(ctx, item)
	if err != nil {
		return err
	}
	configCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	valid, err := process.Service().ValidateConfig(configCtx, &tunnelv1.ValidateConfigRequest{
		ConfigVersion: pinned.ConfigVersion, ConfigJson: raw,
	})
	if err != nil {
		return fmt.Errorf("tunnel config validate: %w", err)
	}
	if !valid.GetValid() {
		return fmt.Errorf("tunnel config rejected: %s", valid.GetDiagnostic())
	}
	metadata, err := process.Service().InspectConfig(configCtx, &tunnelv1.InspectConfigRequest{
		ConfigVersion: pinned.ConfigVersion, ConfigJson: raw,
	})
	if err != nil {
		return fmt.Errorf("tunnel config inspect: %w", err)
	}
	if len(metadata.GetCredentialRefs()) != 0 || len(metadata.GetConnectionSummary()) > 4096 {
		return fmt.Errorf("tunnel transport requests unsupported credential refs or metadata")
	}
	return nil
}

func (r *Runner) Start(ctx context.Context, tunnel tunneldomain.Tunnel,
	endpoint string, credential []byte) (tunnelapplication.Process, error) {
	if err := tunnelapplication.ValidateManagedMCPEndpoint(endpoint); err != nil {
		return nil, err
	}
	item, err := r.CheckInstallation(ctx, tunnel)
	if err != nil {
		return nil, err
	}
	if len(credential) == 0 {
		return nil, fmt.Errorf("tunnel transport requires nonempty credential")
	}
	native, err := r.native(ctx, item)
	if err != nil {
		return nil, fmt.Errorf("tunnel transport start process: %w", err)
	}
	connectCtx, cancel := context.WithTimeout(ctx, rpcConnectTimeout)
	defer cancel()
	res, err := native.Service().Connect(connectCtx, &tunnelv1.ConnectRequest{
		TunnelId: tunnel.ID().String(), ExternalTunnelId: tunnel.ExternalTunnelID(),
		McpEndpoint: endpoint, EphemeralCredential: credential,
	})
	if err != nil {
		// A Connect RPC can have happened before the transport failed.
		// Kill the pinned native package rather than replaying a side effect.
		closeErr := r.host.CloseInstallation(item.ID())
		return nil, errors.Join(fmt.Errorf("tunnel transport Connect uncertain: %w", err), closeErr)
	}
	sessionID := res.GetSessionId()
	if len(sessionID) == 0 || len(sessionID) > 256 || validatePhase(res.GetPhase()) != nil ||
		res.GetPhase() == "failed" || len(res.GetDiagnostic()) > 2048 {
		_ = r.host.CloseInstallation(item.ID())
		return nil, fmt.Errorf("tunnel transport returned invalid session or health")
	}
	// Health is transport-reported, not proof of public reachability.
	probe, err := native.Service().Status(connectCtx, &tunnelv1.StatusRequest{SessionId: sessionID})
	if err != nil || validatePhase(probe.GetPhase()) != nil || probe.GetPhase() == "failed" {
		_ = r.host.CloseInstallation(item.ID())
		return nil, fmt.Errorf("tunnel transport health check failed")
	}
	return newSession(native, sessionID, probe.GetPhase(), r.host, item.ID()), nil
}

var _ tunnelapplication.ProcessRunner = (*Runner)(nil)
