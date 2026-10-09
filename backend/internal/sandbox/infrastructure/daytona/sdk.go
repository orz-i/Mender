package daytona

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	sdkdaytona "github.com/daytona/clients/sdk-go/pkg/daytona"
	sdkerrors "github.com/daytona/clients/sdk-go/pkg/errors"
	"github.com/daytona/clients/sdk-go/pkg/options"
	"github.com/daytona/clients/sdk-go/pkg/types"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

// SDKFactory constructs short-lived explicit-key SDK clients only after
// sandbox.provision or sandbox.acquire has been admitted. No caller/LLM can
// alter the vendor host, target, credential inventory or network policy.
type SDKFactory struct {
	resolver invocationapplication.CredentialResolver
}

func NewSDKFactory(resolver invocationapplication.CredentialResolver) (*SDKFactory, error) {
	if resolver == nil {
		return nil, fmt.Errorf("daytona SDK requires Mender Credential Resolver")
	}
	return &SDKFactory{resolver: resolver}, nil
}
func (f *SDKFactory) Connect(ctx context.Context, t Template) (RemoteClient, error) {
	if f == nil || f.resolver == nil {
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	if err := t.Validate(); err != nil {
		return nil, err
	}
	// The SDK reads DAYTONA_JWT_TOKEN even when APIKey is explicit. Fail closed
	// on ambient alternative authority instead of inheriting it from host env.
	if os.Getenv("DAYTONA_JWT_TOKEN") != "" || os.Getenv("DAYTONA_ORGANIZATION_ID") != "" {
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	secret, err := f.resolver.Resolve(ctx, credentialdomain.Reference(t.CredentialRef.String()))
	if err != nil {
		return nil, fmt.Errorf("daytona Credential Reference unavailable: %w", sandboxdomain.ErrNotAuthorized)
	}
	defer func() {
		for i := range secret {
			secret[i] = 0
		}
	}()
	if len(secret) < 12 || len(secret) > 4096 || strings.TrimSpace(string(secret)) != string(secret) {
		return nil, fmt.Errorf("daytona credential invalid")
	}
	duration := 45 * time.Second
	httpClient := &http.Client{Timeout: duration, Transport: vendorTransport{base: http.DefaultTransport}, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return fmt.Errorf("daytona redirects disabled") }}
	client, err := sdkdaytona.NewClientWithConfig(&types.DaytonaConfig{
		APIKey: string(secret), APIUrl: APIURL, Target: t.Target, OtelEnabled: false,
		Timeout: &duration, HTTPClient: httpClient,
	})
	if err != nil {
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	return &sdkClient{client: client}, nil
}

// vendorTransport limits even remote SDK toolbox-proxy discovery to TLS
// endpoints controlled by Daytona. Do not allow redirects into localhost/host.
type vendorTransport struct{ base http.RoundTripper }

func (v vendorTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request == nil || request.URL == nil {
		return nil, fmt.Errorf("daytona URL unavailable")
	}
	u := request.URL
	host := strings.ToLower(u.Hostname())
	if u.Scheme != "https" || u.User != nil || (u.Port() != "" && u.Port() != "443") || (host != "app.daytona.io" && !strings.HasSuffix(host, ".daytona.io")) {
		return nil, fmt.Errorf("daytona SDK requested an unapproved vendor hostname")
	}
	return v.base.RoundTrip(request)
}

type sdkClient struct{ client *sdkdaytona.Client }

func (c *sdkClient) Close(ctx context.Context) error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Close(ctx)
}
func (c *sdkClient) Create(ctx context.Context, t Template, name string, labels map[string]string) (RemoteSandbox, error) {
	if err := t.Validate(); err != nil {
		return nil, err
	}
	ttl := t.TTLMinutes
	stop := min(ttl, 30)
	deleteInterval := ttl
	queue := 2
	result, err := c.client.Create(ctx, types.ImageParams{
		SandboxBaseParams: types.SandboxBaseParams{
			Name: name, Labels: labels, NetworkBlockAll: true, Public: false,
			TtlMinutes: &ttl, AutoStopInterval: &stop, AutoDeleteInterval: &deleteInterval, QueueTimeout: &queue,
		}, Image: t.ImageDigest, Resources: &types.Resources{CPU: t.CPU, Memory: t.MemoryGiB, Disk: t.DiskGiB, GPU: 0},
	}, options.WithWaitForStart(true), options.WithTimeout(90*time.Second))
	if err != nil {
		return nil, safeRemoteError(ctx, err)
	}
	if result == nil {
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	return &sdkSandbox{inner: result}, nil
}
func (c *sdkClient) Get(ctx context.Context, id string) (RemoteSandbox, error) {
	if err := ValidateExternalID(id); err != nil {
		return nil, err
	}
	result, err := c.client.Get(ctx, id)
	if err != nil {
		return nil, safeRemoteError(ctx, err)
	}
	if result == nil {
		return nil, sandboxdomain.ErrNotFound
	}
	return &sdkSandbox{inner: result}, nil
}
func (c *sdkClient) FindByLabel(ctx context.Context, labels map[string]string) ([]RemoteSandbox, error) {
	iterator := c.client.List(ctx, &sdkdaytona.ListSandboxesQuery{Labels: labels})
	matches := make([]RemoteSandbox, 0, 2)
	for iterator.Next() {
		if len(matches) >= 2 {
			return nil, sandboxdomain.ErrConflict
		}
		value := iterator.Value()
		if value == nil {
			return nil, sandboxdomain.ErrBackendUnavailable
		}
		matches = append(matches, &sdkSandbox{inner: value})
	}
	if err := iterator.Err(); err != nil {
		return nil, safeRemoteError(ctx, err)
	}
	return matches, nil
}

type sdkSandbox struct{ inner *sdkdaytona.Sandbox }

func (s *sdkSandbox) Snapshot(ctx context.Context) (RemoteSnapshot, error) {
	if err := s.inner.RefreshData(ctx); err != nil {
		return RemoteSnapshot{}, safeRemoteError(ctx, err)
	}
	v := s.inner
	blocked := v.NetworkBlockAll != nil && *v.NetworkBlockAll
	return RemoteSnapshot{ID: v.ID, State: string(v.State), Labels: v.Labels, CPU: int(v.Cpu), MemoryGiB: int(v.Memory), DiskGiB: int(v.Disk), Target: v.Target, NetworkBlocked: blocked, Public: v.Public, TTLActive: v.AutoDestroyAt != nil && *v.AutoDestroyAt != ""}, nil
}
func (s *sdkSandbox) WorkingDir(ctx context.Context) (string, error) {
	return s.inner.GetWorkingDir(ctx)
}
func (s *sdkSandbox) Exec(ctx context.Context, command, cwd string, timeout time.Duration) (RemoteExec, error) {
	out, err := s.inner.Process.ExecuteCommand(ctx, command, options.WithCwd(cwd), options.WithExecuteTimeout(timeout))
	if err != nil {
		return RemoteExec{}, safeRemoteError(ctx, err)
	}
	if out == nil {
		return RemoteExec{}, sandboxdomain.ErrBackendUnavailable
	}
	return RemoteExec{ExitCode: out.ExitCode, Output: out.Result}, nil
}
func (s *sdkSandbox) FileInfo(ctx context.Context, filename string) (RemoteFile, error) {
	file, err := s.inner.FileSystem.GetFileInfo(ctx, filename)
	if err != nil {
		return RemoteFile{}, safeRemoteError(ctx, err)
	}
	if file == nil {
		return RemoteFile{}, sandboxdomain.ErrNotFound
	}
	return RemoteFile{Name: file.Name, Path: file.Path, Size: file.Size, Mode: file.Mode, IsDirectory: file.IsDirectory}, nil
}
func (s *sdkSandbox) ListFiles(ctx context.Context, filename string, depth int) ([]RemoteFile, error) {
	if depth < 1 || depth > 4 {
		return nil, sandboxdomain.ErrCapabilityUnavailable
	}
	files, err := s.inner.FileSystem.ListFiles(ctx, filename, options.WithDepth(int32(depth)))
	if err != nil {
		return nil, safeRemoteError(ctx, err)
	}
	if len(files) > maxRemoteCalls {
		return nil, sandboxdomain.ErrCapabilityUnavailable
	}
	result := make([]RemoteFile, 0, len(files))
	for _, file := range files {
		if file != nil {
			result = append(result, RemoteFile{Name: file.Name, Path: file.Path, Size: file.Size, Mode: file.Mode, IsDirectory: file.IsDirectory})
		}
	}
	return result, nil
}
func (s *sdkSandbox) ReadStream(ctx context.Context, filename string) (io.ReadCloser, error) {
	result, err := s.inner.FileSystem.DownloadFileStream(ctx, filename)
	if err != nil {
		return nil, safeRemoteError(ctx, err)
	}
	return result, nil
}
func (s *sdkSandbox) UploadStream(ctx context.Context, source io.Reader, filename string) error {
	return safeRemoteError(ctx, s.inner.FileSystem.UploadFileStream(ctx, source, filename))
}
func (s *sdkSandbox) Move(ctx context.Context, from, to string) error {
	return safeRemoteError(ctx, s.inner.FileSystem.MoveFiles(ctx, from, to))
}
func (s *sdkSandbox) DeleteFile(ctx context.Context, filename string) error {
	return safeRemoteError(ctx, s.inner.FileSystem.DeleteFile(ctx, filename, false))
}
func (s *sdkSandbox) Start(ctx context.Context) error {
	return safeRemoteError(ctx, s.inner.Start(ctx))
}
func (s *sdkSandbox) Stop(ctx context.Context) error { return safeRemoteError(ctx, s.inner.Stop(ctx)) }
func (s *sdkSandbox) Destroy(ctx context.Context) error {
	return safeRemoteError(ctx, s.inner.DeleteAndWait(ctx, 60*time.Second))
}
func (s *sdkSandbox) SetLabels(ctx context.Context, labels map[string]string) error {
	return safeRemoteError(ctx, s.inner.SetLabels(ctx, labels))
}

var safeDaytonaErrorCode = regexp.MustCompile(`^[A-Z0-9_-]{1,48}$`)

// Preserve only a bounded numeric status and validated machine code so real
// API problems are diagnosable without exposing remote error bodies, URLs,
// request headers, Authorization values or credential material.
func safeRemoteError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var remote *sdkerrors.DaytonaError
	if errors.As(err, &remote) {
		code := ""
		if safeDaytonaErrorCode.MatchString(remote.Code) {
			code = remote.Code
		}
		switch remote.StatusCode {
		case 401, 403:
			return fmt.Errorf("daytona remote authorization denied (HTTP %d): %w", remote.StatusCode, sandboxdomain.ErrNotAuthorized)
		case 404:
			return fmt.Errorf("daytona remote resource unavailable (HTTP 404): %w", sandboxdomain.ErrNotFound)
		default:
			if remote.StatusCode >= 100 && remote.StatusCode <= 599 {
				if code != "" {
					return fmt.Errorf("daytona remote request rejected (HTTP %d, code %s): %w", remote.StatusCode, code, sandboxdomain.ErrBackendUnavailable)
				}
				return fmt.Errorf("daytona remote request rejected (HTTP %d): %w", remote.StatusCode, sandboxdomain.ErrBackendUnavailable)
			}
			if code != "" {
				return fmt.Errorf("daytona remote SDK rejected request (code %s): %w", code, sandboxdomain.ErrBackendUnavailable)
			}
		}
	}
	return sandboxdomain.ErrBackendUnavailable
}

var _ Factory = (*SDKFactory)(nil)
var _ RemoteClient = (*sdkClient)(nil)
var _ RemoteSandbox = (*sdkSandbox)(nil)
