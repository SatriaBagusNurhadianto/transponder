// Package store defines the persistence interfaces transponder uses.
package store

import "errors"

// DefaultOrg is the organisation every record belongs to until organisations ship.
const DefaultOrg = "default"

var (
	// ErrNotFound means the requested record does not exist.
	ErrNotFound = errors.New("store: not found")
	// ErrConflict means a record with the same identity already exists.
	ErrConflict = errors.New("store: conflict")
)
