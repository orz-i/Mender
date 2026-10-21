package bootstrap

import (
	"context"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	profileapplication "github.com/orz-i/Mender/backend/internal/profile/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

func TestManagedMCPPortAutoRotationPreservesExistingListener(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = occupied.Close() })

	host, firstPortText, err := net.SplitHostPort(occupied.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	firstPort, err := strconv.Atoi(firstPortText)
	if err != nil {
		t.Fatal(err)
	}
	if firstPort > 65535-managedMCPPortAttempts {
		t.Skip("OS allocated a port too close to 65535 for the bounded rotation test")
	}

	runtime, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "mender.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	profile, err := runtime.Profiles.Create.Execute(ctx, profileapplication.CreateProfileRequest{
		Slug: "rotation", DisplayName: "Port rotation", Mode: profiledomain.ExposureDynamic,
	})
	if err != nil {
		t.Fatal(err)
	}

	start := func() MCPServerInstance {
		t.Helper()
		instance, startErr := startMCPHTTPServerTest(t, runtime, ctx, profile.Slug(), occupied.Addr().String())
		if startErr != nil {
			t.Fatalf("StartMCPHTTPServer: %v", startErr)
		}
		newHost, newPortText, splitErr := net.SplitHostPort(instance.Address)
		if splitErr != nil {
			t.Fatal(splitErr)
		}
		newPort, parseErr := strconv.Atoi(newPortText)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		if newHost != host || newPort <= firstPort || newPort >= firstPort+managedMCPPortAttempts {
			t.Fatalf("rotated from %q to %q, outside bounded loopback range", occupied.Addr(), instance.Address)
		}
		if instance.Endpoint != "http://"+instance.Address+"/mcp" {
			t.Fatalf("Endpoint %q does not reflect live listener %q", instance.Endpoint, instance.Address)
		}
		assertListening := func(address string) {
			conn, dialErr := net.Dial("tcp", address)
			if dialErr != nil {
				t.Fatalf("listener %q is unavailable: %v", address, dialErr)
			}
			_ = conn.Close()
		}
		assertListening(occupied.Addr().String())
		assertListening(instance.Address)
		return instance
	}

	first := start()
	// Restart uses the originally requested address, so the occupied port
	// stays intact and a fresh bound listener is returned.
	restarted, err := restartMCPServerTest(t, runtime, ctx, first.ServerID)
	if err != nil {
		t.Fatalf("RestartMCPServer: %v", err)
	}
	if restarted.Address == occupied.Addr().String() || restarted.Endpoint != "http://"+restarted.Address+"/mcp" {
		t.Fatalf("restart listener = %#v", restarted)
	}
	if current := runtime.ListMCPServers(); len(current) != 1 || current[0].ServerID != restarted.ServerID {
		t.Fatalf("unexpected managed server inventory after restart: %#v", current)
	}
	if err := runtime.StopMCPServer(ctx, restarted.ServerID); err != nil {
		t.Fatal(err)
	}
	// We never stop, close or move the listener that owned the original port.
	original, err := net.Dial("tcp", occupied.Addr().String())
	if err != nil {
		t.Fatalf("original listener not preserved: %v", err)
	}
	_ = original.Close()
}

func TestManagedMCPPortAllocationRetainsPortZeroContractAndRejectsInvalidPort(t *testing.T) {
	t.Parallel()
	listener, err := listenManagedMCPPort("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasSuffix(listener.Addr().String(), ":0") {
		t.Fatalf("port zero was not assigned: %s", listener.Addr())
	}
	_ = listener.Close()

	if invalid, err := listenManagedMCPPort("127.0.0.1:65536"); err == nil {
		_ = invalid.Close()
		t.Fatal("out of range port unexpectedly accepted")
	}
}
