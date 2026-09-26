package domain

import "testing"

func TestDecisionKindsAndReasonCodes(t *testing.T) {
	t.Parallel()

	constructors := []struct {
		name string
		kind DecisionKind
		fn   func(string, string) (Decision, error)
	}{
		{"allow", DecisionAllow, Allow},
		{"deny", DecisionDeny, Deny},
		{"approval", DecisionRequireApproval, RequireApproval},
	}

	for _, tc := range constructors {
		decision, err := tc.fn("local.default", "test")
		if err != nil {
			t.Fatalf("%s constructor error = %v", tc.name, err)
		}
		if decision.Kind() != tc.kind || decision.ReasonCode() != "local.default" {
			t.Fatalf("%s decision = %#v", tc.name, decision)
		}
	}

	if _, err := Allow("NOT VALID", ""); err == nil {
		t.Fatal("Allow() expected invalid reason code error")
	}
}
