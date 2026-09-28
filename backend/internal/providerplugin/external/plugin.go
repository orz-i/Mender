package external

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	installationdomain "github.com/orz-i/Mender/backend/internal/providerplugin/installation/domain"
	providerv1 "github.com/orz-i/Mender/sdk/providerplugin/go/protocol/v1"
)

const (
	maxExternalConfigBytes     = 4 * 1024 * 1024
	maxDiagnostics             = 64
	maxDiagnosticPathBytes     = 512
	maxDiagnosticCodeBytes     = 128
	maxDiagnosticMessageBytes  = 4096
	maxCredentialRefs          = 128
	maxSessionIDBytes          = 512
	maxExternalTools           = 4096
	maxSourceKeyBytes          = 1024
	maxToolNameBytes           = 255
	maxToolTitleBytes          = 255
	maxToolDescriptionBytes    = 16 * 1024
	maxToolSchemaBytes         = 1024 * 1024
	maxToolTags                = 64
	maxToolTagBytes            = 64
	maxExecutionEvents         = 4096
	maxConnectionSummaryBytes  = 4096
	instanceRandomBytes        = 16
	defaultSessionCloseTimeout = 5 * time.Second
)

type processProvider interface {
	Ensure(context.Context, installationdomain.Installation) (*Process, error)
}

// Plugin adapts one installed external package to Mender's internal Provider plugin port.
type Plugin struct {
	installation installationdomain.Installation
	processes    processProvider
	descriptor   providerplugin.Descriptor
}

// NewPlugin probes an installed package once and snapshots its verified descriptor.
func NewPlugin(
	ctx context.Context,
	installation installationdomain.Installation,
	processes processProvider,
) (*Plugin, error) {
	if processes == nil {
		return nil, fmt.Errorf("new external provider plugin: process provider must not be nil")
	}
	if err := installation.Validate(); err != nil {
		return nil, fmt.Errorf("new external provider plugin: installation: %w", err)
	}
	process, err := processes.Ensure(ctx, installation)
	if err != nil {
		return nil, fmt.Errorf("new external provider plugin: process: %w", err)
	}
	raw := process.Descriptor()
	if raw == nil {
		return nil, fmt.Errorf("new external provider plugin: verified descriptor is missing")
	}
	if raw.GetConfigVersion() > math.MaxInt32 {
		return nil, fmt.Errorf(
			"new external provider plugin: config version %d is too large",
			raw.GetConfigVersion(),
		)
	}
	id, err := providerplugin.ParseID(raw.GetPluginId())
	if err != nil {
		return nil, fmt.Errorf("new external provider plugin: descriptor: %w", err)
	}
	descriptor := providerplugin.Descriptor{
		ID:              id,
		Version:         raw.GetPackageVersion(),
		DisplayName:     raw.GetDisplayName(),
		Description:     raw.GetDescription(),
		ConfigVersion:   int(raw.GetConfigVersion()), // #nosec G115 -- bounded above by MaxInt32.
		ProtocolVersion: raw.GetProtocolVersion(),
		ConfigSchema:    append(json.RawMessage(nil), raw.GetConfigSchemaJson()...),
		UISchema:        append(json.RawMessage(nil), raw.GetUiSchemaJson()...),
		Capabilities:    append([]string(nil), raw.GetCapabilities()...),
	}
	if err := descriptor.Validate(); err != nil {
		return nil, fmt.Errorf("new external provider plugin: descriptor: %w", err)
	}
	if descriptor.ID != installation.PluginID() ||
		descriptor.Version != installation.PackageVersion() {
		return nil, fmt.Errorf("new external provider plugin: descriptor identity mismatch")
	}
	return &Plugin{
		installation: installation,
		processes:    processes,
		descriptor:   descriptor,
	}, nil
}

// Descriptor implements providerplugin.Plugin without activating new Provider sessions.
func (p *Plugin) Descriptor() providerplugin.Descriptor {
	if p == nil {
		return providerplugin.Descriptor{}
	}
	return p.descriptor.Clone()
}

// ValidateConfig delegates plugin-owned semantic validation across protocol v1.
func (p *Plugin) ValidateConfig(
	ctx context.Context,
	raw json.RawMessage,
) error {
	if p == nil {
		return fmt.Errorf("validate external provider config: plugin is nil")
	}
	if err := validateExternalConfig(raw); err != nil {
		return err
	}
	process, err := p.processes.Ensure(ctx, p.installation)
	if err != nil {
		return fmt.Errorf("validate external provider config: process: %w", err)
	}
	response, err := process.Service().ValidateConfig(
		ctx,
		&providerv1.ValidateConfigRequest{
			ConfigVersion: uint32(p.descriptor.ConfigVersion), // #nosec G115 -- descriptor config version is positive and bounded during construction.
			ConfigJson:    append([]byte(nil), raw...),
		},
	)
	if err != nil {
		return fmt.Errorf("validate external provider config: rpc: %w", err)
	}
	return validationResponseError(response)
}

// InspectConfig maps bounded Credential Reference metadata into the internal contract.
func (p *Plugin) InspectConfig(
	ctx context.Context,
	raw json.RawMessage,
) (providerplugin.ConfigMetadata, error) {
	if p == nil {
		return providerplugin.ConfigMetadata{}, fmt.Errorf(
			"inspect external provider config: plugin is nil",
		)
	}
	if err := validateExternalConfig(raw); err != nil {
		return providerplugin.ConfigMetadata{}, err
	}
	process, err := p.processes.Ensure(ctx, p.installation)
	if err != nil {
		return providerplugin.ConfigMetadata{}, fmt.Errorf(
			"inspect external provider config: process: %w",
			err,
		)
	}
	response, err := process.Service().InspectConfig(
		ctx,
		&providerv1.InspectConfigRequest{
			ConfigVersion: uint32(p.descriptor.ConfigVersion), // #nosec G115 -- bounded during construction.
			ConfigJson:    append([]byte(nil), raw...),
		},
	)
	if err != nil {
		return providerplugin.ConfigMetadata{}, fmt.Errorf(
			"inspect external provider config: rpc: %w",
			err,
		)
	}
	if response == nil {
		return providerplugin.ConfigMetadata{}, fmt.Errorf(
			"inspect external provider config: empty response",
		)
	}
	if len(response.GetConnectionSummary()) > maxConnectionSummaryBytes {
		return providerplugin.ConfigMetadata{}, fmt.Errorf(
			"inspect external provider config: connection summary exceeds %d bytes",
			maxConnectionSummaryBytes,
		)
	}
	refs, err := parseDeclaredCredentialRefs(raw, response.GetCredentialRefs())
	if err != nil {
		return providerplugin.ConfigMetadata{}, fmt.Errorf(
			"inspect external provider config: %w",
			err,
		)
	}
	metadata, err := providerplugin.NewConfigMetadata(refs)
	if err != nil {
		return providerplugin.ConfigMetadata{}, fmt.Errorf(
			"inspect external provider config: %w",
			err,
		)
	}
	return metadata, nil
}

// Open creates one external Provider session and immutable Tool snapshot.
func (p *Plugin) Open(
	ctx context.Context,
	request providerplugin.OpenRequest,
) (providerplugin.Instance, error) {
	if p == nil {
		return nil, fmt.Errorf("open external provider: plugin is nil")
	}
	if request.ConfigVersion != p.descriptor.ConfigVersion {
		return nil, fmt.Errorf(
			"open external provider: config version %d does not match plugin version %d",
			request.ConfigVersion,
			p.descriptor.ConfigVersion,
		)
	}
	if err := p.ValidateConfig(ctx, request.Config); err != nil {
		return nil, err
	}
	metadata, err := p.InspectConfig(ctx, request.Config)
	if err != nil {
		return nil, err
	}
	process, err := p.processes.Ensure(ctx, p.installation)
	if err != nil {
		return nil, fmt.Errorf("open external provider: process: %w", err)
	}
	instanceID, err := newInstanceID(request.ProviderID)
	if err != nil {
		return nil, fmt.Errorf("open external provider: instance id: %w", err)
	}
	discoveryCredentials, err := resolveCredentials(
		ctx,
		metadata.CredentialRefs,
		request.Credentials,
	)
	if err != nil {
		return nil, fmt.Errorf("open external provider: discovery credentials: %w", err)
	}
	defer zeroProtocolCredentials(discoveryCredentials)

	response, err := process.Service().Open(
		ctx,
		&providerv1.OpenRequest{
			InstanceId:           instanceID,
			ConfigVersion:        uint32(request.ConfigVersion), // #nosec G115 -- request version equals the bounded descriptor version.
			ConfigJson:           append([]byte(nil), request.Config...),
			DiscoveryCredentials: discoveryCredentials,
		},
	)
	if err != nil {
		return nil, fmt.Errorf(
			"open external provider: rpc: %s",
			sanitizeCredentialText(err.Error(), discoveryCredentials),
		)
	}
	if response == nil {
		return nil, fmt.Errorf("open external provider: empty response")
	}
	sessionID := strings.TrimSpace(response.GetSessionId())
	if sessionID == "" {
		return nil, fmt.Errorf("open external provider: session id must not be empty")
	}
	if len(sessionID) > maxSessionIDBytes {
		return nil, fmt.Errorf(
			"open external provider: session id exceeds %d bytes",
			maxSessionIDBytes,
		)
	}

	toolsResponse, err := process.Service().ListTools(
		ctx,
		&providerv1.ListToolsRequest{
			SessionId:            sessionID,
			DiscoveryCredentials: discoveryCredentials,
		},
	)
	if err != nil {
		closeErr := closeSession(context.WithoutCancel(ctx), process.Service(), sessionID)
		return nil, errors.Join(
			fmt.Errorf(
				"open external provider: list tools: %s",
				sanitizeCredentialText(err.Error(), discoveryCredentials),
			),
			closeErr,
		)
	}
	tools, bindings, err := mapTools(
		toolsResponse,
		metadata.CredentialRefs,
	)
	if err != nil {
		closeErr := closeSession(context.WithoutCancel(ctx), process.Service(), sessionID)
		return nil, errors.Join(
			fmt.Errorf("open external provider: tools: %w", err),
			closeErr,
		)
	}
	return &Instance{
		process:     process,
		service:     process.Service(),
		sessionID:   sessionID,
		tools:       tools,
		credentials: bindings,
	}, nil
}

// Instance is one configured Provider session within a reusable external package process.
type Instance struct {
	process *Process
	service providerv1.ProviderPluginServiceClient

	mu          sync.Mutex
	closed      bool
	sessionID   string
	tools       []catalogapplication.DiscoveredTool
	credentials map[string][]credentialdomain.Reference
}

// DiscoverTools implements catalog ToolSource from the immutable external snapshot.
func (i *Instance) DiscoverTools(
	_ context.Context,
	_ catalogdomain.Provider,
) ([]catalogapplication.DiscoveredTool, error) {
	if i == nil {
		return nil, fmt.Errorf("discover external provider tools: instance is nil")
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.closed {
		return nil, fmt.Errorf("discover external provider tools: instance is closed")
	}
	return cloneDiscoveredTools(i.tools), nil
}

// Execute implements invocation ToolExecutor through the external protocol stream.
func (i *Instance) Execute(
	ctx context.Context,
	request invocationapplication.ExecutionRequest,
	resolver invocationapplication.CredentialResolver,
	progress invocationapplication.ProgressReporter,
) (invocationdomain.Outcome, error) {
	if i == nil {
		return invocationdomain.Outcome{}, fmt.Errorf(
			"execute external provider tool: instance is nil",
		)
	}
	i.mu.Lock()
	if i.closed {
		i.mu.Unlock()
		return invocationdomain.Outcome{}, fmt.Errorf(
			"execute external provider tool: instance is closed",
		)
	}
	sessionID := i.sessionID
	process := i.process
	service := i.service
	sourceKey := request.Tool.SourceKey()
	required, exists := i.credentials[sourceKey]
	required = slices.Clone(required)
	i.mu.Unlock()

	if !exists {
		return invocationdomain.Outcome{}, fmt.Errorf(
			"execute external provider tool %q: tool binding not found",
			sourceKey,
		)
	}
	if process == nil || process.Exited() {
		return invocationdomain.Outcome{}, fmt.Errorf(
			"execute external provider tool %q: plugin process is unavailable",
			sourceKey,
		)
	}
	credentials, err := resolveCredentials(ctx, required, resolver)
	if err != nil {
		return invocationdomain.Outcome{}, fmt.Errorf(
			"execute external provider tool %q: credentials: %w",
			sourceKey,
			err,
		)
	}
	defer zeroProtocolCredentials(credentials)

	stream, err := service.ExecuteTool(
		ctx,
		&providerv1.ExecuteToolRequest{
			SessionId:     sessionID,
			InvocationId:  request.InvocationID.String(),
			SourceKey:     sourceKey,
			ArgumentsJson: request.Arguments.Bytes(),
			Credentials:   credentials,
		},
	)
	if err != nil {
		return invocationdomain.Outcome{}, fmt.Errorf(
			"execute external provider tool %q: rpc: %s",
			sourceKey,
			sanitizeCredentialText(err.Error(), credentials),
		)
	}

	var terminal *invocationdomain.Outcome
	for eventCount := 0; ; eventCount++ {
		if eventCount >= maxExecutionEvents {
			return invocationdomain.Outcome{}, fmt.Errorf(
				"execute external provider tool %q: event limit exceeded",
				sourceKey,
			)
		}
		event, recvErr := stream.Recv()
		if errors.Is(recvErr, io.EOF) {
			break
		}
		if recvErr != nil {
			return invocationdomain.Outcome{}, fmt.Errorf(
				"execute external provider tool %q: receive: %s",
				sourceKey,
				sanitizeCredentialText(recvErr.Error(), credentials),
			)
		}
		if event == nil {
			return invocationdomain.Outcome{}, fmt.Errorf(
				"execute external provider tool %q: nil execution event",
				sourceKey,
			)
		}
		if progressEvent := event.GetProgress(); progressEvent != nil {
			if terminal != nil {
				return invocationdomain.Outcome{}, fmt.Errorf(
					"execute external provider tool %q: progress after terminal event",
					sourceKey,
				)
			}
			update, updateErr := invocationdomain.NewProgressUpdate(
				progressEvent.GetMessage(),
			)
			if updateErr != nil {
				return invocationdomain.Outcome{}, fmt.Errorf(
					"execute external provider tool %q: invalid progress: %w",
					sourceKey,
					updateErr,
				)
			}
			if progress != nil {
				progress.Report(update)
			}
			continue
		}
		if terminal != nil {
			return invocationdomain.Outcome{}, fmt.Errorf(
				"execute external provider tool %q: multiple terminal events",
				sourceKey,
			)
		}
		outcome, outcomeErr := protocolOutcome(event, credentials)
		if outcomeErr != nil {
			return invocationdomain.Outcome{}, fmt.Errorf(
				"execute external provider tool %q: %w",
				sourceKey,
				outcomeErr,
			)
		}
		terminal = &outcome
	}
	if terminal == nil {
		return invocationdomain.Outcome{}, fmt.Errorf(
			"execute external provider tool %q: stream ended without terminal event",
			sourceKey,
		)
	}
	return *terminal, nil
}

// Close releases only this Provider session; the Supervisor owns the package process.
func (i *Instance) Close() error {
	if i == nil {
		return nil
	}
	i.mu.Lock()
	if i.closed {
		i.mu.Unlock()
		return nil
	}
	i.closed = true
	service := i.service
	sessionID := i.sessionID
	i.mu.Unlock()

	if service == nil || strings.TrimSpace(sessionID) == "" {
		return nil
	}
	if err := closeSession(context.Background(), service, sessionID); err != nil {
		return fmt.Errorf("close external provider session: %w", err)
	}
	return nil
}

func validationResponseError(response *providerv1.ValidateConfigResponse) error {
	if response == nil {
		return fmt.Errorf("validate external provider config: empty response")
	}
	diagnostics := response.GetDiagnostics()
	if len(diagnostics) > maxDiagnostics {
		return fmt.Errorf(
			"validate external provider config: diagnostics exceed %d entries",
			maxDiagnostics,
		)
	}
	var firstError string
	for index, diagnostic := range diagnostics {
		if diagnostic == nil {
			return fmt.Errorf(
				"validate external provider config: diagnostic %d is nil",
				index,
			)
		}
		if len(diagnostic.GetPath()) > maxDiagnosticPathBytes ||
			len(diagnostic.GetCode()) > maxDiagnosticCodeBytes ||
			len(diagnostic.GetMessage()) > maxDiagnosticMessageBytes {
			return fmt.Errorf(
				"validate external provider config: diagnostic %d exceeds limits",
				index,
			)
		}
		switch diagnostic.GetSeverity() {
		case providerv1.Diagnostic_SEVERITY_ERROR:
			if firstError == "" {
				message := strings.TrimSpace(diagnostic.GetMessage())
				if message == "" {
					message = "plugin rejected configuration"
				}
				if path := strings.TrimSpace(diagnostic.GetPath()); path != "" {
					message = path + ": " + message
				}
				firstError = message
			}
		case providerv1.Diagnostic_SEVERITY_WARNING:
		case providerv1.Diagnostic_SEVERITY_UNSPECIFIED:
			return fmt.Errorf(
				"validate external provider config: diagnostic %d has unspecified severity",
				index,
			)
		default:
			return fmt.Errorf(
				"validate external provider config: diagnostic %d has unknown severity %d",
				index,
				diagnostic.GetSeverity(),
			)
		}
	}
	if !response.GetValid() {
		if firstError == "" {
			firstError = "plugin rejected configuration"
		}
		return fmt.Errorf("validate external provider config: %s", firstError)
	}
	if firstError != "" {
		return fmt.Errorf(
			"validate external provider config: response marked valid but contains error diagnostic: %s",
			firstError,
		)
	}
	return nil
}

func validateExternalConfig(raw json.RawMessage) error {
	if len(raw) == 0 {
		return fmt.Errorf("external provider config: must not be empty")
	}
	if len(raw) > maxExternalConfigBytes {
		return fmt.Errorf(
			"external provider config: exceeds %d bytes",
			maxExternalConfigBytes,
		)
	}
	if !json.Valid(raw) {
		return fmt.Errorf("external provider config: must be valid JSON")
	}
	return nil
}

func parseDeclaredCredentialRefs(
	raw json.RawMessage,
	values []string,
) ([]credentialdomain.Reference, error) {
	if len(values) > maxCredentialRefs {
		return nil, fmt.Errorf(
			"credential references exceed %d entries",
			maxCredentialRefs,
		)
	}
	var config any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("decode config for credential inspection: %w", err)
	}
	refs := make([]credentialdomain.Reference, 0, len(values))
	seen := make(map[credentialdomain.Reference]struct{}, len(values))
	for _, value := range values {
		ref, err := credentialdomain.ParseReference(value)
		if err != nil {
			return nil, fmt.Errorf("invalid credential reference %q: %w", value, err)
		}
		if !jsonContainsExactString(config, ref.String()) {
			return nil, fmt.Errorf(
				"credential reference %s is not declared in persisted config",
				ref,
			)
		}
		if _, exists := seen[ref]; exists {
			continue
		}
		seen[ref] = struct{}{}
		refs = append(refs, ref)
	}
	slices.SortFunc(refs, func(a, b credentialdomain.Reference) int {
		return strings.Compare(a.String(), b.String())
	})
	return refs, nil
}

func jsonContainsExactString(value any, target string) bool {
	switch typed := value.(type) {
	case string:
		return typed == target
	case []any:
		for _, item := range typed {
			if jsonContainsExactString(item, target) {
				return true
			}
		}
	case map[string]any:
		for _, item := range typed {
			if jsonContainsExactString(item, target) {
				return true
			}
		}
	}
	return false
}

func resolveCredentials(
	ctx context.Context,
	refs []credentialdomain.Reference,
	resolver invocationapplication.CredentialResolver,
) ([]*providerv1.Credential, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	if resolver == nil {
		return nil, fmt.Errorf("credential resolver is unavailable")
	}
	result := make([]*providerv1.Credential, 0, len(refs))
	for _, ref := range refs {
		secret, err := resolver.Resolve(ctx, ref)
		if err != nil {
			zeroProtocolCredentials(result)
			return nil, fmt.Errorf("resolve %s: %w", ref, err)
		}
		result = append(result, &providerv1.Credential{
			Reference: ref.String(),
			Value:     secret,
		})
	}
	return result, nil
}

func zeroProtocolCredentials(values []*providerv1.Credential) {
	for _, credential := range values {
		if credential == nil {
			continue
		}
		for index := range credential.Value {
			credential.Value[index] = 0
		}
		credential.Value = nil
	}
}

func sanitizeCredentialText(
	message string,
	credentials []*providerv1.Credential,
) string {
	raw := []byte(message)
	for _, credential := range credentials {
		if credential == nil || len(credential.Value) == 0 {
			continue
		}
		raw = bytes.ReplaceAll(raw, credential.Value, []byte("[redacted]"))
	}
	return string(raw)
}

func mapTools(
	response *providerv1.ListToolsResponse,
	providerRefs []credentialdomain.Reference,
) ([]catalogapplication.DiscoveredTool, map[string][]credentialdomain.Reference, error) {
	if response == nil {
		return nil, nil, fmt.Errorf("empty ListTools response")
	}
	rawTools := response.GetTools()
	if len(rawTools) > maxExternalTools {
		return nil, nil, fmt.Errorf("tool count exceeds %d", maxExternalTools)
	}
	allowedRefs := make(map[credentialdomain.Reference]struct{}, len(providerRefs))
	for _, ref := range providerRefs {
		allowedRefs[ref] = struct{}{}
	}

	tools := make([]catalogapplication.DiscoveredTool, 0, len(rawTools))
	bindings := make(map[string][]credentialdomain.Reference, len(rawTools))
	for index, raw := range rawTools {
		if raw == nil {
			return nil, nil, fmt.Errorf("tool descriptor %d is nil", index)
		}
		sourceKey := strings.TrimSpace(raw.GetSourceKey())
		name := strings.TrimSpace(raw.GetName())
		title := strings.TrimSpace(raw.GetTitle())
		description := strings.TrimSpace(raw.GetDescription())
		if sourceKey == "" || len(sourceKey) > maxSourceKeyBytes {
			return nil, nil, fmt.Errorf("tool %d source key is empty or oversized", index)
		}
		if name == "" || len(name) > maxToolNameBytes {
			return nil, nil, fmt.Errorf("tool %q name is empty or oversized", sourceKey)
		}
		if len(title) > maxToolTitleBytes {
			return nil, nil, fmt.Errorf("tool %q title is oversized", sourceKey)
		}
		if len(description) > maxToolDescriptionBytes {
			return nil, nil, fmt.Errorf("tool %q description is oversized", sourceKey)
		}
		if _, duplicate := bindings[sourceKey]; duplicate {
			return nil, nil, fmt.Errorf("duplicate source key %q", sourceKey)
		}
		input := raw.GetInputSchemaJson()
		if len(input) == 0 || len(input) > maxToolSchemaBytes {
			return nil, nil, fmt.Errorf("tool %q input schema is empty or oversized", sourceKey)
		}
		if _, err := catalogdomain.NewSchema(input); err != nil {
			return nil, nil, fmt.Errorf("tool %q input schema: %w", sourceKey, err)
		}
		output := raw.GetOutputSchemaJson()
		if len(output) > maxToolSchemaBytes {
			return nil, nil, fmt.Errorf("tool %q output schema is oversized", sourceKey)
		}
		if len(output) > 0 {
			if _, err := catalogdomain.NewSchema(output); err != nil {
				return nil, nil, fmt.Errorf("tool %q output schema: %w", sourceKey, err)
			}
		}
		if len(raw.GetTags()) > maxToolTags {
			return nil, nil, fmt.Errorf("tool %q has too many tags", sourceKey)
		}
		tags := make([]string, 0, len(raw.GetTags()))
		for _, rawTag := range raw.GetTags() {
			tag := strings.TrimSpace(rawTag)
			if tag == "" || len(tag) > maxToolTagBytes {
				return nil, nil, fmt.Errorf("tool %q contains invalid tag", sourceKey)
			}
			tags = append(tags, tag)
		}

		refs, err := parseToolCredentialRefs(raw.GetCredentialRefs(), allowedRefs)
		if err != nil {
			return nil, nil, fmt.Errorf("tool %q credentials: %w", sourceKey, err)
		}
		bindings[sourceKey] = refs
		tools = append(tools, catalogapplication.DiscoveredTool{
			SourceKey:    sourceKey,
			Name:         name,
			Title:        title,
			Description:  description,
			InputSchema:  append([]byte(nil), input...),
			OutputSchema: append([]byte(nil), output...),
			Tags:         tags,
		})
	}
	slices.SortFunc(tools, func(a, b catalogapplication.DiscoveredTool) int {
		return strings.Compare(a.SourceKey, b.SourceKey)
	})
	return tools, bindings, nil
}

func parseToolCredentialRefs(
	values []string,
	allowed map[credentialdomain.Reference]struct{},
) ([]credentialdomain.Reference, error) {
	if len(values) > maxCredentialRefs {
		return nil, fmt.Errorf("reference count exceeds %d", maxCredentialRefs)
	}
	refs := make([]credentialdomain.Reference, 0, len(values))
	seen := make(map[credentialdomain.Reference]struct{}, len(values))
	for _, value := range values {
		ref, err := credentialdomain.ParseReference(value)
		if err != nil {
			return nil, err
		}
		if _, exists := allowed[ref]; !exists {
			return nil, fmt.Errorf(
				"reference %s was not declared by Provider config",
				ref,
			)
		}
		if _, duplicate := seen[ref]; duplicate {
			continue
		}
		seen[ref] = struct{}{}
		refs = append(refs, ref)
	}
	slices.SortFunc(refs, func(a, b credentialdomain.Reference) int {
		return strings.Compare(a.String(), b.String())
	})
	return refs, nil
}

func cloneDiscoveredTools(
	values []catalogapplication.DiscoveredTool,
) []catalogapplication.DiscoveredTool {
	result := make([]catalogapplication.DiscoveredTool, 0, len(values))
	for _, value := range values {
		result = append(result, catalogapplication.DiscoveredTool{
			SourceKey:    value.SourceKey,
			Name:         value.Name,
			Title:        value.Title,
			Description:  value.Description,
			InputSchema:  append([]byte(nil), value.InputSchema...),
			OutputSchema: append([]byte(nil), value.OutputSchema...),
			Tags:         append([]string(nil), value.Tags...),
		})
	}
	return result
}

func protocolOutcome(
	event *providerv1.ExecuteToolResponse,
	credentials []*providerv1.Credential,
) (invocationdomain.Outcome, error) {
	switch typed := event.GetEvent().(type) {
	case *providerv1.ExecuteToolResponse_Complete:
		if typed.Complete == nil {
			return invocationdomain.Outcome{}, fmt.Errorf("complete event is nil")
		}
		value, err := invocationdomain.NewJSONValue(typed.Complete.GetContentJson())
		if err != nil {
			return invocationdomain.Outcome{}, fmt.Errorf("complete JSON: %w", err)
		}
		return invocationdomain.Complete(value)
	case *providerv1.ExecuteToolResponse_InputRequired:
		if typed.InputRequired == nil {
			return invocationdomain.Outcome{}, fmt.Errorf("input-required event is nil")
		}
		return invocationdomain.InputRequired(
			typed.InputRequired.GetRequestId(),
			typed.InputRequired.GetMessage(),
			typed.InputRequired.GetInputSchemaJson(),
		)
	case *providerv1.ExecuteToolResponse_Task:
		if typed.Task == nil {
			return invocationdomain.Outcome{}, fmt.Errorf("task event is nil")
		}
		return invocationdomain.Task(
			typed.Task.GetTaskId(),
			typed.Task.GetStatus(),
			typed.Task.GetMessage(),
		)
	case *providerv1.ExecuteToolResponse_Failure:
		if typed.Failure == nil {
			return invocationdomain.Outcome{}, fmt.Errorf("failure event is nil")
		}
		message := sanitizeCredentialText(typed.Failure.GetMessage(), credentials)
		if !utf8.ValidString(message) {
			return invocationdomain.Outcome{}, fmt.Errorf("failure message is invalid UTF-8")
		}
		switch typed.Failure.GetKind() {
		case providerv1.Failure_KIND_TOOL_ERROR:
			return invocationdomain.Failed(
				invocationdomain.FailureSourceTool,
				invocationdomain.FailureToolError,
				message,
			)
		case providerv1.Failure_KIND_PROVIDER_ERROR:
			return invocationdomain.Failed(
				invocationdomain.FailureSourceProvider,
				invocationdomain.FailureProviderError,
				message,
			)
		case providerv1.Failure_KIND_UNSPECIFIED:
			return invocationdomain.Outcome{}, fmt.Errorf(
				"failure kind is unspecified",
			)
		default:
			return invocationdomain.Outcome{}, fmt.Errorf(
				"unknown failure kind %d",
				typed.Failure.GetKind(),
			)
		}
	default:
		return invocationdomain.Outcome{}, fmt.Errorf("execution event kind is missing or unknown")
	}
}

func closeSession(
	ctx context.Context,
	service providerv1.ProviderPluginServiceClient,
	sessionID string,
) error {
	if service == nil || strings.TrimSpace(sessionID) == "" {
		return nil
	}
	closeCtx, cancel := context.WithTimeout(ctx, defaultSessionCloseTimeout)
	defer cancel()
	_, err := service.Close(closeCtx, &providerv1.CloseRequest{SessionId: sessionID})
	if err != nil {
		return fmt.Errorf("close session %s: %w", sessionID, err)
	}
	return nil
}

func newInstanceID(providerID catalogdomain.ProviderID) (string, error) {
	if _, err := catalogdomain.ParseProviderID(providerID.String()); err != nil {
		return "", err
	}
	random := make([]byte, instanceRandomBytes)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return providerID.String() + ":" + hex.EncodeToString(random), nil
}

var _ providerplugin.Plugin = (*Plugin)(nil)
var _ providerplugin.Instance = (*Instance)(nil)
