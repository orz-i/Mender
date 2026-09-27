package application

import (
	"testing"

	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
)

func TestApplyBindingPolicyConstraintUsesMostRestrictiveDecision(t *testing.T) {
	t.Parallel()

	allow, _ := policydomain.Allow("global.allow", "global allow")
	approval, _ := policydomain.RequireApproval(
		"global.approval",
		"global approval",
	)
	deny, _ := policydomain.Deny("global.deny", "global deny")

	tests := []struct {
		name       string
		global     policydomain.Decision
		constraint invocationdomain.BindingPolicyConstraint
		wantKind   policydomain.DecisionKind
		wantReason string
	}{
		{
			name:       "allow inherit",
			global:     allow,
			constraint: invocationdomain.BindingPolicyInherit,
			wantKind:   policydomain.DecisionAllow,
			wantReason: "global.allow",
		},
		{
			name:       "allow approval",
			global:     allow,
			constraint: invocationdomain.BindingPolicyRequireApproval,
			wantKind:   policydomain.DecisionRequireApproval,
			wantReason: "profile.binding.require_approval",
		},
		{
			name:       "allow deny",
			global:     allow,
			constraint: invocationdomain.BindingPolicyDeny,
			wantKind:   policydomain.DecisionDeny,
			wantReason: "profile.binding.deny",
		},
		{
			name:       "approval inherit",
			global:     approval,
			constraint: invocationdomain.BindingPolicyInherit,
			wantKind:   policydomain.DecisionRequireApproval,
			wantReason: "global.approval",
		},
		{
			name:       "approval approval",
			global:     approval,
			constraint: invocationdomain.BindingPolicyRequireApproval,
			wantKind:   policydomain.DecisionRequireApproval,
			wantReason: "global.approval",
		},
		{
			name:       "approval deny",
			global:     approval,
			constraint: invocationdomain.BindingPolicyDeny,
			wantKind:   policydomain.DecisionDeny,
			wantReason: "profile.binding.deny",
		},
		{
			name:       "deny inherit",
			global:     deny,
			constraint: invocationdomain.BindingPolicyInherit,
			wantKind:   policydomain.DecisionDeny,
			wantReason: "global.deny",
		},
		{
			name:       "deny approval",
			global:     deny,
			constraint: invocationdomain.BindingPolicyRequireApproval,
			wantKind:   policydomain.DecisionDeny,
			wantReason: "global.deny",
		},
		{
			name:       "deny deny",
			global:     deny,
			constraint: invocationdomain.BindingPolicyDeny,
			wantKind:   policydomain.DecisionDeny,
			wantReason: "global.deny",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := applyBindingPolicyConstraint(tc.global, tc.constraint)
			if err != nil {
				t.Fatalf("applyBindingPolicyConstraint() error = %v", err)
			}
			if got.Kind() != tc.wantKind || got.ReasonCode() != tc.wantReason {
				t.Fatalf(
					"decision = kind %q reason %q, want %q/%q",
					got.Kind(),
					got.ReasonCode(),
					tc.wantKind,
					tc.wantReason,
				)
			}
		})
	}
}
