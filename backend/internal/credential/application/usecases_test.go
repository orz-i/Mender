package application

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"testing"
	"time"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	"golang.org/x/crypto/ssh"
)

func TestCreateNativeCompensatesSecretOnMetadataFailure(t *testing.T) {
	t.Parallel()

	repository := newMemoryRepository()
	repository.createErr = errors.New("db down")
	secrets := newMemorySecrets()
	clock := fixedClock{now: time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)}

	_, err := NewCreateNative(repository, secrets, clock).Execute(
		context.Background(),
		CreateNativeRequest{
			Namespace:   "github",
			Name:        "work",
			DisplayName: "GitHub Work",
			Secret:      []byte("TOP-SECRET"),
		},
	)
	if err == nil {
		t.Fatal("CreateNative.Execute() expected error")
	}
	ref, _ := credentialdomain.NewReference("github", "work")
	if _, ok := secrets.values[ref]; ok {
		t.Fatal("secret remained after metadata failure")
	}
	if secrets.deleteCalls != 1 {
		t.Fatalf("secret delete calls = %d, want 1", secrets.deleteCalls)
	}
}

func TestSSHPrivateKeyCredentialRequiresNativeKeyringAndValidatedKey(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := newMemoryRepository()
	secrets := newMemorySecrets()
	clock := fixedClock{now: time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)}
	request := CreateNativeRequest{Namespace: "ssh", Name: "build", DisplayName: "SSH build key", Secret: []byte("not a private key")}
	if _, err := NewCreateNative(repo, secrets, clock).Execute(ctx, request); err == nil {
		t.Fatal("unvalidated SSH key entered native credential store")
	}
	if secrets.putCalls != 0 {
		t.Fatal("invalid SSH key reached keyring")
	}
	if _, err := NewCreateEnvironment(repo, clock).Execute(ctx, CreateEnvironmentRequest{
		Namespace: "ssh", Name: "build", DisplayName: "SSH key", EnvironmentVariable: "MY_SSH_PRIVATE_KEY",
	}); err == nil {
		t.Fatal("SSH key accepted insecure environment-backed credential")
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKey(priv, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	request.Secret = pem.EncodeToMemory(block)
	created, err := NewCreateNative(repo, secrets, clock).Execute(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if created.Reference().String() != "credential://ssh/build" || len(secrets.values[created.Reference()]) == 0 {
		t.Fatal("valid SSH credential not stored using native keyring")
	}
	before := append([]byte(nil), secrets.values[created.Reference()]...)
	if _, err := NewRotateNativeSecret(repo, secrets, clock).Execute(ctx, created.Reference(), []byte("wrong key")); err == nil {
		t.Fatal("invalid rotated SSH key accepted")
	}
	if !bytes.Equal(before, secrets.values[created.Reference()]) {
		t.Fatal("failed SSH rotation overwrote keyring secret")
	}
}

func TestCreateEnvironmentPersistsMetadataOnly(t *testing.T) {
	t.Parallel()

	repository := newMemoryRepository()
	clock := fixedClock{now: time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)}
	credential, err := NewCreateEnvironment(repository, clock).Execute(
		context.Background(),
		CreateEnvironmentRequest{
			Namespace:           "github",
			Name:                "ci",
			DisplayName:         "GitHub CI",
			EnvironmentVariable: "GITHUB_TOKEN",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if credential.Source() != credentialdomain.SourceEnvironment ||
		credential.EnvironmentVariable() != "GITHUB_TOKEN" {
		t.Fatalf("Credential = %#v", credential)
	}
	stored, err := repository.Get(context.Background(), credential.Reference())
	if err != nil || stored.Reference() != credential.Reference() {
		t.Fatalf("stored = %#v err=%v", stored, err)
	}
}

func TestCreateRejectsDuplicateBeforeSecretWrite(t *testing.T) {
	t.Parallel()

	repository := newMemoryRepository()
	clock := fixedClock{now: time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)}
	existing, _ := credentialdomain.NewCredential(
		mustRef(t, "github", "work"),
		"Existing",
		credentialdomain.SourceNative,
		"",
		clock.Now(),
	)
	_ = repository.Create(context.Background(), existing)
	secrets := newMemorySecrets()

	_, err := NewCreateNative(repository, secrets, clock).Execute(
		context.Background(),
		CreateNativeRequest{
			Namespace:   "github",
			Name:        "work",
			DisplayName: "Duplicate",
			Secret:      []byte("new-secret"),
		},
	)
	if !errors.Is(err, ErrReferenceConflict) {
		t.Fatalf("error = %v, want ErrReferenceConflict", err)
	}
	if secrets.putCalls != 0 {
		t.Fatalf("secret Put calls = %d, want 0", secrets.putCalls)
	}
}

func TestRotateAndDeleteNativeCredential(t *testing.T) {
	t.Parallel()

	repository := newMemoryRepository()
	secrets := newMemorySecrets()
	clock := fixedClock{now: time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)}
	ref := mustRef(t, "github", "work")
	credential, _ := credentialdomain.NewCredential(
		ref,
		"GitHub Work",
		credentialdomain.SourceNative,
		"",
		clock.Now(),
	)
	_ = repository.Create(context.Background(), credential)
	secrets.values[ref] = []byte("old")

	clock.now = clock.now.Add(time.Minute)
	rotated, err := NewRotateNativeSecret(repository, secrets, clock).Execute(
		context.Background(),
		ref,
		[]byte("new"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(secrets.values[ref], []byte("new")) {
		t.Fatalf("secret = %q", secrets.values[ref])
	}
	if !rotated.UpdatedAt().After(rotated.CreatedAt()) {
		t.Fatalf("timestamps = %v / %v", rotated.CreatedAt(), rotated.UpdatedAt())
	}

	usage := fakeUsageChecker{items: []Usage{{Kind: "profile", Description: "profile default"}}}
	if err := NewDeleteCredential(repository, secrets, usage).Execute(
		context.Background(),
		ref,
	); !errors.Is(err, ErrCredentialInUse) {
		t.Fatalf("delete in-use error = %v", err)
	}
	if _, ok := secrets.values[ref]; !ok {
		t.Fatal("in-use delete removed secret")
	}

	if err := NewDeleteCredential(repository, secrets, fakeUsageChecker{}).Execute(
		context.Background(),
		ref,
	); err != nil {
		t.Fatal(err)
	}
	if _, ok := secrets.values[ref]; ok {
		t.Fatal("delete left secret")
	}
	if _, err := repository.Get(context.Background(), ref); !errors.Is(err, ErrCredentialNotFound) {
		t.Fatalf("Get(deleted) error = %v", err)
	}
}

type memoryRepository struct {
	items     map[credentialdomain.Reference]credentialdomain.Credential
	createErr error
	saveErr   error
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{
		items: make(map[credentialdomain.Reference]credentialdomain.Credential),
	}
}

func (m *memoryRepository) Create(
	_ context.Context,
	credential credentialdomain.Credential,
) error {
	if m.createErr != nil {
		return m.createErr
	}
	if _, exists := m.items[credential.Reference()]; exists {
		return ErrReferenceConflict
	}
	m.items[credential.Reference()] = credential
	return nil
}

func (m *memoryRepository) Save(
	_ context.Context,
	credential credentialdomain.Credential,
) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	if _, exists := m.items[credential.Reference()]; !exists {
		return ErrCredentialNotFound
	}
	m.items[credential.Reference()] = credential
	return nil
}

func (m *memoryRepository) Get(
	_ context.Context,
	ref credentialdomain.Reference,
) (credentialdomain.Credential, error) {
	item, ok := m.items[ref]
	if !ok {
		return credentialdomain.Credential{}, ErrCredentialNotFound
	}
	return item, nil
}

func (m *memoryRepository) List(
	context.Context,
) ([]credentialdomain.Credential, error) {
	items := make([]credentialdomain.Credential, 0, len(m.items))
	for _, item := range m.items {
		items = append(items, item)
	}
	return items, nil
}

func (m *memoryRepository) Delete(
	_ context.Context,
	ref credentialdomain.Reference,
) error {
	if _, ok := m.items[ref]; !ok {
		return ErrCredentialNotFound
	}
	delete(m.items, ref)
	return nil
}

type memorySecrets struct {
	values      map[credentialdomain.Reference][]byte
	putCalls    int
	deleteCalls int
}

func newMemorySecrets() *memorySecrets {
	return &memorySecrets{
		values: make(map[credentialdomain.Reference][]byte),
	}
}

func (m *memorySecrets) Put(
	_ context.Context,
	ref credentialdomain.Reference,
	secret []byte,
) error {
	m.putCalls++
	m.values[ref] = append([]byte(nil), secret...)
	return nil
}

func (m *memorySecrets) Get(
	_ context.Context,
	ref credentialdomain.Reference,
) ([]byte, error) {
	value, ok := m.values[ref]
	if !ok {
		return nil, ErrSecretNotFound
	}
	return append([]byte(nil), value...), nil
}

func (m *memorySecrets) Delete(
	_ context.Context,
	ref credentialdomain.Reference,
) error {
	m.deleteCalls++
	if _, ok := m.values[ref]; !ok {
		return ErrSecretNotFound
	}
	delete(m.values, ref)
	return nil
}

type fakeUsageChecker struct {
	items []Usage
	err   error
}

func (f fakeUsageChecker) Usages(
	context.Context,
	credentialdomain.Reference,
) ([]Usage, error) {
	return append([]Usage(nil), f.items...), f.err
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time {
	return c.now
}

func mustRef(t *testing.T, namespace, name string) credentialdomain.Reference {
	t.Helper()
	ref, err := credentialdomain.NewReference(namespace, name)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}
