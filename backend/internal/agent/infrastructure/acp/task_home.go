package acp

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

// scopeGovernedAgentHome is only used for a task with a verified restricted
// ToolSupply on macOS. The Seatbelt profile still forbids the owner's HOME:
// this new empty per-process scratch area is inside its *existing* writable
// system-temp boundary. No user settings, tokens or credential files are copied.
//
// The owning ACP session must invoke release AFTER its complete subprocess
// group has been terminated and reaped, including on cancellation and errors.
func scopeGovernedAgentHome(cmd *exec.Cmd) (release func() error, err error) {
	// Never allow a governed task to inherit the Runtime's entire environment.
	// Production LocalFactory always builds an explicit, bounded environment.
	if cmd == nil || cmd.Env == nil {
		return nil, fmt.Errorf("%w: governed Agent needs an explicit process environment", agentdomain.ErrUnavailable)
	}
	// Fixed macOS system temp root, not a user- or agent-controlled TMPDIR.
	// MkdirTemp creates mode 0700 with a cryptographically random suffix.
	path, err := os.MkdirTemp("/private/tmp", "mender-agent-home-")
	if err != nil {
		return nil, fmt.Errorf("%w: create private governed Agent HOME: %w", agentdomain.ErrUnavailable, err)
	}
	filtered := make([]string, 0, len(cmd.Env)+1)
	for _, entry := range cmd.Env {
		if !strings.HasPrefix(entry, "HOME=") {
			filtered = append(filtered, entry)
		}
	}
	cmd.Env = append(filtered, "HOME="+path)
	return func() error { return os.RemoveAll(path) }, nil
}
