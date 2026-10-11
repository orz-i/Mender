package sqlite

import (
	"context"
	"fmt"
	"time"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sshremote "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/sshremote"
)

// CreateReadySSH atomically pins a verified SSH remote configuration and
// the owner-admitted ready run. No private key material reaches SQLite.
func (s *Store) CreateReadySSH(ctx context.Context, item sandboxdomain.Sandbox, run sandboxdomain.Run, t sshremote.Template) error {
	if s == nil || s.db == nil || item.Backend() != sandboxdomain.BackendSSHRemote || item.Validate() != nil ||
		run.SandboxID != item.ID() || run.State != sandboxdomain.StateReady || run.Generation != 1 ||
		run.ExternalID != "ssh_remote:"+item.ID().String() || t.Validate() != nil {
		return sandboxdomain.ErrBackendUnavailable
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	spec := item.Spec()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = tx.ExecContext(ctx, `INSERT INTO sandboxes(
 id,display_name,backend,backend_plugin_id,backend_extension_id,backend_installation_id,isolation,local_root,owner_kind,owner_ref,created_at)
 VALUES(?,?,?,?,?,?,?,?,?,?,?)`, spec.ID.String(), spec.Name, string(spec.Backend),
		spec.Binding.PluginID.String(), spec.Binding.ExtensionID.String(), spec.Binding.InstallationID.String(),
		string(spec.Isolation), spec.LocalRoot, spec.OwnerKind, spec.OwnerRef, now)
	if err != nil {
		return fmt.Errorf("store SSH sandbox identity: %w", err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO sandbox_ssh_remote_templates(
 sandbox_id,credential_ref,host,port,login_user,workspace_root,pinned_host_key,created_at)
 VALUES(?,?,?,?,?,?,?,?)`, spec.ID.String(), t.CredentialRef.String(), t.Host, t.Port, t.User, t.Root, t.HostPublicKey, now)
	if err != nil {
		return fmt.Errorf("store SSH immutable binding: %w", err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO sandbox_runs(sandbox_id,generation,state,external_id,updated_at)
 VALUES(?,?,?,?,?)`, run.SandboxID.String(), run.Generation, string(run.State), run.ExternalID, now)
	if err != nil {
		return fmt.Errorf("store SSH run: %w", err)
	}
	return tx.Commit()
}
func (s *Store) GetSSHRemoteTemplate(ctx context.Context, id sandboxdomain.ID) (sshremote.Template, error) {
	if s == nil || s.db == nil {
		return sshremote.Template{}, sandboxdomain.ErrBackendUnavailable
	}
	var t sshremote.Template
	var port int
	var ref string
	err := s.db.QueryRowContext(ctx, `SELECT host,port,login_user,workspace_root,pinned_host_key,credential_ref
 FROM sandbox_ssh_remote_templates WHERE sandbox_id=?`, id.String()).Scan(
		&t.Host, &port, &t.User, &t.Root, &t.HostPublicKey, &ref)
	if err != nil {
		return sshremote.Template{}, sandboxdomain.ErrBackendUnavailable
	}
	if port < 1 || port > 65535 {
		return sshremote.Template{}, sandboxdomain.ErrBackendUnavailable
	}
	t.Port = uint16(port)
	t.CredentialRef = credentialdomain.Reference(ref)
	if t.Validate() != nil {
		return sshremote.Template{}, sandboxdomain.ErrBackendUnavailable
	}
	return t, nil
}
func (s *Store) SSHCredentialBindings(ctx context.Context, ref credentialdomain.Reference) ([]sandboxdomain.ID, error) {
	if s == nil || s.db == nil {
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	rows, err := s.db.QueryContext(ctx, `SELECT t.sandbox_id FROM sandbox_ssh_remote_templates t
 JOIN sandbox_runs r ON r.sandbox_id=t.sandbox_id
 WHERE t.credential_ref=? AND r.state!='destroyed'
 ORDER BY t.sandbox_id`, ref.String())
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := make([]sandboxdomain.ID, 0)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		id, err := sandboxdomain.ParseID(raw)
		if err != nil {
			return nil, err
		}
		result = append(result, id)
	}
	return result, rows.Err()
}

// Compile-time interface check: TemplateLookup for the guarded SSH backend.
var _ sshremote.TemplateLookup = (*Store)(nil)
