package application

import "strings"

// SourceDiscoverable is the inert attribution/availability predicate shared by
// discovery adapters and management source selection. It grants no execution.
func SourceDiscoverable(kind, name, resourceID string, unavailable bool) bool {
	if unavailable || strings.TrimSpace(name) == "" {
		return false
	}
	switch kind {
	case "provider":
		return true
	case "agent", "sandbox":
		return resourceID != ""
	default:
		return false
	}
}
