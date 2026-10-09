package plugin

import (
	"context"
	"encoding/json"
	"testing"
)

const testSandboxID = "sbx_0123456789abcdef0123456789abcdef"

func TestWorkspaceDescriptorAndConfig(t *testing.T) {
	t.Parallel()
	d := Descriptor()
	if d.ID != ID || d.ConfigVersion != 2 || len(d.Capabilities) != 4 {
		t.Fatalf("descriptor %+v", d)
	}
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(Config{SandboxID: testSandboxID})
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateConfig(context.Background(), raw); err != nil {
		t.Fatal(err)
	}
	metadata, err := InspectConfig(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.ConnectionSummary != "sandbox:"+testSandboxID || len(metadata.CredentialRefs) != 0 {
		t.Fatalf("metadata %+v", metadata)
	}
}
func TestConfigVersion2RejectsLegacyRootOrInvalidIDs(t *testing.T) {
	t.Parallel()
	for _, bad := range []string{
		`{"root":"/tmp"}`,
		`{"sandboxId":"sbx_bad"}`,
		`{"sandboxId":"` + testSandboxID + `","root":"/tmp"}`,
		`{"sandboxId":"` + testSandboxID + `"} {"sandboxId":"` + testSandboxID + `"}`,
	} {
		if err := ValidateConfig(context.Background(), json.RawMessage(bad)); err == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
}
