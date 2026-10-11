package resolver

import (
	"context"
	"errors"
	"testing"
	"time"

	credentialapplication "github.com/orz-i/Mender/backend/internal/credential/application"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
)

func TestResolverNativeAndEnvironmentSources(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	nativeRef, _ := credentialdomain.NewReference("github", "work")
	envRef, _ := credentialdomain.NewReference("search", "prod")
	native, _ := credentialdomain.NewCredential(
		nativeRef,
		"GitHub Work",
		credentialdomain.SourceNative,
		"",
		now,
	)
	environment, _ := credentialdomain.NewCredential(
		envRef,
		"Search Prod",
		credentialdomain.SourceEnvironment,
		"SEARCH_TOKEN",
		now,
	)
	repository := fakeRepository{
		nativeRef: native,
		envRef:    environment,
	}
	secrets := fakeSecretStore{
		nativeRef: []byte("native-secret"),
	}
	resolver := New(
		repository,
		secrets,
		func(name string) (string, bool) {
			if name == "SEARCH_TOKEN" {
				return "env-secret", true
			}
			return "", false
		},
	)

	nativeValue, err := resolver.Resolve(context.Background(), nativeRef)
	if err != nil {
		t.Fatal(err)
	}
	if string(nativeValue) != "native-secret" {
		t.Fatalf("native secret = %q", nativeValue)
	}

	envValue, err := resolver.Resolve(context.Background(), envRef)
	if err != nil {
		t.Fatal(err)
	}
	if string(envValue) != "env-secret" {
		t.Fatalf("environment secret = %q", envValue)
	}
}

func TestSSHPrivateKeyResolverRejectsLegacyEnvironmentCredential(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ref, _ := credentialdomain.NewReference("ssh", "legacy")
	legacy, err := credentialdomain.NewCredential(ref, "Legacy SSH private key",
		credentialdomain.SourceEnvironment, "SSH_PRIVATE_KEY", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	called := false
	resolver := New(fakeRepository{ref: legacy}, fakeSecretStore{}, func(string) (string, bool) {
		called = true
		return "never-allow-environment-SSH-secret", true
	})
	if value, err := resolver.Resolve(ctx, ref); value != nil ||
		!errors.Is(err, credentialapplication.ErrSecretSourceUnavailable) || called {
		t.Fatalf("legacy environment SSH secret resolved or env accessed: %v, value length %d, lookup %v", err, len(value), called)
	}
}

func TestResolverMissingEnvironmentFailsClosed(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	ref, _ := credentialdomain.NewReference("search", "prod")
	item, _ := credentialdomain.NewCredential(
		ref,
		"Search Prod",
		credentialdomain.SourceEnvironment,
		"SEARCH_TOKEN",
		now,
	)
	resolver := New(
		fakeRepository{ref: item},
		fakeSecretStore{},
		func(string) (string, bool) { return "", false },
	)
	if _, err := resolver.Resolve(context.Background(), ref); !errors.Is(
		err,
		credentialapplication.ErrEnvironmentMissing,
	) {
		t.Fatalf("Resolve() error = %v", err)
	}
}

type fakeRepository map[credentialdomain.Reference]credentialdomain.Credential

func (f fakeRepository) Create(context.Context, credentialdomain.Credential) error {
	panic("unexpected Create")
}

func (f fakeRepository) Save(context.Context, credentialdomain.Credential) error {
	panic("unexpected Save")
}

func (f fakeRepository) Get(
	_ context.Context,
	ref credentialdomain.Reference,
) (credentialdomain.Credential, error) {
	item, ok := f[ref]
	if !ok {
		return credentialdomain.Credential{}, credentialapplication.ErrCredentialNotFound
	}
	return item, nil
}

func (f fakeRepository) List(context.Context) ([]credentialdomain.Credential, error) {
	panic("unexpected List")
}

func (f fakeRepository) Delete(context.Context, credentialdomain.Reference) error {
	panic("unexpected Delete")
}

type fakeSecretStore map[credentialdomain.Reference][]byte

func (f fakeSecretStore) Put(
	context.Context,
	credentialdomain.Reference,
	[]byte,
) error {
	panic("unexpected Put")
}

func (f fakeSecretStore) Get(
	_ context.Context,
	ref credentialdomain.Reference,
) ([]byte, error) {
	value, ok := f[ref]
	if !ok {
		return nil, credentialapplication.ErrSecretNotFound
	}
	return append([]byte(nil), value...), nil
}

func (f fakeSecretStore) Delete(context.Context, credentialdomain.Reference) error {
	panic("unexpected Delete")
}
