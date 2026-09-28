package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	maxEngineResponseBytes = 4 * 1024 * 1024
	engineHTTPTimeout      = 90 * time.Second
)

type engineHTTPClient struct {
	client *http.Client
}

func newEngineHTTPClient() engineHTTPClient {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil

	return engineHTTPClient{
		client: &http.Client{
			Transport: transport,
			Timeout:   engineHTTPTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func (c engineHTTPClient) closeIdleConnections() {
	if c.client != nil {
		c.client.CloseIdleConnections()
	}
}

func (c engineHTTPClient) postJSON(
	ctx context.Context,
	endpoint string,
	headers http.Header,
	requestValue any,
	responseValue any,
) error {
	if c.client == nil {
		c = newEngineHTTPClient()
	}

	rawRequest, err := json.Marshal(requestValue)
	if err != nil {
		return fmt.Errorf("encode model tool request: %w", err)
	}

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		endpoint,
		bytes.NewReader(rawRequest),
	)
	if err != nil {
		return fmt.Errorf("build model tool request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	for name, values := range headers {
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}

	response, err := c.client.Do(request)
	if err != nil {
		return fmt.Errorf("execute model tool request: %w", err)
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode < http.StatusOK ||
		response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf(
			"model tool provider returned HTTP status %s",
			response.Status,
		)
	}

	rawResponse, err := readEngineResponse(response.Body)
	if err != nil {
		return fmt.Errorf("read model tool response: %w", err)
	}

	if responseValue == nil {
		return nil
	}
	if err := json.Unmarshal(rawResponse, responseValue); err != nil {
		return fmt.Errorf("decode model tool response: invalid JSON")
	}
	return nil
}

func readEngineResponse(reader io.Reader) ([]byte, error) {
	if reader == nil {
		return nil, fmt.Errorf("response body is unavailable")
	}

	raw, err := io.ReadAll(io.LimitReader(reader, maxEngineResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxEngineResponseBytes {
		return nil, fmt.Errorf(
			"response exceeds %d bytes",
			maxEngineResponseBytes,
		)
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil, fmt.Errorf("response body is empty")
	}
	return raw, nil
}
