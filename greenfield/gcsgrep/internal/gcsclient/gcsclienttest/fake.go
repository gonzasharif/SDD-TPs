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
	"time"

	"gcsgrep/internal/gcsclient"
)

// Object is one simulated GCS object.
type Object struct {
	Name    string
	Content string
	// OpenFailures are returned by successive Open calls, one per call, before
	// Open starts behaving as configured by OpenErr and Content. They
	// simulate errors that go away on a retry (NFR-3).
	OpenFailures []error
	// OpenErr, if set, is returned by every Open call that is not consumed
	// by OpenFailures, instead of a stream.
	OpenErr error
	// ReadErr, if set, is returned by the stream after Content is fully
	// delivered, simulating a connection that breaks mid-read.
	ReadErr error
}

// Fake is an in-memory gcsclient.Client. Objects are listed in slice order,
// which tests use as the GCS listing order.
type Fake struct {
	Objects []Object
	// ListFailures are returned by successive List calls, one per call,
	// before List starts behaving as configured by ListErr and Objects.
	ListFailures []error
	// ListErr, if set, is returned by every List call that is not consumed
	// by ListFailures.
	ListErr error

	mu           sync.Mutex
	listPrefixes []string
	listTimes    []time.Time
	opened       []string
	openTimes    map[string][]time.Time
	failuresUsed map[string]int
	listFailUsed int
}

var _ gcsclient.Client = (*Fake)(nil)

// List returns the objects whose name starts with prefix, in slice order.
func (f *Fake) List(ctx context.Context, bucket, prefix string) ([]gcsclient.ObjectInfo, error) {
	f.mu.Lock()
	f.listPrefixes = append(f.listPrefixes, prefix)
	f.listTimes = append(f.listTimes, time.Now())
	var injected error
	if f.listFailUsed < len(f.ListFailures) {
		injected = f.ListFailures[f.listFailUsed]
		f.listFailUsed++
	}
	f.mu.Unlock()

	if injected != nil {
		return nil, injected
	}
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
	if f.openTimes == nil {
		f.openTimes = map[string][]time.Time{}
		f.failuresUsed = map[string]int{}
	}
	f.openTimes[object] = append(f.openTimes[object], time.Now())
	f.mu.Unlock()

	for _, o := range f.Objects {
		if o.Name != object {
			continue
		}
		if injected := f.nextOpenFailure(o); injected != nil {
			return nil, injected
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

// nextOpenFailure consumes and returns the next OpenFailures entry of o, or
// nil once they are used up.
func (f *Fake) nextOpenFailure(o Object) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	used := f.failuresUsed[o.Name]
	if used >= len(o.OpenFailures) {
		return nil
	}
	f.failuresUsed[o.Name] = used + 1
	return o.OpenFailures[used]
}

// ListTimes is the instant of each List call, in call order.
func (f *Fake) ListTimes() []time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]time.Time(nil), f.listTimes...)
}

// OpenTimes is the instant of each Open call for object, in call order.
func (f *Fake) OpenTimes(object string) []time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]time.Time(nil), f.openTimes[object]...)
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
