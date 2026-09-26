package provider

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type headerRoundTripper struct {
	base    http.RoundTripper
	headers http.Header
	origin  string
}

func (t headerRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.Header = request.Header.Clone()
	if canonicalOrigin(request.URL) == t.origin {
		for name, values := range t.headers {
			clone.Header.Del(name)
			for _, value := range values {
				clone.Header.Add(name, value)
			}
		}
	}
	return t.base.RoundTrip(clone)
}

func httpClientWithHeaders(
	client *http.Client,
	endpoint string,
	headers map[string]string,
) (*http.Client, error) {
	if len(headers) == 0 {
		return client, nil
	}

	endpointURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("configure MCP HTTP headers: parse endpoint: %w", err)
	}
	origin := canonicalOrigin(endpointURL)

	var result http.Client
	if client != nil {
		result = *client
	}
	base := result.Transport
	if base == nil {
		base = http.DefaultTransport
	}

	headerValues := make(http.Header, len(headers))
	for name, value := range headers {
		headerValues.Set(name, value)
	}
	result.Transport = headerRoundTripper{
		base:    base,
		headers: headerValues,
		origin:  origin,
	}

	previousRedirect := result.CheckRedirect
	result.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if canonicalOrigin(request.URL) != origin {
			return fmt.Errorf(
				"MCP HTTP redirect from %s to a different origin %s is forbidden when custom headers are configured",
				origin,
				canonicalOrigin(request.URL),
			)
		}
		if previousRedirect != nil {
			return previousRedirect(request, via)
		}
		if len(via) >= 10 {
			return fmt.Errorf("MCP HTTP stopped after 10 redirects")
		}
		return nil
	}

	return &result, nil
}

func canonicalOrigin(value *url.URL) string {
	if value == nil {
		return ""
	}
	scheme := strings.ToLower(value.Scheme)
	host := strings.ToLower(value.Hostname())
	port := value.Port()
	if port == "" {
		switch scheme {
		case "http":
			port = "80"
		case "https":
			port = "443"
		}
	}
	return scheme + "://" + host + ":" + port
}
