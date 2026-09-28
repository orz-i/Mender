package providerplugin

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
)

func TestManagerEnsureReusesCurrentInstance(t *testing.T) {
	t.Parallel()

	providerID := managerProviderID(t, "prov_00000000000000000000000000000001")
	plugin := newManagerPlugin()
	manager, configs, routes := newTestManager(t, plugin, managerConfig(providerID, "r1"))

	first, err := manager.Ensure(context.Background(), providerID)
	if err != nil {
		t.Fatalf("Ensure(first) error = %v", err)
	}
	second, err := manager.Ensure(context.Background(), providerID)
	if err != nil {
		t.Fatalf("Ensure(second) error = %v", err)
	}

	if first != second {
		t.Fatal("Ensure() returned a different instance for the same revision")
	}
	if got := plugin.openCount(); got != 1 {
		t.Fatalf("Open() count = %d, want 1", got)
	}
	if got := routes.registerCount(providerID); got != 1 {
		t.Fatalf("Register() count = %d, want 1", got)
	}
	if _, ok := configs.get(providerID); !ok {
		t.Fatal("test configuration unexpectedly missing")
	}
}

func TestManagerConcurrentEnsureOpensOncePerProvider(t *testing.T) {
	t.Parallel()

	providerID := managerProviderID(t, "prov_00000000000000000000000000000002")
	plugin := newManagerPlugin()
	manager, _, _ := newTestManager(t, plugin, managerConfig(providerID, "r1"))

	const callers = 16
	start := make(chan struct{})
	results := make(chan Instance, callers)
	errs := make(chan error, callers)

	var wg sync.WaitGroup
	wg.Add(callers)
	for range callers {
		go func() {
			defer wg.Done()
			<-start
			instance, err := manager.Ensure(context.Background(), providerID)
			results <- instance
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("Ensure() error = %v", err)
		}
	}

	var first Instance
	for instance := range results {
		if first == nil {
			first = instance
			continue
		}
		if instance != first {
			t.Fatal("concurrent Ensure() returned different instances")
		}
	}
	if got := plugin.openCount(); got != 1 {
		t.Fatalf("Open() count = %d, want 1", got)
	}
}

func TestManagerActivatesDifferentProvidersIndependently(t *testing.T) {
	t.Parallel()

	firstID := managerProviderID(t, "prov_00000000000000000000000000000003")
	secondID := managerProviderID(t, "prov_00000000000000000000000000000004")
	plugin := newManagerPlugin()

	started := make(chan catalogdomain.ProviderID, 2)
	release := make(chan struct{})
	plugin.openHook = func(_ context.Context, request OpenRequest) (Instance, error) {
		started <- request.ProviderID
		<-release
		return newManagerInstance(), nil
	}

	manager, configs, _ := newTestManager(t, plugin, managerConfig(firstID, "r1"))
	configs.set(managerConfig(secondID, "r1"))

	errs := make(chan error, 2)
	go func() {
		_, err := manager.Ensure(context.Background(), firstID)
		errs <- err
	}()
	go func() {
		_, err := manager.Ensure(context.Background(), secondID)
		errs <- err
	}()

	seen := make(map[catalogdomain.ProviderID]struct{}, 2)
	for len(seen) < 2 {
		select {
		case providerID := <-started:
			seen[providerID] = struct{}{}
		case <-time.After(2 * time.Second):
			close(release)
			t.Fatal("different Provider activations were serialized behind one lock")
		}
	}
	close(release)

	for range 2 {
		if err := <-errs; err != nil {
			t.Fatalf("Ensure() error = %v", err)
		}
	}
}

func TestManagerReplacesChangedRevisionAndReloads(t *testing.T) {
	t.Parallel()

	providerID := managerProviderID(t, "prov_00000000000000000000000000000005")
	plugin := newManagerPlugin()
	manager, configs, routes := newTestManager(t, plugin, managerConfig(providerID, "r1"))

	firstRaw, err := manager.Ensure(context.Background(), providerID)
	if err != nil {
		t.Fatalf("Ensure(first) error = %v", err)
	}
	first := firstRaw.(*managerInstance)

	updated := managerConfig(providerID, "r2")
	updated.Config = json.RawMessage(`{"value":2}`)
	configs.set(updated)

	secondRaw, err := manager.Ensure(context.Background(), providerID)
	if err != nil {
		t.Fatalf("Ensure(second) error = %v", err)
	}
	second := secondRaw.(*managerInstance)
	if first == second {
		t.Fatal("changed revision reused the old instance")
	}
	if got := first.closeCount(); got != 1 {
		t.Fatalf("first Close() count = %d, want 1", got)
	}
	if got := routes.executor(providerID); got != second {
		t.Fatalf("executor route = %T, want replacement instance", got)
	}

	thirdRaw, err := manager.Reload(context.Background(), providerID)
	if err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	third := thirdRaw.(*managerInstance)
	if third == second {
		t.Fatal("Reload() reused the current instance")
	}
	if got := second.closeCount(); got != 1 {
		t.Fatalf("second Close() count = %d, want 1", got)
	}
	if got := plugin.openCount(); got != 3 {
		t.Fatalf("Open() count = %d, want 3", got)
	}
}

func TestManagerReplacesNonReusableInstanceWithoutConfigChange(t *testing.T) {
	t.Parallel()

	providerID := managerProviderID(t, "prov_00000000000000000000000000000016")
	plugin := newManagerPlugin()
	first := newManagerReusableInstance(false)
	second := newManagerReusableInstance(true)
	var opened atomic.Int32
	plugin.openHook = func(context.Context, OpenRequest) (Instance, error) {
		if opened.Add(1) == 1 {
			return first, nil
		}
		return second, nil
	}
	manager, _, routes := newTestManager(t, plugin, managerConfig(providerID, "same-revision"))

	firstRaw, err := manager.Ensure(context.Background(), providerID)
	if err != nil {
		t.Fatalf("Ensure(first) error = %v", err)
	}
	if firstRaw != first {
		t.Fatal("Ensure(first) returned unexpected instance")
	}
	secondRaw, err := manager.Ensure(context.Background(), providerID)
	if err != nil {
		t.Fatalf("Ensure(second) error = %v", err)
	}
	if secondRaw != second {
		t.Fatal("Ensure(second) did not replace non-reusable instance")
	}
	if first.closeCount() != 1 {
		t.Fatalf("stale Close() count = %d, want 1", first.closeCount())
	}
	if plugin.openCount() != 2 {
		t.Fatalf("Open() count = %d, want 2", plugin.openCount())
	}
	if got := routes.executor(providerID); got != second {
		t.Fatalf("executor route = %T, want replacement instance", got)
	}
}

func TestManagerInstallationPinChangeReplacesInstance(t *testing.T) {
	t.Parallel()

	providerID := managerProviderID(
		t,
		"prov_00000000000000000000000000000015",
	)
	plugin := newManagerPlugin()
	firstInstallation, err := ParseInstallationID(
		"pinst_00000000000000000000000000000001",
	)
	if err != nil {
		t.Fatal(err)
	}
	secondInstallation, err := ParseInstallationID(
		"pinst_00000000000000000000000000000002",
	)
	if err != nil {
		t.Fatal(err)
	}
	firstConfig := managerConfig(providerID, "same-revision")
	firstConfig.InstallationID = &firstInstallation

	configs := &managerConfigSource{
		configs: make(map[catalogdomain.ProviderID]Configuration),
	}
	configs.set(firstConfig)
	routes := &managerRoutes{
		executors: make(map[catalogdomain.ProviderID]invocationapplication.ToolExecutor),
	}
	manager, err := NewManager(
		managerResolver{plugin: plugin},
		configs,
		routes,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.CloseAll() })

	firstRaw, err := manager.Ensure(context.Background(), providerID)
	if err != nil {
		t.Fatalf("Ensure(first) error = %v", err)
	}
	first := firstRaw.(*managerInstance)

	secondConfig := firstConfig
	secondConfig.InstallationID = &secondInstallation
	configs.set(secondConfig)

	secondRaw, err := manager.Ensure(context.Background(), providerID)
	if err != nil {
		t.Fatalf("Ensure(second) error = %v", err)
	}
	second := secondRaw.(*managerInstance)
	if second == first {
		t.Fatal("changed InstallationID reused the old instance")
	}
	if first.closeCount() != 1 {
		t.Fatalf("first Close() count = %d, want 1", first.closeCount())
	}
	if plugin.openCount() != 2 {
		t.Fatalf("Open() count = %d, want 2", plugin.openCount())
	}
}

func TestManagerCloseProviderAllowsLaterReactivation(t *testing.T) {
	t.Parallel()

	providerID := managerProviderID(t, "prov_00000000000000000000000000000006")
	plugin := newManagerPlugin()
	manager, _, routes := newTestManager(t, plugin, managerConfig(providerID, "r1"))

	firstRaw, err := manager.Ensure(context.Background(), providerID)
	if err != nil {
		t.Fatal(err)
	}
	first := firstRaw.(*managerInstance)

	if err := manager.CloseProvider(providerID); err != nil {
		t.Fatalf("CloseProvider() error = %v", err)
	}
	if got := first.closeCount(); got != 1 {
		t.Fatalf("Close() count = %d, want 1", got)
	}
	if got := routes.executor(providerID); got != nil {
		t.Fatalf("executor route remained after close: %T", got)
	}

	second, err := manager.Ensure(context.Background(), providerID)
	if err != nil {
		t.Fatalf("Ensure(after close) error = %v", err)
	}
	if second == first {
		t.Fatal("Ensure(after close) reused the closed instance")
	}
}

func TestManagerCloseAllIsIdempotentAndRejectsFutureEnsure(t *testing.T) {
	t.Parallel()

	firstID := managerProviderID(t, "prov_00000000000000000000000000000007")
	secondID := managerProviderID(t, "prov_00000000000000000000000000000008")
	plugin := newManagerPlugin()
	manager, configs, routes := newTestManager(t, plugin, managerConfig(firstID, "r1"))
	configs.set(managerConfig(secondID, "r1"))

	firstRaw, err := manager.Ensure(context.Background(), firstID)
	if err != nil {
		t.Fatal(err)
	}
	secondRaw, err := manager.Ensure(context.Background(), secondID)
	if err != nil {
		t.Fatal(err)
	}
	first := firstRaw.(*managerInstance)
	second := secondRaw.(*managerInstance)

	if err := manager.CloseAll(); err != nil {
		t.Fatalf("CloseAll() error = %v", err)
	}
	if err := manager.CloseAll(); err != nil {
		t.Fatalf("CloseAll(second) error = %v", err)
	}
	if first.closeCount() != 1 || second.closeCount() != 1 {
		t.Fatalf(
			"Close() counts = (%d, %d), want (1, 1)",
			first.closeCount(),
			second.closeCount(),
		)
	}
	if routes.executor(firstID) != nil || routes.executor(secondID) != nil {
		t.Fatal("executor route remained after CloseAll()")
	}

	_, err = manager.Ensure(context.Background(), firstID)
	if !errors.Is(err, ErrManagerClosed) {
		t.Fatalf("Ensure(after CloseAll) error = %v, want ErrManagerClosed", err)
	}
}

func TestManagerRejectsUnsupportedConfigVersionBeforeOpen(t *testing.T) {
	t.Parallel()

	providerID := managerProviderID(t, "prov_00000000000000000000000000000009")
	plugin := newManagerPlugin()
	config := managerConfig(providerID, "r1")
	config.ConfigVersion = 2
	manager, _, _ := newTestManager(t, plugin, config)

	_, err := manager.Ensure(context.Background(), providerID)
	if !errors.Is(err, ErrUnsupportedConfigVersion) {
		t.Fatalf("Ensure() error = %v, want ErrUnsupportedConfigVersion", err)
	}
	if got := plugin.openCount(); got != 0 {
		t.Fatalf("Open() count = %d, want 0", got)
	}
}

func managerProviderID(t *testing.T, value string) catalogdomain.ProviderID {
	t.Helper()
	id, err := catalogdomain.ParseProviderID(value)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func managerConfig(providerID catalogdomain.ProviderID, revision string) Configuration {
	return Configuration{
		ProviderID:    providerID,
		PluginID:      ID("io.mender.provider.mcp"),
		ConfigVersion: 1,
		Revision:      revision,
		Config:        json.RawMessage(`{"transport":{"kind":"stdio"}}`),
	}
}

func newTestManager(
	t *testing.T,
	plugin *managerPlugin,
	config Configuration,
) (*Manager, *managerConfigSource, *managerRoutes) {
	t.Helper()

	registry := NewRegistry()
	if err := registry.Register(plugin); err != nil {
		t.Fatal(err)
	}
	configs := &managerConfigSource{configs: make(map[catalogdomain.ProviderID]Configuration)}
	configs.set(config)
	routes := &managerRoutes{executors: make(map[catalogdomain.ProviderID]invocationapplication.ToolExecutor)}
	manager, err := NewManager(registry, configs, routes, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = manager.CloseAll()
	})
	return manager, configs, routes
}

type managerResolver struct {
	plugin Plugin
}

func (r managerResolver) Resolve(
	_ context.Context,
	config Configuration,
) (Resolution, error) {
	descriptor := r.plugin.Descriptor()
	if descriptor.ID != config.PluginID {
		return Resolution{}, errors.New("plugin id mismatch")
	}
	return Resolution{Plugin: r.plugin, Descriptor: descriptor}, nil
}

type managerConfigSource struct {
	mu      sync.RWMutex
	configs map[catalogdomain.ProviderID]Configuration
}

func (s *managerConfigSource) GetProviderConfig(
	_ context.Context,
	providerID catalogdomain.ProviderID,
) (Configuration, error) {
	s.mu.RLock()
	config, ok := s.configs[providerID]
	s.mu.RUnlock()
	if !ok {
		return Configuration{}, errors.New("configuration not found")
	}
	config.Config = cloneConfig(config.Config)
	return config, nil
}

func (s *managerConfigSource) set(config Configuration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	config.Config = cloneConfig(config.Config)
	s.configs[config.ProviderID] = config
}

func (s *managerConfigSource) get(
	providerID catalogdomain.ProviderID,
) (Configuration, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	config, ok := s.configs[providerID]
	return config, ok
}

type managerRoutes struct {
	mu        sync.RWMutex
	executors map[catalogdomain.ProviderID]invocationapplication.ToolExecutor
	registers map[catalogdomain.ProviderID]int
}

func (r *managerRoutes) Register(
	providerID catalogdomain.ProviderID,
	executor invocationapplication.ToolExecutor,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.registers == nil {
		r.registers = make(map[catalogdomain.ProviderID]int)
	}
	r.executors[providerID] = executor
	r.registers[providerID]++
	return nil
}

func (r *managerRoutes) Unregister(providerID catalogdomain.ProviderID) error {
	r.mu.Lock()
	delete(r.executors, providerID)
	r.mu.Unlock()
	return nil
}

func (r *managerRoutes) executor(
	providerID catalogdomain.ProviderID,
) invocationapplication.ToolExecutor {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.executors[providerID]
}

func (r *managerRoutes) registerCount(providerID catalogdomain.ProviderID) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.registers[providerID]
}

type managerPlugin struct {
	descriptor Descriptor

	mu       sync.Mutex
	opens    int
	openHook func(context.Context, OpenRequest) (Instance, error)
}

func newManagerPlugin() *managerPlugin {
	return &managerPlugin{
		descriptor: Descriptor{
			ID:            ID("io.mender.provider.mcp"),
			Version:       "1.0.0",
			DisplayName:   "MCP",
			ConfigVersion: 1,
		},
	}
}

func (p *managerPlugin) Descriptor() Descriptor {
	return p.descriptor
}

func (*managerPlugin) ValidateConfig(context.Context, json.RawMessage) error {
	return nil
}

func (*managerPlugin) InspectConfig(
	context.Context,
	json.RawMessage,
) (ConfigMetadata, error) {
	return ConfigMetadata{}, nil
}

func (p *managerPlugin) Open(
	ctx context.Context,
	request OpenRequest,
) (Instance, error) {
	p.mu.Lock()
	p.opens++
	hook := p.openHook
	p.mu.Unlock()
	if hook != nil {
		return hook(ctx, request)
	}
	return newManagerInstance(), nil
}

func (p *managerPlugin) openCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.opens
}

type managerInstance struct {
	closes atomic.Int32
}

func newManagerInstance() *managerInstance {
	return &managerInstance{}
}

func (*managerInstance) DiscoverTools(
	context.Context,
	catalogdomain.Provider,
) ([]catalogapplication.DiscoveredTool, error) {
	return nil, nil
}

func (*managerInstance) Execute(
	context.Context,
	invocationapplication.ExecutionRequest,
	invocationapplication.CredentialResolver,
	invocationapplication.ProgressReporter,
) (invocationdomain.Outcome, error) {
	value, err := invocationdomain.NewJSONValue([]byte("null"))
	if err != nil {
		return invocationdomain.Outcome{}, err
	}
	return invocationdomain.Complete(value)
}

func (i *managerInstance) Close() error {
	i.closes.Add(1)
	return nil
}

func (i *managerInstance) closeCount() int {
	return int(i.closes.Load())
}

type managerReusableInstance struct {
	*managerInstance
	reusable atomic.Bool
}

func newManagerReusableInstance(reusable bool) *managerReusableInstance {
	instance := &managerReusableInstance{managerInstance: newManagerInstance()}
	instance.reusable.Store(reusable)
	return instance
}

func (i *managerReusableInstance) Reusable() bool {
	return i != nil && i.reusable.Load()
}
