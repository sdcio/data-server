package api

import (
	"context"
	"errors"
)

// Sentinels are matched with errors.Is; implementations wrap them with context.
var (
	// ErrDatastoreLocked: another action holds the datastore; retry later.
	ErrDatastoreLocked = errors.New("datastore is locked, other action is ongoing")
	// ErrNotConnected: the southbound target is unreachable; retry later.
	ErrNotConnected = errors.New("not connected")
	// ErrResourceExhausted: a resource limit was hit; retry later.
	ErrResourceExhausted = errors.New("resource exhausted")
	// ErrUnknownDatastore: the named datastore does not exist.
	ErrUnknownDatastore = errors.New("unknown datastore")
	// ErrInvalidArgument: the request is malformed.
	ErrInvalidArgument = errors.New("invalid argument")
)

// IsRetryable reports whether err is transient, so the same request may
// succeed later. Untyped errors are not retryable.
func IsRetryable(err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, ErrDatastoreLocked),
		errors.Is(err, ErrNotConnected),
		errors.Is(err, ErrResourceExhausted),
		errors.Is(err, context.DeadlineExceeded):
		return true
	default:
		return false
	}
}
