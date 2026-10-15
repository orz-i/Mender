package plugin

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	maxDescriptionBytes = 16 * 1024 * 1024
	sourceHTTPTimeout   = 15 * time.Second
)

type loadedSource struct {
	data     []byte
	location *url.URL
}

type sourceLoader struct {
	httpClient *http.Client
}

func newSourceLoader() sourceLoader {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// Description sources are explicit Provider configuration. Do not add
	// ambient proxy routing or proxy credentials from the Mender environment.
	transport.Proxy = nil
	return sourceLoader{
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   sourceHTTPTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func (l sourceLoader) Load(
	ctx context.Context,
	source SourceConfig,
) (loadedSource, error) {
	if err := source.Validate(); err != nil {
		return loadedSource{}, err
	}

	switch source.Kind {
	case SourceURL:
		return l.loadURL(ctx, strings.TrimSpace(source.URL))
	case SourceFile:
		return l.loadFile(strings.TrimSpace(source.Path))
	default:
		return loadedSource{}, fmt.Errorf(
			"load OpenAPI description: unsupported source kind %q",
			source.Kind,
		)
	}
}

func (l sourceLoader) loadURL(
	ctx context.Context,
	rawURL string,
) (loadedSource, error) {
	client := l.httpClient
	if client == nil {
		client = newSourceLoader().httpClient
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return loadedSource{}, fmt.Errorf("load OpenAPI description URL: %w", err)
	}
	request.Header.Set(
		"Accept",
		"application/vnd.oai.openapi+json, application/vnd.oai.openapi, application/json, application/yaml, text/yaml, text/plain",
	)

	response, err := client.Do(request)
	if err != nil {
		return loadedSource{}, fmt.Errorf("load OpenAPI description URL: %w", err)
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode < http.StatusOK ||
		response.StatusCode >= http.StatusMultipleChoices {
		return loadedSource{}, fmt.Errorf(
			"load OpenAPI description URL: unexpected HTTP status %s",
			response.Status,
		)
	}

	data, err := readBounded(response.Body, maxDescriptionBytes)
	if err != nil {
		return loadedSource{}, fmt.Errorf("load OpenAPI description URL: %w", err)
	}

	location, err := url.Parse(rawURL)
	if err != nil {
		return loadedSource{}, fmt.Errorf(
			"load OpenAPI description URL: parse source location: %w",
			err,
		)
	}
	return loadedSource{
		data:     data,
		location: location,
	}, nil
}

func (sourceLoader) loadFile(path string) (loadedSource, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return loadedSource{}, fmt.Errorf(
			"load OpenAPI description file: resolve path: %w",
			err,
		)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return loadedSource{}, fmt.Errorf("load OpenAPI description file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return loadedSource{}, fmt.Errorf(
			"load OpenAPI description file: source must be a regular file",
		)
	}
	if info.Size() > maxDescriptionBytes {
		return loadedSource{}, fmt.Errorf(
			"load OpenAPI description file: description exceeds %d bytes",
			maxDescriptionBytes,
		)
	}

	file, err := os.Open(absolute) //nolint:gosec // explicit locally configured absolute Provider source.
	if err != nil {
		return loadedSource{}, fmt.Errorf("load OpenAPI description file: %w", err)
	}
	defer func() { _ = file.Close() }()

	data, err := readBounded(file, maxDescriptionBytes)
	if err != nil {
		return loadedSource{}, fmt.Errorf("load OpenAPI description file: %w", err)
	}
	slashed := filepath.ToSlash(absolute)
	if !strings.HasPrefix(slashed, "/") {
		slashed = "/" + slashed
	}
	location := &url.URL{
		Scheme: "file",
		Path:   slashed,
	}
	return loadedSource{
		data:     data,
		location: location,
	}, nil
}

func readBounded(reader io.Reader, limit int64) ([]byte, error) {
	if reader == nil {
		return nil, fmt.Errorf("source reader must not be nil")
	}
	if limit < 1 {
		return nil, fmt.Errorf("source byte limit must be positive")
	}

	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("description exceeds %d bytes", limit)
	}
	return data, nil
}
