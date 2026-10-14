package sqlite

import (
	"context"
	"errors"
	"testing"

	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

// Destroy changes the Run state, not the Sandbox row. Even valid foreign keys
// therefore cannot prevent logical deletion with a live, resource-owned
// Tool Suite / Provider facade. Verify both explicit bindings and pre-claim
// provider configurations on a second independent SQLite connection.
func TestSSHDestroyAtomicallyChecksEveryToolSuiteReference(t *testing.T) {
	cases := []struct{ name, plugin, version, association string }{
		{"unclaimed_workspace", "io.mender.provider.workspace", "2", ""},
		{"unclaimed_jobs", "io.mender.provider.sandbox-jobs", "1", ""},
		{"claimed_workspace", "", "", "sandbox_tool_suite_bindings"},
		{"claimed_jobs", "", "", "sandbox_job_suite_bindings"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			filename, id, expected, owner := newFencedSSHFixture(t)
			ctx := context.Background()
			first, err := Open(ctx, filename)
			if err != nil {
				t.Fatal(err)
			}
			defer first.Close()
			second, err := Open(ctx, filename)
			if err != nil {
				t.Fatal(err)
			}
			defer second.Close()

			if tc.association != "" {
				if _, err = first.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS providers (id TEXT PRIMARY KEY)`); err != nil {
					t.Fatal(err)
				}
				if _, err = first.db.ExecContext(ctx, `INSERT INTO providers(id) VALUES('provider_test')`); err != nil {
					t.Fatal(err)
				}
				var query string
				if tc.association == "sandbox_tool_suite_bindings" {
					query = `INSERT INTO sandbox_tool_suite_bindings(sandbox_id,suite_id,provider_id,created_at) VALUES(?,'sandbox.workspace','provider_test','now')`
				} else {
					query = `INSERT INTO sandbox_job_suite_bindings(sandbox_id,provider_id,created_at) VALUES(?,'provider_test','now')`
				}
				if _, err = first.db.ExecContext(ctx, query, id.String()); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err = first.db.ExecContext(ctx, `INSERT INTO provider_configs(plugin_id,plugin_installation_id,config_version,config_json) VALUES(?,NULL,?,?)`,
					tc.plugin, tc.version, `{"sandboxId":"`+id.String()+`"}`); err != nil {
					t.Fatal(err)
				}
			}

			if _, err = second.ChangeSSHRemoteLifecycle(ctx, expected, gov.ActionSandboxDestroy, owner); !errors.Is(err, sandboxdomain.ErrConflict) {
				t.Fatalf("destroy allowed with %s reference: %v", tc.name, err)
			}
			still, err := first.GetRun(ctx, id)
			if err != nil || still.State != sandboxdomain.StateReady || still.Generation != expected.Generation {
				t.Fatalf("failed Destroy changed Run: %+v %v", still, err)
			}

			// Explicitly release test-owned reference, after which the normal
			// passive SSH lifecycle (without enrolled Jobs) may proceed.
			if tc.association != "" {
				if _, err = first.db.ExecContext(ctx, "DELETE FROM "+tc.association+" WHERE sandbox_id=?", id.String()); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err = first.db.ExecContext(ctx, `DELETE FROM provider_configs WHERE plugin_id=?`, tc.plugin); err != nil {
					t.Fatal(err)
				}
			}
			destroyed, err := second.ChangeSSHRemoteLifecycle(ctx, expected, gov.ActionSandboxDestroy, owner)
			if err != nil || destroyed.State != sandboxdomain.StateDestroyed {
				t.Fatalf("reference released but Destroy failed: %+v %v", destroyed, err)
			}
		})
	}
}
