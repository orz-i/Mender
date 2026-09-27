package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"slices"
	"testing"

	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	_ "modernc.org/sqlite"
)

func TestOpenMigratesProfileV1ToToolBindingColumns(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "mender.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE profile_schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE profiles (
		id TEXT PRIMARY KEY,
		slug TEXT NOT NULL UNIQUE,
		display_name TEXT NOT NULL,
		exposure_mode TEXT NOT NULL
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE profile_tools (
		profile_id TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
		tool_id TEXT NOT NULL,
		direct INTEGER NOT NULL CHECK (direct IN (0, 1)),
		PRIMARY KEY(profile_id, tool_id)
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		"INSERT INTO profile_schema_migrations(version, applied_at) VALUES (1, 'v1')",
	); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO profiles(id, slug, display_name, exposure_mode)
		 VALUES (?, ?, ?, ?)`,
		"prof_00000000000000000000000000000001",
		"default",
		"Default",
		"hybrid",
	); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO profile_tools(profile_id, tool_id, direct)
		 VALUES (?, ?, ?)`,
		"prof_00000000000000000000000000000001",
		"tool_00000000000000000000000000000001",
		1,
	); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() { _ = store.Close() }()

	var version int
	if err := store.db.QueryRowContext(
		context.Background(),
		"SELECT MAX(version) FROM profile_schema_migrations",
	).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 3 {
		t.Fatalf("schema version = %d, want 3", version)
	}

	rows, err := store.db.QueryContext(
		context.Background(),
		"PRAGMA table_info(profile_tools)",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()

	var columns []string
	for rows.Next() {
		var (
			cid          int
			name         string
			columnType   string
			notNull      int
			defaultValue any
			primaryKey   int
		)
		if err := rows.Scan(
			&cid,
			&name,
			&columnType,
			&notNull,
			&defaultValue,
			&primaryKey,
		); err != nil {
			t.Fatal(err)
		}
		columns = append(columns, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"alias",
		"title_override",
		"description_override",
		"policy_constraint",
		"concurrency_constraint",
		"credential_refs_json",
	} {
		if !slices.Contains(columns, expected) {
			t.Fatalf("profile_tools columns = %v, missing %q", columns, expected)
		}
	}

	slug, _ := profiledomain.NewSlug("default")
	profile, err := store.GetProfileBySlug(context.Background(), slug)
	if err != nil {
		t.Fatalf("GetProfileBySlug() error = %v", err)
	}
	bindings := profile.Bindings()
	if len(bindings) != 1 {
		t.Fatalf("Bindings() = %#v", bindings)
	}
	binding := bindings[0]
	if !binding.Direct() ||
		binding.Alias() != "" ||
		binding.TitleOverride() != "" ||
		binding.DescriptionOverride() != "" ||
		binding.PolicyConstraint() != profiledomain.PolicyInherit ||
		binding.ConcurrencyConstraint() != profiledomain.ConcurrencyInherit ||
		len(binding.CredentialRefs()) != 0 {
		t.Fatalf("migrated binding = %#v", binding)
	}
}
