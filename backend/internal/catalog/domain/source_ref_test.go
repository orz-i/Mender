package domain

import "testing"

func TestSourceRefRoundTripAndExactKinds(t *testing.T) {
	for _, tc := range []struct {
		kind SourceKind
		id   string
	}{
		{SourceProvider, "prov_0123456789abcdef0123456789abcdef"},
		{SourceAgent, "agent_0123456789abcdef0123456789abcdef"},
		{SourceNative, "workspace"},
		{SourceNative, "jobs"},
	} {
		ref, err := NewSourceRef(tc.kind, tc.id)
		if err != nil {
			t.Fatalf("NewSourceRef(%q,%q): %v", tc.kind, tc.id, err)
		}
		parsed, err := ParseSourceRef(ref.String())
		if err != nil || parsed != ref || parsed.ID() != tc.id || parsed.Kind() != tc.kind {
			t.Fatalf("source roundtrip: ref=%+v parsed=%+v err=%v", ref, parsed, err)
		}
	}
}

func TestSourceRefRejectsUntrustedNamesAndAmbiguousIdentity(t *testing.T) {
	for _, raw := range []string{
		"", "provider", "provider:", "source:prov_0123456789abcdef0123456789abcdef",
		"provider:agent_0123456789abcdef0123456789abcdef",
		"agent:prov_0123456789abcdef0123456789abcdef",
		"agent:agent_0123456789abcdef0123456789ABCDEf",
		"native:read", "native:browser", "native:workspace:other",
		"native:Workspace", "sandbox:sbx_0123456789abcdef0123456789abcdef",
		"provider:prov_0123456789abcdef0123456789abcdef ",
	} {
		if ref, err := ParseSourceRef(raw); err == nil {
			t.Errorf("accepted source %q => %v", raw, ref)
		}
	}
	if (SourceRef{}).Validate() == nil || (SourceRef{}).String() != "" {
		t.Fatal("zero source must be invalid")
	}
}
