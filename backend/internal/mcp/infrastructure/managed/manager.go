package managed

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

	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	tunnelapplication "github.com/orz-i/Mender/backend/internal/tunnel/application"
	tunneldomain "github.com/orz-i/Mender/backend/internal/tunnel/domain"
)

var (
	ErrInvalidMCPServerAddress = errors.New("invalid managed MCP server address")
	ErrMCPServerNotFound       = errors.New("managed MCP server not found")
)

type Instance struct {
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

type runningServer struct {
	info            Instance
	desiredAddress  string
	server          *http.Server
	listener        net.Listener
	tunnelProcess   tunnelapplication.Process
	pendingTunnelID string
	stopping        bool
}

func (r *Manager) List() []Instance {
	r.mu.Lock()
	defer r.mu.Unlock()

	items := make([]Instance, 0, len(r.servers))
	for _, server := range r.servers {
		items = append(items, server.info)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].StartedAt.Before(items[j].StartedAt)
	})
	return items
}

func (r *Manager) Start(
	ctx context.Context,
	profileSlug profiledomain.Slug,
	address string,
	controlContext governancedomain.Context,
) (Instance, error) {
	address = strings.TrimSpace(address)
	if err := validateLoopbackMCPAddress(address); err != nil {
		return Instance{}, err
	}

	profile, err := r.deps.LoadBySlug(ctx, profileSlug)
	if err != nil {
		return Instance{}, fmt.Errorf("start MCP server: profile: %w", err)
	}
	if err := profile.validate(); err != nil {
		return Instance{}, fmt.Errorf("start MCP server: delivery: %w", err)
	}
	if profile.Slug() != profileSlug {
		return Instance{}, errors.New("start MCP server: Profile lookup identity changed")
	}
	catalog := managedMCPToolCatalog{
		revision:    r.deps.CatalogRevision,
		fingerprint: r.fingerprint,
	}
	catalog.initialRevision, err = catalog.revision(ctx)
	if err != nil {
		return Instance{}, fmt.Errorf("start MCP server: catalog revision: %w", err)
	}
	catalog.initialFingerprint, err = catalog.fingerprint(ctx, profile)
	if err != nil {
		return Instance{}, fmt.Errorf("start MCP server: catalog fingerprint: %w", err)
	}
	initial, err := r.deps.BuildHTTP(ctx, profile, controlContext)
	if err != nil {
		return Instance{}, fmt.Errorf("start MCP server: adapter: %w", err)
	}
	confirmedVersions, err := r.deps.Revisions(ctx, profile.ID())
	if err != nil {
		return Instance{}, fmt.Errorf("start MCP server: confirm Profile revision: %w", err)
	}
	confirmedCatalogRevision, err := catalog.revision(ctx)
	if err != nil {
		return Instance{}, fmt.Errorf("start MCP server: confirm Catalog revision: %w", err)
	}
	if confirmedVersions.Profile != profile.Profile.Revision() || confirmedVersions.Delivery != profile.Config.Revision() || confirmedCatalogRevision != catalog.initialRevision {
		return Instance{}, errors.New("start MCP server: Profile or Catalog changed while constructing initial MCP projection")
	}
	// A managed HTTP endpoint must follow committed Profile ToolBinding
	// AND canonical Tool metadata revisions, including mutations from a second Runtime/CLI process.
	// Keep the listener and tunnel stable while swapping stateless SDK
	// projections atomically on the next HTTP request.
	liveProfile, err := newManagedProfileMCPHTTPHandler(
		profile, initial,
		r.deps.Revisions, r.deps.LoadByID,
		func(reqCtx context.Context, updated Snapshot) (http.Handler, error) {
			return r.deps.BuildHTTP(reqCtx, updated, controlContext)
		},
		catalog,
	)
	if err != nil {
		return Instance{}, fmt.Errorf("start MCP server: Profile refresh: %w", err)
	}

	listener, err := listenManagedMCPPort(address)
	if err != nil {
		return Instance{}, fmt.Errorf("start MCP server: %w", err)
	}

	actualAddress := listener.Addr().String()
	endpoint := "http://" + actualAddress + "/mcp"
	serverID, err := newMCPServerID()
	if err != nil {
		_ = listener.Close()
		return Instance{}, fmt.Errorf("start MCP server: id: %w", err)
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

	instance := Instance{
		ServerID:  serverID,
		Profile:   profile.Slug().String(),
		Transport: "streamable-http",
		Address:   actualAddress,
		Endpoint:  endpoint,
		Status:    "running",
		StartedAt: time.Now().UTC(),
	}
	managed := &runningServer{info: instance, desiredAddress: address, server: httpServer, listener: listener}
	if id, ok := profile.TunnelID(); ok {
		managed.pendingTunnelID = id.String()
	}

	r.mu.Lock()
	r.servers[serverID] = managed
	r.mu.Unlock()

	serverLifecycleCtx := context.WithoutCancel(ctx)
	go r.serveManagedMCPServer(serverLifecycleCtx, serverID, managed)

	if tunnelID, ok := profile.TunnelID(); ok {
		process, tunnel, tunnelErr := r.deps.StartTunnel(ctx, tunnelID, endpoint)
		if tunnelErr != nil {
			stopErr := r.Stop(ctx, serverID)
			return Instance{}, errors.Join(
				fmt.Errorf("start MCP server: tunnel: %w", tunnelErr),
				stopErr,
			)
		}

		r.mu.Lock()
		current, stillRunning := r.servers[serverID]
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
		r.mu.Unlock()
		if !attached {
			_ = process.Stop(context.Background())
			return Instance{}, fmt.Errorf(
				"start MCP server: local server stopped before tunnel attached",
			)
		}
		go r.watchManagedTunnel(serverID, managed, process)
	}

	return instance, nil
}

func (r *Manager) Stop(ctx context.Context, serverID string) error {
	r.mu.Lock()
	managed, ok := r.servers[serverID]
	if !ok || managed.stopping {
		r.mu.Unlock()
		return ErrMCPServerNotFound
	}
	// Keep the resource visible as active until process disconnect + listener
	// shutdown both finish, preventing Tunnel delete during teardown.
	managed.stopping = true
	r.mu.Unlock()
	tunnelErr := stopManagedTunnel(ctx, managed.tunnelProcess)
	serverErr := shutdownHTTPServer(ctx, managed.server)
	r.mu.Lock()
	if r.servers[serverID] == managed {
		delete(r.servers, serverID)
	}
	r.mu.Unlock()
	if err := errors.Join(tunnelErr, serverErr); err != nil {
		return fmt.Errorf("stop MCP server %s: %w", serverID, err)
	}
	return nil
}

// IsTunnelActive is a host-owned, side-effect-free usage predicate. It also
// covers a server between listener startup and Tunnel process attachment.
func (r *Manager) IsTunnelActive(id tunneldomain.TunnelID) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, managed := range r.servers {
		if managed.pendingTunnelID == id.String() || managed.info.TunnelID == id.String() {
			return true
		}
	}
	return false
}

func (r *Manager) Restart(
	ctx context.Context,
	serverID string,
	controlContext governancedomain.Context,
) (Instance, error) {
	r.mu.Lock()
	managed, ok := r.servers[serverID]
	r.mu.Unlock()
	if !ok {
		return Instance{}, ErrMCPServerNotFound
	}

	profileSlug, err := profiledomain.NewSlug(managed.info.Profile)
	if err != nil {
		return Instance{}, fmt.Errorf("restart MCP server: profile: %w", err)
	}
	address := managed.desiredAddress
	if err := r.Stop(ctx, serverID); err != nil {
		return Instance{}, err
	}
	return r.Start(ctx, profileSlug, address, controlContext)
}

func (r *Manager) serveManagedMCPServer(
	ctx context.Context,
	serverID string,
	managed *runningServer,
) {
	_ = managed.server.Serve(managed.listener)

	r.mu.Lock()
	current, ok := r.servers[serverID]
	if ok && current == managed && !managed.stopping {
		delete(r.servers, serverID)
	}
	wasUnmanaged := ok && current == managed && !managed.stopping
	r.mu.Unlock()
	if wasUnmanaged {
		stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		_ = stopManagedTunnel(stopCtx, managed.tunnelProcess)
	}
}

func (r *Manager) watchManagedTunnel(serverID string, managed *runningServer,
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
func (r *Manager) setManagedTunnelStatus(serverID string, managed *runningServer,
	process tunnelapplication.Process, status string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if current, ok := r.servers[serverID]; ok && current == managed &&
		current.tunnelProcess == process {
		current.info.TunnelStatus = status
	}
}

func (r *Manager) Close() error {
	r.mu.Lock()
	servers := make([]*runningServer, 0, len(r.servers))
	for _, server := range r.servers {
		servers = append(servers, server)
	}
	r.servers = make(map[string]*runningServer)
	r.mu.Unlock()

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
