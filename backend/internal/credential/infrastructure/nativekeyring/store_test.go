package nativekeyring

import (
	"context"
	"testing"

	credentialapplication "github.com/orz-i/Mender/backend/internal/credential/application"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	"github.com/zalando/go-keyring"
)

func TestStoreRoundTripWithMockKeyring(t *testing.T) {
	keyring.MockInit()

	ref, err := credentialdomain.NewReference("github", "work")
	if err != nil {
		t.Fatal(err)
	}
	store := Store{}
	if err := store.Put(context.Background(), ref, []byte("secret-value")); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "secret-value" {
		t.Fatalf("Get() = %q", got)
	}
	got[0] = 'X'
	again, err := store.Get(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if string(again) != "secret-value" {
		t.Fatalf("Get() did not return independent bytes: %q", again)
	}

	if err := store.Delete(context.Background(), ref); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(context.Background(), ref); err != credentialapplication.ErrSecretNotFound {
		t.Fatalf("Get(deleted) error = %v", err)
	}
}
