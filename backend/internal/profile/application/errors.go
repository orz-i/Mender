package application

import "errors"

var (
	ErrProfileNotFound = errors.New("profile: not found")
	ErrSlugConflict    = errors.New("profile: slug already exists")
)
