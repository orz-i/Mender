package transport

import (
	"testing"

	providerv1 "github.com/orz-i/Mender/sdk/providerplugin/go/protocol/v1"
)

func TestVersionedPluginSets(t *testing.T) {
	t.Parallel()

	sets, err := VersionedPluginSets([]uint32{1}, nil)
	if err != nil {
		t.Fatalf("VersionedPluginSets() error = %v", err)
	}
	set, ok := sets[1]
	if !ok || set[PluginName] == nil {
		t.Fatalf("VersionedPluginSets() = %#v", sets)
	}

	if _, err := VersionedPluginSets(nil, nil); err == nil {
		t.Fatal("VersionedPluginSets(nil) error = nil")
	}
	if _, err := VersionedPluginSets([]uint32{1, 1}, nil); err == nil {
		t.Fatal("VersionedPluginSets(duplicate) error = nil")
	}
}

func TestServerConfigRequiresImplementation(t *testing.T) {
	t.Parallel()

	if _, err := ServerConfig([]uint32{1}, nil); err == nil {
		t.Fatal("ServerConfig() error = nil")
	}

	server := &stubServer{}
	config, err := ServerConfig([]uint32{1}, server)
	if err != nil {
		t.Fatalf("ServerConfig() error = %v", err)
	}
	if config.GRPCServer == nil || config.VersionedPlugins[1][PluginName] == nil {
		t.Fatalf("ServerConfig() = %#v", config)
	}
}

type stubServer struct {
	providerv1.UnimplementedProviderPluginServiceServer
}
