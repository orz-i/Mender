// Package process supervises external Mender plugin installations without
// depending on any particular extension protocol or resource domain.
package process

import (
	"context"
	"errors"
	"fmt"
	"sync"

	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
	installationapp "github.com/orz-i/Mender/backend/internal/plugin/installation/application"
	installationdomain "github.com/orz-i/Mender/backend/internal/plugin/installation/domain"
)

// Client owns one verified package process. A typed extension dialer negotiates
// the transport and verifies the runtime descriptor before returning it.
type Client interface {
	Exited() bool
	Close() error
}

// Dialer is a typed extension protocol endpoint, not a generic service locator.
// The host verifies package integrity and exact identity before invoking Connect.
type Dialer interface {
	Connect(context.Context, installationdomain.Installation, installationapp.ResolvedPackage) (Client, error)
}

type PackageResolver interface {
	Resolve(context.Context, installationdomain.Installation) (installationapp.ResolvedPackage, error)
}

type Supervisor struct {
	packages PackageResolver
	mu       sync.Mutex
	closed   bool
	dialers  map[platformplugin.Kind]Dialer
	slots    map[installationdomain.InstallationID]*slot
}

type slot struct {
	mu     sync.Mutex
	active *running
}

type identity struct {
	id        installationdomain.InstallationID
	pluginID  platformplugin.ID
	version   string
	digest    string
	extension platformplugin.Extension
}

type running struct {
	key    identity
	client Client
}

func NewSupervisor(packages PackageResolver) (*Supervisor, error) {
	if packages == nil {
		return nil, errors.New("plugin process supervisor: package resolver is required")
	}
	return &Supervisor{packages: packages, dialers: make(map[platformplugin.Kind]Dialer), slots: make(map[installationdomain.InstallationID]*slot)}, nil
}

// Register binds one protocol implementation. Registration is only permitted
// before the first process/slot is allocated so dispatch cannot change in flight.
func (s *Supervisor) Register(kind platformplugin.Kind, dialer Dialer) error {
	if s == nil {
		return errors.New("plugin process supervisor: nil host")
	}
	if err := kind.Validate(); err != nil {
		return err
	}
	if dialer == nil {
		return errors.New("plugin process supervisor: dialer is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || len(s.slots) != 0 {
		return errors.New("plugin process supervisor: registration has closed")
	}
	if _, ok := s.dialers[kind]; ok {
		return fmt.Errorf("plugin process supervisor: %s dialer already registered", kind)
	}
	s.dialers[kind] = dialer
	return nil
}

// Ensure returns one connected, reusable client for an enabled installation.
// It does not silently restart an uncertain side-effecting extension action:
// callers own their own operation and replay/reconcile semantics.
func (s *Supervisor) Ensure(ctx context.Context, item installationdomain.Installation) (Client, error) {
	if s == nil {
		return nil, errors.New("plugin process supervisor: nil host")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := item.Validate(); err != nil {
		return nil, fmt.Errorf("plugin process supervisor: installation: %w", err)
	}
	if item.Status() != installationdomain.StatusEnabled {
		return nil, fmt.Errorf("plugin process supervisor: installation %s is %s", item.ID(), item.Status())
	}
	ext := item.Extension()
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, errors.New("plugin process supervisor is closed")
	}
	dialer := s.dialers[ext.Kind]
	if dialer == nil {
		s.mu.Unlock()
		return nil, fmt.Errorf("plugin process supervisor: unsupported extension kind %s", ext.Kind)
	}
	place := s.slots[item.ID()]
	if place == nil {
		place = &slot{}
		s.slots[item.ID()] = place
	}
	s.mu.Unlock()

	place.mu.Lock()
	defer place.mu.Unlock()
	key := identity{id: item.ID(), pluginID: item.PluginID(), version: item.PackageVersion(), digest: item.Digest(), extension: ext}
	if place.active != nil {
		if place.active.key != key {
			return nil, fmt.Errorf("plugin process supervisor: installation %s identity or digest changed", item.ID())
		}
		if !place.active.client.Exited() {
			return place.active.client, nil
		}
		_ = place.active.client.Close()
		place.active = nil
	}
	resolved, err := s.packages.Resolve(ctx, item)
	if err != nil {
		return nil, fmt.Errorf("plugin process supervisor: verify package: %w", err)
	}
	if resolved.Manifest.PluginID != item.PluginID().String() ||
		resolved.Manifest.Version != item.PackageVersion() ||
		len(resolved.Manifest.Extensions) != 1 || resolved.Manifest.Extensions[0] != ext {
		return nil, fmt.Errorf("plugin process supervisor: resolved package identity/extension mismatch")
	}
	client, err := dialer.Connect(ctx, item, resolved)
	if err != nil {
		return nil, fmt.Errorf("plugin process supervisor: connect %s: %w", ext.Kind, err)
	}
	if client == nil {
		return nil, errors.New("plugin process supervisor: dialer returned nil client")
	}
	if err := ctx.Err(); err != nil {
		_ = client.Close()
		return nil, err
	}
	if client.Exited() {
		_ = client.Close()
		return nil, errors.New("plugin process supervisor: dialer returned exited client")
	}
	place.active = &running{key: key, client: client}
	return client, nil
}

// Running is local and side-effect free; it never launches a package or probes
// a remote service.
func (s *Supervisor) Running(id installationdomain.InstallationID) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	place := s.slots[id]
	s.mu.Unlock()
	if place == nil {
		return false
	}
	place.mu.Lock()
	defer place.mu.Unlock()
	return place.active != nil && !place.active.client.Exited()
}

func (s *Supervisor) CloseInstallation(id installationdomain.InstallationID) error {
	if s == nil {
		return nil
	}
	if _, err := installationdomain.ParseInstallationID(id.String()); err != nil {
		return err
	}
	s.mu.Lock()
	place := s.slots[id]
	s.mu.Unlock()
	if place == nil {
		return nil
	}
	place.mu.Lock()
	defer place.mu.Unlock()
	if place.active == nil {
		return nil
	}
	err := place.active.client.Close()
	place.active = nil
	if err != nil {
		return fmt.Errorf("plugin process supervisor: close %s: %w", id, err)
	}
	return nil
}

func (s *Supervisor) CloseAll() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	places := make([]*slot, 0, len(s.slots))
	for _, place := range s.slots {
		places = append(places, place)
	}
	s.mu.Unlock()
	var errs []error
	for _, place := range places {
		place.mu.Lock()
		if place.active != nil {
			if err := place.active.client.Close(); err != nil {
				errs = append(errs, err)
			}
			place.active = nil
		}
		place.mu.Unlock()
	}
	return errors.Join(errs...)
}
