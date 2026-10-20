package domain

// ValidAntigravityConversationID validates the canonical opaque UUID emitted by
// Agy v1.4.2 init.conversation_id. This is syntax admission only: the Runner
// MUST still compare the live pre-prompt init identity with the stored ID.
func ValidAntigravityConversationID(id string) bool {
	if len(id) != 36 {
		return false
	}
	for i := 0; i < len(id); i++ {
		switch i {
		case 8, 13, 18, 23:
			if id[i] != '-' {
				return false
			}
		default:
			b := id[i]
			if (b < '0' || b > '9') && (b < 'a' || b > 'f') {
				return false
			}
		}
	}
	return true
}
