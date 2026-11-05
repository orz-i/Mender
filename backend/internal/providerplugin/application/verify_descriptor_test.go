package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
	installationdomain "github.com/orz-i/Mender/backend/internal/plugin/installation/domain"
	"github.com/orz-i/Mender/backend/internal/providerplugin"
)

func TestVerifyDescriptorCleansUpAndPreservesFailures(t *testing.T) {
	t.Parallel()
	for _, failure := range []string{"probe", "persist", "cleanup", "probe and cleanup"} {
		t.Run(failure, func(t *testing.T) {
			item, descriptor := verificationFixture(t)
			probeErr, persistErr, cleanupErr := errors.New("probe failed"), errors.New("snapshot write failed"), errors.New("cleanup failed")
			store := &verificationInstallations{item: item}
			probe := &verificationProbe{descriptor: descriptor}
			processes := &verificationProcesses{}
			if strings.Contains(failure, "probe") {
				probe.err = probeErr
			}
			if strings.Contains(failure, "persist") {
				store.err = persistErr
			}
			if strings.Contains(failure, "cleanup") {
				processes.err = cleanupErr
			}
			got, err := NewVerifyDescriptor(store, probe, processes, nil).Execute(t.Context(), item.ID())
			if processes.calls != 1 || probe.calls != 1 {
				t.Fatalf("probe calls=%d cleanup calls=%d", probe.calls, processes.calls)
			}
			if probe.err != nil && !errors.Is(err, probeErr) || store.err != nil && !errors.Is(err, persistErr) || processes.err != nil && !errors.Is(err, cleanupErr) {
				t.Fatalf("verification lost a failure: %v", err)
			}
			_, verified := got.VerifiedDescriptor()
			if verified || store.saved {
				t.Fatalf("failed verification published a snapshot: verified=%v persisted=%v", verified, store.saved)
			}
		})
	}
}

func TestVerifyDescriptorPersistsExactSnapshotAndThenStaysOffline(t *testing.T) {
	t.Parallel()
	item, descriptor := verificationFixture(t)
	store := &verificationInstallations{item: item}
	probe := &verificationProbe{descriptor: descriptor}
	processes := &verificationProcesses{}
	cache := &verificationCache{}
	uc := NewVerifyDescriptor(store, probe, processes, cache)
	got, err := uc.Execute(t.Context(), item.ID())
	if err != nil {
		t.Fatal(err)
	}
	verified, present := got.VerifiedDescriptor()
	if !present || verified.PluginID != item.PluginID() || got.ID() != item.ID() || got.Digest() != item.Digest() {
		t.Fatalf("verified installation identity changed: %+v present=%v", verified, present)
	}
	if _, err := uc.Execute(t.Context(), item.ID()); err != nil {
		t.Fatal(err)
	}
	if probe.calls != 1 || processes.calls != 1 || cache.calls != 1 {
		t.Fatalf("already verified package was probed again: probe=%d cleanup=%d cache=%d", probe.calls, processes.calls, cache.calls)
	}
}

func verificationFixture(t *testing.T) (installationdomain.Installation, providerplugin.Descriptor) {
	t.Helper()
	id, err := providerplugin.ParseInstallationID("pinst_00000000000000000000000000000098")
	if err != nil {
		t.Fatal(err)
	}
	pluginID, err := providerplugin.ParseID("dev.example.verify")
	if err != nil {
		t.Fatal(err)
	}
	item, err := installationdomain.NewInstallation(id, pluginID, "1.0.0", installationdomain.SourceLocal,
		"/tmp/mender-verify-provider-fixture", "sha256:"+strings.Repeat("8", 64), []byte(`{
		"manifestVersion":2,"pluginId":"dev.example.verify","version":"1.0.0",
		"extensions":[{"id":"main","kind":"provider.tools","protocolVersion":1}],
		"displayName":"Verification","entrypoints":{"linux/amd64":"bin/plugin"}
	}`), installationdomain.StatusEnabled, time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	return item, providerplugin.Descriptor{ID: pluginID, Version: "1.0.0", DisplayName: "Verification", ConfigVersion: 1, ProtocolVersion: 1}
}

type verificationInstallations struct {
	item  installationdomain.Installation
	err   error
	saved bool
}

func (s *verificationInstallations) Get(context.Context, providerplugin.InstallationID) (installationdomain.Installation, error) {
	return s.item, nil
}

func (s *verificationInstallations) SetVerifiedDescriptor(_ context.Context, _ providerplugin.InstallationID, descriptor platformplugin.Descriptor) error {
	if s.err != nil {
		return s.err
	}
	var err error
	s.item, err = s.item.WithVerifiedDescriptor(descriptor)
	s.saved = err == nil
	return err
}

type verificationProbe struct {
	descriptor providerplugin.Descriptor
	err        error
	calls      int
}

func (p *verificationProbe) ProbeInstallation(context.Context, providerplugin.InstallationID) (providerplugin.Resolution, error) {
	p.calls++
	return providerplugin.Resolution{Descriptor: p.descriptor}, p.err
}

type verificationProcesses struct {
	err   error
	calls int
}

func (p *verificationProcesses) CloseInstallation(providerplugin.InstallationID) error {
	p.calls++
	return p.err
}

type verificationCache struct{ calls int }

func (c *verificationCache) ForgetInstallation(providerplugin.InstallationID) { c.calls++ }
