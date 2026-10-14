package sshremote

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"strings"

	"github.com/pkg/sftp"
)

// v2HostOperations is exclusively an SSH backend adapter. The full contract
// must be implemented; a legacy v1 runner cannot pretend to provide it.
type v2HostOperations interface {
	VerifyV2(context.Context, string) error
	PersistV2Directory(context.Context, string) error
	ResolveV2UnderLock(context.Context, v2RemoteResolution) error
}
type v2RemoteResolution struct {
	TaskDir       string
	ManifestHash  string
	ScriptHash    string
	LaunchHash    string
	LaunchPresent bool
	Marker        []byte
}

// v2Durability is used only by the in-process fixture to model fsync and
// injected lost acknowledgements. In production we ALWAYS call SFTP File.Sync.
type v2Durability func(*sftp.File) error

func (s *jobStore) v2FileSyncAvailable() bool {
	if s == nil || s.sftp == nil {
		return false
	}
	if s.v2SyncFixture != nil {
		return true
	}
	version, ok := s.sftp.HasExtension("fsync@openssh.com")
	return ok && version == "1"
}

// writeV2Exclusive is never a fallback from v1. A partial exclusive create is
// sticky unknown; it may not be overwritten or replayed after a timeout.
func (s *jobStore) writeV2Exclusive(ctx context.Context, id, name string, data []byte, host v2HostOperations) error {
	if !s.v2FileSyncAvailable() || host == nil {
		return ErrJobUnavailable
	}
	if err := s.checkTask(ctx, id); err != nil {
		return ErrJobUnknown
	}
	if len(data) == 0 || len(data) > 128<<10 {
		return ErrJobInvalid
	}
	switch name {
	case "manifest.json", "run.sh", "launch.intent":
	default:
		return ErrJobInvalid
	}
	p := path.Join(s.taskPath(id), name)
	f, err := s.sftp.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
	if err != nil {
		return ErrJobUnknown
	}
	if err = s.sftp.Chmod(p, 0600); err != nil {
		_ = f.Close()
		return ErrJobUnknown
	}
	written, writeErr := f.Write(data)
	if written != len(data) || writeErr != nil {
		_ = f.Close()
		return ErrJobUnknown
	}
	syncFile := f.Sync
	if s.v2SyncFixture != nil {
		syncFile = func() error { return s.v2SyncFixture(f) }
	}
	syncErr := syncFile()
	closeErr := f.Close()
	if syncErr != nil || closeErr != nil || ctx.Err() != nil {
		return ErrJobUnknown
	}
	if err = host.PersistV2Directory(ctx, s.taskPath(id)); err != nil {
		return ErrJobUnknown
	}
	// Verify the exact byte content after durability acknowledgement. Never
	// infer a complete record from a close or an unverified rename.
	got, err := s.readFile(ctx, id, name, len(data))
	if err != nil || !bytesEqual(got, data) {
		return ErrJobUnknown
	}
	return nil
}
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (s *jobStore) readV2Manifest(ctx context.Context, id string) (v2GateManifest, []byte, error) {
	raw, err := s.readFile(ctx, id, "manifest.json", 2048)
	if err != nil {
		return v2GateManifest{}, nil, ErrJobUnknown
	}
	var m v2GateManifest
	if json.Unmarshal(raw, &m) != nil || m.Version != 2 || m.TaskID != id || m.SandboxID != s.sandboxID.String() ||
		m.Unit != "mender-"+id+".service" || !hex64(m.CommandDigest) || !hex64(m.ScriptDigest) ||
		!hex32(m.AttemptID) || m.RunGeneration == 0 {
		return v2GateManifest{}, nil, ErrJobUnknown
	}
	return m, raw, nil
}

func hex64(s string) bool { return len(s) == 64 && strings.Trim(s, "0123456789abcdef") == "" }
func hex32(s string) bool { return len(s) == 32 && strings.Trim(s, "0123456789abcdef") == "" }

type v2Evidence struct {
	manifest      v2GateManifest
	manifestHash  string
	scriptHash    string
	launchHash    string
	launchPresent bool
	resolved      bool
	started       bool
	startPartial  bool
	terminal      bool
	cancel        bool
	exitCode      *int
	fingerprint   string
	resolution    v2ResolutionMarker
}

// v2ResolutionMarker is operational disposition only, never a shell result.
// It is immutable plugin-owned provenance and does not confer Profile rights.
type v2ResolutionMarker struct {
	Version             int    `json:"version"`
	TaskID              string `json:"taskId"`
	Disposition         string `json:"disposition"`
	ManifestHash        string `json:"manifestHash"`
	ScriptHash          string `json:"scriptHash"`
	AttemptID           string `json:"attemptId"`
	EvidenceFingerprint string `json:"evidenceFingerprint"`
	Actor               string `json:"actor"`
	ReasonDigest        string `json:"reasonDigest"`
}

// readV2Evidence enforces exact prepared file identity and private permissions
// before a record can ever become an operational recovery candidate. A mixed
// state, partial file or symlink fails closed; absence is not negative proof.
func (s *jobStore) readV2Evidence(ctx context.Context, id string) (v2Evidence, error) {
	m, raw, err := s.readV2Manifest(ctx, id)
	if err != nil {
		return v2Evidence{}, err
	}
	ev := v2Evidence{manifest: m}
	manifestHash := sha256.Sum256(raw)
	ev.manifestHash = hex.EncodeToString(manifestHash[:])
	script, err := s.readFile(ctx, id, "run.sh", 128<<10)
	if err != nil {
		return ev, ErrJobUnknown
	}
	scriptHash := sha256.Sum256(script)
	ev.scriptHash = hex.EncodeToString(scriptHash[:])
	if ev.scriptHash != m.ScriptDigest {
		return ev, ErrJobUnknown
	}
	if strings.Contains(string(script), "\x00") {
		return ev, ErrJobUnknown
	}
	// A trusted first-party v2 script is a non-bypassable execution gate. A
	// stronger script signature may be added later; no unverified path is used.
	for _, required := range []string{"flock -x 9", "resolution.json", "start.intent", "sync -f"} {
		if !strings.Contains(string(script), required) {
			return ev, ErrJobUnknown
		}
	}
	optional := func(name string, max int) ([]byte, bool, error) {
		p := path.Join(s.taskPath(id), name)
		info, e := s.sftp.Lstat(p)
		if errors.Is(e, os.ErrNotExist) {
			return nil, false, nil
		}
		if e != nil || !info.Mode().IsRegular() || info.Mode()&fs.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 ||
			info.Size() > int64(max) || info.Size() < 0 {
			return nil, false, ErrJobUnknown
		}
		content, e := s.readFile(ctx, id, name, max)
		if e != nil {
			return nil, false, ErrJobUnknown
		}
		return content, true, nil
	}
	if b, exists, e := optional("launch.intent", 256); e != nil {
		return ev, e
	} else if exists {
		ev.launchPresent = true
		want := "v2:" + id + ":" + m.AttemptID + ":" + m.CommandDigest + "\n"
		if string(b) != want {
			return ev, ErrJobUnknown
		}
		hash := sha256.Sum256(b)
		ev.launchHash = hex.EncodeToString(hash[:])
	}
	// A partial start marker means the gate may have run, but no durable commit
	// was proven. That blocks recovery and lifecycle, never fabricates failure.
	if _, exists, e := optional("start.intent.tmp", 256); e != nil {
		return ev, e
	} else if exists {
		ev.startPartial = true
	}
	if b, exists, e := optional("start.intent", 256); e != nil {
		return ev, e
	} else if exists {
		if string(b) != "v2:"+id+":"+m.AttemptID+":"+m.CommandDigest+"\n" {
			return ev, ErrJobUnknown
		}
		ev.started = true
	}
	if _, exists, e := optional("exit.tmp", 32); e != nil {
		return ev, e
	} else if exists {
		ev.startPartial = true
	}
	if b, exists, e := optional("exit", 32); e != nil {
		return ev, e
	} else if exists {
		if !ev.started {
			return ev, ErrJobUnknown
		}
		code, e := parseExit(strings.TrimSuffix(string(b), "\n"))
		if e != nil {
			return ev, ErrJobUnknown
		}
		ev.terminal = true
		ev.exitCode = &code
	}
	if _, exists, e := optional("cancel", 256); e != nil {
		return ev, e
	} else if exists {
		ev.cancel = true
	}
	if b, exists, e := optional("resolution.json", 2048); e != nil {
		return ev, e
	} else if exists {
		if json.Unmarshal(b, &ev.resolution) != nil || ev.resolution.Version != 2 ||
			ev.resolution.TaskID != id || ev.resolution.Disposition != "aborted_before_launch" ||
			ev.resolution.ManifestHash != ev.manifestHash || ev.resolution.ScriptHash != ev.scriptHash ||
			ev.resolution.AttemptID != m.AttemptID ||
			ev.resolution.EvidenceFingerprint != v2EvidenceFingerprint(id, ev, false) ||
			!hex64(ev.resolution.ReasonDigest) || ev.resolution.Actor == "" || ev.started || ev.terminal || ev.startPartial {
			return ev, ErrJobUnknown
		}
		ev.resolved = true
	}
	// Fingerprint is recomputable and binds immutable manifest/script,
	// launch evidence, and mutable decision/terminal state. A post-resolve
	// read also verifies that the marker named exactly its pre-decision proof.
	ev.fingerprint = v2EvidenceFingerprint(id, ev, ev.resolved)
	return ev, nil
}
func v2EvidenceFingerprint(id string, ev v2Evidence, resolved bool) string {
	signature := strings.Join([]string{"mender-ssh-v2-evidence", id, ev.manifestHash, ev.scriptHash,
		ev.launchHash, fmt.Sprint(ev.launchPresent), fmt.Sprint(ev.started), fmt.Sprint(ev.startPartial),
		fmt.Sprint(ev.terminal), fmt.Sprint(ev.cancel), fmt.Sprint(resolved)}, ":")
	digest := sha256.Sum256([]byte(signature))
	return hex.EncodeToString(digest[:])
}
