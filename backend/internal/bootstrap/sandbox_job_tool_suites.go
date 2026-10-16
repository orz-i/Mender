package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sshjobsplugin "github.com/orz-i/Mender/backend/internal/sshjobs/infrastructure/plugin"
	suitedomain "github.com/orz-i/Mender/backend/internal/toolsuite/domain"
	suitecatalogue "github.com/orz-i/Mender/backend/internal/toolsuite/infrastructure/curated"
)

func (r *Runtime) sandboxJobCapabilitySnapshot(ctx context.Context, id sandboxdomain.ID) (bool, string, error) {
	run, err := r.Sandboxes.GetRun(ctx, id)
	if err != nil {
		return false, "", err
	}
	offered, err := r.Sandboxes.VerifiedSSHJobContracts(ctx, id)
	if err != nil {
		return false, "Only a verified root SSH Remote backend provides Durable Jobs", nil
	}
	definition, ok := suitecatalogue.Find(suitedomain.SandboxJobs)
	if !ok {
		return false, "Reviewed Jobs suite not registered", suitedomain.ErrUnavailable
	}
	if err = suitedomain.RequireContracts(definition, suitedomain.SandboxResource, offered); err != nil {
		return false, "Required SSH Jobs guarantees unavailable", nil
	}
	if run.State != sandboxdomain.StateReady {
		return false, "New SSH Jobs require a Ready Sandbox; read-only recovery remains possible on Stopped", nil
	}
	return true, "", nil
}

// SandboxJobTools lists ONLY persisted, explicit job-facade associations. It
// does not adopt a manually supplied Provider, start a remote unit, connect to
// SSH, resolve a credential, create a Provider or expose Tools to a Profile.
func (r *Runtime) SandboxJobTools(ctx context.Context, id sandboxdomain.ID, caller gov.Context) (SandboxToolSuite, error) {
	if err := r.sandboxToolOwner(caller); err != nil {
		return SandboxToolSuite{}, err
	}
	if _, err := sandboxdomain.ParseID(id.String()); err != nil {
		return SandboxToolSuite{}, err
	}
	available, issue, err := r.sandboxJobCapabilitySnapshot(ctx, id)
	if err != nil {
		return SandboxToolSuite{}, err
	}
	result := SandboxToolSuite{SuiteID: suitedomain.SandboxJobs, SandboxID: id, Status: "missing", Available: available, Issue: issue, Tools: []SandboxTool{}}
	configs, err := r.providerConfigs.List(ctx)
	if err != nil {
		return result, err
	}
	matches := []catalogdomain.ProviderID{}
	for _, config := range configs {
		if config.PluginID != sshjobsplugin.ID || config.InstallationID != nil || config.ConfigVersion != sshjobsplugin.ConfigVersion {
			continue
		}
		decoded, e := sshjobsplugin.DecodeConfig(config.Config)
		if e == nil && decoded.SandboxID == id.String() {
			matches = append(matches, config.ProviderID)
		}
	}
	pinned, err := r.Sandboxes.store.GetJobSuite(ctx, id)
	if err != nil {
		return result, err
	}
	if len(matches) > 1 || (pinned != "" && (len(matches) != 1 || pinned != matches[0].String())) {
		result.Status = "conflict"
		return result, nil
	}
	if len(matches) == 0 {
		if !available {
			result.Status = "incompatible"
		}
		return result, nil
	}
	providerID := matches[0]
	provider, err := r.Catalog.GetProvider.Execute(ctx, providerID)
	if err != nil {
		result.Status = "conflict"
		return result, nil
	}
	result.ProviderID = providerID.String()
	result.ProviderName = provider.DisplayName()
	tools, err := r.Catalog.store.ListProviderTools(ctx, providerID)
	if err != nil {
		return result, err
	}
	present := map[string]bool{}
	for _, tool := range tools {
		result.Tools = append(result.Tools, SandboxTool{ToolID: tool.ID().String(), SourceKey: tool.SourceKey(), Name: tool.Name(), Title: tool.Title()})
		present[tool.SourceKey()] = true
	}
	definition, _ := suitecatalogue.Find(suitedomain.SandboxJobs)
	result.Status = "ready"
	// One exact, host-managed Provider may have been persisted before the
	// Suite claim completed. Treat it as incomplete and allow a fresh explicit
	// owner prepare to Sync/Claim it; never silently adopt on a GET.
	if pinned == "" {
		result.Status = "incomplete"
	}
	if len(present) != len(definition.ToolSourceKeys) {
		result.Status = "incomplete"
	}
	for _, key := range definition.ToolSourceKeys {
		if !present[key] {
			result.Status = "incomplete"
		}
	}
	// An existing suite remains recoverable while the SSH Sandbox is Stopped;
	// Available refers to creating NEW jobs, never read-only Task observations.
	return result, nil
}
func jobFacadeNamespace(id sandboxdomain.ID) string {
	return "sandbox-jobs-" + strings.TrimPrefix(id.String(), "sbx_")
}

// EnsureSandboxJobTools is explicit owner-only supply, independent from the
// existing Workspace facade. It does not grant Profile ToolBindings or Jobs
// execution permission, and generic Provider create cannot mint this facade.
func (r *Runtime) EnsureSandboxJobTools(ctx context.Context, id sandboxdomain.ID, caller gov.Context) (SandboxToolSuite, error) {
	if err := r.sandboxToolOwner(caller); err != nil {
		return SandboxToolSuite{}, err
	}
	item, err := r.Sandboxes.Get(ctx, id)
	if err != nil {
		return SandboxToolSuite{}, err
	}
	if item.OwnerKind() != string(caller.Principal().Kind()) || item.OwnerRef() != caller.Principal().Reference() {
		return SandboxToolSuite{}, sandboxdomain.ErrNotAuthorized
	}
	state, err := r.SandboxJobTools(ctx, id, caller)
	if err != nil {
		return state, err
	}
	if !state.Available {
		return state, fmt.Errorf("%w: SSH Jobs requirements unavailable", suitedomain.ErrIncompatible)
	}
	if state.Status == "conflict" {
		return state, sandboxdomain.ErrConflict
	}
	if state.Status == "ready" {
		return state, nil
	}
	if state.ProviderID == "" {
		config, e := json.Marshal(sshjobsplugin.Config{SandboxID: id.String()})
		if e != nil {
			return state, e
		}
		provider, e := r.registerConfiguredProvider(ctx, catalogapplication.RegisterProviderRequest{
			Namespace: jobFacadeNamespace(id), DisplayName: item.Spec().Name + " Jobs",
			Description: "Owner-managed SSH Durable Jobs for " + item.Spec().Name}, sshjobsplugin.ID, config, caller, true)
		if e != nil {
			current, readErr := r.SandboxJobTools(ctx, id, caller)
			if readErr != nil || current.ProviderID == "" || current.Status == "conflict" {
				return state, fmt.Errorf("create SSH Jobs facade: %w", e)
			}
			state = current
		} else {
			state.ProviderID = provider.ID().String()
		}
	}
	providerID, e := catalogdomain.ParseProviderID(state.ProviderID)
	if e != nil {
		return state, e
	}
	if _, e = r.syncConfiguredProviderForOwner(ctx, providerID, caller, true); e != nil {
		return state, e
	}
	if e = r.Sandboxes.store.ClaimJobSuite(ctx, id, providerID.String()); e != nil {
		return state, e
	}
	return r.SandboxJobTools(ctx, id, caller)
}

// RemoveSandboxJobTools retires only the host Tool facade, never task records.
// It rejects Profile/Policy references, remote active or unknown tasks, and
// concurrent Submit/Cancel. The SSH lifecycle obligation is never removed.
func (r *Runtime) RemoveSandboxJobTools(ctx context.Context, id sandboxdomain.ID, caller gov.Context) (retErr error) {
	if err := r.sandboxToolOwner(caller); err != nil {
		return err
	}
	item, err := r.Sandboxes.Get(ctx, id)
	if err != nil {
		return err
	}
	if item.OwnerKind() != string(caller.Principal().Kind()) || item.OwnerRef() != caller.Principal().Reference() {
		return sandboxdomain.ErrNotAuthorized
	}
	state, err := r.SandboxJobTools(ctx, id, caller)
	if err != nil {
		return err
	}
	if state.Status == "conflict" || (state.Status == "incomplete" && state.ProviderID != "") {
		// A partially created facade is repaired by Ensure, not implicitly
		// retired; in particular it must not skip its resource ownership claim.
		return sandboxdomain.ErrConflict
	}
	if state.ProviderID == "" {
		return nil
	}
	providerID, err := catalogdomain.ParseProviderID(state.ProviderID)
	if err != nil {
		return err
	}
	if err = r.ensureProviderUnused(ctx, providerID); err != nil {
		return err
	}
	run, err := r.Sandboxes.GetRun(ctx, id)
	if err != nil {
		return err
	}
	managed, err := r.Sandboxes.store.SSHJobManaged(ctx, id)
	if err != nil {
		return err
	}
	var token string
	if managed {
		token, err = r.Sandboxes.store.ReserveSSHJobLifecycle(ctx, run, caller)
		if err != nil {
			return err
		}
	} else if run.State == sandboxdomain.StateReady {
		token, err = r.Sandboxes.store.ReserveExecution(ctx, id, run.Generation, caller, sandboxdomain.CapabilityJobCancel)
		if err != nil {
			return err
		}
	} else if run.State != sandboxdomain.StateStopped {
		return sandboxdomain.ErrConflict
	}
	if token != "" {
		defer func() {
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			retErr = errors.Join(retErr, r.Sandboxes.store.ReleaseExecution(cleanup, id, token))
		}()
	}
	if managed {
		if err = r.Sandboxes.sshRemote.QuiescentJobs(ctx, id); err != nil {
			return err
		}
	}
	if err = r.Sandboxes.store.ReleaseJobSuite(ctx, id, state.ProviderID); err != nil {
		return err
	}
	if err = r.DeleteConfiguredProvider(ctx, providerID); err != nil {
		restoreErr := r.Sandboxes.store.ClaimJobSuite(context.WithoutCancel(ctx), id, state.ProviderID)
		return errors.Join(err, restoreErr)
	}
	return nil
}
