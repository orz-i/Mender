package dockerengine

import (
	"context"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	apiimage "github.com/moby/moby/api/types/image"
	mobyclient "github.com/moby/moby/client"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

const maxCachedImages = 64

// CachedImage exposes only a locally verified immutable manifest reference,
// never mutable tags as creation authority or daemon-wide mutable controls.
type CachedImage struct {
	ImageDigest string `json:"imageDigest"`
	DisplayName string `json:"displayName"`
}

type imageListAPI interface {
	ImageList(context.Context, mobyclient.ImageListOptions) (mobyclient.ImageListResult, error)
}

func (i *Inspector) ListCachedImages(ctx context.Context) ([]CachedImage, error) {
	if i == nil || i.api == nil {
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	if _, err := i.Probe(ctx); err != nil {
		return nil, err
	}
	lister, ok := i.api.(imageListAPI)
	if !ok {
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	limited, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	found, err := lister.ImageList(limited, mobyclient.ImageListOptions{})
	if err != nil {
		return nil, safeDaemonError(limited, err)
	}
	// The daemon may supply multiple references for one image, and its
	// repository tags are only display hints. Pin just the verified digest.
	type candidate struct {
		cached  CachedImage
		imageID string
	}
	pending := map[string]candidate{}
	for _, item := range found.Items {
		if !contentAddressPattern.MatchString(item.ID) {
			continue
		}
		for _, raw := range item.RepoDigests {
			ref, ok := canonicalLocalImageDigest(raw)
			if !ok {
				continue
			}
			if _, exists := pending[ref]; exists {
				continue
			}
			pending[ref] = candidate{cached: CachedImage{
				ImageDigest: ref,
				DisplayName: cachedImageDisplayName(item, ref),
			}, imageID: item.ID}
		}
	}
	refs := make([]string, 0, len(pending))
	for ref := range pending {
		refs = append(refs, ref)
	}
	slices.Sort(refs)
	results := make([]CachedImage, 0, min(len(refs), maxCachedImages))
	for _, ref := range refs {
		if limited.Err() != nil {
			return nil, limited.Err()
		}
		if len(results) >= maxCachedImages {
			break
		}
		reply, err := i.api.ImageInspect(limited, ref)
		if err != nil {
			// An image disappearing between listing and inspecting is not
			// selectable. Do not expose daemon-controlled error metadata.
			continue
		}
		v := reply.InspectResponse
		if v.ID != pending[ref].imageID || v.Os != "linux" || v.Architecture == "" ||
			!locallyVerifiedDigest(v.RepoDigests, ref) {
			continue
		}
		results = append(results, pending[ref].cached)
	}
	slices.SortFunc(results, func(a, b CachedImage) int {
		if c := strings.Compare(a.DisplayName, b.DisplayName); c != 0 {
			return c
		}
		return strings.Compare(a.ImageDigest, b.ImageDigest)
	})
	return results, nil
}

// Docker Hub short references (alpine@..., org/repo@...) are canonically
// expanded only to docker.io, never to a caller-specified registry.
func canonicalLocalImageDigest(raw string) (string, bool) {
	parts := strings.Split(raw, "@sha256:")
	if len(parts) != 2 || len(parts[1]) != 64 {
		return "", false
	}
	repo := parts[0]
	first, _, _ := strings.Cut(repo, "/")
	if !strings.Contains(first, ".") && !strings.Contains(first, ":") {
		if strings.ContainsRune(repo, '/') {
			repo = "docker.io/" + repo
		} else {
			repo = "docker.io/library/" + repo
		}
	}
	ref := repo + "@sha256:" + parts[1]
	if !imageDigestPattern.MatchString(ref) || len(ref) > 320 {
		return "", false
	}
	return ref, true
}

func cachedImageDisplayName(item apiimage.Summary, canonical string) string {
	repo := strings.SplitN(canonical, "@sha256:", 2)[0]
	tags := slices.Clone(item.RepoTags)
	slices.Sort(tags)
	for _, tag := range tags {
		name, _, ok := strings.Cut(tag, ":")
		if !ok {
			continue
		}
		normalized, valid := canonicalLocalImageDigest(name + "@sha256:" + strings.Repeat("a", 64))
		if !valid || !strings.HasPrefix(normalized, repo+"@sha256:") {
			continue
		}
		if utf8.ValidString(tag) && len(tag) <= 160 {
			return tag
		}
	}
	return repo
}
