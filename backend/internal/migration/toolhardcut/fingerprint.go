package toolhardcut

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// semanticFingerprint covers every physical user, migration, audit, plugin,
// approval, grant, and FTS table, not only Tool-owned tables. A credential
// update or late Agent grant must invalidate the review manifest even if
// Catalog/Policy IDs did not change. The report contains only a digest.
//
// All table/row material remains in the process, never in Report JSON.
// Per-table row hashes are sorted to avoid making SQLite scan order part of
// the on-disk source identity (including WITHOUT ROWID tables).
func semanticFingerprint(ctx context.Context, tx *sql.Tx) (string, error) {
	h := sha256.New()
	rows, err := tx.QueryContext(ctx, `SELECT type,name,coalesce(sql,'') FROM sqlite_master
  WHERE type IN ('table','view','index','trigger') ORDER BY type,name`)
	if err != nil {
		return "", err
	}
	tables := []string{}
	for rows.Next() {
		var kind, name, ddl string
		if err := rows.Scan(&kind, &name, &ddl); err != nil {
			_ = rows.Close()
			return "", err
		}
		appendFingerprint(h, kind, name, ddl)
		if kind == "table" {
			tables = append(tables, name)
		}
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return "", err
	}
	for _, table := range tables {
		// Names originate from sqlite_master, not user data. Quote-escape them
		// anyway so unusual legacy schema names cannot alter the SQL statement.
		escaped := strings.ReplaceAll(table, `"`, `""`)
		// #nosec G202 -- SELECT-only query; identifier is from sqlite_master and double-quote escaped.
		records, err := tx.QueryContext(ctx, `SELECT * FROM "`+escaped+`"`)
		if err != nil {
			return "", fmt.Errorf("fingerprint %s: %w", table, err)
		}
		cols, err := records.Columns()
		if err != nil {
			_ = records.Close()
			return "", err
		}
		appendFingerprint(h, "table-data", table, strings.Join(cols, "\x00"))
		cells := make([]any, len(cols))
		pointers := make([]any, len(cols))
		for i := range pointers {
			pointers[i] = &cells[i]
		}
		rowDigests := []string{}
		for records.Next() {
			if err := records.Scan(pointers...); err != nil {
				_ = records.Close()
				return "", err
			}
			rowHash := sha256.New()
			for _, value := range cells {
				bytes, err := json.Marshal(value)
				if err != nil {
					_ = records.Close()
					return "", err
				}
				appendFingerprint(rowHash, string(bytes))
			}
			rowDigests = append(rowDigests, hex.EncodeToString(rowHash.Sum(nil)))
		}
		err = records.Err()
		_ = records.Close()
		if err != nil {
			return "", err
		}
		sort.Strings(rowDigests)
		for _, fingerprint := range rowDigests {
			appendFingerprint(h, "row", fingerprint)
		}
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}
