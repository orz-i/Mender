package plugin

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"

	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
)

const maxResponseBytes = 8 * 1024 * 1024

func newExecutionHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// Provider execution should not silently inherit proxy credentials or route
	// decisions from ambient process environment. Proxy support can be added as
	// an explicit reviewed Provider capability later.
	transport.Proxy = nil

	return &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func executeHTTPRequest(
	ctx context.Context,
	client *http.Client,
	request *http.Request,
) (invocationdomain.Outcome, error) {
	if client == nil {
		return invocationdomain.Outcome{}, fmt.Errorf(
			"execute OpenAPI operation: HTTP client is unavailable",
		)
	}
	if request == nil {
		return invocationdomain.Outcome{}, fmt.Errorf(
			"execute OpenAPI operation: request is unavailable",
		)
	}

	response, err := client.Do(request.WithContext(ctx))
	if err != nil {
		return invocationdomain.Outcome{}, fmt.Errorf(
			"execute OpenAPI operation: %w",
			err,
		)
	}
	defer func() { _ = response.Body.Close() }()

	raw, err := readBounded(response.Body, maxResponseBytes)
	if err != nil {
		return invocationdomain.Outcome{}, fmt.Errorf(
			"execute OpenAPI operation response: %w",
			err,
		)
	}

	if response.StatusCode < http.StatusOK ||
		response.StatusCode >= http.StatusMultipleChoices {
		failed, err := invocationdomain.Failed(
			invocationdomain.FailureSourceTool,
			invocationdomain.FailureToolError,
			"upstream HTTP request failed with status "+response.Status,
		)
		if err != nil {
			return invocationdomain.Outcome{}, fmt.Errorf(
				"map OpenAPI HTTP failure: %w",
				err,
			)
		}
		return failed, nil
	}

	if len(bytes.TrimSpace(raw)) == 0 {
		raw = []byte("null")
	} else if contentType := strings.TrimSpace(response.Header.Get("Content-Type")); contentType != "" &&
		!isConcreteJSONMediaType(contentType) {
		return invocationdomain.Outcome{}, fmt.Errorf(
			"execute OpenAPI operation response: successful response Content-Type %q is not JSON",
			contentType,
		)
	}

	value, err := invocationdomain.NewJSONValue(raw)
	if err != nil {
		return invocationdomain.Outcome{}, fmt.Errorf(
			"execute OpenAPI operation response: expected valid JSON",
		)
	}
	complete, err := invocationdomain.Complete(value)
	if err != nil {
		return invocationdomain.Outcome{}, fmt.Errorf(
			"map OpenAPI complete response: %w",
			err,
		)
	}
	return complete, nil
}
