package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	policyapplication "github.com/orz-i/Mender/backend/internal/policy/application"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
	provideractivationapplication "github.com/orz-i/Mender/backend/internal/provideractivation/application"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
)

func TestProviderActivationPolicyDeniesBeforePluginOpen(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	runtime := newProviderTestRuntime(t)
	probe := newActivationProbePlugin(t)
	if err := runtime.providerPlugins.Register(probe); err != nil {
		t.Fatal(err)
	}

	provider, err := runtime.RegisterConfiguredProvider(
		ctx,
		catalogapplication.RegisterProviderRequest{
			Namespace:   "activation-probe",
			DisplayName: "Activation Probe",
		},
		probe.descriptor.ID,
		json.RawMessage("{}"),
		testCatalogGovernanceContext(
			t,
			governancedomain.OriginInternal,
			"activation-registration",
		),
	)
	if err != nil {
		t.Fatalf("RegisterConfiguredProvider() error = %v", err)
	}
	if probe.openCalls.Load() != 0 {
		t.Fatalf("registration unexpectedly opened plugin: %d", probe.openCalls.Load())
	}

	_, err = runtime.Policy.CreateRule.Execute(
		ctx,
		policyapplication.RuleInput{
			DisplayName: "Deny probe activation",
			Enabled:     true,
			Decision:    policydomain.DecisionDeny,
			Match: policydomain.MatchSpec{
				Actions: []governancedomain.Action{
					governancedomain.ActionProviderActivate,
				},
				ProviderIDs: []string{provider.ID().String()},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = runtime.SyncConfiguredProvider(
		ctx,
		provider.ID(),
		testCatalogGovernanceContext(
			t,
			governancedomain.OriginInternal,
			"activation-denied",
		),
	)
	if !errors.Is(err, provideractivationapplication.ErrPolicyDenied) {
		t.Fatalf("SyncConfiguredProvider() error = %v, want policy denial", err)
	}
	if probe.openCalls.Load() != 0 {
		t.Fatalf("denied activation called Plugin.Open %d time(s)", probe.openCalls.Load())
	}

	records, err := runtime.ListProviderActivationAudit.Execute(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) < 2 {
		t.Fatalf("activation audit records = %d, want registration + denial", len(records))
	}
	latest := records[0]
	if latest.ProviderID() != provider.ID() ||
		latest.Status() != "denied" ||
		latest.PolicyKind() != policydomain.DecisionDeny ||
		latest.PrincipalRef() != "test:activation-denied" {
		t.Fatalf("latest activation audit = %#v", latest)
	}
}

type activationProbePlugin struct {
	descriptor providerplugin.Descriptor
	openCalls  atomic.Int32
}

func newActivationProbePlugin(t *testing.T) *activationProbePlugin {
	t.Helper()
	id, err := providerplugin.ParseID("dev.mender.activation-probe")
	if err != nil {
		t.Fatal(err)
	}
	return &activationProbePlugin{
		descriptor: providerplugin.Descriptor{
			ID:            id,
			Version:       "1.0.0",
			DisplayName:   "Activation Probe",
			Description:   "Provider activation admission test plugin",
			ConfigVersion: 1,
			ConfigSchema:  json.RawMessage("{\"type\":\"object\"}"),
		},
	}
}

func (p *activationProbePlugin) Descriptor() providerplugin.Descriptor {
	return p.descriptor
}

func (*activationProbePlugin) ValidateConfig(context.Context, json.RawMessage) error {
	return nil
}

func (*activationProbePlugin) InspectConfig(
	context.Context,
	json.RawMessage,
) (providerplugin.ConfigMetadata, error) {
	return providerplugin.ConfigMetadata{}, nil
}

func (p *activationProbePlugin) Open(
	context.Context,
	providerplugin.OpenRequest,
) (providerplugin.Instance, error) {
	p.openCalls.Add(1)
	return nil, errors.New("activation probe must not open when denied")
}
