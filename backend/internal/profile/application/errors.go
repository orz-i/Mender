package application

import "errors"

var (
	ErrProfileNotFound     = errors.New("profile: not found")
	ErrSlugConflict        = errors.New("profile: slug already exists")
	ErrBindingNameConflict = errors.New("profile: tool binding machine name conflict")
	ErrInvalidBinding      = errors.New("profile: invalid tool binding")
)
