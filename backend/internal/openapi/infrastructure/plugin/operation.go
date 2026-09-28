package plugin

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/getkin/kin-openapi/openapi3"
)

const (
	maxOpenAPIToolNameBytes        = 240
	maxOpenAPIToolTitleBytes       = 255
	maxOpenAPIToolDescriptionBytes = 16000
	maxOpenAPIOperationTags        = 32
	maxOpenAPITagBytes             = 64
)

var supportedOperationMethods = map[string]struct{}{
	http.MethodGet:    {},
	http.MethodPost:   {},
	http.MethodPut:    {},
	http.MethodPatch:  {},
	http.MethodDelete: {},
}

type operationBinding struct {
	sourceKey   string
	name        string
	title       string
	description string
	method      string
	path        string
	serverURL   string
	tags        []string

	parameters  []parameterBinding
	requestBody *requestBodyBinding

	inputSchema  []byte
	outputSchema []byte

	pathItem  *openapi3.PathItem
	operation *openapi3.Operation
}

type parameterBinding struct {
	Name     string
	In       string
	Style    string
	Explode  bool
	Required bool
}

type requestBodyBinding struct {
	MediaType string
	Required  bool
}

type operationCandidate struct {
	path       string
	method     string
	pathItem   *openapi3.PathItem
	operation  *openapi3.Operation
	sourceKey  string
	supported  bool
	methodRank int
}

func discoverOperationBindings(
	parsed parsedDocument,
	config Config,
) ([]operationBinding, error) {
	if parsed.document == nil {
		return nil, fmt.Errorf("discover OpenAPI operations: document must not be nil")
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}

	candidates, err := enumerateOperationCandidates(parsed.document)
	if err != nil {
		return nil, err
	}
	selected, err := selectOperationCandidates(candidates, config.Operations)
	if err != nil {
		return nil, err
	}

	bindings := make([]operationBinding, 0, len(selected))
	seenNames := make(map[string]string, len(selected))
	for _, candidate := range selected {
		if !candidate.supported {
			return nil, fmt.Errorf(
				"discover OpenAPI operation %q: HTTP method %s is not supported",
				candidate.sourceKey,
				candidate.method,
			)
		}
		binding, err := buildOperationBinding(parsed, config, candidate)
		if err != nil {
			return nil, fmt.Errorf(
				"discover OpenAPI operation %q: %w",
				candidate.sourceKey,
				err,
			)
		}
		if previous, exists := seenNames[binding.name]; exists {
			return nil, fmt.Errorf(
				"discover OpenAPI operation %q: Tool name %q collides with SourceKey %q",
				binding.sourceKey,
				binding.name,
				previous,
			)
		}
		seenNames[binding.name] = binding.sourceKey
		bindings = append(bindings, binding)
	}

	slices.SortFunc(bindings, func(a, b operationBinding) int {
		return strings.Compare(a.sourceKey, b.sourceKey)
	})
	return bindings, nil
}

func enumerateOperationCandidates(
	document *openapi3.T,
) ([]operationCandidate, error) {
	if document.Paths == nil {
		return nil, nil
	}

	paths := document.Paths.Keys()
	slices.Sort(paths)

	candidates := make([]operationCandidate, 0)
	seenSourceKeys := make(map[string]string)
	for _, path := range paths {
		pathItem := document.Paths.Value(path)
		if pathItem == nil {
			continue
		}
		for method, operation := range pathItem.Operations() {
			if operation == nil {
				continue
			}
			method = strings.ToUpper(strings.TrimSpace(method))
			sourceKey := operationSourceKey(method, path, operation)
			if sourceKey == "" {
				return nil, fmt.Errorf(
					"discover OpenAPI operations: %s %s has empty SourceKey",
					method,
					path,
				)
			}
			if previous, exists := seenSourceKeys[sourceKey]; exists {
				return nil, fmt.Errorf(
					"discover OpenAPI operations: duplicate SourceKey %q for %s and %s %s",
					sourceKey,
					previous,
					method,
					path,
				)
			}
			seenSourceKeys[sourceKey] = method + " " + path
			_, supported := supportedOperationMethods[method]
			candidates = append(candidates, operationCandidate{
				path:       path,
				method:     method,
				pathItem:   pathItem,
				operation:  operation,
				sourceKey:  sourceKey,
				supported:  supported,
				methodRank: operationMethodRank(method),
			})
		}
	}

	slices.SortFunc(candidates, func(a, b operationCandidate) int {
		if order := strings.Compare(a.path, b.path); order != 0 {
			return order
		}
		if a.methodRank < b.methodRank {
			return -1
		}
		if a.methodRank > b.methodRank {
			return 1
		}
		return strings.Compare(a.method, b.method)
	})
	return candidates, nil
}

func selectOperationCandidates(
	candidates []operationCandidate,
	filter OperationFilter,
) ([]operationCandidate, error) {
	if err := filter.Validate(); err != nil {
		return nil, err
	}

	available := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		available[candidate.sourceKey] = struct{}{}
	}

	include := make(map[string]struct{}, len(filter.Include))
	for _, raw := range filter.Include {
		value := strings.TrimSpace(raw)
		if _, exists := available[value]; !exists {
			return nil, fmt.Errorf(
				"OpenAPI plugin config: include SourceKey %q does not exist",
				value,
			)
		}
		include[value] = struct{}{}
	}
	exclude := make(map[string]struct{}, len(filter.Exclude))
	for _, raw := range filter.Exclude {
		value := strings.TrimSpace(raw)
		if _, exists := available[value]; !exists {
			return nil, fmt.Errorf(
				"OpenAPI plugin config: exclude SourceKey %q does not exist",
				value,
			)
		}
		exclude[value] = struct{}{}
	}

	selected := make([]operationCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if len(include) != 0 {
			if _, keep := include[candidate.sourceKey]; !keep {
				continue
			}
		}
		if _, drop := exclude[candidate.sourceKey]; drop {
			continue
		}
		selected = append(selected, candidate)
	}
	return selected, nil
}

func operationSourceKey(
	method string,
	path string,
	operation *openapi3.Operation,
) string {
	if operation != nil {
		if operationID := strings.TrimSpace(operation.OperationID); operationID != "" {
			return operationID
		}
	}
	return strings.ToLower(strings.TrimSpace(method)) + " " + strings.TrimSpace(path)
}

func operationMethodRank(method string) int {
	switch method {
	case http.MethodGet:
		return 0
	case http.MethodPost:
		return 1
	case http.MethodPut:
		return 2
	case http.MethodPatch:
		return 3
	case http.MethodDelete:
		return 4
	case http.MethodHead:
		return 5
	case http.MethodOptions:
		return 6
	case http.MethodTrace:
		return 7
	case "CONNECT":
		return 8
	default:
		return 100
	}
}

func buildOperationBinding(
	parsed parsedDocument,
	config Config,
	candidate operationCandidate,
) (operationBinding, error) {
	parameters, err := mergedOperationParameters(
		candidate.pathItem,
		candidate.operation,
	)
	if err != nil {
		return operationBinding{}, err
	}

	normalizer := newSchemaNormalizer(
		parsed.document,
		parsed.sourceVersion,
		schemaRequest,
	)
	inputSchema, parameterBindings, requestBody, err := buildOperationInputSchema(
		normalizer,
		parameters,
		candidate.operation.RequestBody,
	)
	if err != nil {
		return operationBinding{}, err
	}
	outputSchema, err := buildOperationOutputSchema(
		parsed,
		candidate.operation.Responses,
	)
	if err != nil {
		return operationBinding{}, err
	}
	serverURL, err := resolveEffectiveServer(
		parsed,
		config,
		candidate.pathItem,
		candidate.operation,
	)
	if err != nil {
		return operationBinding{}, err
	}

	name := operationToolName(
		candidate.operation.OperationID,
		candidate.sourceKey,
		candidate.method,
		candidate.path,
	)
	title := operationTitle(candidate)
	description := operationDescription(candidate)

	return operationBinding{
		sourceKey:    candidate.sourceKey,
		name:         name,
		title:        title,
		description:  description,
		method:       candidate.method,
		path:         candidate.path,
		serverURL:    serverURL,
		tags:         operationTags(candidate),
		parameters:   parameterBindings,
		requestBody:  requestBody,
		inputSchema:  inputSchema,
		outputSchema: outputSchema,
		pathItem:     candidate.pathItem,
		operation:    candidate.operation,
	}, nil
}

func mergedOperationParameters(
	pathItem *openapi3.PathItem,
	operation *openapi3.Operation,
) ([]*openapi3.Parameter, error) {
	merged := make(map[string]*openapi3.Parameter)
	add := func(refs openapi3.Parameters) error {
		for _, ref := range refs {
			if ref == nil || ref.Value == nil {
				return fmt.Errorf("parameter reference is unresolved")
			}
			parameter := ref.Value
			key := parameterIdentity(parameter)
			if key == "" {
				return fmt.Errorf("parameter has invalid location/name")
			}
			merged[key] = parameter
		}
		return nil
	}

	if pathItem != nil {
		if err := add(pathItem.Parameters); err != nil {
			return nil, err
		}
	}
	if operation != nil {
		if err := add(operation.Parameters); err != nil {
			return nil, err
		}
	}

	result := make([]*openapi3.Parameter, 0, len(merged))
	for _, parameter := range merged {
		result = append(result, parameter)
	}
	slices.SortFunc(result, func(a, b *openapi3.Parameter) int {
		if rankA, rankB := parameterLocationRank(a.In), parameterLocationRank(b.In); rankA != rankB {
			if rankA < rankB {
				return -1
			}
			return 1
		}
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return result, nil
}

func parameterIdentity(parameter *openapi3.Parameter) string {
	if parameter == nil {
		return ""
	}
	location := strings.ToLower(strings.TrimSpace(parameter.In))
	name := strings.TrimSpace(parameter.Name)
	if location == "" || name == "" {
		return ""
	}
	if location == openapi3.ParameterInHeader {
		name = strings.ToLower(name)
	}
	return location + ":" + name
}

func parameterLocationRank(location string) int {
	switch strings.ToLower(location) {
	case openapi3.ParameterInPath:
		return 0
	case openapi3.ParameterInQuery:
		return 1
	case openapi3.ParameterInHeader:
		return 2
	case openapi3.ParameterInCookie:
		return 3
	default:
		return 100
	}
}

func buildOperationInputSchema(
	normalizer *schemaNormalizer,
	parameters []*openapi3.Parameter,
	requestBodyRef *openapi3.RequestBodyRef,
) ([]byte, []parameterBinding, *requestBodyBinding, error) {
	root := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
	}
	properties := make(map[string]any)
	root["properties"] = properties

	groups := make(map[string]map[string]any)
	groupRequired := make(map[string][]string)
	bindings := make([]parameterBinding, 0, len(parameters))

	for _, parameter := range parameters {
		binding, schema, err := buildParameterSchema(normalizer, parameter)
		if err != nil {
			return nil, nil, nil, err
		}
		groupName := canonicalParameterGroup(parameter.In)
		if groupName == "" {
			return nil, nil, nil, fmt.Errorf(
				"parameter %q uses unsupported location %q",
				parameter.Name,
				parameter.In,
			)
		}
		group := groups[groupName]
		if group == nil {
			group = map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties":           map[string]any{},
			}
			groups[groupName] = group
			properties[groupName] = group
		}
		groupProperties := group["properties"].(map[string]any)
		groupProperties[parameter.Name] = schema
		if parameter.Required || strings.EqualFold(parameter.In, openapi3.ParameterInPath) {
			groupRequired[groupName] = append(groupRequired[groupName], parameter.Name)
		}
		bindings = append(bindings, binding)
	}

	rootRequired := make([]string, 0, len(groupRequired)+1)
	for groupName, required := range groupRequired {
		slices.Sort(required)
		items := make([]any, 0, len(required))
		for _, value := range required {
			items = append(items, value)
		}
		groups[groupName]["required"] = items
		rootRequired = append(rootRequired, groupName)
	}

	requestBody, bodyRequired, err := buildRequestBodySchema(
		normalizer,
		requestBodyRef,
		properties,
		root,
	)
	if err != nil {
		return nil, nil, nil, err
	}
	if bodyRequired {
		rootRequired = append(rootRequired, "body")
	}
	if len(rootRequired) != 0 {
		slices.Sort(rootRequired)
		items := make([]any, 0, len(rootRequired))
		for _, value := range rootRequired {
			items = append(items, value)
		}
		root["required"] = items
	}

	normalizer.attachDefs(root)
	raw, err := json.Marshal(root)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("encode canonical Tool input schema: %w", err)
	}
	return raw, bindings, requestBody, nil
}

func buildParameterSchema(
	normalizer *schemaNormalizer,
	parameter *openapi3.Parameter,
) (parameterBinding, any, error) {
	if parameter == nil {
		return parameterBinding{}, nil, fmt.Errorf("parameter must not be nil")
	}
	location := strings.ToLower(strings.TrimSpace(parameter.In))
	if canonicalParameterGroup(location) == "" {
		return parameterBinding{}, nil, fmt.Errorf(
			"parameter %q uses unsupported location %q",
			parameter.Name,
			parameter.In,
		)
	}
	if len(parameter.Content) != 0 {
		return parameterBinding{}, nil, fmt.Errorf(
			"parameter %q uses content-based serialization, which is not supported",
			parameter.Name,
		)
	}
	if parameter.Schema == nil || parameter.Schema.Value == nil {
		return parameterBinding{}, nil, fmt.Errorf(
			"parameter %q must have a resolved schema",
			parameter.Name,
		)
	}
	if (location == openapi3.ParameterInHeader || location == openapi3.ParameterInCookie) &&
		isSensitiveOrdinaryParameter(parameter.Name) {
		return parameterBinding{}, nil, fmt.Errorf(
			"parameter %q appears to contain authentication material; model it as an OpenAPI Security Scheme",
			parameter.Name,
		)
	}

	serialization, err := parameter.SerializationMethod()
	if err != nil {
		return parameterBinding{}, nil, fmt.Errorf(
			"parameter %q serialization: %w",
			parameter.Name,
			err,
		)
	}
	if err := validateParameterSerialization(parameter, serialization); err != nil {
		return parameterBinding{}, nil, err
	}

	normalized, err := normalizer.normalizeRef(parameter.Schema)
	if err != nil {
		return parameterBinding{}, nil, fmt.Errorf(
			"parameter %q schema: %w",
			parameter.Name,
			err,
		)
	}
	if schemaMap, ok := normalized.(map[string]any); ok &&
		strings.TrimSpace(parameter.Description) != "" {
		if _, exists := schemaMap["description"]; !exists {
			schemaMap["description"] = truncateUTF8Bytes(
				strings.TrimSpace(parameter.Description),
				maxOpenAPIToolDescriptionBytes,
			)
		}
	}

	return parameterBinding{
		Name:     parameter.Name,
		In:       location,
		Style:    serialization.Style,
		Explode:  serialization.Explode,
		Required: parameter.Required || location == openapi3.ParameterInPath,
	}, normalized, nil
}

func validateParameterSerialization(
	parameter *openapi3.Parameter,
	serialization *openapi3.SerializationMethod,
) error {
	if serialization == nil {
		return fmt.Errorf("parameter %q has no serialization method", parameter.Name)
	}
	switch strings.ToLower(parameter.In) {
	case openapi3.ParameterInPath:
		if serialization.Style != "simple" {
			return fmt.Errorf(
				"parameter %q path style %q is not supported",
				parameter.Name,
				serialization.Style,
			)
		}
	case openapi3.ParameterInQuery:
		if serialization.Style != "form" {
			return fmt.Errorf(
				"parameter %q query style %q is not supported",
				parameter.Name,
				serialization.Style,
			)
		}
	case openapi3.ParameterInHeader:
		if serialization.Style != "simple" {
			return fmt.Errorf(
				"parameter %q header style %q is not supported",
				parameter.Name,
				serialization.Style,
			)
		}
	case openapi3.ParameterInCookie:
		if serialization.Style != "form" {
			return fmt.Errorf(
				"parameter %q cookie style %q is not supported",
				parameter.Name,
				serialization.Style,
			)
		}
	}

	types := parameter.Schema.Value.Type
	if types == nil || types.IsEmpty() {
		return fmt.Errorf(
			"parameter %q schema must declare a supported scalar or array type",
			parameter.Name,
		)
	}
	nonNull := make([]string, 0, len(types.Slice()))
	for _, typ := range types.Slice() {
		if typ != "null" {
			nonNull = append(nonNull, typ)
		}
	}
	if len(nonNull) != 1 {
		return fmt.Errorf(
			"parameter %q schema must have exactly one non-null type",
			parameter.Name,
		)
	}
	switch nonNull[0] {
	case "string", "integer", "number", "boolean":
		return nil
	case "array":
		items := parameter.Schema.Value.Items
		if items == nil || items.Value == nil || items.Value.Type == nil {
			return fmt.Errorf(
				"parameter %q array items must declare a scalar type",
				parameter.Name,
			)
		}
		itemTypes := items.Value.Type.Slice()
		nonNullItems := make([]string, 0, len(itemTypes))
		for _, typ := range itemTypes {
			if typ != "null" {
				nonNullItems = append(nonNullItems, typ)
			}
		}
		if len(nonNullItems) != 1 {
			return fmt.Errorf(
				"parameter %q array items must have one scalar type",
				parameter.Name,
			)
		}
		switch nonNullItems[0] {
		case "string", "integer", "number", "boolean":
			return nil
		default:
			return fmt.Errorf(
				"parameter %q array item type %q is not supported",
				parameter.Name,
				nonNullItems[0],
			)
		}
	default:
		return fmt.Errorf(
			"parameter %q type %q is not supported",
			parameter.Name,
			nonNull[0],
		)
	}
}

func buildRequestBodySchema(
	normalizer *schemaNormalizer,
	ref *openapi3.RequestBodyRef,
	properties map[string]any,
	_ map[string]any,
) (*requestBodyBinding, bool, error) {
	if ref == nil {
		return nil, false, nil
	}
	if ref.Value == nil {
		return nil, false, fmt.Errorf("request body reference is unresolved")
	}
	mediaType, media, err := selectJSONMediaType(ref.Value.Content)
	if err != nil {
		return nil, false, err
	}

	var bodySchema any = map[string]any{}
	if media != nil && media.Schema != nil {
		bodySchema, err = normalizer.normalizeRef(media.Schema)
		if err != nil {
			return nil, false, fmt.Errorf("request body schema: %w", err)
		}
	}
	properties["body"] = bodySchema
	return &requestBodyBinding{
		MediaType: mediaType,
		Required:  ref.Value.Required,
	}, ref.Value.Required, nil
}

func selectJSONMediaType(
	content openapi3.Content,
) (string, *openapi3.MediaType, error) {
	if len(content) == 0 {
		return "", nil, fmt.Errorf("request body has no media type")
	}
	if media, exists := content["application/json"]; exists {
		return "application/json", media, nil
	}

	candidates := make([]string, 0)
	for raw := range content {
		mediaType, _, err := mime.ParseMediaType(raw)
		if err != nil {
			continue
		}
		mediaType = strings.ToLower(mediaType)
		if strings.Contains(mediaType, "*") {
			continue
		}
		if strings.HasPrefix(mediaType, "application/") &&
			strings.HasSuffix(mediaType, "+json") {
			candidates = append(candidates, raw)
		}
	}
	if len(candidates) == 0 {
		return "", nil, fmt.Errorf(
			"request body must provide application/json or a concrete application/*+json media type",
		)
	}
	slices.Sort(candidates)
	selected := candidates[0]
	normalized, _, _ := mime.ParseMediaType(selected)
	return strings.ToLower(normalized), content[selected], nil
}

func canonicalParameterGroup(location string) string {
	switch strings.ToLower(strings.TrimSpace(location)) {
	case openapi3.ParameterInPath:
		return "path"
	case openapi3.ParameterInQuery:
		return "query"
	case openapi3.ParameterInHeader:
		return "headers"
	case openapi3.ParameterInCookie:
		return "cookies"
	default:
		return ""
	}
}

func operationToolName(
	operationID string,
	sourceKey string,
	method string,
	path string,
) string {
	operationID = strings.TrimSpace(operationID)
	if operationID != "" &&
		validMachineName(operationID) &&
		len(operationID) <= maxOpenAPIToolNameBytes {
		return operationID
	}

	baseInput := operationID
	if baseInput == "" {
		baseInput = strings.ToLower(method) + "_" + path
	}
	base := sanitizeMachineName(baseInput)
	if base == "" {
		base = "operation"
	}

	hash := sha256.Sum256([]byte(sourceKey))
	suffix := "_" + hex.EncodeToString(hash[:])[:10]
	maxBase := maxOpenAPIToolNameBytes - len(suffix)
	if len(base) > maxBase {
		base = truncateUTF8Bytes(base, maxBase)
	}
	base = strings.Trim(base, "_.-")
	if base == "" {
		base = "operation"
	}
	return base + suffix
}

func validMachineName(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if !validMachineNameRune(r) {
			return false
		}
	}
	return true
}

func sanitizeMachineName(value string) string {
	var builder strings.Builder
	lastUnderscore := false
	for _, r := range value {
		if validMachineNameRune(r) {
			builder.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			builder.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(builder.String(), "_")
}

func validMachineNameRune(r rune) bool {
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		r == '_' || r == '-' || r == '.'
}

func operationTitle(candidate operationCandidate) string {
	value := strings.TrimSpace(candidate.operation.Summary)
	if value == "" {
		value = strings.TrimSpace(candidate.operation.OperationID)
	}
	if value == "" {
		value = candidate.sourceKey
	}
	return truncateUTF8Bytes(value, maxOpenAPIToolTitleBytes)
}

func operationDescription(candidate operationCandidate) string {
	summary := strings.TrimSpace(candidate.operation.Summary)
	description := strings.TrimSpace(candidate.operation.Description)

	parts := make([]string, 0, 3)
	if summary != "" {
		parts = append(parts, summary)
	}
	if description != "" && description != summary {
		parts = append(parts, description)
	}
	parts = append(parts, candidate.method+" "+candidate.path)
	return truncateUTF8Bytes(
		strings.Join(parts, "\n\n"),
		maxOpenAPIToolDescriptionBytes,
	)
}

func operationTags(candidate operationCandidate) []string {
	result := []string{
		"openapi",
		"http",
		"http:" + strings.ToLower(candidate.method),
	}
	seen := map[string]struct{}{
		"openapi": {},
		"http":    {},
		"http:" + strings.ToLower(candidate.method): {},
	}
	for _, raw := range candidate.operation.Tags {
		if len(result) >= 3+maxOpenAPIOperationTags {
			break
		}
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		value = "oas:" + truncateUTF8Bytes(value, maxOpenAPITagBytes-len("oas:"))
		key := strings.ToLower(value)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}

func isSensitiveOrdinaryParameter(name string) bool {
	var builder strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
		}
	}
	switch builder.String() {
	case "authorization",
		"proxyauthorization",
		"xapikey",
		"apikey",
		"accesstoken",
		"password",
		"secret",
		"session":
		return true
	default:
		return false
	}
}

func truncateUTF8Bytes(value string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(value) <= maxBytes {
		return value
	}
	end := maxBytes
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return strings.TrimSpace(value[:end])
}
