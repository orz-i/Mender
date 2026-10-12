package process

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	nativeHealthInterval     = 750 * time.Millisecond
	nativeHealthTimeout      = 1500 * time.Millisecond
	nativeHealthStartupGrace = 40 * time.Second
	nativeHealthPollMaxAge   = 90 * time.Second
	nativeHealthMaxBody      = 32 << 10
)

// Use only loopback HTTP. Never route the per-process admin port through
// environment-configured HTTP(S) proxies or follow untrusted redirects.
var nativeHealthClient = &http.Client{
	Transport: &http.Transport{Proxy: nil},
	Timeout:   nativeHealthTimeout,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

type nativeHealthSnapshot struct {
	Live       bool `json:"live"`
	Ready      bool `json:"ready"`
	Components map[string]struct {
		Status  string `json:"status"`
		State   string `json:"state"`
		Details struct {
			LastSuccess string `json:"last_success"`
		} `json:"details"`
	} `json:"components"`
}

func (p *childProcess) watchHealth() {
	ticker := time.NewTicker(nativeHealthInterval)
	defer ticker.Stop()
	healthURL := "http://" + p.healthAddress + "/health?details=true"
	for {
		select {
		case <-p.exited:
			return
		default:
		}
		phase, err := sampleNativeHealth(healthURL, time.Now())
		if err != nil {
			// Absence of an observable health endpoint is not evidence of
			// readiness; after a bounded grace period it is degraded.
			phase = "starting"
		}
		if phase == "starting" && (time.Since(p.startedAt) >= nativeHealthStartupGrace ||
			p.TunnelHealth() == "ready" || p.TunnelHealth() == "degraded") {
			phase = "degraded"
		}
		p.setHealth(phase)
		select {
		case <-p.exited:
			return
		case <-ticker.C:
		}
	}
}

// sampleNativeHealth deliberately retains only typed readiness facts; raw
// diagnostics can contain host paths and process details and are discarded.
func sampleNativeHealth(url string, now time.Time) (string, error) {
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("native Tunnel health request unavailable")
	}
	response, err := nativeHealthClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("native Tunnel health endpoint unavailable")
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("native Tunnel health endpoint not ready")
	}
	var snapshot nativeHealthSnapshot
	if err := json.NewDecoder(io.LimitReader(response.Body, nativeHealthMaxBody)).Decode(&snapshot); err != nil {
		return "", fmt.Errorf("native Tunnel health response invalid")
	}
	return nativeHealthPhase(snapshot, now), nil
}

func nativeHealthPhase(snapshot nativeHealthSnapshot, now time.Time) string {
	if !snapshot.Live {
		return "degraded"
	}
	control, found := snapshot.Components["control-plane"]
	if !found {
		return "degraded"
	}
	if control.Status != "ok" {
		if control.Status == "starting" {
			return "starting"
		}
		return "degraded"
	}
	if !snapshot.Ready || control.State != "polling" || control.Details.LastSuccess == "" {
		return "starting"
	}
	lastSuccess, err := time.Parse(time.RFC3339Nano, control.Details.LastSuccess)
	if err != nil {
		return "degraded"
	}
	age := now.Sub(lastSuccess)
	if age < -5*time.Second || age > nativeHealthPollMaxAge {
		return "degraded"
	}
	return "ready"
}
