package binding

import (
	agentapp "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

// Desktop fixture success follows the same native terminal/process evidence
// contract as real Agent backends, without creating a native Session.
func observeDesktopFixtureCompletion(cfg agentapp.RunConfig) error {
	for _, event := range []agentdomain.ExecutionEvent{
		{Kind: agentdomain.EventProcessStarted, ProcessPID: 70220},
		{Kind: agentdomain.EventPromptSent},
		{Kind: agentdomain.EventTerminalSeen},
		{Kind: agentdomain.EventLeaderReaped},
	} {
		if err := cfg.Observe(event); err != nil {
			return err
		}
	}
	return nil
}
