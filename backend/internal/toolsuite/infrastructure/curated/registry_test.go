package curated

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
	suite "github.com/orz-i/Mender/backend/internal/toolsuite/domain"
)

func TestToolSuiteCapabilitiesFailClosedForBrowserOnlyAndPartialWorkspace(t *testing.T) {
	values := List()
	if len(values) != 3 {
		t.Fatalf("suite inventory %d", len(values))
	}
	for _, v := range values {
		if err := v.Validate(); err != nil {
			t.Fatalf("%s: %v", v.SuiteID, err)
		}
	}
	workspace, _ := Find(suite.SandboxWorkspace)
	browser, _ := Find(suite.SandboxBrowser)
	agent, _ := Find(suite.AgentDelegation)
	required := inlineContracts(suite.FileRead, suite.FileSearch, suite.FileEdit, suite.ProcessShell)
	if err := suite.RequireContracts(workspace, suite.SandboxResource, required); err != nil {
		t.Fatal(err)
	}
	for _, actual := range [][]platformplugin.CapabilityContract{
		inlineContracts(suite.BrowserSession),
		inlineContracts(suite.FileRead, suite.FileSearch),
		inlineContracts(suite.FileRead, suite.FileSearch, suite.FileEdit),
		inlineContracts(suite.BrowserSession, suite.ProcessShell),
		{{ID: "job.submit", Version: platformplugin.CapabilityVersion{Major: 1}, Mode: platformplugin.ExecutionDetached}},
	} {
		if err := suite.RequireContracts(workspace, suite.SandboxResource, actual); !errors.Is(err, suite.ErrIncompatible) {
			t.Fatalf("workspace must reject %v: %v", actual, err)
		}
	}
	if err := suite.RequireContracts(workspace, suite.AgentResource, required); !errors.Is(err, suite.ErrIncompatible) {
		t.Fatalf("wrong resource kind: %v", err)
	}
	if err := suite.RequireContracts(browser, suite.SandboxResource, required); !errors.Is(err, suite.ErrUnavailable) {
		t.Fatalf("unimplemented browser suite accepted: %v", err)
	}
	if err := suite.RequireContracts(agent, suite.AgentResource, []platformplugin.CapabilityContract{{ID: suite.AgentTask,
		Version: platformplugin.CapabilityVersion{Major: 1}, Mode: platformplugin.ExecutionDetached}}); err != nil {
		t.Fatal(err)
	}
	if len(browser.ToolSourceKeys) != 0 {
		t.Fatal("unimplemented Browser advertised tools")
	}
	// Public Tool Suite JSON remains the existing ID-only management contract.
	// Contract versions/traits are host-internal eligibility metadata.
	encoded, err := json.Marshal(workspace)
	if err != nil || !strings.Contains(string(encoded), `"requiredCapabilities"`) ||
		strings.Contains(string(encoded), "requiredContracts") || strings.Contains(string(encoded), "guarantees") {
		t.Fatalf("management projection changed: %s %v", encoded, err)
	}
	values[0].ToolSourceKeys[0] = "malicious"
	values[0].RequiredContracts[0].ID = "evil.task"
	next, _ := Find(suite.AgentDelegation)
	if slices.Contains(next.ToolSourceKeys, "malicious") || next.RequiredContracts[0].ID != suite.AgentTask {
		t.Fatal("catalogue mutation")
	}
}
