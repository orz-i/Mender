package httpapi

import (
	"strings"
	"testing"
	"time"

	installationdomain "github.com/orz-i/Mender/backend/internal/plugin/installation/domain"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
)

func TestPluginInstallationProjectionIncludesVerifiedDescriptor(t *testing.T) {
	t.Parallel()

	id, err := providerplugin.ParseInstallationID(
		"pinst_0123456789abcdef0123456789abcdef",
	)
	if err != nil {
		t.Fatal(err)
	}
	pluginID, err := providerplugin.ParseID("dev.example.echo")
	if err != nil {
		t.Fatal(err)
	}
	manifestJSON := []byte(
		"{\n" +
			"  \"manifestVersion\": 2,\n" +
			"  \"pluginId\": \"dev.example.echo\",\n" +
			"  \"version\": \"1.2.3\",\n" +
			"  \"extensions\": [{\"id\":\"main\",\"kind\":\"provider.tools\",\"protocolVersion\":1}],\n" +
			"  \"displayName\": \"Example Echo\",\n" +
			"  \"description\": \"Echo Provider plugin\",\n" +
			"  \"entrypoints\": {\"linux/amd64\": \"bin/echo\"}\n" +
			"}",
	)
	item, err := installationdomain.NewInstallation(
		id,
		pluginID,
		"1.2.3",
		installationdomain.SourceLocal,
		"/tmp/example-plugin",
		"sha256:"+strings.Repeat("a", 64),
		manifestJSON,
		installationdomain.StatusDisabled,
		time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	native, err := providerplugin.PlatformDescriptor(providerplugin.Descriptor{
		ID:              pluginID,
		Version:         "1.2.3",
		DisplayName:     "Example Echo",
		Description:     "Echo Provider plugin",
		ConfigVersion:   2,
		ProtocolVersion: 1,
		Capabilities:    []string{"echo"},
	})
	if err != nil {
		t.Fatal(err)
	}
	item, err = item.WithVerifiedDescriptor(native)
	if err != nil {
		t.Fatal(err)
	}

	got := pluginInstallationProjection(item)
	if got["displayName"] != "Example Echo" ||
		got["description"] != "Echo Provider plugin" ||
		got["configVersion"] != 2 ||
		got["protocolVersion"] != uint32(1) {
		t.Fatalf("descriptor projection = %#v", got)
	}
	capabilities, ok := got["capabilities"].([]string)
	if !ok || len(capabilities) != 1 || capabilities[0] != "echo" {
		t.Fatalf("capabilities = %#v", got["capabilities"])
	}
}
