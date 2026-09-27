package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	policyapplication "github.com/orz-i/Mender/backend/internal/policy/application"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func Open(ctx context.Context, path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("open policy sqlite: path must not be empty")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("open policy sqlite: resolve path: %w", err)
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(absolute)}
	query := u.Query()
	query.Add("_pragma", "foreign_keys(1)")
	query.Add("_pragma", "busy_timeout(5000)")
	u.RawQuery = query.Encode()

	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, fmt.Errorf("open policy sqlite: %w", err)
	}
	store := &Store{db: db}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open policy sqlite: ping: %w", err)
	}
	if err := migrate(ctx, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open policy sqlite: migrate: %w", err)
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) GetPolicySet(ctx context.Context) (policydomain.PolicySet, error) {
	var rawDefault string
	if err := s.db.QueryRowContext(
		ctx,
		"SELECT default_decision FROM policy_settings WHERE singleton_id = 1",
	).Scan(&rawDefault); err != nil {
		return policydomain.PolicySet{}, fmt.Errorf("get policy settings: %w", err)
	}
	defaultDecision := policydomain.DecisionKind(rawDefault)
	if err := policydomain.ValidateDecisionKind(defaultDecision); err != nil {
		return policydomain.PolicySet{}, fmt.Errorf("restore policy default: %w", err)
	}

	rows, err := s.db.QueryContext(
		ctx,
		`SELECT rule_id, display_name, enabled, decision, match_json, created_at, updated_at
		FROM policy_rules
		ORDER BY rule_id`,
	)
	if err != nil {
		return policydomain.PolicySet{}, fmt.Errorf("list policy rules: %w", err)
	}
	defer func() { _ = rows.Close() }()

	rules := make([]policydomain.Rule, 0)
	for rows.Next() {
		rule, err := scanRule(rows)
		if err != nil {
			return policydomain.PolicySet{}, fmt.Errorf("list policy rules: %w", err)
		}
		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		return policydomain.PolicySet{}, fmt.Errorf("list policy rules: rows: %w", err)
	}
	policy, err := policydomain.NewPolicySet(defaultDecision, rules)
	if err != nil {
		return policydomain.PolicySet{}, fmt.Errorf("restore policy set: %w", err)
	}
	return policy, nil
}

func (s *Store) SetDefaultDecision(
	ctx context.Context,
	decision policydomain.DecisionKind,
	now time.Time,
) error {
	if err := policydomain.ValidateDecisionKind(decision); err != nil {
		return err
	}
	result, err := s.db.ExecContext(
		ctx,
		`UPDATE policy_settings
		SET default_decision = ?, updated_at = ?
		WHERE singleton_id = 1`,
		string(decision),
		now.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("set policy default: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("set policy default: rows affected: %w", err)
	}
	if count != 1 {
		return fmt.Errorf("set policy default: settings row is missing")
	}
	return nil
}

func (s *Store) CreateRule(ctx context.Context, rule policydomain.Rule) error {
	matchJSON, err := encodeMatch(rule.Match())
	if err != nil {
		return fmt.Errorf("create policy rule: encode match: %w", err)
	}
	_, err = s.db.ExecContext(
		ctx,
		`INSERT INTO policy_rules(
			rule_id, display_name, enabled, decision, match_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		rule.ID().String(),
		rule.DisplayName(),
		boolInt(rule.Enabled()),
		string(rule.Decision()),
		matchJSON,
		rule.CreatedAt().Format(time.RFC3339Nano),
		rule.UpdatedAt().Format(time.RFC3339Nano),
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") ||
			strings.Contains(strings.ToLower(err.Error()), "primary key") {
			return policyapplication.ErrRuleConflict
		}
		return fmt.Errorf("create policy rule: %w", err)
	}
	return nil
}

func (s *Store) GetRule(
	ctx context.Context,
	id policydomain.RuleID,
) (policydomain.Rule, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT rule_id, display_name, enabled, decision, match_json, created_at, updated_at
		FROM policy_rules WHERE rule_id = ?`,
		id.String(),
	)
	rule, err := scanRule(row)
	if errors.Is(err, sql.ErrNoRows) {
		return policydomain.Rule{}, policyapplication.ErrRuleNotFound
	}
	if err != nil {
		return policydomain.Rule{}, fmt.Errorf("get policy rule: %w", err)
	}
	return rule, nil
}

func (s *Store) UpdateRule(ctx context.Context, rule policydomain.Rule) error {
	matchJSON, err := encodeMatch(rule.Match())
	if err != nil {
		return fmt.Errorf("update policy rule: encode match: %w", err)
	}
	result, err := s.db.ExecContext(
		ctx,
		`UPDATE policy_rules
		SET display_name = ?, enabled = ?, decision = ?, match_json = ?,
			created_at = ?, updated_at = ?
		WHERE rule_id = ?`,
		rule.DisplayName(),
		boolInt(rule.Enabled()),
		string(rule.Decision()),
		matchJSON,
		rule.CreatedAt().Format(time.RFC3339Nano),
		rule.UpdatedAt().Format(time.RFC3339Nano),
		rule.ID().String(),
	)
	if err != nil {
		return fmt.Errorf("update policy rule: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update policy rule: rows affected: %w", err)
	}
	if count == 0 {
		return policyapplication.ErrRuleNotFound
	}
	return nil
}

func (s *Store) DeleteRule(ctx context.Context, id policydomain.RuleID) error {
	result, err := s.db.ExecContext(
		ctx,
		"DELETE FROM policy_rules WHERE rule_id = ?",
		id.String(),
	)
	if err != nil {
		return fmt.Errorf("delete policy rule: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete policy rule: rows affected: %w", err)
	}
	if count == 0 {
		return policyapplication.ErrRuleNotFound
	}
	return nil
}

type rowScanner interface {
	Scan(...any) error
}

func scanRule(scanner rowScanner) (policydomain.Rule, error) {
	var rawID, displayName, rawDecision, rawMatch, rawCreatedAt, rawUpdatedAt string
	var enabled int
	if err := scanner.Scan(
		&rawID,
		&displayName,
		&enabled,
		&rawDecision,
		&rawMatch,
		&rawCreatedAt,
		&rawUpdatedAt,
	); err != nil {
		return policydomain.Rule{}, err
	}
	id, err := policydomain.ParseRuleID(rawID)
	if err != nil {
		return policydomain.Rule{}, fmt.Errorf("parse policy rule id: %w", err)
	}
	match, err := decodeMatch(rawMatch)
	if err != nil {
		return policydomain.Rule{}, fmt.Errorf("parse policy rule match: %w", err)
	}
	createdAt, err := time.Parse(time.RFC3339Nano, rawCreatedAt)
	if err != nil {
		return policydomain.Rule{}, fmt.Errorf("parse policy rule created_at: %w", err)
	}
	updatedAt, err := time.Parse(time.RFC3339Nano, rawUpdatedAt)
	if err != nil {
		return policydomain.Rule{}, fmt.Errorf("parse policy rule updated_at: %w", err)
	}
	return policydomain.RestoreRule(
		id,
		displayName,
		enabled == 1,
		policydomain.DecisionKind(rawDecision),
		match,
		createdAt,
		updatedAt,
	)
}

type matchRecord struct {
	Origins       []string         `json:"origins"`
	ScopeKinds    []string         `json:"scopeKinds"`
	ProfileRefs   []string         `json:"profileRefs"`
	ProviderIDs   []string         `json:"providerIds"`
	ToolIDs       []string         `json:"toolIds"`
	FactKnowledge string           `json:"factKnowledge"`
	EffectsAny    []string         `json:"effectsAny"`
	ResourcesAny  []resourceRecord `json:"resourcesAny"`
}

type resourceRecord struct {
	Kind     string `json:"kind"`
	Operator string `json:"operator"`
	Value    string `json:"value"`
}

func encodeMatch(match policydomain.Match) (string, error) {
	record := matchRecord{
		Origins:       stringsFromOrigins(match.Origins()),
		ScopeKinds:    stringsFromScopeKinds(match.ScopeKinds()),
		ProfileRefs:   match.ProfileRefs(),
		ProviderIDs:   match.ProviderIDs(),
		ToolIDs:       match.ToolIDs(),
		FactKnowledge: string(match.FactKnowledge()),
		EffectsAny:    match.EffectsAny(),
		ResourcesAny:  resourceRecords(match.ResourcesAny()),
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func decodeMatch(raw string) (policydomain.Match, error) {
	var record matchRecord
	if err := json.Unmarshal([]byte(raw), &record); err != nil {
		return policydomain.Match{}, err
	}
	resources := make([]policydomain.ResourceMatcher, 0, len(record.ResourcesAny))
	for i, item := range record.ResourcesAny {
		matcher, err := policydomain.NewResourceMatcher(
			item.Kind,
			policydomain.ResourceOperator(item.Operator),
			item.Value,
		)
		if err != nil {
			return policydomain.Match{}, fmt.Errorf("resourcesAny[%d]: %w", i, err)
		}
		resources = append(resources, matcher)
	}
	return policydomain.NewMatch(policydomain.MatchSpec{
		Origins:       originsFromStrings(record.Origins),
		ScopeKinds:    scopeKindsFromStrings(record.ScopeKinds),
		ProfileRefs:   record.ProfileRefs,
		ProviderIDs:   record.ProviderIDs,
		ToolIDs:       record.ToolIDs,
		FactKnowledge: policydomain.FactKnowledge(record.FactKnowledge),
		EffectsAny:    record.EffectsAny,
		ResourcesAny:  resources,
	})
}

func stringsFromOrigins(values []policydomain.Origin) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, string(value))
	}
	return result
}

func originsFromStrings(values []string) []policydomain.Origin {
	result := make([]policydomain.Origin, 0, len(values))
	for _, value := range values {
		result = append(result, policydomain.Origin(value))
	}
	return result
}

func stringsFromScopeKinds(values []policydomain.ScopeKind) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, string(value))
	}
	return result
}

func scopeKindsFromStrings(values []string) []policydomain.ScopeKind {
	result := make([]policydomain.ScopeKind, 0, len(values))
	for _, value := range values {
		result = append(result, policydomain.ScopeKind(value))
	}
	return result
}

func resourceRecords(values []policydomain.ResourceMatcher) []resourceRecord {
	result := make([]resourceRecord, 0, len(values))
	for _, value := range values {
		result = append(result, resourceRecord{
			Kind:     value.Kind(),
			Operator: string(value.Operator()),
			Value:    value.Value(),
		})
	}
	return result
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

var _ policyapplication.Repository = (*Store)(nil)
