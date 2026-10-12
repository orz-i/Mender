package curated

import (
	"github.com/orz-i/Mender/backend/internal/integration/domain"
)

var reviewed = []domain.Offering{{
	OfferingID:       "io.mender.integration.anysearch",
	DisplayName:      "AnySearch",
	Description:      "Search the web and retrieve pages through AnySearch's hosted MCP service.",
	Kind:             "remote-mcp",
	DriverPluginID:   "io.mender.provider.mcp",
	Transport:        "streamable-http",
	Endpoint:         "https://api.anysearch.com/mcp",
	Headers:          map[string]string{"X-Anysearch-Client": "mcp/1.0.0"},
	DefaultNamespace: "anysearch",
	Authentication:   "optional-bearer",
	SourceURL:        "https://github.com/anysearch-ai/anysearch-mcp-server",
}}

// List only copies reviewed inert metadata. It does not probe, activate,
// provision or resolve secrets.
func List() []domain.Offering {
	out := make([]domain.Offering, 0, len(reviewed))
	for _, offering := range reviewed {
		out = append(out, offering.Clone())
	}
	return out
}
