package plugin

import (
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strings"
)

const (
	maxAnswerBytes      = 128 * 1024
	maxSources          = 128
	maxSourceURLBytes   = 4096
	maxSourceTitleBytes = 1024
	maxQueries          = 64
	maxQueryBytes       = 8192
)

// Source is normalized citation metadata from one hosted-search engine.
type Source struct {
	URL   string `json:"url"`
	Title string `json:"title"`
}

// SearchResult is the engine-independent result of hosted web search.
type SearchResult struct {
	Answer  string   `json:"answer"`
	Sources []Source `json:"sources"`
	Queries []string `json:"queries"`
}

// Normalize validates and deterministically normalizes a search result.
func (r SearchResult) Normalize() (SearchResult, error) {
	answer := strings.TrimSpace(r.Answer)
	if answer == "" {
		return SearchResult{}, fmt.Errorf("normalize hosted search result: answer must not be empty")
	}
	if len(answer) > maxAnswerBytes {
		return SearchResult{}, fmt.Errorf(
			"normalize hosted search result: answer exceeds %d bytes",
			maxAnswerBytes,
		)
	}
	if len(r.Sources) > maxSources {
		return SearchResult{}, fmt.Errorf(
			"normalize hosted search result: sources exceed %d entries",
			maxSources,
		)
	}
	if len(r.Queries) > maxQueries {
		return SearchResult{}, fmt.Errorf(
			"normalize hosted search result: queries exceed %d entries",
			maxQueries,
		)
	}

	sourceByURL := make(map[string]Source, len(r.Sources))
	for _, source := range r.Sources {
		rawURL := strings.TrimSpace(source.URL)
		title := strings.TrimSpace(source.Title)
		if rawURL == "" {
			return SearchResult{}, fmt.Errorf(
				"normalize hosted search result: source URL must not be empty",
			)
		}
		if len(rawURL) > maxSourceURLBytes {
			return SearchResult{}, fmt.Errorf(
				"normalize hosted search result: source URL exceeds %d bytes",
				maxSourceURLBytes,
			)
		}
		if len(title) > maxSourceTitleBytes {
			return SearchResult{}, fmt.Errorf(
				"normalize hosted search result: source title exceeds %d bytes",
				maxSourceTitleBytes,
			)
		}
		parsed, err := url.Parse(rawURL)
		if err != nil ||
			(parsed.Scheme != "http" && parsed.Scheme != "https") ||
			parsed.Host == "" {
			return SearchResult{}, fmt.Errorf(
				"normalize hosted search result: source URL %q must be absolute HTTP(S)",
				rawURL,
			)
		}
		if parsed.User != nil {
			return SearchResult{}, fmt.Errorf(
				"normalize hosted search result: source URL %q must not contain userinfo",
				rawURL,
			)
		}
		if _, exists := sourceByURL[rawURL]; !exists {
			sourceByURL[rawURL] = Source{URL: rawURL, Title: title}
		}
	}

	sources := make([]Source, 0, len(sourceByURL))
	for _, source := range sourceByURL {
		sources = append(sources, source)
	}
	slices.SortFunc(sources, func(a, b Source) int {
		if order := strings.Compare(a.URL, b.URL); order != 0 {
			return order
		}
		return strings.Compare(a.Title, b.Title)
	})

	querySet := make(map[string]struct{}, len(r.Queries))
	for _, raw := range r.Queries {
		query := strings.TrimSpace(raw)
		if query == "" {
			continue
		}
		if len(query) > maxQueryBytes {
			return SearchResult{}, fmt.Errorf(
				"normalize hosted search result: query exceeds %d bytes",
				maxQueryBytes,
			)
		}
		querySet[query] = struct{}{}
	}
	queries := make([]string, 0, len(querySet))
	for query := range querySet {
		queries = append(queries, query)
	}
	slices.Sort(queries)

	return SearchResult{
		Answer:  answer,
		Sources: sources,
		Queries: queries,
	}, nil
}

// JSON encodes a normalized result as canonical JSON bytes.
func (r SearchResult) JSON() ([]byte, error) {
	normalized, err := r.Normalize()
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(normalized)
	if err != nil {
		return nil, fmt.Errorf("encode hosted search result: %w", err)
	}
	return raw, nil
}
