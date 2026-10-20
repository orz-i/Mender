package bootstrap

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	tunnelapplication "github.com/orz-i/Mender/backend/internal/tunnel/application"
	tunneldomain "github.com/orz-i/Mender/backend/internal/tunnel/domain"
)

var (
	ErrInvalidMCPServerAddress = errors.New("invalid managed MCP server address")
	ErrMCPServerNotFound       = errors.New("managed MCP server not found")
)

type MCPServerInstance struct {
	ServerID          string
	Profile           string
	Transport         string
	Address           string
	Endpoint          string
	Status            string
	StartedAt         time.Time
	TunnelID          string
	TunnelDisplayName string
	TunnelProvider    string
	TunnelExternalID  string
	TunnelStatus      string
}

type managedMCPServer struct {
	info            MCPServerInstance
	desiredAddress  string
	server          *http.Server
	listener        net.Listener
	tunnelProcess   tunnelapplication.Process
	pendingTunnelID string
	stopping        bool
}

func (r *Runtime) ListMCPServers() []MCPServerInstance {
	r.mcpServersMu.Lock()
	defer r.mcpServersMu.Unlock()

	items := make([]MCPServerInstance, 0, len(r.mcpServers))
	for _, server := range r.mcpServers {
		items = append(items, server.info)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].StartedAt.Before(items[j].StartedAt)
	})
	return items
}

func (r *Runtime) StartMCPHTTPServer(
	ctx context.Context,
	profileSlug profiledomain.Slug,
	address string,
	controlContext governancedomain.Context,
) (MCPServerInstance, error) {
	address = strings.TrimSpace(address)
	if err := validateLoopbackMCPAddress(address); err != nil {
		return MCPServerInstance{}, err
	}

	profile, err := r.Profiles.Get.Execute(ctx, profileSlug)
	if err != nil {
		return MCPServerInstance{}, fmt.Errorf("start MCP server: profile: %w", err)
	}
	catalog := managedMCPToolCatalog{
		revision:    r.Catalog.store.CatalogRevision,
		fingerprint: r.profileToolMetadataFingerprint,
	}
	catalog.initialRevision, err = catalog.revision(ctx)
	if err != nil {
		return MCPServerInstance{}, fmt.Errorf("start MCP server: catalog revision: %w", err)
	}
	catalog.initialFingerprint, err = catalog.fingerprint(ctx, profile)
	if err != nil {
		return MCPServerInstance{}, fmt.Errorf("start MCP server: catalog fingerprint: %w", err)
	}
	activationContext, err := profileActivationContext(controlContext, profile)
	if err != nil {
		return MCPServerInstance{}, fmt.Errorf("start MCP server: governance context: %w", err)
	}
	if err := r.ActivateProfileProviders(ctx, profile, activationContext); err != nil {
		return MCPServerInstance{}, fmt.Errorf("start MCP server: %w", err)
	}
	adapter, err := r.NewMCPServer(ctx, profile)
	if err != nil {
		return MCPServerInstance{}, fmt.Errorf("start MCP server: adapter: %w", err)
	}
	confirmedProfileRevision, err := r.Profiles.GetRevision.Execute(ctx, profile.ID())
	if err != nil {
		return MCPServerInstance{}, fmt.Errorf("start MCP server: confirm Profile revision: %w", err)
	}
	confirmedCatalogRevision, err := catalog.revision(ctx)
	if err != nil {
		return MCPServerInstance{}, fmt.Errorf("start MCP server: confirm Catalog revision: %w", err)
	}
	if confirmedProfileRevision != profile.Revision() || confirmedCatalogRevision != catalog.initialRevision {
		return MCPServerInstance{}, errors.New("start MCP server: Profile or Catalog changed while constructing initial MCP projection")
	}
	// A managed HTTP endpoint must follow committed Profile ToolBinding
	// AND canonical Tool metadata revisions, including mutations from a second Runtime/CLI process.
	// Keep the listener and tunnel stable while swapping stateless SDK
	// projections atomically on the next HTTP request.
	liveProfile, err := newManagedProfileMCPHTTPHandler(
		profile, adapter.StreamableHTTPHandler(),
		r.Profiles.GetRevision.Execute, r.Profiles.GetByID.Execute,
		func(reqCtx context.Context, updated profiledomain.Profile) (http.Handler, error) {
			activationContext, activationErr := profileActivationContext(controlContext, updated)
			if activationErr != nil {
				return nil, activationErr
			}
			if activationErr = r.ActivateProfileProviders(reqCtx, updated, activationContext); activationErr != nil {
				return nil, activationErr
			}
			fresh, buildErr := r.NewMCPServer(reqCtx, updated)
			if buildErr != nil {
				return nil, buildErr
			}
			return fresh.StreamableHTTPHandler(), nil
		},
		catalog,
	)
	if err != nil {
		return MCPServerInstance{}, fmt.Errorf("start MCP server: Profile refresh: %w", err)
	}

	listener, err := net.Listen("tcp", address)
	if err != nil {
		return MCPServerInstance{}, fmt.Errorf("start MCP server: listen on %s: %w", address, err)
	}

	actualAddress := listener.Addr().String()
	endpoint := "http://" + actualAddress + "/mcp"
	serverID, err := newMCPServerID()
	if err != nil {
		_ = listener.Close()
		return MCPServerInstance{}, fmt.Errorf("start MCP server: id: %w", err)
	}

	mux := http.NewServeMux()
	mux.Handle(
		"/mcp",
		localMCPRequestGuard(liveProfile),
	)
	httpServer := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	instance := MCPServerInstance{
		ServerID:  serverID,
		Profile:   profile.Slug().String(),
		Transport: "streamable-http",
		Address:   actualAddress,
		Endpoint:  endpoint,
		Status:    "running",
		StartedAt: time.Now().UTC(),
	}
	managed := &managedMCPServer{info: instance, desiredAddress: address, server: httpServer, listener: listener}
	if id, ok := profile.TunnelID(); ok {
		managed.pendingTunnelID = id.String()
	}

	r.mcpServersMu.Lock()
	r.mcpServers[serverID] = managed
	r.mcpServersMu.Unlock()

	serverLifecycleCtx := context.WithoutCancel(ctx)
	go r.serveManagedMCPServer(serverLifecycleCtx, serverID, managed)

	if tunnelID, ok := profile.TunnelID(); ok {
		process, tunnel, tunnelErr := r.Tunnels.Start.Execute(ctx, tunnelID, endpoint)
		if tunnelErr != nil {
			stopErr := r.StopMCPServer(ctx, serverID)
			return MCPServerInstance{}, errors.Join(
				fmt.Errorf("start MCP server: tunnel: %w", tunnelErr),
				stopErr,
			)
		}

		r.mcpServersMu.Lock()
		current, stillRunning := r.mcpServers[serverID]
		attached := stillRunning && current == managed && !managed.stopping
		if attached {
			managed.tunnelProcess = process
			managed.info.TunnelID = tunnel.ID().String()
			managed.info.TunnelDisplayName = tunnel.DisplayName()
			managed.info.TunnelProvider = string(tunnel.Provider())
			managed.info.TunnelExternalID = tunnel.ExternalTunnelID()
			// Child startup is not evidence of downstream reachability.
			// Only a typed health reporter may claim ready/degraded.
			managed.info.TunnelStatus = "starting"
			if observed, ok := process.(tunnelapplication.HealthReporter); ok {
				managed.info.TunnelStatus = observed.TunnelHealth()
			}
			instance = managed.info
		}
		r.mcpServersMu.Unlock()
		if !attached {
			_ = process.Stop(context.Background())
			return MCPServerInstance{}, fmt.Errorf(
				"start MCP server: local server stopped before tunnel attached",
			)
		}
		go r.watchManagedTunnel(serverID, managed, process)
	}

	return instance, nil
}

func (r *Runtime) StopMCPServer(ctx context.Context, serverID string) error {
	r.mcpServersMu.Lock()
	managed, ok := r.mcpServers[serverID]
	if !ok || managed.stopping {
		r.mcpServersMu.Unlock()
		return ErrMCPServerNotFound
	}
	// Keep the resource visible as active until process disconnect + listener
	// shutdown both finish, preventing Tunnel delete during teardown.
	managed.stopping = true
	r.mcpServersMu.Unlock()
	tunnelErr := stopManagedTunnel(ctx, managed.tunnelProcess)
	serverErr := shutdownHTTPServer(ctx, managed.server)
	r.mcpServersMu.Lock()
	if r.mcpServers[serverID] == managed {
		delete(r.mcpServers, serverID)
	}
	r.mcpServersMu.Unlock()
	if err := errors.Join(tunnelErr, serverErr); err != nil {
		return fmt.Errorf("stop MCP server %s: %w", serverID, err)
	}
	return nil
}

// isTunnelActive is a host-owned, side-effect-free usage predicate. It also
// covers a server between listener startup and Tunnel process attachment.
func (r *Runtime) isTunnelActive(id tunneldomain.TunnelID) bool {
	r.mcpServersMu.Lock()
	defer r.mcpServersMu.Unlock()
	for _, managed := range r.mcpServers {
		if managed.pendingTunnelID == id.String() || managed.info.TunnelID == id.String() {
			return true
		}
	}
	return false
}

func (r *Runtime) RestartMCPServer(
	ctx context.Context,
	serverID string,
	controlContext governancedomain.Context,
) (MCPServerInstance, error) {
	r.mcpServersMu.Lock()
	managed, ok := r.mcpServers[serverID]
	r.mcpServersMu.Unlock()
	if !ok {
		return MCPServerInstance{}, ErrMCPServerNotFound
	}

	profileSlug, err := profiledomain.NewSlug(managed.info.Profile)
	if err != nil {
		return MCPServerInstance{}, fmt.Errorf("restart MCP server: profile: %w", err)
	}
	address := managed.desiredAddress
	if err := r.StopMCPServer(ctx, serverID); err != nil {
		return MCPServerInstance{}, err
	}
	return r.StartMCPHTTPServer(ctx, profileSlug, address, controlContext)
}

func (r *Runtime) ActivateProfileProviders(
	ctx context.Context,
	profile profiledomain.Profile,
	governanceContext governancedomain.Context,
) error {
	if err := governanceContext.Validate(); err != nil {
		return fmt.Errorf("activate profile providers: governance context: %w", err)
	}
	if governanceContext.Scope().Kind() != governancedomain.ScopeProfile ||
		governanceContext.Scope().Reference() != profile.ID().String() {
		return fmt.Errorf("activate profile providers: governance scope must match Profile %s", profile.ID())
	}
	providerSet := make(map[catalogdomain.ProviderID]struct{})
	for _, toolID := range profile.ToolIDs() {
		tool, err := r.Catalog.DescribeTool.Execute(ctx, toolID)
		if err != nil {
			return fmt.Errorf(
				"activate profile provider: describe %s: %w",
				toolID,
				err,
			)
		}
		providerSet[tool.ProviderID()] = struct{}{}
	}

	providerIDs := make([]catalogdomain.ProviderID, 0, len(providerSet))
	for providerID := range providerSet {
		providerIDs = append(providerIDs, providerID)
	}
	sort.Slice(providerIDs, func(i, j int) bool {
		return providerIDs[i].String() < providerIDs[j].String()
	})

	for _, providerID := range providerIDs {
		if err := r.activateConfiguredProvider(ctx, providerID, governanceContext); err != nil {
			return fmt.Errorf(
				"activate profile provider %s: %w",
				providerID,
				err,
			)
		}
	}
	return nil
}

func (r *Runtime) serveManagedMCPServer(
	ctx context.Context,
	serverID string,
	managed *managedMCPServer,
) {
	_ = managed.server.Serve(managed.listener)

	r.mcpServersMu.Lock()
	current, ok := r.mcpServers[serverID]
	if ok && current == managed && !managed.stopping {
		delete(r.mcpServers, serverID)
	}
	wasUnmanaged := ok && current == managed && !managed.stopping
	r.mcpServersMu.Unlock()
	if wasUnmanaged {
		stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		_ = stopManagedTunnel(stopCtx, managed.tunnelProcess)
	}
}

func (r *Runtime) watchManagedTunnel(serverID string, managed *managedMCPServer,
	process tunnelapplication.Process) {
	reporter, hasHealth := process.(tunnelapplication.HealthReporter)
	if !hasHealth {
		<-process.Done()
		r.setManagedTunnelStatus(serverID, managed, process, "exited")
		return
	}
	ticker := time.NewTicker(400 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-process.Done():
			r.setManagedTunnelStatus(serverID, managed, process, "exited")
			return
		case <-ticker.C:
			// Reads cached session metadata only, without launching plugins
			// or resolving credentials in the management plane.
			status := reporter.TunnelHealth()
			r.setManagedTunnelStatus(serverID, managed, process, status)
		}
	}
}
func (r *Runtime) setManagedTunnelStatus(serverID string, managed *managedMCPServer,
	process tunnelapplication.Process, status string) {
	r.mcpServersMu.Lock()
	defer r.mcpServersMu.Unlock()
	if current, ok := r.mcpServers[serverID]; ok && current == managed &&
		current.tunnelProcess == process {
		current.info.TunnelStatus = status
	}
}

func (r *Runtime) closeManagedMCPServers() error {
	r.mcpServersMu.Lock()
	servers := make([]*managedMCPServer, 0, len(r.mcpServers))
	for _, server := range r.mcpServers {
		servers = append(servers, server)
	}
	r.mcpServers = make(map[string]*managedMCPServer)
	r.mcpServersMu.Unlock()

	errs := make([]error, 0, len(servers)*2)
	for _, server := range servers {
		errs = append(
			errs,
			stopManagedTunnel(context.Background(), server.tunnelProcess),
			shutdownHTTPServer(context.Background(), server.server),
		)
	}
	return errors.Join(errs...)
}

func stopManagedTunnel(
	ctx context.Context,
	process tunnelapplication.Process,
) error {
	if process == nil {
		return nil
	}
	if err := process.Stop(ctx); err != nil {
		return fmt.Errorf("stop tunnel: %w", err)
	}
	return nil
}

func shutdownHTTPServer(ctx context.Context, server *http.Server) error {
	shutdownCtx := ctx
	cancel := func() {}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		shutdownCtx, cancel = context.WithTimeout(ctx, time.Second)
	}
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		if closeErr := server.Close(); closeErr != nil {
			return errors.Join(err, closeErr)
		}
	}
	return nil
}

func localMCPRequestGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if !isLoopbackRequestHost(request.Host) {
			http.Error(w, "MCP request Host must be localhost or loopback", http.StatusForbidden)
			return
		}

		if origin := strings.TrimSpace(request.Header.Get("Origin")); origin != "" {
			parsed, err := url.Parse(origin)
			if err != nil ||
				(parsed.Scheme != "http" && parsed.Scheme != "https") ||
				!isLoopbackHostname(parsed.Hostname()) {
				http.Error(
					w,
					"MCP request Origin must be localhost or loopback",
					http.StatusForbidden,
				)
				return
			}
		}

		if strings.EqualFold(
			strings.TrimSpace(request.Header.Get("Sec-Fetch-Site")),
			"cross-site",
		) {
			http.Error(w, "cross-site MCP requests are forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, request)
	})
}

func isLoopbackRequestHost(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		return isLoopbackHostname(host)
	}
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		value = strings.TrimSuffix(strings.TrimPrefix(value, "["), "]")
	}
	return isLoopbackHostname(value)
}

func isLoopbackHostname(host string) bool {
	host = strings.TrimSpace(host)
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func validateLoopbackMCPAddress(address string) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidMCPServerAddress, err)
	}
	if port == "" {
		return fmt.Errorf("%w: port is required", ErrInvalidMCPServerAddress)
	}
	if isLoopbackHostname(host) {
		return nil
	}
	return fmt.Errorf(
		"%w: host must be localhost or a loopback IP",
		ErrInvalidMCPServerAddress,
	)
}

func newMCPServerID() (string, error) {
	raw := make([]byte, 8)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return "srv_" + hex.EncodeToString(raw), nil
}
