// Package gcsclienttest provides an in-memory gcsclient.Client for tests.
// It lets scanner and app tests simulate listings, open errors (FR-9,
// FR-16) and streams that break mid-read (NFR-3) without touching GCS, and
// counts every call so tests can assert that a run never reached GCS.
package gcsclienttest

import (
	"context"
	"io"
	"strings"
	"sync"

	"gcsgrep/internal/gcsclient"
)

// Object is one simulated GCS object.
type Object struct {
	Name    string
	Content string
	// OpenErr, if set, is returned by Open instead of a stream.
	OpenErr error
	// ReadErr, if set, is returned by the stream after Content is fully
	// delivered, simulating a connection that breaks mid-read.
	ReadErr error
}

// Fake is an in-memory gcsclient.Client. Objects are listed in slice order,
// which tests use as the GCS listing order.
type Fake struct {
	Objects []Object
	ListErr error

	mu           sync.Mutex
	listPrefixes []string
	opened       []string
}

var _ gcsclient.Client = (*Fake)(nil)

// List returns the objects whose name starts with prefix, in slice order.
func (f *Fake) List(ctx context.Context, bucket, prefix string) ([]gcsclient.ObjectInfo, error) {
	f.mu.Lock()
	f.listPrefixes = append(f.listPrefixes, prefix)
	f.mu.Unlock()

	if f.ListErr != nil {
		return nil, f.ListErr
	}
	var out []gcsclient.ObjectInfo
	for _, o := range f.Objects {
		if strings.HasPrefix(o.Name, prefix) {
			out = append(out, gcsclient.ObjectInfo{Name: o.Name, Size: int64(len(o.Content))})
		}
	}
	return out, nil
}

// Open returns a stream over the object's content.
func (f *Fake) Open(ctx context.Context, bucket, object string) (io.ReadCloser, error) {
	f.mu.Lock()
	f.opened = append(f.opened, object)
	f.mu.Unlock()

	for _, o := range f.Objects {
		if o.Name != object {
			continue
		}
		if o.OpenErr != nil {
			return nil, o.OpenErr
		}
		if o.ReadErr != nil {
			return io.NopCloser(io.MultiReader(strings.NewReader(o.Content), errReader{o.ReadErr})), nil
		}
		return io.NopCloser(strings.NewReader(o.Content)), nil
	}
	return nil, gcsclient.ErrObjectNotFound
}

// ListCalls is how many times List was called.
func (f *Fake) ListCalls() int {
	return len(f.ListPrefixes())
}

// ListPrefixes is the prefix passed to each List call, in call order.
func (f *Fake) ListPrefixes() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.listPrefixes...)
}

// Opened is the names passed to Open, in call order.
func (f *Fake) Opened() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.opened...)
}

// Calls is the total number of calls to GCS (List + Open).
func (f *Fake) Calls() int {
	return f.ListCalls() + len(f.Opened())
}

type errReader struct{ err error }

func (e errReader) Read([]byte) (int, error) { return 0, e.err }
