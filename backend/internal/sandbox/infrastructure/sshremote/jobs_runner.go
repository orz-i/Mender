package sshremote

import (
	"context"
	"errors"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// systemdJobRunner is deliberately confined to the SSH backend. Type=exec
// ensures systemd acknowledges an actual exec request; --collect permits unit
// GC because on-host result files (not unit memory or journal) own completion.
// Unit and script path are derived from host-verified identities, not tool args.
type systemdJobRunner struct {
	client *ssh.Client
	user   string
}

func (r systemdJobRunner) Start(ctx context.Context, unit, script string) error {
	if r.client == nil || !validUnit(unit) || script == "" || r.user != "root" {
		return ErrJobUnavailable
	}
	cmd := "systemd-run --quiet --collect --service-type=exec --unit=" + quoted(strings.TrimSuffix(unit, ".service")) +
		" --property=User=" + quoted(r.user) +
		" --property=RuntimeMaxSec=14400s --property=TasksMax=64 --property=MemoryMax=1073741824 -- /bin/sh " + quoted(script)
	_, err := r.run(ctx, cmd)
	if err != nil {
		return ErrJobUnknown
	}
	return nil
}
func (r systemdJobRunner) Observe(ctx context.Context, unit string) (JobState, error) {
	if r.client == nil || !validUnit(unit) {
		return JobUnknown, ErrJobUnavailable
	}
	output, err := r.run(ctx, "systemctl --no-pager --plain show --property=LoadState,ActiveState "+quoted(unit))
	if err != nil {
		return JobUnknown, ErrJobUnknown
	}
	state := ""
	load := ""
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "ActiveState=") {
			state = strings.TrimPrefix(line, "ActiveState=")
		}
		if strings.HasPrefix(line, "LoadState=") {
			load = strings.TrimPrefix(line, "LoadState=")
		}
	}
	if load == "loaded" && (state == "active" || state == "activating" || state == "reloading" || state == "deactivating") {
		return JobRunning, nil
	}
	// No transient unit or failed/inactive after GC does not prove completion.
	return JobUnknown, nil
}
func (r systemdJobRunner) Stop(ctx context.Context, unit string) error {
	if r.client == nil || !validUnit(unit) {
		return ErrJobUnavailable
	}
	_, err := r.run(ctx, "systemctl --no-pager stop "+quoted(unit))
	if err != nil {
		return ErrJobUnknown
	}
	return nil
}
func validUnit(name string) bool {
	const prefix = "mender-sjob_"
	if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".service") {
		return false
	}
	id := strings.TrimSuffix(strings.TrimPrefix(name, "mender-"), ".service")
	return validateJobID(id) == nil
}
func (r systemdJobRunner) run(ctx context.Context, command string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	session, err := r.client.NewSession()
	if err != nil {
		return "", ErrJobUnavailable
	}
	defer func() { _ = session.Close() }()
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 15*time.Second {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
	}
	stop := context.AfterFunc(ctx, func() { _ = session.Close() })
	defer stop()
	out := &limitedOutput{limit: 4096}
	stderr := &limitedOutput{limit: 1024}
	session.Stdout = out
	session.Stderr = stderr
	if err = session.Run(command); err != nil {
		return "", errors.Join(ErrJobUnknown, ctx.Err())
	}
	if ctx.Err() != nil || out.truncated || stderr.truncated {
		return "", ErrJobUnknown
	}
	return out.data.String(), nil
}
