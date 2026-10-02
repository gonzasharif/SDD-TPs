// Package gcsclient is gcsgrep's anti-corruption layer over Google Cloud
// Storage (see "Contexto delimitado" in gcsgrep-design.md).
//
// This file holds the SDK-free part of the package: the Client interface the
// rest of gcsgrep depends on, and the domain errors the GCS implementation
// (gcs.go) translates SDK errors into. The interface has no write, delete, or
// permission methods — that omission is what makes BR-1 ("gcsgrep never
// writes to GCS") a structural property of the code instead of a rule someone
// has to remember to follow (VC-15.2).
package gcsclient

import (
	"context"
	"errors"
	"io"
)

// ObjectInfo describes a listed object without its content.
type ObjectInfo struct {
	Name string
	Size int64
}

// Client is the read-only surface gcsgrep uses to talk to GCS.
type Client interface {
	// List returns every object under bucket/prefix, in the order GCS lists
	// them (lexicographic by name). It never reads object content — it's the
	// cheap listing call the BR-3 count guardrail relies on before any content
	// is read.
	List(ctx context.Context, bucket, prefix string) ([]ObjectInfo, error)
	// Open returns a streaming reader for a single object's content. The
	// caller is responsible for closing it.
	Open(ctx context.Context, bucket, object string) (io.ReadCloser, error)
}

// Domain errors. Implementations wrap them (errors.Is still matches) so the
// rest of gcsgrep can decide which message to print without importing the
// GCS SDK.
var (
	// ErrPermissionDenied: the credentials cannot list the bucket (FR-16.3)
	// or read the object (FR-9.1). HTTP 403.
	ErrPermissionDenied = errors.New("permission denied")
	// ErrBucketNotFound: the bucket in the location does not exist
	// (FR-16.4). HTTP 404 on listing.
	ErrBucketNotFound = errors.New("bucket does not exist")
	// ErrObjectNotFound: a listed object no longer exists when opened
	// (FR-9.3). HTTP 404 on opening.
	ErrObjectNotFound = errors.New("object not found")
)

// ErrTransient marks a failure that may succeed if tried again: a timeout, a
// reset connection, or HTTP 408, 429 or 5xx (NFR-3). Implementations wrap
// SDK errors with Transient so the retry decorator (WithRetries) can decide
// without knowing anything about the SDK.
var ErrTransient = errors.New("transient error")

// Transient wraps err so errors.Is(result, ErrTransient) is true. The
// message stays err's own, so it can be shown as the <detalle> of NFR-3's
// literal messages.
func Transient(err error) error {
	return &transientError{err: err}
}

type transientError struct{ err error }

func (e *transientError) Error() string { return e.err.Error() }
func (e *transientError) Unwrap() error { return e.err }
func (e *transientError) Is(target error) bool {
	return target == ErrTransient
}

// RetriesExhaustedError is returned when every attempt failed with a
// transient error (NFR-3). Err is the failure of the last attempt.
type RetriesExhaustedError struct {
	Attempts int
	Err      error
}

func (e *RetriesExhaustedError) Error() string { return e.Err.Error() }
func (e *RetriesExhaustedError) Unwrap() error { return e.Err }
