package process

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	tunnelapplication "github.com/orz-i/Mender/backend/internal/tunnel/application"
)

func fixtureHealthJSON(live, ready bool, status, state, lastSuccess string) string {
	value := map[string]any{
		"live":  live,
		"ready": ready,
		"components": map[string]any{
			"control-plane": map[string]any{
				"status":  status,
				"state":   state,
				"details": map[string]any{"last_success": lastSuccess},
			},
			// This component is intentionally disabled on the native daemon.
			// Its disabled state must not suppress a successful native poll.
			"cloudflared": map[string]any{"status": "disabled", "state": "disabled"},
		},
	}
	raw, _ := json.Marshal(value)
	return string(raw)
}

func TestNativeHealthRequiresRecentSuccessfulControlPlanePoll(t *testing.T) {
	now := time.Now().UTC()
	recent := now.Add(-2 * time.Second).Format(time.RFC3339Nano)
	old := now.Add(-3 * time.Minute).Format(time.RFC3339Nano)
	tests := []struct {
		name, body, phase string
	}{
		{name: "ready with recent poll and optional cloudflared disabled",
			body: fixtureHealthJSON(true, true, "ok", "polling", recent), phase: "ready"},
		{name: "not yet ready", body: fixtureHealthJSON(true, false, "ok", "polling", recent), phase: "starting"},
		{name: "poll has not succeeded", body: fixtureHealthJSON(true, true, "ok", "polling", ""), phase: "starting"},
		{name: "failed control plane", body: fixtureHealthJSON(true, false, "degraded", "retrying", recent), phase: "degraded"},
		{name: "stale successful poll", body: fixtureHealthJSON(true, true, "ok", "polling", old), phase: "degraded"},
		{name: "unexpected control plane state", body: fixtureHealthJSON(true, true, "ok", "unknown", recent), phase: "starting"},
		{name: "not live", body: fixtureHealthJSON(false, true, "ok", "polling", recent), phase: "degraded"},
		{name: "invalid poll timestamp", body: fixtureHealthJSON(true, true, "ok", "polling", "yesterday"), phase: "degraded"},
		{name: "future poll timestamp", body: fixtureHealthJSON(true, true, "ok", "polling", now.Add(time.Hour).Format(time.RFC3339Nano)), phase: "degraded"},
		{name: "no control plane evidence", body: `{"live":true,"ready":true,"components":{}}`, phase: "degraded"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var report nativeHealthSnapshot
			if err := json.Unmarshal([]byte(test.body), &report); err != nil {
				t.Fatal(err)
			}
			if got := nativeHealthPhase(report, now); got != test.phase {
				t.Fatalf("health phase = %q, want %q", got, test.phase)
			}
		})
	}
}

func TestNativeHealthReaderRejectsUntrustedRemoteOrOversizedResponse(t *testing.T) {
	now := time.Now().UTC()
	for _, response := range []struct {
		code int
		body string
	}{
		{http.StatusServiceUnavailable, "down"},
		{http.StatusOK, "not json"},
		{http.StatusOK, strings.Repeat("X", nativeHealthMaxBody+1)},
		{http.StatusFound, "/redirect"},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/redirect" {
				t.Fatal("redirect must not be followed")
			}
			if response.code == http.StatusFound {
				w.Header().Set("Location", "http://example.com")
			}
			w.WriteHeader(response.code)
			_, _ = w.Write([]byte(response.body))
		}))
		if _, err := sampleNativeHealth(server.URL+"/health?details=true", now); err == nil {
			t.Fatalf("accepted unsafe health response %d", response.code)
		}
		server.Close()
	}
	// A response must never become an arbitrary diagnostic message: headers,
	// raw native details, URLs and credentials are not cached by the watcher.
}

func TestNativeHealthFixtureProcess(t *testing.T) {
	if os.Getenv("MENDER_NATIVE_HEALTH_FIXTURE") != "1" {
		return
	}
	address := os.Getenv("MENDER_NATIVE_HEALTH_ADDR")
	started := time.Now()
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		duration := time.Since(started)
		status, state := "starting", "starting"
		ready := false
		recent := ""
		switch {
		case duration >= 1300*time.Millisecond:
			status, state = "degraded", "retrying"
		case duration >= 350*time.Millisecond:
			status, state = "ok", "polling"
			ready = true
			recent = time.Now().UTC().Format(time.RFC3339Nano)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, fixtureHealthJSON(true, ready, status, state, recent))
	})
	listener, err := net.Listen("tcp4", address)
	if err != nil {
		os.Exit(82)
	}
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 2 * time.Second}
	_ = server.Serve(listener)
	os.Exit(0)
}

type nativeHealthFixtureLauncher struct{}

func (nativeHealthFixtureLauncher) Command(ctx context.Context, arguments []string, environment []string) (*exec.Cmd, error) {
	health := ""
	for _, argument := range arguments {
		if strings.HasPrefix(argument, "--health.listen-addr=") {
			health = strings.TrimPrefix(argument, "--health.listen-addr=")
		}
	}
	if health == "" {
		return nil, fmt.Errorf("native health address missing")
	}
	binary, err := os.Executable()
	if err != nil {
		return nil, err
	}
	command := exec.CommandContext( // #nosec G204 -- the current trusted test binary and static test target.
		ctx, binary, "-test.run=^TestNativeHealthFixtureProcess$")
	command.Env = append(os.Environ(), environment...)
	command.Env = append(command.Env,
		"MENDER_NATIVE_HEALTH_FIXTURE=1",
		"MENDER_NATIVE_HEALTH_ADDR="+health,
	)
	return command, nil
}

func waitTunnelPhase(t *testing.T, reporter tunnelapplication.HealthReporter, want string) {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if reporter.TunnelHealth() == want {
			return
		}
		time.Sleep(35 * time.Millisecond)
	}
	t.Fatalf("TunnelHealth = %q, want %q", reporter.TunnelHealth(), want)
}
func TestRunnerSamplesLiveChildWithoutCredentialsOrReplaying(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
	defer cancel()
	runner := NewRunner(nativeHealthFixtureLauncher{})
	process, err := runner.Start(ctx, testTunnel(t), "http://127.0.0.1:1234/mcp", []byte("do-not-log-secret"))
	if err != nil {
		t.Fatal(err)
	}
	child, ok := process.(*childProcess)
	if !ok {
		t.Fatalf("unexpected process %T", process)
	}
	defer func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer stopCancel()
		_ = process.Stop(stopCtx)
	}()
	if child.command.Env != nil {
		t.Fatal("native API key remains in parent command environment")
	}
	waitTunnelPhase(t, child, "ready")
	waitTunnelPhase(t, child, "degraded")
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer stopCancel()
	if err := process.Stop(stopCtx); err != nil {
		t.Fatal(err)
	}
	waitTunnelPhase(t, child, "exited")
	select {
	case <-process.Done():
	default:
		t.Fatal("native child Done channel not completed")
	}
}

func TestNativeUnobservableHealthEventuallyDegrades(t *testing.T) {
	child := &childProcess{
		exited: make(chan struct{}), healthAddress: "127.0.0.1:1",
		health: "starting", startedAt: time.Now().Add(-nativeHealthStartupGrace - time.Second),
	}
	done := make(chan struct{})
	go func() { child.watchHealth(); close(done) }()
	deadline := time.After(3 * time.Second)
	for child.TunnelHealth() != "degraded" {
		select {
		case <-deadline:
			close(child.exited)
			<-done
			t.Fatal("unobservable daemon remained in starting")
		case <-time.After(25 * time.Millisecond):
		}
	}
	close(child.exited)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("health watcher leaked after exit")
	}
}
