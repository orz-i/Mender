package curated

import (
	"errors"
	"slices"
	"testing"

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
	required := []suite.Capability{suite.FileRead, suite.FileSearch, suite.FileEdit, suite.ProcessShell}
	if err := suite.RequireCapabilities(workspace, suite.SandboxResource, required); err != nil {
		t.Fatal(err)
	}
	for _, actual := range [][]suite.Capability{
		{suite.BrowserSession},
		{suite.FileRead, suite.FileSearch},
		{suite.FileRead, suite.FileSearch, suite.FileEdit},
		{suite.BrowserSession, suite.ProcessShell},
	} {
		if err := suite.RequireCapabilities(workspace, suite.SandboxResource, actual); !errors.Is(err, suite.ErrIncompatible) {
			t.Fatalf("workspace must reject %v: %v", actual, err)
		}
	}
	if err := suite.RequireCapabilities(workspace, suite.AgentResource, required); !errors.Is(err, suite.ErrIncompatible) {
		t.Fatalf("wrong resource kind: %v", err)
	}
	if err := suite.RequireCapabilities(browser, suite.SandboxResource, required); !errors.Is(err, suite.ErrUnavailable) {
		t.Fatalf("unimplemented browser suite accepted: %v", err)
	}
	if err := suite.RequireCapabilities(agent, suite.AgentResource, []suite.Capability{suite.AgentTask}); err != nil {
		t.Fatal(err)
	}
	if len(browser.ToolSourceKeys) != 0 {
		t.Fatal("unimplemented Browser advertised tools")
	}
	values[0].ToolSourceKeys[0] = "malicious"
	next, _ := Find(suite.AgentDelegation)
	if slices.Contains(next.ToolSourceKeys, "malicious") {
		t.Fatal("catalogue mutation")
	}
}
