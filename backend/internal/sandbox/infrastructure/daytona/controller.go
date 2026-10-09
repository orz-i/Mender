package daytona

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

type RunStore interface {
	Get(context.Context, sandboxdomain.ID) (sandboxdomain.Sandbox, error)
	GetRun(context.Context, sandboxdomain.ID) (sandboxdomain.Run, error)
	TransitionRun(context.Context, sandboxdomain.Run, sandboxdomain.State, string) (sandboxdomain.Run, error)
	TransitionIfUnleased(context.Context, sandboxdomain.Run, sandboxdomain.State, string, bool) (sandboxdomain.Run, error)
	FinishDockerLifecycle(context.Context, sandboxdomain.Run, sandboxdomain.State, string, governancedomain.Action, governancedomain.Context) (sandboxdomain.Run, error)
}
type ActionAdmitter interface {
	AdmitAction(context.Context, governancedomain.Context, sandboxdomain.Sandbox, governancedomain.Action) error
}
type Controller struct {
	store     RunStore
	templates TemplateLookup
	factory   Factory
	admit     ActionAdmitter
}

func NewController(store RunStore, templates TemplateLookup, factory Factory, admit ActionAdmitter) (*Controller, error) {
	if store == nil || templates == nil || factory == nil || admit == nil {
		return nil, fmt.Errorf("daytona requires run store, immutable template, credential factory and Policy gate")
	}
	return &Controller{store: store, templates: templates, factory: factory, admit: admit}, nil
}
func remoteLabels(id sandboxdomain.ID, generation uint64) map[string]string {
	return map[string]string{labelSandboxID: id.String(), labelGeneration: strconv.FormatUint(generation, 10), labelBackend: "daytona"}
}
func remoteName(id sandboxdomain.ID, generation uint64) string {
	return "mender-sbx-" + string(id)[4:] + "-g" + strconv.FormatUint(generation, 10)
}
func (c *Controller) prepare(ctx context.Context, caller governancedomain.Context, id sandboxdomain.ID, action governancedomain.Action) (sandboxdomain.Sandbox, sandboxdomain.Run, Template, error) {
	if c == nil || c.store == nil || c.templates == nil || c.factory == nil || c.admit == nil {
		return sandboxdomain.Sandbox{}, sandboxdomain.Run{}, Template{}, sandboxdomain.ErrBackendUnavailable
	}
	if err := caller.Validate(); err != nil {
		return sandboxdomain.Sandbox{}, sandboxdomain.Run{}, Template{}, err
	}
	item, err := c.store.Get(ctx, id)
	if err != nil {
		return sandboxdomain.Sandbox{}, sandboxdomain.Run{}, Template{}, err
	}
	if err := ValidateSandbox(item); err != nil {
		return sandboxdomain.Sandbox{}, sandboxdomain.Run{}, Template{}, err
	}
	subject := caller.Principal()
	if item.OwnerKind() != string(subject.Kind()) || item.OwnerRef() != subject.Reference() {
		return sandboxdomain.Sandbox{}, sandboxdomain.Run{}, Template{}, sandboxdomain.ErrNotAuthorized
	}
	run, err := c.store.GetRun(ctx, id)
	if err != nil {
		return sandboxdomain.Sandbox{}, sandboxdomain.Run{}, Template{}, err
	}
	if err := c.admit.AdmitAction(ctx, caller, item, action); err != nil {
		return sandboxdomain.Sandbox{}, sandboxdomain.Run{}, Template{}, err
	}
	template, err := c.templates.GetDaytonaTemplate(ctx, id)
	if err != nil {
		return sandboxdomain.Sandbox{}, sandboxdomain.Run{}, Template{}, sandboxdomain.ErrBackendUnavailable
	}
	if err := template.Validate(); err != nil {
		return sandboxdomain.Sandbox{}, sandboxdomain.Run{}, Template{}, err
	}
	return item, run, template, nil
}

// Provision is intentionally one-shot. If Daytona's response is uncertain,
// the durable Run becomes unknown and no remote create is automatically replayed.
func (c *Controller) Provision(ctx context.Context, caller governancedomain.Context, id sandboxdomain.ID) (sandboxdomain.Run, error) {
	_, run, t, err := c.prepare(ctx, caller, id, governancedomain.ActionSandboxProvision)
	if err != nil {
		return sandboxdomain.Run{}, err
	}
	if run.State != sandboxdomain.StateDefined {
		return sandboxdomain.Run{}, sandboxdomain.ErrConflict
	}
	client, err := c.factory.Connect(ctx, t)
	if err != nil {
		return sandboxdomain.Run{}, err
	}
	defer closeRemote(client)
	staged, err := c.store.TransitionRun(ctx, run, sandboxdomain.StateProvisioning, "")
	if err != nil {
		return sandboxdomain.Run{}, err
	}
	sandbox, err := client.Create(ctx, t, remoteName(id, staged.Generation), remoteLabels(id, staged.Generation))
	if err != nil {
		return sandboxdomain.Run{}, errors.Join(err, c.unknown(ctx, staged, ""))
	}
	if sandbox == nil {
		return sandboxdomain.Run{}, errors.Join(sandboxdomain.ErrBackendUnavailable, c.unknown(ctx, staged, ""))
	}
	snapshot, err := sandbox.Snapshot(ctx)
	remoteID := snapshot.ID
	if ValidateExternalID(remoteID) != nil {
		remoteID = ""
	}
	if err != nil {
		return sandboxdomain.Run{}, errors.Join(err, c.unknown(ctx, staged, remoteID))
	}
	if err := verifySnapshot(ctx, sandbox, id, staged.Generation, t, "started"); err != nil {
		return sandboxdomain.Run{}, errors.Join(err, c.unknown(ctx, staged, remoteID))
	}
	next, err := c.store.TransitionRun(ctx, staged, sandboxdomain.StateReady, remoteID)
	if err != nil {
		return sandboxdomain.Run{}, errors.Join(err, c.unknown(ctx, staged, remoteID))
	}
	return next, nil
}

// Reconcile is remote READ ONLY. It can recover an uncertain instance only if
// exactly one Mender-labelled remote resource matches all trusted constraints.
func (c *Controller) Reconcile(ctx context.Context, caller governancedomain.Context, id sandboxdomain.ID) (sandboxdomain.Run, error) {
	_, run, t, err := c.prepare(ctx, caller, id, governancedomain.ActionSandboxProvision)
	if err != nil {
		return sandboxdomain.Run{}, err
	}
	if run.State != sandboxdomain.StateUnknown && run.State != sandboxdomain.StateProvisioning {
		return sandboxdomain.Run{}, sandboxdomain.ErrConflict
	}
	client, err := c.factory.Connect(ctx, t)
	if err != nil {
		return sandboxdomain.Run{}, err
	}
	defer closeRemote(client)
	var remote RemoteSandbox
	if run.ExternalID != "" {
		if err := ValidateExternalID(run.ExternalID); err != nil {
			return sandboxdomain.Run{}, sandboxdomain.ErrConflict
		}
		remote, err = client.Get(ctx, run.ExternalID)
		if err != nil {
			return sandboxdomain.Run{}, err
		}
	} else {
		matches, findErr := client.FindByLabel(ctx, remoteLabels(id, run.Generation))
		if findErr != nil {
			return sandboxdomain.Run{}, findErr
		}
		if len(matches) != 1 {
			return sandboxdomain.Run{}, sandboxdomain.ErrConflict
		}
		remote = matches[0]
	}
	if remote == nil {
		return sandboxdomain.Run{}, sandboxdomain.ErrNotFound
	}
	snapshot, err := remote.Snapshot(ctx)
	if err != nil {
		return sandboxdomain.Run{}, err
	}
	if run.ExternalID != "" && run.ExternalID != snapshot.ID {
		return sandboxdomain.Run{}, sandboxdomain.ErrConflict
	}
	observed := sandboxdomain.StateReady
	expectedState := "started"
	if snapshot.State == "stopped" {
		observed = sandboxdomain.StateStopped
		expectedState = "stopped"
	}
	if err := verifySnapshot(ctx, remote, id, run.Generation, t, expectedState); err != nil {
		return sandboxdomain.Run{}, err
	}
	return c.store.TransitionRun(ctx, run, observed, snapshot.ID)
}

// ChangeLifecycle uses the same durable SQLite generation/lease and Provider
// reference fencing as Docker. Unknown remote outcomes must be reconciled.
func (c *Controller) ChangeLifecycle(ctx context.Context, caller governancedomain.Context, id sandboxdomain.ID, action governancedomain.Action) (sandboxdomain.Run, error) {
	if action != governancedomain.ActionSandboxStart && action != governancedomain.ActionSandboxStop && action != governancedomain.ActionSandboxDestroy {
		return sandboxdomain.Run{}, sandboxdomain.ErrCapabilityUnavailable
	}
	_, run, t, err := c.prepare(ctx, caller, id, action)
	if err != nil {
		return sandboxdomain.Run{}, err
	}
	switch action {
	case governancedomain.ActionSandboxStart:
		if run.State != sandboxdomain.StateStopped {
			return sandboxdomain.Run{}, sandboxdomain.ErrConflict
		}
	case governancedomain.ActionSandboxStop:
		if run.State != sandboxdomain.StateReady {
			return sandboxdomain.Run{}, sandboxdomain.ErrConflict
		}
	case governancedomain.ActionSandboxDestroy:
		if run.State != sandboxdomain.StateReady && run.State != sandboxdomain.StateStopped {
			return sandboxdomain.Run{}, sandboxdomain.ErrConflict
		}
	}
	if err := ValidateExternalID(run.ExternalID); err != nil {
		return sandboxdomain.Run{}, sandboxdomain.ErrConflict
	}
	client, err := c.factory.Connect(ctx, t)
	if err != nil {
		return sandboxdomain.Run{}, err
	}
	defer closeRemote(client)
	remote, err := client.Get(ctx, run.ExternalID)
	if err != nil {
		return sandboxdomain.Run{}, err
	}
	previousState := "started"
	if run.State == sandboxdomain.StateStopped {
		previousState = "stopped"
	}
	if err := verifySnapshot(ctx, remote, id, run.Generation, t, previousState); err != nil {
		return sandboxdomain.Run{}, err
	}
	intermediate := sandboxdomain.StateStopping
	switch action {
	case governancedomain.ActionSandboxStart:
		intermediate = sandboxdomain.StateProvisioning
	case governancedomain.ActionSandboxDestroy:
		intermediate = sandboxdomain.StateDestroying
	}
	fenced, err := c.store.TransitionIfUnleased(ctx, run, intermediate, run.ExternalID, action == governancedomain.ActionSandboxDestroy)
	if err != nil {
		return sandboxdomain.Run{}, err
	}
	switch action {
	case governancedomain.ActionSandboxStop:
		err = remote.Stop(ctx)
	case governancedomain.ActionSandboxStart:
		err = remote.Start(ctx)
		if err == nil {
			err = remote.SetLabels(ctx, remoteLabels(id, fenced.Generation))
		}
	case governancedomain.ActionSandboxDestroy:
		err = remote.Destroy(ctx)
	}
	if err != nil {
		return sandboxdomain.Run{}, errors.Join(err, c.unknown(ctx, fenced, run.ExternalID))
	}
	nextState := sandboxdomain.StateStopped
	expected := "stopped"
	if action == governancedomain.ActionSandboxStart {
		nextState = sandboxdomain.StateReady
		expected = "started"
	}
	if action == governancedomain.ActionSandboxDestroy {
		nextState = sandboxdomain.StateDestroyed
	}
	if action != governancedomain.ActionSandboxDestroy {
		if err := verifySnapshot(ctx, remote, id, fenced.Generation, t, expected); err != nil {
			return sandboxdomain.Run{}, errors.Join(err, c.unknown(ctx, fenced, run.ExternalID))
		}
	}
	next, err := c.store.FinishDockerLifecycle(ctx, fenced, nextState, run.ExternalID, action, caller)
	if err != nil {
		return sandboxdomain.Run{}, errors.Join(err, c.unknown(ctx, fenced, run.ExternalID))
	}
	return next, nil
}
func (c *Controller) unknown(ctx context.Context, staged sandboxdomain.Run, remoteID string) error {
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_, err := c.store.TransitionRun(cleanup, staged, sandboxdomain.StateUnknown, remoteID)
	if err != nil {
		return fmt.Errorf("daytona unknown state could not be persisted: %w", err)
	}
	return nil
}
func closeRemote(client RemoteClient) {
	if client == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = client.Close(ctx)
}
