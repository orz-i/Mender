package server

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
)

// SourceHeader is the compact header for a discovery source.
type SourceHeader struct {
	SourceID     string `json:"sourceId"`
	SourceKind   string `json:"sourceKind"`
	SourceName   string `json:"sourceName"`
	ToolCount    int    `json:"toolCount"`
	ResourceID   string `json:"resourceId,omitempty"`
	ProviderName string `json:"providerName,omitempty"`
}

// ToolItem is the concise representation of a tool in tool views.
type ToolItem struct {
	ToolID string `json:"toolId"`
	Name   string `json:"name"`
	Title  string `json:"title,omitempty"`
}

// GroupItem represents a source and its query-matched tools.
type GroupItem struct {
	Source SourceHeader `json:"source"`
	Tools  []ToolItem   `json:"tools"`
}

type searchInput struct {
	View     *string `json:"view"`
	Query    string  `json:"query"`
	SourceID string  `json:"sourceId"`
	Cursor   string  `json:"cursor"`
	Limit    *int    `json:"limit"`
}

type searchCursor struct {
	Version     int    `json:"version"`
	View        string `json:"v"`
	Query       string `json:"q"`
	SourceID    string `json:"s"`
	Limit       int    `json:"l"`
	Offset      int    `json:"o"`
	Fingerprint string `json:"fp"`
}

const maxSearchCursorOffset = 10_000_000

func encodeCursor(c searchCursor) string {
	c.Version = 1
	data, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(data)
}

func decodeCursor(raw string) (searchCursor, error) {
	if len(raw) > 4096 || raw == "" {
		return searchCursor{}, fmt.Errorf("invalid cursor length")
	}
	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return searchCursor{}, fmt.Errorf("invalid cursor encoding")
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var c searchCursor
	if err := decoder.Decode(&c); err != nil {
		return searchCursor{}, fmt.Errorf("invalid cursor payload: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return searchCursor{}, fmt.Errorf("invalid trailing cursor payload")
	}
	if c.Version != 1 || (c.View != "sources" && c.View != "tools") ||
		c.Limit < 1 || c.Limit > 100 || c.Offset < 1 ||
		c.Offset > maxSearchCursorOffset || len(c.Fingerprint) != sha256.Size*2 {
		return searchCursor{}, fmt.Errorf("invalid cursor fields")
	}
	if _, err := hex.DecodeString(c.Fingerprint); err != nil {
		return searchCursor{}, fmt.Errorf("invalid cursor fingerprint")
	}
	return c, nil
}

func (a *Adapter) addMetaTools() {
	a.server.AddTool(
		&mcp.Tool{
			Name:        metaSearchName,
			Title:       "Search Mender tools",
			Description: "Find Profile-visible tool sources by Agent, Workspace or service name (default view=sources). Then request view=tools with sourceId to list tools. For a tool name across sources use view=tools and query. Describe a toolId for its schema before calling it.",
			InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"view":{"type":"string","enum":["sources","tools"],"description":"Default sources; tools lists tools within a source or searches across sources."},"query":{"type":"string","description":"Optional for sources or tools within a source; required for cross-source tool lookup."},"sourceId":{"type":"string","description":"Stable sourceId returned from source search (existing ProviderID)."},"cursor":{"type":"string","description":"Opaque nextCursor from the previous page."},"limit":{"type":"integer","minimum":1,"maximum":100}}}`),
		},
		a.searchHandler,
	)

	a.server.AddTool(
		&mcp.Tool{
			Name:        metaDescribeName,
			Title:       "Describe Mender tool",
			Description: "Describe one tool visible in this Mender profile",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"toolId":{"type":"string"}},"required":["toolId"]}`),
		},
		a.describeHandler,
	)

	a.server.AddTool(
		&mcp.Tool{
			Name:        metaCallName,
			Title:       "Call Mender tool",
			Description: "Invoke one tool visible in this Mender profile",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"toolId":{"type":"string"},"arguments":{"type":"object"}},"required":["toolId","arguments"]}`),
		},
		a.callHandler,
	)
}

func (a *Adapter) calculateFingerprint(ctx context.Context, sources map[catalogdomain.ProviderID]ToolSource) (string, error) {
	hasher := sha256.New()

	// 1. Profile ID
	hasher.Write([]byte(a.profile.ID().String()))
	hasher.Write([]byte("|"))

	// 2. Catalog revision (if wired)
	if a.deps.CatalogRevision != nil {
		rev, err := a.deps.CatalogRevision(ctx)
		if err != nil {
			return "", fmt.Errorf("catalog discovery revision: %w", err)
		}
		if rev < 0 {
			return "", fmt.Errorf("catalog discovery revision is negative")
		}
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], uint64(rev))
		hasher.Write(b[:])
	}
	hasher.Write([]byte("|"))

	// 3. Sorted Profile ToolIDs and their effective presentations
	toolIDs := a.profile.ToolIDs()
	sortedToolIDs := make([]catalogdomain.ToolID, len(toolIDs))
	copy(sortedToolIDs, toolIDs)
	sort.Slice(sortedToolIDs, func(i, j int) bool {
		return sortedToolIDs[i].String() < sortedToolIDs[j].String()
	})

	for _, tid := range sortedToolIDs {
		tool, ok := a.tools[tid]
		if !ok {
			continue
		}
		pres := effectivePresentation(a.profile, tool)
		hasher.Write([]byte(tid.String()))
		hasher.Write([]byte("|"))
		hasher.Write([]byte(tool.ProviderID().String()))
		hasher.Write([]byte("|"))
		hasher.Write([]byte(pres.name))
		hasher.Write([]byte("|"))
		hasher.Write([]byte(pres.title))
		hasher.Write([]byte("|"))
		hasher.Write([]byte(pres.description))
		hasher.Write([]byte("|"))
	}

	// 4. Sorted ProviderIDs and their resolved ToolSources
	pids := make([]catalogdomain.ProviderID, 0, len(sources))
	for pid := range sources {
		pids = append(pids, pid)
	}
	sort.Slice(pids, func(i, j int) bool {
		return pids[i].String() < pids[j].String()
	})

	for _, pid := range pids {
		src := sources[pid]
		hasher.Write([]byte(pid.String()))
		hasher.Write([]byte("|"))
		hasher.Write([]byte(src.Kind))
		hasher.Write([]byte("|"))
		hasher.Write([]byte(src.Name))
		hasher.Write([]byte("|"))
		hasher.Write([]byte(src.ResourceID))
		hasher.Write([]byte("|"))
		hasher.Write([]byte(src.ProviderName))
		hasher.Write([]byte("|"))
		if src.Unavailable {
			hasher.Write([]byte("unavailable"))
		}
		hasher.Write([]byte("|"))
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

type toolWithPresentation struct {
	tool         catalogdomain.Tool
	presentation toolPresentation
}

func (a *Adapter) toolsForProvider(pid catalogdomain.ProviderID) []toolWithPresentation {
	toolIDs := a.profile.ToolIDs()
	var list []toolWithPresentation
	for _, tid := range toolIDs {
		t, ok := a.tools[tid]
		if !ok || t.ProviderID() != pid {
			continue
		}
		list = append(list, toolWithPresentation{
			tool:         t,
			presentation: effectivePresentation(a.profile, t),
		})
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].tool.ID().String() < list[j].tool.ID().String()
	})
	return list
}

// rankTextMatch is a bounded lexical signal; repeated matches in verbose
// descriptions or many identical tools must not outweigh exact source identity.
func rankTextMatch(value, query string, exact, prefix, substring int) int {
	v := strings.ToLower(value)
	if v == "" || query == "" {
		return 0
	}
	if v == query {
		return exact
	}
	if strings.HasPrefix(v, query) {
		return prefix
	}
	if strings.Contains(v, query) {
		return substring
	}
	return 0
}

func scoreSource(src ToolSource, q string, tools []toolWithPresentation) int {
	score := rankTextMatch(src.Name, q, 1000, 800, 700)
	score = max(score, rankTextMatch(src.ProviderName, q, 900, 700, 650))
	if src.Kind != "unclaimed" {
		score = max(score, rankTextMatch(src.Kind, q, 850, 600, 600))
		if src.Kind == "sandbox" && q == "workspace" {
			score = max(score, 850)
		}
	}
	if src.ResourceID != "" && strings.Contains(strings.ToLower(src.ResourceID), q) {
		score = max(score, 400)
	}

	// Only the strongest tool-name/title/tag signal participates in source
	// ranking. Do not aggregate match scores across hundreds of same-name
	// tools and do not rank by long, generic upstream descriptions.
	for _, twp := range tools {
		score = max(score, rankTextMatch(twp.presentation.name, q, 500, 350, 250))
		score = max(score, rankTextMatch(twp.tool.Name(), q, 500, 350, 250))
		score = max(score, rankTextMatch(twp.presentation.title, q, 300, 200, 150))
		score = max(score, rankTextMatch(twp.tool.Title(), q, 300, 200, 150))
		for _, tag := range twp.tool.Tags() {
			score = max(score, rankTextMatch(tag, q, 100, 80, 50))
		}
	}
	return score
}

func scoreTool(twp toolWithPresentation, src ToolSource, q string) int {
	score := 0
	presNameLower := strings.ToLower(twp.presentation.name)
	canonNameLower := strings.ToLower(twp.tool.Name())
	presTitleLower := strings.ToLower(twp.presentation.title)
	canonTitleLower := strings.ToLower(twp.tool.Title())

	// Exact tool name / alias
	if strings.EqualFold(twp.presentation.name, q) || strings.EqualFold(twp.tool.Name(), q) {
		score += 1000
	} else if strings.HasPrefix(presNameLower, q) || strings.HasPrefix(canonNameLower, q) {
		score += 700
	} else if strings.Contains(presNameLower, q) || strings.Contains(canonNameLower, q) {
		score += 600
	}

	// Title / override
	if strings.EqualFold(twp.presentation.title, q) || strings.EqualFold(twp.tool.Title(), q) {
		score += 900
	} else if strings.Contains(presTitleLower, q) || strings.Contains(canonTitleLower, q) {
		score += 500
	}

	// Source name / display
	if strings.EqualFold(src.Name, q) {
		score += 800
	} else if strings.Contains(strings.ToLower(src.Name), q) {
		score += 450
	}
	if src.ProviderName != "" {
		if strings.EqualFold(src.ProviderName, q) {
			score += 750
		} else if strings.Contains(strings.ToLower(src.ProviderName), q) {
			score += 420
		}
	}

	// Source kind
	if src.Kind != "" && src.Kind != "unclaimed" {
		if strings.EqualFold(src.Kind, q) || (src.Kind == "sandbox" && q == "workspace") {
			score += 650
		} else if strings.Contains(strings.ToLower(src.Kind), q) {
			score += 400
		}
	}

	// Tags
	for _, tag := range twp.tool.Tags() {
		if strings.EqualFold(tag, q) || strings.Contains(strings.ToLower(tag), q) {
			score += 300
			break
		}
	}

	// Description (effective or canonical)
	presDescLower := strings.ToLower(twp.presentation.description)
	canonDescLower := strings.ToLower(twp.tool.Description())
	if strings.Contains(presDescLower, q) || strings.Contains(canonDescLower, q) {
		score += 100
	}

	return score
}

func (a *Adapter) searchHandler(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var input searchInput
	if len(request.Params.Arguments) > 0 {
		decoder := json.NewDecoder(strings.NewReader(string(request.Params.Arguments)))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			return nil, fmt.Errorf("mender search tools: decode input: %w", err)
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			return nil, fmt.Errorf("mender search tools: unexpected trailing input")
		}
	}

	view := "sources"
	if input.View != nil {
		view = *input.View
	}
	if view != "sources" && view != "tools" {
		return nil, fmt.Errorf("mender search tools: invalid view %q", view)
	}

	limit := 20
	if input.Limit != nil {
		limit = *input.Limit
	}
	if limit < 1 || limit > 100 {
		return nil, fmt.Errorf("mender search tools: limit %d must be between 1 and 100", limit)
	}

	var targetPID catalogdomain.ProviderID
	if input.SourceID != "" {
		var err error
		targetPID, err = catalogdomain.ParseProviderID(input.SourceID)
		if err != nil {
			return nil, fmt.Errorf("mender search tools: invalid sourceId %q: %w", input.SourceID, err)
		}
	}

	trimmedQuery := strings.TrimSpace(input.Query)
	if len(trimmedQuery) > 1024 {
		return nil, fmt.Errorf("mender search tools: query is too long")
	}
	normalizedQuery := strings.ToLower(trimmedQuery)

	if view == "tools" && input.SourceID == "" && trimmedQuery == "" {
		return nil, fmt.Errorf("mender search tools: unscoped tool browsing requires a query")
	}

	offset := 0
	var cursor searchCursor
	if input.Cursor != "" {
		var err error
		cursor, err = decodeCursor(input.Cursor)
		if err != nil {
			return nil, fmt.Errorf("mender search tools: invalid cursor: %w", err)
		}
		if cursor.View != view || cursor.Query != normalizedQuery ||
			cursor.SourceID != input.SourceID || cursor.Limit != limit {
			return nil, fmt.Errorf("mender search tools: cursor scope mismatch")
		}
		offset = cursor.Offset
	}

	// Providers present in Profile.ToolIDs() / a.tools ONLY
	profilePIDsMap := make(map[catalogdomain.ProviderID]struct{})
	sourceToolCounts := make(map[catalogdomain.ProviderID]int)
	for _, tool := range a.tools {
		pid := tool.ProviderID()
		profilePIDsMap[pid] = struct{}{}
		sourceToolCounts[pid]++
	}

	// If sourceId is outside the profile, return empty results without metadata
	if input.SourceID != "" {
		if _, ok := profilePIDsMap[targetPID]; !ok {
			if view == "tools" {
				return structuredResult(map[string]any{
					"tools":      []ToolItem{},
					"nextCursor": "",
				})
			}
			return structuredResult(map[string]any{
				"sources":    []SourceHeader{},
				"nextCursor": "",
			})
		}
	}

	pidsToResolve := make([]catalogdomain.ProviderID, 0, len(profilePIDsMap))
	for pid := range profilePIDsMap {
		pidsToResolve = append(pidsToResolve, pid)
	}
	sort.Slice(pidsToResolve, func(i, j int) bool {
		return pidsToResolve[i].String() < pidsToResolve[j].String()
	})

	sources, err := a.resolveSources(ctx, pidsToResolve)
	if err != nil {
		return nil, fmt.Errorf("mender search tools: resolve sources: %w", err)
	}

	fingerprint, err := a.calculateFingerprint(ctx, sources)
	if err != nil {
		return nil, fmt.Errorf("mender search tools: fingerprint: %w", err)
	}

	if input.Cursor != "" && cursor.Fingerprint != fingerprint {
		return nil, fmt.Errorf("mender search tools: stale cursor")
	}

	// Filtering follows the Profile allowlist and verified Host attribution.
	// Historical Profile ToolBindings remain untouched, but unclaimed or
	// source-wide unavailable Providers never become selectable search hits.
	visiblePIDs := make([]catalogdomain.ProviderID, 0, len(pidsToResolve))
	for _, pid := range pidsToResolve {
		if sources[pid].Discoverable() {
			visiblePIDs = append(visiblePIDs, pid)
		}
	}
	pidsToResolve = visiblePIDs
	if input.SourceID != "" && !sources[targetPID].Discoverable() {
		if view == "sources" {
			return structuredResult(map[string]any{"sources": []SourceHeader{}, "nextCursor": ""})
		}
		return structuredResult(map[string]any{"tools": []ToolItem{}, "nextCursor": ""})
	}

	if view == "sources" {
		type scoredSource struct {
			header SourceHeader
			score  int
		}
		var matched []scoredSource

		for _, pid := range pidsToResolve {
			if input.SourceID != "" && pid != targetPID {
				continue
			}
			src := sources[pid]
			header := SourceHeader{
				SourceID:     pid.String(),
				SourceKind:   src.Kind,
				SourceName:   src.Name,
				ToolCount:    sourceToolCounts[pid],
				ResourceID:   src.ResourceID,
				ProviderName: src.ProviderName,
			}

			if normalizedQuery == "" {
				matched = append(matched, scoredSource{header: header, score: 1})
				continue
			}

			score := scoreSource(src, normalizedQuery, a.toolsForProvider(pid))
			if score > 0 {
				matched = append(matched, scoredSource{header: header, score: score})
			}
		}

		sort.Slice(matched, func(i, j int) bool {
			if matched[i].score != matched[j].score {
				return matched[i].score > matched[j].score
			}
			return matched[i].header.SourceID < matched[j].header.SourceID
		})

		total := len(matched)
		if offset > total {
			offset = total
		}
		end := offset + limit
		if end > total {
			end = total
		}
		page := make([]SourceHeader, 0, end-offset)
		for _, m := range matched[offset:end] {
			page = append(page, m.header)
		}
		nextCursor := ""
		if end < total {
			nextCursor = encodeCursor(searchCursor{
				View:        view,
				Query:       normalizedQuery,
				SourceID:    input.SourceID,
				Limit:       limit,
				Offset:      end,
				Fingerprint: fingerprint,
			})
		}

		return structuredResult(map[string]any{
			"sources":    page,
			"nextCursor": nextCursor,
		})
	}

	// view == "tools"
	if input.SourceID != "" {
		toolsInSource := a.toolsForProvider(targetPID)
		src := sources[targetPID]
		srcHeader := SourceHeader{
			SourceID:     targetPID.String(),
			SourceKind:   src.Kind,
			SourceName:   src.Name,
			ToolCount:    sourceToolCounts[targetPID],
			ResourceID:   src.ResourceID,
			ProviderName: src.ProviderName,
		}

		type scoredTool struct {
			item  ToolItem
			score int
		}
		var matched []scoredTool
		for _, twp := range toolsInSource {
			item := ToolItem{
				ToolID: twp.tool.ID().String(),
				Name:   twp.presentation.name,
				Title:  twp.presentation.title,
			}
			if normalizedQuery == "" {
				matched = append(matched, scoredTool{item: item, score: 1})
				continue
			}
			s := scoreTool(twp, src, normalizedQuery)
			if s > 0 {
				matched = append(matched, scoredTool{item: item, score: s})
			}
		}

		// Avoid description-only noise when a tool name, title, tag or
		// source already matches. Without stronger hits, descriptive
		// search remains available to the caller.
		hasStrongMatch := false
		for _, match := range matched {
			if match.score > 100 {
				hasStrongMatch = true
				break
			}
		}
		if hasStrongMatch {
			filtered := matched[:0]
			for _, match := range matched {
				if match.score > 100 {
					filtered = append(filtered, match)
				}
			}
			matched = filtered
		}

		sort.Slice(matched, func(i, j int) bool {
			if matched[i].score != matched[j].score {
				return matched[i].score > matched[j].score
			}
			if matched[i].item.Name != matched[j].item.Name {
				return matched[i].item.Name < matched[j].item.Name
			}
			return matched[i].item.ToolID < matched[j].item.ToolID
		})

		total := len(matched)
		if offset > total {
			offset = total
		}
		end := offset + limit
		if end > total {
			end = total
		}
		page := make([]ToolItem, 0, end-offset)
		for _, m := range matched[offset:end] {
			page = append(page, m.item)
		}
		nextCursor := ""
		if end < total {
			nextCursor = encodeCursor(searchCursor{
				View:        view,
				Query:       normalizedQuery,
				SourceID:    input.SourceID,
				Limit:       limit,
				Offset:      end,
				Fingerprint: fingerprint,
			})
		}

		return structuredResult(map[string]any{
			"source":     srcHeader,
			"tools":      page,
			"nextCursor": nextCursor,
		})
	}

	// view == "tools", no sourceId, nonempty query -> group by source
	type matchedToolEntry struct {
		pid        catalogdomain.ProviderID
		sourceRank int
		toolScore  int
		item       ToolItem
	}
	var allMatched []matchedToolEntry

	for _, pid := range pidsToResolve {
		src := sources[pid]
		toolsInSource := a.toolsForProvider(pid)
		srcBaseScore := scoreSource(src, normalizedQuery, toolsInSource)

		maxToolScoreInSource := 0
		type localMatched struct {
			twp   toolWithPresentation
			score int
		}
		var localList []localMatched
		for _, twp := range toolsInSource {
			s := scoreTool(twp, src, normalizedQuery)
			if s > 0 {
				if s > maxToolScoreInSource {
					maxToolScoreInSource = s
				}
				localList = append(localList, localMatched{twp: twp, score: s})
			}
		}
		if len(localList) == 0 {
			continue
		}

		sourceRank := srcBaseScore
		if maxToolScoreInSource > sourceRank {
			sourceRank = maxToolScoreInSource
		}

		for _, lm := range localList {
			allMatched = append(allMatched, matchedToolEntry{
				pid:        pid,
				sourceRank: sourceRank,
				toolScore:  lm.score,
				item: ToolItem{
					ToolID: lm.twp.tool.ID().String(),
					Name:   lm.twp.presentation.name,
					Title:  lm.twp.presentation.title,
				},
			})
		}
	}

	// Suppress weak, description-only hits only if there are actual
	// named matches somewhere in the Profile. A descriptive-only query
	// still works when no stronger match exists.
	hasStrongMatch := false
	for _, match := range allMatched {
		if match.toolScore > 100 {
			hasStrongMatch = true
			break
		}
	}
	if hasStrongMatch {
		filtered := allMatched[:0]
		for _, match := range allMatched {
			if match.toolScore > 100 {
				filtered = append(filtered, match)
			}
		}
		allMatched = filtered
	}

	sort.Slice(allMatched, func(i, j int) bool {
		if allMatched[i].sourceRank != allMatched[j].sourceRank {
			return allMatched[i].sourceRank > allMatched[j].sourceRank
		}
		if allMatched[i].pid.String() != allMatched[j].pid.String() {
			return allMatched[i].pid.String() < allMatched[j].pid.String()
		}
		if allMatched[i].toolScore != allMatched[j].toolScore {
			return allMatched[i].toolScore > allMatched[j].toolScore
		}
		if allMatched[i].item.Name != allMatched[j].item.Name {
			return allMatched[i].item.Name < allMatched[j].item.Name
		}
		return allMatched[i].item.ToolID < allMatched[j].item.ToolID
	})

	total := len(allMatched)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	chunk := allMatched[offset:end]

	var groups []GroupItem
	groupIndex := make(map[catalogdomain.ProviderID]int)

	for _, entry := range chunk {
		idx, found := groupIndex[entry.pid]
		if !found {
			src := sources[entry.pid]
			srcHeader := SourceHeader{
				SourceID:     entry.pid.String(),
				SourceKind:   src.Kind,
				SourceName:   src.Name,
				ToolCount:    sourceToolCounts[entry.pid],
				ResourceID:   src.ResourceID,
				ProviderName: src.ProviderName,
			}
			idx = len(groups)
			groups = append(groups, GroupItem{
				Source: srcHeader,
				Tools:  []ToolItem{},
			})
			groupIndex[entry.pid] = idx
		}
		groups[idx].Tools = append(groups[idx].Tools, entry.item)
	}
	if groups == nil {
		groups = []GroupItem{}
	}

	nextCursor := ""
	if end < total {
		nextCursor = encodeCursor(searchCursor{
			View:        view,
			Query:       normalizedQuery,
			SourceID:    input.SourceID,
			Limit:       limit,
			Offset:      end,
			Fingerprint: fingerprint,
		})
	}

	return structuredResult(map[string]any{
		"groups":     groups,
		"nextCursor": nextCursor,
	})
}

func (a *Adapter) describeHandler(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var input struct {
		ToolID string `json:"toolId"`
	}
	if err := json.Unmarshal(request.Params.Arguments, &input); err != nil {
		return nil, fmt.Errorf("mender describe tool: decode input: %w", err)
	}

	toolID, err := catalogdomain.ParseToolID(input.ToolID)
	if err != nil {
		return nil, fmt.Errorf("mender describe tool: %w", err)
	}
	binding, selected := a.profile.Binding(toolID)
	if !selected {
		return nil, ErrToolOutsideProfile
	}

	tool, ok := a.tools[toolID]
	if !ok {
		tool, err = a.deps.DescribeTool(ctx, toolID)
		if err != nil {
			return nil, err
		}
	}
	// Describe is discovery, not execution authority. A bound Tool whose
	// source is unclaimed or known unavailable must not expose a schema merely
	// because a caller retained its historical ToolID.
	sources, err := a.resolveSources(ctx, []catalogdomain.ProviderID{tool.ProviderID()})
	if err != nil {
		return nil, fmt.Errorf("mender describe tool: resolve source: %w", err)
	}
	if !sources[tool.ProviderID()].Discoverable() {
		return nil, ErrToolOutsideProfile
	}

	var inputSchema any
	if err := json.Unmarshal(tool.InputSchema().Bytes(), &inputSchema); err != nil {
		return nil, fmt.Errorf("mender describe tool: input schema: %w", err)
	}
	var outputSchema any
	if schema := tool.OutputSchema(); schema != nil {
		if err := json.Unmarshal(schema.Bytes(), &outputSchema); err != nil {
			return nil, fmt.Errorf("mender describe tool: output schema: %w", err)
		}
	}

	presentation := effectivePresentation(a.profile, tool)
	if binding.ToolID() != tool.ID() {
		return nil, fmt.Errorf("mender describe tool: Profile binding identity mismatch")
	}
	return structuredResult(map[string]any{
		"toolId":       tool.ID().String(),
		"providerId":   tool.ProviderID().String(),
		"name":         presentation.name,
		"title":        presentation.title,
		"description":  presentation.description,
		"inputSchema":  inputSchema,
		"outputSchema": outputSchema,
		"tags":         tool.Tags(),
	})
}

func (a *Adapter) callHandler(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var input struct {
		ToolID    string          `json:"toolId"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(request.Params.Arguments, &input); err != nil {
		return nil, fmt.Errorf("mender call tool: decode input: %w", err)
	}

	toolID, err := catalogdomain.ParseToolID(input.ToolID)
	if err != nil {
		return nil, fmt.Errorf("mender call tool: %w", err)
	}
	if len(input.Arguments) == 0 {
		input.Arguments = json.RawMessage("{}")
	}

	response, err := a.deps.InvokeTool(ctx, invocationapplication.InvokeToolRequest{
		ToolID:    toolID,
		Arguments: input.Arguments,
		Context:   a.invocationContext,
	})
	if err != nil {
		return nil, err
	}
	return mcpResult(response.Outcome)
}

func structuredResult(value any) (*mcp.CallToolResult, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode MCP structured result: %w", err)
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(raw)},
		},
		StructuredContent: value,
	}, nil
}
