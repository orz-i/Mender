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
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

var (
	ErrInvalidMCPServerAddress = errors.New("invalid managed MCP server address")
	ErrMCPServerNotFound       = errors.New("managed MCP server not found")
)

type MCPServerInstance struct {
	ServerID  string
	Profile   string
	Transport string
	Address   string
	Endpoint  string
	Status    string
	StartedAt time.Time
}

type managedMCPServer struct {
	info           MCPServerInstance
	desiredAddress string
	server         *http.Server
	listener       net.Listener
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
) (MCPServerInstance, error) {
	address = strings.TrimSpace(address)
	if err := validateLoopbackMCPAddress(address); err != nil {
		return MCPServerInstance{}, err
	}

	profile, err := r.Profiles.Get.Execute(ctx, profileSlug)
	if err != nil {
		return MCPServerInstance{}, fmt.Errorf("start MCP server: profile: %w", err)
	}
	if err := r.ActivateProfileProviders(ctx, profile); err != nil {
		return MCPServerInstance{}, fmt.Errorf("start MCP server: %w", err)
	}
	adapter, err := r.NewMCPServer(ctx, profile)
	if err != nil {
		return MCPServerInstance{}, fmt.Errorf("start MCP server: adapter: %w", err)
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
		localMCPRequestGuard(adapter.StreamableHTTPHandler()),
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
	managed := &managedMCPServer{
		info:           instance,
		desiredAddress: address,
		server:         httpServer,
		listener:       listener,
	}

	r.mcpServersMu.Lock()
	r.mcpServers[serverID] = managed
	r.mcpServersMu.Unlock()

	go r.serveManagedMCPServer(serverID, managed)
	return instance, nil
}

func (r *Runtime) StopMCPServer(
	ctx context.Context,
	serverID string,
) error {
	r.mcpServersMu.Lock()
	managed, ok := r.mcpServers[serverID]
	if ok {
		delete(r.mcpServers, serverID)
	}
	r.mcpServersMu.Unlock()

	if !ok {
		return ErrMCPServerNotFound
	}
	if err := shutdownHTTPServer(ctx, managed.server); err != nil {
		return fmt.Errorf("stop MCP server %s: %w", serverID, err)
	}
	return nil
}

func (r *Runtime) RestartMCPServer(
	ctx context.Context,
	serverID string,
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
	return r.StartMCPHTTPServer(ctx, profileSlug, address)
}

func (r *Runtime) ActivateProfileProviders(
	ctx context.Context,
	profile profiledomain.Profile,
) error {
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
		if err := r.EnsureConfiguredMCPProvider(ctx, providerID); err != nil {
			return fmt.Errorf(
				"activate profile provider %s: %w",
				providerID,
				err,
			)
		}
	}
	return nil
}

func (r *Runtime) EnsureConfiguredMCPProvider(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
) error {
	r.providerActivationMu.Lock()
	defer r.providerActivationMu.Unlock()

	if r.executors.HasProvider(providerID) {
		return nil
	}
	return r.ActivateConfiguredMCPProvider(ctx, providerID)
}

func (r *Runtime) serveManagedMCPServer(
	serverID string,
	managed *managedMCPServer,
) {
	_ = managed.server.Serve(managed.listener)

	r.mcpServersMu.Lock()
	if current, ok := r.mcpServers[serverID]; ok && current == managed {
		delete(r.mcpServers, serverID)
	}
	r.mcpServersMu.Unlock()
}

func (r *Runtime) closeManagedMCPServers() error {
	r.mcpServersMu.Lock()
	servers := make([]*managedMCPServer, 0, len(r.mcpServers))
	for _, server := range r.mcpServers {
		servers = append(servers, server)
	}
	r.mcpServers = make(map[string]*managedMCPServer)
	r.mcpServersMu.Unlock()

	errs := make([]error, 0, len(servers))
	for _, server := range servers {
		errs = append(errs, shutdownHTTPServer(context.Background(), server.server))
	}
	return errors.Join(errs...)
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
