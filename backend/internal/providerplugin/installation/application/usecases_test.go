package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	installationdomain "github.com/orz-i/Mender/backend/internal/providerplugin/installation/domain"
	"github.com/orz-i/Mender/backend/internal/providerplugin/manifest"
)

func TestInstallLocalPersistsManagedPackage(t *testing.T) {
	t.Parallel()

	id := appTestInstallationID(t, "01")
	pkg := appTestManagedPackage("dev.example.echo", "1.2.3", "a")
	repository := newFakeRepository()
	packages := &fakePackageStore{installResult: pkg}
	usecase := NewInstallLocal(
		repository,
		packages,
		fakeIDGenerator{id: id},
		fakeClock{now: time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)},
	)

	installed, err := usecase.Execute(context.Background(), "/source/plugin")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if installed.ID() != id ||
		installed.PluginID().String() != "dev.example.echo" ||
		installed.PackageVersion() != "1.2.3" ||
		installed.Digest() != pkg.Digest ||
		installed.Status() != installationdomain.StatusEnabled {
		t.Fatalf("Execute() = %+v", installed)
	}
	if packages.installCalls != 1 || packages.removeCalls != 0 {
		t.Fatalf("package calls install=%d remove=%d", packages.installCalls, packages.removeCalls)
	}
	persisted, err := repository.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Digest() != pkg.Digest {
		t.Fatalf("persisted digest = %q", persisted.Digest())
	}
}

func TestInstallLocalCompensatesDuplicateAndPersistenceFailure(t *testing.T) {
	t.Parallel()

	t.Run("duplicate digest", func(t *testing.T) {
		id := appTestInstallationID(t, "02")
		existingID := appTestInstallationID(t, "03")
		pkg := appTestManagedPackage("dev.example.echo", "1.0.0", "b")
		repository := newFakeRepository()
		existing := appTestInstallation(t, existingID, pkg)
		repository.items[existingID] = existing
		repository.digest[pkg.Digest] = existingID
		packages := &fakePackageStore{installResult: pkg}
		usecase := NewInstallLocal(
			repository,
			packages,
			fakeIDGenerator{id: id},
			fakeClock{now: time.Now().UTC()},
		)

		_, err := usecase.Execute(context.Background(), "/source/plugin")
		if !errors.Is(err, ErrPackageAlreadyInstalled) {
			t.Fatalf("Execute() error = %v, want ErrPackageAlreadyInstalled", err)
		}
		if packages.removeCalls != 1 || packages.lastRemoved != id {
			t.Fatalf("cleanup calls=%d id=%s", packages.removeCalls, packages.lastRemoved)
		}
	})

	t.Run("persistence failure", func(t *testing.T) {
		id := appTestInstallationID(t, "04")
		pkg := appTestManagedPackage("dev.example.echo", "1.0.0", "c")
		repository := newFakeRepository()
		repository.createErr = errors.New("database unavailable")
		packages := &fakePackageStore{installResult: pkg}
		usecase := NewInstallLocal(
			repository,
			packages,
			fakeIDGenerator{id: id},
			fakeClock{now: time.Now().UTC()},
		)

		if _, err := usecase.Execute(context.Background(), "/source/plugin"); err == nil {
			t.Fatal("Execute() error = nil")
		}
		if packages.removeCalls != 1 || packages.lastRemoved != id {
			t.Fatalf("cleanup calls=%d id=%s", packages.removeCalls, packages.lastRemoved)
		}
	})
}

func TestRemoveInstallationHonorsUsageGuard(t *testing.T) {
	t.Parallel()

	id := appTestInstallationID(t, "05")
	pkg := appTestManagedPackage("dev.example.echo", "1.0.0", "d")
	repository := newFakeRepository()
	repository.items[id] = appTestInstallation(t, id, pkg)
	repository.digest[pkg.Digest] = id
	packages := &fakePackageStore{}
	usage := fakeUsageChecker{
		usages: []Usage{{Kind: "provider", Description: "provider prov_example"}},
	}
	usecase := NewRemoveInstallation(repository, packages, usage)

	err := usecase.Execute(context.Background(), id)
	if !errors.Is(err, ErrInstallationInUse) {
		t.Fatalf("Execute() error = %v, want ErrInstallationInUse", err)
	}
	if packages.removeCalls != 0 {
		t.Fatalf("Remove() called %d times", packages.removeCalls)
	}
	if _, err := repository.Get(context.Background(), id); err != nil {
		t.Fatalf("installation was removed despite usage guard: %v", err)
	}
}

func TestRemoveInstallationDeletesMetadataBeforePackageFiles(t *testing.T) {
	t.Parallel()

	id := appTestInstallationID(t, "06")
	pkg := appTestManagedPackage("dev.example.echo", "1.0.0", "e")
	repository := newFakeRepository()
	repository.items[id] = appTestInstallation(t, id, pkg)
	repository.digest[pkg.Digest] = id
	order := make([]string, 0, 2)
	repository.onDelete = func() {
		order = append(order, "metadata")
	}
	packages := &fakePackageStore{
		onRemove: func() {
			order = append(order, "package")
		},
	}
	usecase := NewRemoveInstallation(repository, packages, nil)

	if err := usecase.Execute(context.Background(), id); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(order) != 2 || order[0] != "metadata" || order[1] != "package" {
		t.Fatalf("removal order = %v", order)
	}
	if _, err := repository.Get(context.Background(), id); !errors.Is(err, ErrInstallationNotFound) {
		t.Fatalf("Get() error = %v", err)
	}
}

func TestSetInstallationStatusReturnsUpdatedRecord(t *testing.T) {
	t.Parallel()

	id := appTestInstallationID(t, "07")
	pkg := appTestManagedPackage("dev.example.echo", "1.0.0", "f")
	repository := newFakeRepository()
	repository.items[id] = appTestInstallation(t, id, pkg)
	repository.digest[pkg.Digest] = id
	usecase := NewSetInstallationStatus(repository)

	updated, err := usecase.Execute(context.Background(), id, installationdomain.StatusDisabled)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if updated.Status() != installationdomain.StatusDisabled {
		t.Fatalf("Status() = %q", updated.Status())
	}
}

type fakeRepository struct {
	items     map[installationdomain.InstallationID]installationdomain.Installation
	digest    map[string]installationdomain.InstallationID
	createErr error
	onDelete  func()
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		items:  make(map[installationdomain.InstallationID]installationdomain.Installation),
		digest: make(map[string]installationdomain.InstallationID),
	}
}

func (r *fakeRepository) Create(
	_ context.Context,
	item installationdomain.Installation,
) error {
	if r.createErr != nil {
		return r.createErr
	}
	if _, exists := r.items[item.ID()]; exists {
		return ErrInstallationConflict
	}
	if _, exists := r.digest[item.Digest()]; exists {
		return ErrInstallationConflict
	}
	r.items[item.ID()] = item
	r.digest[item.Digest()] = item.ID()
	return nil
}

func (r *fakeRepository) Get(
	_ context.Context,
	id installationdomain.InstallationID,
) (installationdomain.Installation, error) {
	item, ok := r.items[id]
	if !ok {
		return installationdomain.Installation{}, ErrInstallationNotFound
	}
	return item, nil
}

func (r *fakeRepository) GetByDigest(
	_ context.Context,
	digest string,
) (installationdomain.Installation, error) {
	id, ok := r.digest[digest]
	if !ok {
		return installationdomain.Installation{}, ErrInstallationNotFound
	}
	return r.items[id], nil
}

func (r *fakeRepository) List(
	_ context.Context,
) ([]installationdomain.Installation, error) {
	items := make([]installationdomain.Installation, 0, len(r.items))
	for _, item := range r.items {
		items = append(items, item)
	}
	return items, nil
}

func (r *fakeRepository) SetStatus(
	_ context.Context,
	id installationdomain.InstallationID,
	status installationdomain.Status,
) error {
	item, ok := r.items[id]
	if !ok {
		return ErrInstallationNotFound
	}
	updated, err := item.WithStatus(status)
	if err != nil {
		return err
	}
	r.items[id] = updated
	return nil
}

func (r *fakeRepository) Delete(
	_ context.Context,
	id installationdomain.InstallationID,
) error {
	item, ok := r.items[id]
	if !ok {
		return ErrInstallationNotFound
	}
	delete(r.items, id)
	delete(r.digest, item.Digest())
	if r.onDelete != nil {
		r.onDelete()
	}
	return nil
}

type fakePackageStore struct {
	installResult ManagedPackage
	installErr    error
	removeErr     error
	installCalls  int
	removeCalls   int
	lastRemoved   installationdomain.InstallationID
	onRemove      func()
}

func (p *fakePackageStore) InstallLocal(
	_ context.Context,
	_ string,
	_ installationdomain.InstallationID,
) (ManagedPackage, error) {
	p.installCalls++
	return p.installResult, p.installErr
}

func (p *fakePackageStore) Remove(
	_ context.Context,
	id installationdomain.InstallationID,
) error {
	p.removeCalls++
	p.lastRemoved = id
	if p.onRemove != nil {
		p.onRemove()
	}
	return p.removeErr
}

type fakeIDGenerator struct {
	id  installationdomain.InstallationID
	err error
}

func (g fakeIDGenerator) NewInstallationID() (installationdomain.InstallationID, error) {
	return g.id, g.err
}

type fakeClock struct {
	now time.Time
}

func (c fakeClock) Now() time.Time {
	return c.now
}

type fakeUsageChecker struct {
	usages []Usage
	err    error
}

func (c fakeUsageChecker) Usages(
	context.Context,
	installationdomain.InstallationID,
) ([]Usage, error) {
	return c.usages, c.err
}

func appTestManagedPackage(pluginID, version, digestRune string) ManagedPackage {
	raw := []byte(`{
		"manifestVersion": 1,
		"pluginId": "` + pluginID + `",
		"version": "` + version + `",
		"protocolVersions": [1],
		"displayName": "Example",
		"description": "Example",
		"entrypoints": {"linux/amd64": "bin/plugin"}
	}`)
	parsed, err := manifest.Parse(raw)
	if err != nil {
		panic(err)
	}
	return ManagedPackage{
		Manifest:        parsed,
		ManifestJSON:    raw,
		Digest:          "sha256:" + strings.Repeat(digestRune, 64),
		SourceReference: "/tmp/example",
	}
}

func appTestInstallation(
	t *testing.T,
	id installationdomain.InstallationID,
	pkg ManagedPackage,
) installationdomain.Installation {
	t.Helper()
	pluginID, err := providerplugin.ParseID(pkg.Manifest.PluginID)
	if err != nil {
		t.Fatal(err)
	}
	item, err := installationdomain.NewInstallation(
		id,
		pluginID,
		pkg.Manifest.Version,
		installationdomain.SourceLocal,
		pkg.SourceReference,
		pkg.Digest,
		pkg.ManifestJSON,
		installationdomain.StatusEnabled,
		time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	return item
}

func appTestInstallationID(t *testing.T, suffix string) installationdomain.InstallationID {
	t.Helper()
	id, err := installationdomain.ParseInstallationID(
		"pinst_" + strings.Repeat("0", 30) + suffix,
	)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
