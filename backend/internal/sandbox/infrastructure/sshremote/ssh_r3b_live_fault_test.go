//go:build liveacceptance

package sshremote

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	"github.com/orz-i/Mender/backend/internal/credential/infrastructure/nativekeyring"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	_ "modernc.org/sqlite"
)

// Live fault injection is package-internal and build-tagged. Production
// code has no fault switch or synthetic Job API. A new TaskID is reserved
// ONLY inside the explicit dedicated acceptance Sandbox, not the original.
type r3bLivePrelaunchFaultRunner struct {
	base         systemdJobRunner
	persistCount int
	startCount   int
}

func (r *r3bLivePrelaunchFaultRunner) Start(ctx context.Context, unit, script string) error {
	r.startCount++
	return ErrJobUnknown // Test must fail if production reaches this point.
}
func (r *r3bLivePrelaunchFaultRunner) Observe(ctx context.Context, unit string) (JobState, error) {
	return r.base.Observe(ctx, unit)
}
func (r *r3bLivePrelaunchFaultRunner) Stop(ctx context.Context, unit string) error {
	return r.base.Stop(ctx, unit)
}
func (r *r3bLivePrelaunchFaultRunner) VerifyV2(ctx context.Context, base string) error {
	return r.base.VerifyV2(ctx, base)
}
func (r *r3bLivePrelaunchFaultRunner) PersistV2Directory(ctx context.Context, dir string) error {
	r.persistCount++
	// 1: base dir, 2: manifest file, 3: script file. Simulate a lost
	// durability ACK at this precise remote boundary, BEFORE launch.intent
	// is written and before any systemd-run is contacted.
	if r.persistCount == 3 {
		return ErrJobUnknown
	}
	return r.base.PersistV2Directory(ctx, dir)
}
func (r *r3bLivePrelaunchFaultRunner) ResolveV2UnderLock(ctx context.Context, req v2RemoteResolution) error {
	return r.base.ResolveV2UnderLock(ctx, req)
}
func (r *r3bLivePrelaunchFaultRunner) ProveV2ResolutionDurable(ctx context.Context, taskDir, digest string) error {
	return r.base.ProveV2ResolutionDurable(ctx, taskDir, digest)
}

type r3bKeychainResolver struct{}

func (r3bKeychainResolver) Resolve(ctx context.Context, ref credentialdomain.Reference) ([]byte, error) {
	// Never print or persist private key bytes. Matches Mender's own native
	// keychain source and requires an approved local user's unlocked Keychain.
	return (nativekeyring.Store{}).Get(ctx, ref)
}

func TestR3bRealRockyPrelaunchFaultInjection(t *testing.T) {
	if os.Getenv("MENDER_SSH_LIVE_FAULT_INJECTION") != "enabled" {
		t.Skip("real SSH prelaunch fault injection requires explicit operator opt-in")
	}
	db := strings.TrimSpace(os.Getenv("MENDER_DB"))
	if db == "" {
		t.Fatal("explicit MENDER_DB required")
	}
	id, err := sandboxdomain.ParseID(os.Getenv("MENDER_SSH_LIVE_TARGET_ID"))
	if err != nil {
		t.Fatal("explicit acceptance SandboxID required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	data, err := sql.Open("sqlite", db)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = data.Close() }()

	// Hard guard: never stage into Rocky user Sandbox; only the newly-created
	// dedicated acceptance resource is eligible. Do not trust a raw ID alone.
	var name, state string
	var generation uint64
	if err = data.QueryRowContext(ctx, `SELECT s.display_name,r.state,r.generation FROM sandboxes s
	    JOIN sandbox_runs r ON r.sandbox_id=s.id WHERE s.id=?`, id.String()).Scan(&name, &state, &generation); err != nil {
		t.Fatal(err)
	}
	if name != "R3b Rocky acceptance temporary" || state != "ready" || generation != 1 {
		t.Fatal("live fault injection refused non-dedicated or non-Ready resource")
	}
	var host string
	var port int
	var user, root, hostKey, rawReference string
	if err = data.QueryRowContext(ctx, `SELECT host,port,login_user,workspace_root,pinned_host_key,credential_ref
	    FROM sandbox_ssh_remote_templates WHERE sandbox_id=?`, id.String()).Scan(
		&host, &port, &user, &root, &hostKey, &rawReference); err != nil {
		t.Fatal(err)
	}
	ref, err := credentialdomain.ParseReference(rawReference)
	if err != nil {
		t.Fatal(err)
	}
	if user != "root" || port <= 0 || port > 65535 {
		t.Fatal("refuse unsupported SSH identity")
	}
	// #nosec G115 -- explicit SQLite port validated to be 1..65535.
	template := Template{Host: host, Port: uint16(port), User: user, Root: root, HostPublicKey: hostKey, CredentialRef: ref}
	if err = template.Validate(); err != nil {
		t.Fatal(err)
	}
	conn, err := (Dialer{Credentials: r3bKeychainResolver{}, Trust: NewHostTrustStore(filepath.Join(filepath.Dir(db), "ssh_known_hosts"))}).Connect(ctx, template)
	if err != nil {
		t.Fatalf("pinned SSH connection unavailable: %v", err)
	}
	defer func() { _ = conn.Close() }()
	runner := &r3bLivePrelaunchFaultRunner{base: systemdJobRunner{client: conn.SSH, user: template.User}}
	jobs, err := newJobs(id, template.Root, conn.SFTP, runner)
	if err != nil {
		t.Fatal(err)
	}
	if err = jobs.Prepare(ctx); err != nil {
		t.Fatal(err)
	}
	if err = jobs.CheckV2(ctx); err != nil {
		t.Fatalf("real fsync/flock prereq absent: %v", err)
	}
	before, err := jobs.InspectRecovery(ctx)
	if err != nil || len(before.Records) != 1 || before.Records[0].Phase != "terminal" {
		t.Fatalf("fault injection refused to modify unexpected inventory: %d records, err=%v", len(before.Records), err)
	}
	var randomKey [16]byte
	if _, err = rand.Read(randomKey[:]); err != nil {
		t.Fatal(err)
	}
	snapshot, err := jobs.Submit(ctx, JobRequest{
		IdempotencyKey: hex.EncodeToString(randomKey[:]),
		Command:        "printf 'mender-r3b-should-never-start\\n'",
	}, generation)
	if !errors.Is(err, ErrJobUnknown) || snapshot.TaskID == "" || runner.persistCount != 3 || runner.startCount != 0 {
		t.Fatalf("prelaunch fault injected at wrong stage: task=%s persisted=%d start=%d err=%v",
			snapshot.TaskID, runner.persistCount, runner.startCount, err)
	}
	after, err := jobs.InspectRecovery(ctx)
	if err != nil || len(after.Records) != 2 {
		t.Fatalf("fault record unreadable: count=%d err=%v", len(after.Records), err)
	}
	for _, ev := range after.Records {
		if ev.TaskID == snapshot.TaskID {
			if ev.Phase != "prelaunch_candidate" || !ev.Recoverable || ev.Activity != "uncertain" {
				t.Fatalf("real v2 prelaunch candidate unverified: %+v", ev)
			}
			if err = jobs.Quiescent(ctx); !errors.Is(err, ErrJobUnknown) {
				t.Fatalf("prelaunch candidate allowed Stop/Destroy before owner Resolve: %v", err)
			}
			t.Logf("LIVE_PRELAUNCH_FAULT_PASS: sandbox=%s taskId=%s generation=%d fingerprint=%s (no launch.intent, no systemd-run; owner Resolve required)", id, snapshot.TaskID, generation, ev.Fingerprint)
			return
		}
	}
	t.Fatalf("reserved task %s not visible after live prelaunch fault", snapshot.TaskID)
}
