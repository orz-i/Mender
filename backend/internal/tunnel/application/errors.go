package application

import "errors"

var (
	ErrTunnelNotFound             = errors.New("tunnel: not found")
	ErrTunnelConflict             = errors.New("tunnel: already exists")
	ErrTunnelInUse                = errors.New("tunnel: in use")
	ErrTunnelTransportUnavailable = errors.New("tunnel: exact transport installation unavailable")
)
