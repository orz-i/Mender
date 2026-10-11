package external

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	installationdomain "github.com/orz-i/Mender/backend/internal/plugin/installation/domain"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	providerv1 "github.com/orz-i/Mender/sdk/providerplugin/go/protocol/v1"
)

func TestResolverPinsSideBySidePackageVersions(t *testing.T) {
	t.Parallel()

	pluginID, err := providerplugin.ParseID("dev.example.echo")
	if err != nil {
		t.Fatal(err)
	}
	first := resolverTestInstallation(
		t,
		"pinst_00000000000000000000000000000001",
		pluginID,
		"1.0.0",
		"a",
	)
	second := resolverTestInstallation(
		t,
		"pinst_00000000000000000000000000000002",
		pluginID,
		"2.0.0",
		"b",
	)
	source := &resolverInstallationSource{
		items: map[providerplugin.InstallationID]installationdomain.Installation{
			first.ID():  first,
			second.ID(): second,
		},
	}
	processes := resolverProcessProvider{}
	resolver, err := NewResolver(
		providerplugin.NewRegistry(),
		source,
		processes,
	)
	if err != nil {
		t.Fatal(err)
	}

	firstProviderID, _ := catalogdomain.ParseProviderID(
		"prov_00000000000000000000000000000001",
	)
	secondProviderID, _ := catalogdomain.ParseProviderID(
		"prov_00000000000000000000000000000002",
	)
	firstConfig, err := providerplugin.NewInstalledConfiguration(
		firstProviderID,
		pluginID,
		first.ID(),
		1,
		[]byte("{}"),
	)
	if err != nil {
		t.Fatal(err)
	}
	secondConfig, err := providerplugin.NewInstalledConfiguration(
		secondProviderID,
		pluginID,
		second.ID(),
		1,
		[]byte("{}"),
	)
	if err != nil {
		t.Fatal(err)
	}

	firstResolved, err := resolver.Resolve(context.Background(), firstConfig)
	if err != nil {
		t.Fatal(err)
	}
	secondResolved, err := resolver.Resolve(context.Background(), secondConfig)
	if err != nil {
		t.Fatal(err)
	}
	if firstResolved.Descriptor.Version != "1.0.0" {
		t.Fatalf(
			"first resolved version = %q, want 1.0.0",
			firstResolved.Descriptor.Version,
		)
	}
	if secondResolved.Descriptor.Version != "2.0.0" {
		t.Fatalf(
			"second resolved version = %q, want 2.0.0",
			secondResolved.Descriptor.Version,
		)
	}
	if firstResolved.Plugin == secondResolved.Plugin {
		t.Fatal("side-by-side installations resolved to the same plugin instance")
	}

	again, err := resolver.Resolve(context.Background(), firstConfig)
	if err != nil {
		t.Fatal(err)
	}
	if again.Plugin != firstResolved.Plugin {
		t.Fatal("exact installation pin did not reuse its cached plugin")
	}
}

func TestResolverFailsClosedWhenPinnedInstallationDigestChanges(t *testing.T) {
	t.Parallel()

	pluginID, _ := providerplugin.ParseID("dev.example.echo")
	installation := resolverTestInstallation(
		t,
		"pinst_00000000000000000000000000000003",
		pluginID,
		"1.0.0",
		"c",
	)
	source := &resolverInstallationSource{
		items: map[providerplugin.InstallationID]installationdomain.Installation{
			installation.ID(): installation,
		},
	}
	resolver, err := NewResolver(
		providerplugin.NewRegistry(),
		source,
		resolverProcessProvider{},
	)
	if err != nil {
		t.Fatal(err)
	}
	providerID, _ := catalogdomain.ParseProviderID(
		"prov_00000000000000000000000000000003",
	)
	config, err := providerplugin.NewInstalledConfiguration(
		providerID,
		pluginID,
		installation.ID(),
		1,
		[]byte("{}"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.Resolve(context.Background(), config); err != nil {
		t.Fatal(err)
	}

	mutated := resolverTestInstallation(
		t,
		installation.ID().String(),
		pluginID,
		"1.0.0",
		"d",
	)
	source.items[installation.ID()] = mutated

	if _, err := resolver.Resolve(context.Background(), config); err == nil ||
		!strings.Contains(err.Error(), "digest changed") {
		t.Fatalf("Resolve() error = %v", err)
	}
}

type resolverInstallationSource struct {
	items map[providerplugin.InstallationID]installationdomain.Installation
}

func (s *resolverInstallationSource) Get(
	_ context.Context,
	id installationdomain.InstallationID,
) (installationdomain.Installation, error) {
	item, ok := s.items[id]
	if !ok {
		return installationdomain.Installation{}, fmt.Errorf("installation not found")
	}
	return item, nil
}

type resolverProcessProvider struct{}

func (resolverProcessProvider) Ensure(
	_ context.Context,
	installation installationdomain.Installation,
) (*Process, error) {
	return &Process{
		descriptor: &providerv1.Descriptor{
			PluginId:         installation.PluginID().String(),
			PackageVersion:   installation.PackageVersion(),
			DisplayName:      "Echo " + installation.PackageVersion(),
			Description:      "Resolver test plugin",
			ProtocolVersion:  1,
			ConfigVersion:    1,
			ConfigSchemaJson: []byte("{}"),
		},
	}, nil
}

func resolverTestInstallation(
	t *testing.T,
	rawID string,
	pluginID providerplugin.ID,
	version string,
	digestRune string,
) installationdomain.Installation {
	t.Helper()

	id, err := providerplugin.ParseInstallationID(rawID)
	if err != nil {
		t.Fatal(err)
	}
	manifestJSON := []byte(fmt.Sprintf(
		`{
			"manifestVersion": 2,
			"pluginId": %q,
			"version": %q,
			"extensions": [{"id":"main","kind":"provider.tools","protocolVersion":1}],
			"displayName": "Resolver Test",
			"description": "Resolver test package",
			"entrypoints": {"linux/amd64": "bin/plugin"}
		}`,
		pluginID.String(),
		version,
	))
	item, err := installationdomain.NewInstallation(
		id,
		pluginID,
		version,
		installationdomain.SourceLocal,
		"/tmp/"+id.String(),
		"sha256:"+strings.Repeat(digestRune, 64),
		manifestJSON,
		installationdomain.StatusEnabled,
		time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	native, err := providerplugin.PlatformDescriptor(providerplugin.Descriptor{
		ID:              pluginID,
		Version:         version,
		DisplayName:     "Echo " + version,
		Description:     "Resolver test plugin",
		ProtocolVersion: 1,
		ConfigVersion:   1,
		ConfigSchema:    []byte("{}"),
	})
	if err != nil {
		t.Fatal(err)
	}
	item, err = item.WithVerifiedDescriptor(native)
	if err != nil {
		t.Fatal(err)
	}
	return item
}
