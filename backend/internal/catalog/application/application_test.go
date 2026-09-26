package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/orz-i/Mender/backend/internal/catalog/domain"
)

func TestRegisterAndGetProvider(t *testing.T) {
	t.Parallel()

	store := newMemoryCatalog()
	expectedID := mustProviderID(t, "prov_00000000000000000000000000000001")
	ids := &fixedIDs{
		providerIDs: []domain.ProviderID{expectedID},
	}
	clock := fixedClock{now: testTime()}

	register := NewRegisterProvider(store, ids, clock)
	provider, err := register.Execute(context.Background(), RegisterProviderRequest{
		Namespace:   "github",
		DisplayName: "GitHub",
		Description: "Repository tools",
	})
	if err != nil {
		t.Fatalf("RegisterProvider.Execute() error = %v", err)
	}

	if provider.ID() != expectedID {
		t.Fatalf("Provider ID = %q, want %q", provider.ID(), expectedID)
	}
	if provider.Namespace().String() != "github" {
		t.Fatalf("Namespace = %q", provider.Namespace())
	}

	get := NewGetProvider(store)
	loaded, err := get.Execute(context.Background(), provider.ID())
	if err != nil {
		t.Fatalf("GetProvider.Execute() error = %v", err)
	}
	if loaded.ID() != provider.ID() {
		t.Fatalf("loaded Provider ID = %q, want %q", loaded.ID(), provider.ID())
	}
}

func TestUpdateProviderPreservesIdentityAndNamespace(t *testing.T) {
	t.Parallel()

	store := newMemoryCatalog()
	id := mustProviderID(t, "prov_00000000000000000000000000000001")
	namespace, _ := domain.NewNamespace("github")
	created := testTime()
	provider, _ := domain.NewProvider(id, namespace, "GitHub", "Old", created)
	if err := store.CreateProvider(context.Background(), provider); err != nil {
		t.Fatal(err)
	}

	update := NewUpdateProvider(
		store,
		store,
		fixedClock{now: created.Add(time.Hour)},
	)
	updated, err := update.Execute(context.Background(), UpdateProviderRequest{
		ProviderID:  id,
		DisplayName: "GitHub Work",
		Description: "Updated",
	})
	if err != nil {
		t.Fatalf("UpdateProvider.Execute() error = %v", err)
	}

	if updated.ID() != id || updated.Namespace() != namespace {
		t.Fatalf("identity changed: %#v", updated)
	}
	if updated.DisplayName() != "GitHub Work" || updated.Description() != "Updated" {
		t.Fatalf("metadata = %#v", updated)
	}

	loaded, err := store.GetProvider(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.UpdatedAt() != created.Add(time.Hour) {
		t.Fatalf("updatedAt = %v", loaded.UpdatedAt())
	}
}

func TestListFindAndRemoveProviders(t *testing.T) {
	t.Parallel()

	store := newMemoryCatalog()
	firstID := mustProviderID(t, "prov_00000000000000000000000000000001")
	secondID := mustProviderID(t, "prov_00000000000000000000000000000002")
	firstNS, _ := domain.NewNamespace("alpha")
	secondNS, _ := domain.NewNamespace("beta")
	first, _ := domain.NewProvider(firstID, firstNS, "Alpha", "", testTime())
	second, _ := domain.NewProvider(secondID, secondNS, "Beta", "", testTime())
	if err := store.CreateProvider(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateProvider(context.Background(), second); err != nil {
		t.Fatal(err)
	}

	list := NewListProviders(store)
	providers, err := list.Execute(context.Background())
	if err != nil {
		t.Fatalf("ListProviders.Execute() error = %v", err)
	}
	if len(providers) != 2 {
		t.Fatalf("ListProviders = %+v", providers)
	}

	find := NewFindProviderByNamespace(store)
	found, err := find.Execute(context.Background(), secondNS)
	if err != nil {
		t.Fatalf("FindProviderByNamespace.Execute() error = %v", err)
	}
	if found.ID() != secondID {
		t.Fatalf("found ProviderID = %q, want %q", found.ID(), secondID)
	}

	remove := NewRemoveProvider(store)
	if err := remove.Execute(context.Background(), firstID); err != nil {
		t.Fatalf("RemoveProvider.Execute() error = %v", err)
	}
	if _, err := store.GetProvider(context.Background(), firstID); !errors.Is(err, ErrProviderNotFound) {
		t.Fatalf("GetProvider(removed) error = %v", err)
	}
}

func TestRegisterProviderPropagatesNamespaceConflict(t *testing.T) {
	t.Parallel()

	store := newMemoryCatalog()
	clock := fixedClock{now: testTime()}
	ids := &fixedIDs{
		providerIDs: []domain.ProviderID{
			mustProviderID(t, "prov_00000000000000000000000000000001"),
			mustProviderID(t, "prov_00000000000000000000000000000002"),
		},
	}
	register := NewRegisterProvider(store, ids, clock)

	request := RegisterProviderRequest{Namespace: "github", DisplayName: "GitHub"}
	if _, err := register.Execute(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if _, err := register.Execute(context.Background(), request); !errors.Is(err, ErrNamespaceConflict) {
		t.Fatalf("second registration error = %v, want ErrNamespaceConflict", err)
	}
}

func TestSyncProviderToolsPreservesIdentityAndReplacesSnapshot(t *testing.T) {
	t.Parallel()

	store := newMemoryCatalog()
	providerID := mustProviderID(t, "prov_00000000000000000000000000000001")
	namespace, _ := domain.NewNamespace("github")
	provider, _ := domain.NewProvider(providerID, namespace, "GitHub", "", testTime())
	if err := store.CreateProvider(context.Background(), provider); err != nil {
		t.Fatal(err)
	}

	existingID := mustToolID(t, "tool_00000000000000000000000000000001")
	input := mustSchema(t, `{"type":"object"}`)
	old, _ := domain.NewTool(existingID, providerID, domain.ToolSpec{
		SourceKey:   "issues.create",
		Name:        "create_issue",
		InputSchema: input,
	}, testTime())
	if err := store.ReplaceProviderTools(context.Background(), providerID, []domain.Tool{old}); err != nil {
		t.Fatal(err)
	}

	source := staticToolSource{tools: []DiscoveredTool{
		{
			SourceKey:   "repos.get",
			Name:        "get_repository",
			Title:       "Get repository",
			Description: "Read repository metadata",
			InputSchema: []byte(`{"type":"object"}`),
			Tags:        []string{"read"},
		},
		{
			SourceKey:    "issues.create",
			Name:         "issue_create",
			Title:        "Create issue",
			Description:  "Create a repository issue",
			InputSchema:  []byte(`{"type":"object","properties":{"title":{"type":"string"}}}`),
			OutputSchema: []byte(`{"type":"object"}`),
			Tags:         []string{"issues", "write"},
		},
	}}

	newID := mustToolID(t, "tool_00000000000000000000000000000002")
	ids := &fixedIDs{toolIDs: []domain.ToolID{newID}}
	syncUseCase := NewSyncProviderTools(
		store,
		store,
		source,
		ids,
		fixedClock{now: testTime().Add(time.Hour)},
	)

	result, err := syncUseCase.Execute(context.Background(), providerID)
	if err != nil {
		t.Fatalf("SyncProviderTools.Execute() error = %v", err)
	}
	if result.Created != 1 || result.Updated != 1 || result.Removed != 0 || result.Total != 2 {
		t.Fatalf("Sync result = %+v", result)
	}

	tools, _ := store.ListProviderTools(context.Background(), providerID)
	byKey := make(map[string]domain.Tool, len(tools))
	for _, tool := range tools {
		byKey[tool.SourceKey()] = tool
	}

	if byKey["issues.create"].ID() != existingID {
		t.Fatalf("existing ToolID changed: %q", byKey["issues.create"].ID())
	}
	if byKey["issues.create"].Name() != "issue_create" {
		t.Fatalf("existing metadata not refreshed: %q", byKey["issues.create"].Name())
	}
	if byKey["repos.get"].ID() != newID {
		t.Fatalf("new ToolID = %q, want %q", byKey["repos.get"].ID(), newID)
	}
}

func TestSyncProviderToolsRejectsDuplicateSourceKeys(t *testing.T) {
	t.Parallel()

	store, providerID := catalogWithProvider(t)
	source := staticToolSource{tools: []DiscoveredTool{
		{SourceKey: "same", Name: "one", InputSchema: []byte(`{"type":"object"}`)},
		{SourceKey: "same", Name: "two", InputSchema: []byte(`{"type":"object"}`)},
	}}

	useCase := NewSyncProviderTools(
		store,
		store,
		source,
		&fixedIDs{},
		fixedClock{now: testTime()},
	)

	_, err := useCase.Execute(context.Background(), providerID)
	if !errors.Is(err, ErrDuplicateSourceKey) {
		t.Fatalf("SyncProviderTools.Execute() error = %v, want ErrDuplicateSourceKey", err)
	}
}

func TestDescribeAndSearchToolsUseApplicationPorts(t *testing.T) {
	t.Parallel()

	store, providerID := catalogWithProvider(t)
	input := mustSchema(t, `{"type":"object"}`)
	githubTool, _ := domain.NewTool(
		mustToolID(t, "tool_00000000000000000000000000000001"),
		providerID,
		domain.ToolSpec{
			SourceKey:   "issues.create",
			Name:        "create_issue",
			Title:       "Create issue",
			Description: "Create a GitHub issue",
			InputSchema: input,
			Tags:        []string{"github", "issue"},
		},
		testTime(),
	)
	other, _ := domain.NewTool(
		mustToolID(t, "tool_00000000000000000000000000000002"),
		providerID,
		domain.ToolSpec{
			SourceKey:   "repos.get",
			Name:        "get_repository",
			Title:       "Get repository",
			Description: "Read repository metadata",
			InputSchema: input,
			Tags:        []string{"github", "read"},
		},
		testTime(),
	)
	if err := store.ReplaceProviderTools(context.Background(), providerID, []domain.Tool{githubTool, other}); err != nil {
		t.Fatal(err)
	}

	describe := NewDescribeTool(store)
	described, err := describe.Execute(context.Background(), githubTool.ID())
	if err != nil {
		t.Fatalf("DescribeTool.Execute() error = %v", err)
	}
	if described.ID() != githubTool.ID() {
		t.Fatalf("DescribeTool ID = %q", described.ID())
	}

	search := NewSearchTools(store)
	results, err := search.Execute(context.Background(), SearchToolsRequest{
		Query: "issue",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("SearchTools.Execute() error = %v", err)
	}
	if len(results) != 1 || results[0].ToolID != githubTool.ID() || results[0].Rank != 1 {
		t.Fatalf("SearchTools results = %+v", results)
	}

	restricted, err := search.Execute(context.Background(), SearchToolsRequest{
		Query:          "github",
		Limit:          10,
		AllowedToolIDs: []domain.ToolID{other.ID()},
	})
	if err != nil {
		t.Fatalf("restricted SearchTools.Execute() error = %v", err)
	}
	if len(restricted) != 1 || restricted[0].ToolID != other.ID() {
		t.Fatalf("restricted SearchTools results = %+v", restricted)
	}

	empty, err := search.Execute(context.Background(), SearchToolsRequest{
		Query:          "github",
		Limit:          10,
		AllowedToolIDs: []domain.ToolID{},
	})
	if err != nil {
		t.Fatalf("empty scoped SearchTools.Execute() error = %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("empty scoped SearchTools results = %+v", empty)
	}
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time {
	return c.now
}

type fixedIDs struct {
	providerIDs []domain.ProviderID
	toolIDs     []domain.ToolID
}

func (g *fixedIDs) NewProviderID() (domain.ProviderID, error) {
	if len(g.providerIDs) == 0 {
		return "", errors.New("no provider IDs remaining")
	}
	id := g.providerIDs[0]
	g.providerIDs = g.providerIDs[1:]
	return id, nil
}

func (g *fixedIDs) NewToolID() (domain.ToolID, error) {
	if len(g.toolIDs) == 0 {
		return "", errors.New("no tool IDs remaining")
	}
	id := g.toolIDs[0]
	g.toolIDs = g.toolIDs[1:]
	return id, nil
}

type staticToolSource struct {
	tools []DiscoveredTool
	err   error
}

func (s staticToolSource) DiscoverTools(
	context.Context,
	domain.Provider,
) ([]DiscoveredTool, error) {
	if s.err != nil {
		return nil, s.err
	}
	result := make([]DiscoveredTool, len(s.tools))
	copy(result, s.tools)
	return result, nil
}

type memoryCatalog struct {
	providersByID       map[domain.ProviderID]domain.Provider
	providerByNamespace map[domain.Namespace]domain.ProviderID
	toolsByID           map[domain.ToolID]domain.Tool
	toolsByProvider     map[domain.ProviderID][]domain.ToolID
}

func newMemoryCatalog() *memoryCatalog {
	return &memoryCatalog{
		providersByID:       make(map[domain.ProviderID]domain.Provider),
		providerByNamespace: make(map[domain.Namespace]domain.ProviderID),
		toolsByID:           make(map[domain.ToolID]domain.Tool),
		toolsByProvider:     make(map[domain.ProviderID][]domain.ToolID),
	}
}

func (m *memoryCatalog) CreateProvider(_ context.Context, provider domain.Provider) error {
	if _, exists := m.providerByNamespace[provider.Namespace()]; exists {
		return ErrNamespaceConflict
	}
	m.providersByID[provider.ID()] = provider
	m.providerByNamespace[provider.Namespace()] = provider.ID()
	return nil
}

func (m *memoryCatalog) UpdateProvider(
	_ context.Context,
	provider domain.Provider,
) error {
	current, exists := m.providersByID[provider.ID()]
	if !exists {
		return ErrProviderNotFound
	}
	if current.Namespace() != provider.Namespace() {
		return errors.New("memory catalog provider namespace changed")
	}
	m.providersByID[provider.ID()] = provider
	return nil
}

func (m *memoryCatalog) ListProviders(context.Context) ([]domain.Provider, error) {
	providers := make([]domain.Provider, 0, len(m.providersByID))
	for _, provider := range m.providersByID {
		providers = append(providers, provider)
	}
	return providers, nil
}

func (m *memoryCatalog) FindProviderByNamespace(
	_ context.Context,
	namespace domain.Namespace,
) (domain.Provider, error) {
	id, exists := m.providerByNamespace[namespace]
	if !exists {
		return domain.Provider{}, ErrProviderNotFound
	}
	return m.providersByID[id], nil
}

func (m *memoryCatalog) RemoveProvider(
	_ context.Context,
	id domain.ProviderID,
) error {
	provider, exists := m.providersByID[id]
	if !exists {
		return ErrProviderNotFound
	}
	delete(m.providersByID, id)
	delete(m.providerByNamespace, provider.Namespace())
	for _, toolID := range m.toolsByProvider[id] {
		delete(m.toolsByID, toolID)
	}
	delete(m.toolsByProvider, id)
	return nil
}

func (m *memoryCatalog) GetProvider(
	_ context.Context,
	id domain.ProviderID,
) (domain.Provider, error) {
	provider, exists := m.providersByID[id]
	if !exists {
		return domain.Provider{}, ErrProviderNotFound
	}
	return provider, nil
}

func (m *memoryCatalog) ListProviderTools(
	_ context.Context,
	providerID domain.ProviderID,
) ([]domain.Tool, error) {
	ids := m.toolsByProvider[providerID]
	tools := make([]domain.Tool, 0, len(ids))
	for _, id := range ids {
		tools = append(tools, m.toolsByID[id])
	}
	return tools, nil
}

func (m *memoryCatalog) ReplaceProviderTools(
	_ context.Context,
	providerID domain.ProviderID,
	tools []domain.Tool,
) error {
	for _, id := range m.toolsByProvider[providerID] {
		delete(m.toolsByID, id)
	}

	ids := make([]domain.ToolID, 0, len(tools))
	for _, tool := range tools {
		m.toolsByID[tool.ID()] = tool
		ids = append(ids, tool.ID())
	}
	m.toolsByProvider[providerID] = ids
	return nil
}

func (m *memoryCatalog) GetTool(
	_ context.Context,
	id domain.ToolID,
) (domain.Tool, error) {
	tool, exists := m.toolsByID[id]
	if !exists {
		return domain.Tool{}, ErrToolNotFound
	}
	return tool, nil
}

func (m *memoryCatalog) SearchTools(
	_ context.Context,
	query string,
	limit int,
	scope ToolSearchScope,
) ([]domain.Tool, error) {
	query = strings.ToLower(query)
	result := make([]domain.Tool, 0, limit)

	allowed := make(map[domain.ToolID]struct{}, len(scope.AllowedToolIDs))
	for _, id := range scope.AllowedToolIDs {
		allowed[id] = struct{}{}
	}

	for _, tool := range m.toolsByID {
		if scope.ProviderID != nil && tool.ProviderID() != *scope.ProviderID {
			continue
		}
		if scope.AllowedToolIDs != nil {
			if _, visible := allowed[tool.ID()]; !visible {
				continue
			}
		}
		haystack := strings.ToLower(fmt.Sprintf(
			"%s %s %s %s",
			tool.Name(),
			tool.Title(),
			tool.Description(),
			strings.Join(tool.Tags(), " "),
		))
		if strings.Contains(haystack, query) {
			result = append(result, tool)
			if len(result) == limit {
				break
			}
		}
	}
	return result, nil
}

func catalogWithProvider(t *testing.T) (*memoryCatalog, domain.ProviderID) {
	t.Helper()

	store := newMemoryCatalog()
	providerID := mustProviderID(t, "prov_00000000000000000000000000000001")
	namespace, err := domain.NewNamespace("github")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := domain.NewProvider(providerID, namespace, "GitHub", "", testTime())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateProvider(context.Background(), provider); err != nil {
		t.Fatal(err)
	}
	return store, providerID
}

func mustProviderID(t *testing.T, raw string) domain.ProviderID {
	t.Helper()
	id, err := domain.ParseProviderID(raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func mustToolID(t *testing.T, raw string) domain.ToolID {
	t.Helper()
	id, err := domain.ParseToolID(raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func mustSchema(t *testing.T, raw string) domain.Schema {
	t.Helper()
	schema, err := domain.NewSchema([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	return schema
}

func testTime() time.Time {
	return time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
}
