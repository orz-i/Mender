package application

import "errors"

var (
	ErrProfileNotFound       = errors.New("profile: not found")
	ErrRevisionConflict      = errors.New("profile: stale revision")
	ErrToolReferenceConflict = errors.New("profile: tool binding references a deleted tool")
	ErrSlugConflict          = errors.New("profile: slug already exists")
	ErrBindingNameConflict   = errors.New("profile: tool binding machine name conflict")
	ErrInvalidBinding        = errors.New("profile: invalid tool binding")
)
