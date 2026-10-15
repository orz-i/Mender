package bootstrap

import (
	auditsqlite "github.com/orz-i/Mender/backend/internal/audit/infrastructure/sqlite"
	governanceapplication "github.com/orz-i/Mender/backend/internal/governance/application"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationevents "github.com/orz-i/Mender/backend/internal/invocation/infrastructure/events"
	invocationexecutionguard "github.com/orz-i/Mender/backend/internal/invocation/infrastructure/executionguard"
	invocationidentity "github.com/orz-i/Mender/backend/internal/invocation/infrastructure/identity"
	invocationjsonschema "github.com/orz-i/Mender/backend/internal/invocation/infrastructure/jsonschema"
	invocationresultguard "github.com/orz-i/Mender/backend/internal/invocation/infrastructure/resultguard"
	invocationrouter "github.com/orz-i/Mender/backend/internal/invocation/infrastructure/router"
	provideractivationapplication "github.com/orz-i/Mender/backend/internal/provideractivation/application"
	provideractivationidentity "github.com/orz-i/Mender/backend/internal/provideractivation/infrastructure/identity"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	providerconfigstore "github.com/orz-i/Mender/backend/internal/providerplugin/configstore"
)

// runtimeInvocationWiring is the bounded composition boundary for every
// Tool invocation and Provider activation adapter. NewRuntime owns the
// dependencies; this helper owns their exact governance order and defaults.
type runtimeInvocationWiring struct {
	options         RuntimeOptions
	catalog         *Catalog
	profiles        *Profiles
	providerConfigs *providerconfigstore.Store
	plugins         *providerplugin.Registry
	policy          *Policies
	governance      *RuntimeGovernance
	executors       *invocationrouter.Router
	credentials     *Credentials
	audit           *auditsqlite.Store
	approval        governanceapplication.ApprovalGate
}

func (w runtimeInvocationWiring) compose() (provideractivationapplication.Admit, invocationapplication.InvokeTool) {
	providerActivation := provideractivationapplication.NewAdmit(
		w.policy.Evaluate,
		w.approval,
		w.governance.controller,
		w.audit,
		provideractivationidentity.NewGenerator(),
		systemClock{},
	)

	schemaValidator := invocationjsonschema.Validator{}
	bindingExecutionGuard := w.options.BindingExecutionGuard
	if bindingExecutionGuard == nil {
		bindingExecutionGuard = invocationexecutionguard.NewProfileBindingSerial()
	}
	observer := w.options.InvocationObserver
	if observer == nil {
		observer = invocationevents.NoopObserver{}
	}
	invokeTool := invocationapplication.NewInvokeTool(
		w.catalog.store,
		newInvocationProfileScopeResolver(w.profiles.GetByID),
		schemaValidator,
		newInvocationGovernanceFactExtractor(w.providerConfigs, w.plugins),
		newInvocationPolicyEvaluator(w.policy.Evaluate),
		w.approval,
		w.governance.controller,
		bindingExecutionGuard,
		invocationresultguard.New(schemaValidator),
		observer,
		w.executors,
		w.credentials.Resolver,
		w.audit,
		invocationidentity.NewGenerator(),
		systemClock{},
		w.options.DefaultTimeout,
	)
	return providerActivation, invokeTool
}
