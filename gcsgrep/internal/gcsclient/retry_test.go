package gcsclient

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"google.golang.org/api/googleapi"
)

// scriptedClient fails Open with the given errors, in order, then
// succeeds. It counts every attempt.
type scriptedClient struct {
	errs     []error
	attempts int
}

func (s *scriptedClient) List(ctx context.Context, bucket, prefix string) ([]ObjectInfo, error) {
	return nil, nil
}

func (s *scriptedClient) Open(ctx context.Context, bucket, object string) (io.ReadCloser, error) {
	s.attempts++
	if s.attempts <= len(s.errs) {
		return nil, s.errs[s.attempts-1]
	}
	return io.NopCloser(strings.NewReader("content")), nil
}

// newTestRetryingClient never really sleeps: it records each requested
// delay, and applies a fixed jitter factor.
func newTestRetryingClient(next Client, jitter float64, slept *[]time.Duration) *retryingClient {
	return &retryingClient{
		next: next,
		sleep: func(ctx context.Context, d time.Duration) error {
			*slept = append(*slept, d)
			return nil
		},
		jitter: func() float64 { return jitter },
	}
}

func unavailable() error { return &googleapi.Error{Code: 503, Message: "backend unavailable"} }

// VC-23 (a): two transient failures followed by success recover the
// object, waiting 500ms and then 1s.
func TestRetry_RecoversAfterTwoTransientFailures(t *testing.T) {
	next := &scriptedClient{errs: []error{unavailable(), unavailable()}}
	var slept []time.Duration
	c := newTestRetryingClient(next, 1, &slept)

	rc, err := c.Open(context.Background(), "b", "o")

	if err != nil {
		t.Fatalf("expected the third attempt to succeed, got %v", err)
	}
	rc.Close()
	if next.attempts != 3 {
		t.Errorf("attempts = %d, want 3", next.attempts)
	}
	if want := []time.Duration{500 * time.Millisecond, time.Second}; !equalDurations(slept, want) {
		t.Errorf("delays = %v, want %v", slept, want)
	}
}

// VC-23 (b): three transient failures give up after exactly 3 attempts.
func TestRetry_GivesUpAfterThreeAttempts(t *testing.T) {
	next := &scriptedClient{errs: []error{unavailable(), unavailable(), unavailable(), unavailable()}}
	var slept []time.Duration
	c := newTestRetryingClient(next, 1, &slept)

	_, err := c.Open(context.Background(), "b", "o")

	if err == nil {
		t.Fatalf("expected an error after 3 failed attempts")
	}
	if next.attempts != 3 {
		t.Errorf("attempts = %d, want exactly 3", next.attempts)
	}
}

// VC-23 (c): a permanent error (403, 404) fails at once, with no retry.
func TestRetry_PermanentErrorsAreNotRetried(t *testing.T) {
	for _, code := range []int{403, 404} {
		next := &scriptedClient{errs: []error{&googleapi.Error{Code: code}}}
		var slept []time.Duration
		c := newTestRetryingClient(next, 1, &slept)

		_, err := c.Open(context.Background(), "b", "o")

		if err == nil {
			t.Fatalf("%d: expected the error to be returned", code)
		}
		if next.attempts != 1 || len(slept) != 0 {
			t.Errorf("%d: attempts = %d, delays = %v; want 1 attempt and no wait", code, next.attempts, slept)
		}
	}
}

// NFR-3 names timeouts as transient; the SDK's own check misses them
// when wrapped the way net/http returns them.
func TestRetry_TimeoutsAreTransient(t *testing.T) {
	next := &scriptedClient{errs: []error{timeoutError{}}}
	var slept []time.Duration
	c := newTestRetryingClient(next, 1, &slept)

	if _, err := c.Open(context.Background(), "b", "o"); err != nil {
		t.Fatalf("a timeout should be retried and then succeed, got %v", err)
	}
	if next.attempts != 2 {
		t.Errorf("attempts = %d, want 2", next.attempts)
	}
}

// The jitter factor scales each delay (±20% is applied by randomJitter).
func TestRetry_JitterScalesDelays(t *testing.T) {
	next := &scriptedClient{errs: []error{unavailable()}}
	var slept []time.Duration
	c := newTestRetryingClient(next, 1.2, &slept)

	c.Open(context.Background(), "b", "o")

	if want := []time.Duration{600 * time.Millisecond}; !equalDurations(slept, want) {
		t.Errorf("delays = %v, want %v", slept, want)
	}
	for range 1000 {
		if j := randomJitter(); j < 0.8 || j > 1.2 {
			t.Fatalf("randomJitter() = %v, want within [0.8, 1.2]", j)
		}
	}
}

// A cancelled run stops retrying instead of waiting out the backoff.
func TestRetry_StopsWhenContextIsCancelled(t *testing.T) {
	next := &scriptedClient{errs: []error{unavailable(), unavailable()}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := &retryingClient{next: next, sleep: sleepContext, jitter: randomJitter}

	if _, err := c.Open(ctx, "b", "o"); err == nil {
		t.Fatalf("expected the error from the only attempt")
	}
	if next.attempts != 1 {
		t.Errorf("attempts = %d, want 1 with a cancelled context", next.attempts)
	}
}

type timeoutError struct{}

func (timeoutError) Error() string   { return "i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return false }

func equalDurations(a, b []time.Duration) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
