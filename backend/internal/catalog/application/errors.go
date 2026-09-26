package application

import "errors"

var (
	// ErrProviderNotFound indicates that a requested Provider does not exist.
	ErrProviderNotFound = errors.New("catalog: provider not found")
	// ErrNamespaceConflict indicates that a Provider namespace is already registered.
	ErrNamespaceConflict = errors.New("catalog: provider namespace already exists")
	// ErrToolNotFound indicates that a requested Tool does not exist.
	ErrToolNotFound = errors.New("catalog: tool not found")
	// ErrDuplicateSourceKey indicates that one discovery snapshot contains duplicate source identities.
	ErrDuplicateSourceKey = errors.New("catalog: duplicate tool source key")
	// ErrInvalidSearchQuery indicates that a discovery query is empty.
	ErrInvalidSearchQuery = errors.New("catalog: search query must not be empty")
	// ErrInvalidSearchLimit indicates that a discovery limit is outside supported bounds.
	ErrInvalidSearchLimit = errors.New("catalog: search limit must be between 1 and 100")
)
