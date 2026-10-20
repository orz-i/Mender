package domain

import "testing"

func TestValidAntigravityConversationIDIsCanonicalUUID(t *testing.T) {
	for _, valid := range []string{
		"dfe3c2a6-211b-48dc-bfa8-a5d2ccf51b85",
		"055a398f-db14-4c5f-abbb-1bf03f8120a7",
	} {
		if !ValidAntigravityConversationID(valid) {
			t.Fatalf("CLI conversation ID %q was rejected", valid)
		}
	}
	for _, invalid := range []string{
		"", "no-session", "exact-conversation-123",
		"00000000-0000-0000-0000-00000000000",
		"00000000-0000-0000-0000-000000000000\n",
		"--dangerously-skip-permissions", "/tmp/cwd", "dfe3c2a6-211b-48dc-bfa8-a5d2ccf51b85\x00",
		"DFE3C2A6-211B-48DC-BFA8-A5D2CCF51B85",
		"dfe3c2a6_211b-48dc-bfa8-a5d2ccf51b85",
	} {
		if ValidAntigravityConversationID(invalid) {
			t.Errorf("accepted unsafe CLI conversation ID %q", invalid)
		}
	}
}
