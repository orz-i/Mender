package domain

import "testing"

func TestWorkspaceRefRequiresStableOpaqueIdentity(t *testing.T) {
	raw := "wrk_0123456789abcdef0123456789abcdef"
	ref, err := ParseRef(raw)
	if err != nil || ref.String() != raw || ref.Validate() != nil {
		t.Fatalf("valid workspace ref: %q err %v", ref, err)
	}
	for _, bad := range []string{
		"", "wrk_", "wrk_../project",
		"wrk_0123456789ABCDEF0123456789ABCDEF",
		"sbx_0123456789abcdef0123456789abcdef",
		"/Users/mini/Coding/Mender",
		"wrk_0123456789abcdef0123456789abcdef ",
	} {
		if ref, err := ParseRef(bad); err == nil {
			t.Errorf("accepted %q as %q", bad, ref)
		}
	}
	if (Ref("")).Validate() == nil {
		t.Fatal("zero workspace ref is not valid")
	}
}
