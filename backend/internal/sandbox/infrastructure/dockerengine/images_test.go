//go:build !windows

package dockerengine

import (
	"context"
	"errors"
	"strings"
	"testing"

	apiimage "github.com/moby/moby/api/types/image"
	mobyclient "github.com/moby/moby/client"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

func TestCanonicalLocalImageDigests(t *testing.T) {
	t.Parallel()
	digest := strings.Repeat("a", 64)
	for raw, want := range map[string]string{
		"alpine@sha256:" + digest:                         "docker.io/library/alpine@sha256:" + digest,
		"pgvector/pgvector@sha256:" + digest:              "docker.io/pgvector/pgvector@sha256:" + digest,
		"docker.io/library/alpine@sha256:" + digest:       "docker.io/library/alpine@sha256:" + digest,
		"ghcr.io/open-telemetry/release@sha256:" + digest: "ghcr.io/open-telemetry/release@sha256:" + digest,
	} {
		got, ok := canonicalLocalImageDigest(raw)
		if !ok || got != want {
			t.Fatalf("%s -> %s (%v), want %s", raw, got, ok, want)
		}
	}
	for _, raw := range []string{
		"alpine:latest", "sha256:" + digest, "alpine@sha256:abcd",
		"EXAMPLE/IMAGE@sha256:" + digest, "docker.io/alpine@sha256:" + digest,
		"docker.io/library/alpine@sha256:" + strings.Repeat("f", 63) + "!",
		"tcp://127.0.0.1:2375/alpine@sha256:" + digest,
	} {
		if got, ok := canonicalLocalImageDigest(raw); ok {
			t.Fatalf("accepted invalid repo %s => %s", raw, got)
		}
	}
}

func TestLocalVerifiedDigestsAcceptOnlyExactDockerHubAliases(t *testing.T) {
	t.Parallel()
	digest := strings.Repeat("a", 64)
	for _, tc := range []struct {
		expected string
		known    []string
		yes      bool
	}{
		{"docker.io/library/alpine@sha256:" + digest, []string{"alpine@sha256:" + digest}, true},
		{"docker.io/pgvector/pgvector@sha256:" + digest, []string{"pgvector/pgvector@sha256:" + digest}, true},
		{"docker.io/pgvector/pgvector@sha256:" + digest, []string{"pgvector/pgvector@sha256:" + strings.Repeat("b", 64)}, false},
		{"ghcr.io/owner/image@sha256:" + digest, []string{"owner/image@sha256:" + digest}, false},
	} {
		if got := locallyVerifiedDigest(tc.known, tc.expected); got != tc.yes {
			t.Fatalf("digest match %s => %v, want %v", tc.expected, got, tc.yes)
		}
	}
}

func TestCachedImagesRequireLiveVerifiedManifestAndNeverMutate(t *testing.T) {
	t.Parallel()
	inspector, fake := newProbeFixture(t)
	digest := strings.Repeat("a", 64)
	immutable := "docker.io/library/alpine@sha256:" + digest
	id := "sha256:" + strings.Repeat("c", 64)
	fake.imageList = mobyclient.ImageListResult{Items: []apiimage.Summary{
		{ID: id, RepoTags: []string{"alpine:3.22"}, RepoDigests: []string{"alpine@sha256:" + digest}},
		{ID: "sha256:" + strings.Repeat("f", 64), RepoTags: []string{"work:latest"}, RepoDigests: nil},
	}}
	fake.image = mobyclient.ImageInspectResult{InspectResponse: apiimage.InspectResponse{
		ID: id, RepoDigests: []string{"alpine@sha256:" + digest}, Os: "linux", Architecture: "arm64",
	}}
	images, err := inspector.ListCachedImages(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(images) != 1 || images[0].ImageDigest != immutable || images[0].DisplayName != "alpine:3.22" {
		t.Fatalf("unsafe cached images: %+v", images)
	}
	if fake.imageListCalls != 1 || fake.imageCalls != 1 {
		t.Fatalf("expected read-only one list + one inspect: %+v", fake)
	}
	fake.image.Os = "windows"
	images, err = inspector.ListCachedImages(context.Background())
	if err != nil || len(images) != 0 {
		t.Fatalf("untrusted platform exposed: %+v, %v", images, err)
	}
	fake.image.Os = "linux"
	fake.image.ID = "sha256:" + strings.Repeat("d", 64)
	images, err = inspector.ListCachedImages(context.Background())
	if err != nil || len(images) != 0 {
		t.Fatalf("mismatched content exposed: %+v, %v", images, err)
	}
	fake.image.ID = id
	fake.imageListErr = errors.New("daemon secret detail")
	if _, err = inspector.ListCachedImages(context.Background()); !errors.Is(err, sandboxdomain.ErrBackendUnavailable) || strings.Contains(err.Error(), "secret") {
		t.Fatalf("daemon error leakage: %v", err)
	}
}
