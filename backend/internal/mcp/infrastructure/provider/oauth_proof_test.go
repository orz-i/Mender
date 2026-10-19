package provider

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
)

type oauthExecutorProof struct {
	tokenResolutionAttempts int
	authorized              bool
}

func (p *oauthExecutorProof) Resolve(context.Context, credentialdomain.Reference) ([]byte, error) {
	p.tokenResolutionAttempts++
	return nil, errors.New("tokens must not be statically resolved")
}
func (p *oauthExecutorProof) AuthorizeCredential(context.Context, credentialdomain.Reference) error {
	if !p.authorized {
		return errors.New("authorization denied")
	}
	return nil
}

func TestOAuthMCPExecutorRequiresNonsecretProfileCredentialProof(t *testing.T) {
	var calls int
	connection := connectTestServer(t, func(server *mcp.Server) {
		server.AddTool(
			&mcp.Tool{Name: "secured", InputSchema: json.RawMessage(`{"type":"object"}`)},
			func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				calls++
				return &mcp.CallToolResult{StructuredContent: map[string]any{"ok": true}}, nil
			},
		)
	})
	ref, _ := credentialdomain.NewReference("oauth", "work")
	connection.setRequiredCredentials([]credentialdomain.Reference{ref})
	connection.SetOAuthCredentialRef(ref)
	input, _ := invocationdomain.NewJSONValue([]byte("{}"))
	request := invocationapplication.ExecutionRequest{Tool: executorTestTool(t, "secured"), Arguments: input}
	denied := &oauthExecutorProof{}
	if _, err := connection.Execute(context.Background(), request, denied, nil); err == nil {
		t.Fatal("unapproved OAuth Credential reached upstream")
	}
	if calls != 0 || denied.tokenResolutionAttempts != 0 {
		t.Fatal("OAuth unauthorized request used secret or reached upstream")
	}
	allowed := &oauthExecutorProof{authorized: true}
	result, err := connection.Execute(context.Background(), request, allowed, nil)
	if err != nil || result.Kind() != invocationdomain.OutcomeKindComplete || calls != 1 {
		t.Fatalf("approved OAuth proof rejected or not called: %+v %v calls=%d", result, err, calls)
	}
	if allowed.tokenResolutionAttempts != 0 {
		t.Fatal("OAuth proof resolved bearer tokens as static Credential bytes")
	}
	if _, err := connection.Execute(context.Background(), request, &executorCredentialResolver{}, nil); err == nil || calls != 1 {
		t.Fatal("OAuth executor allowed a CredentialResolver without authorizing capability")
	}
}
