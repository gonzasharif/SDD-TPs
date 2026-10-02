package gcsclient

import (
	"context"
	"errors"
	"io"
	"math/rand/v2"
	"time"
)

// RetryPolicy is NFR-3's retry policy for listing and opening.
type RetryPolicy struct {
	// Attempts is the total number of tries, the first one included.
	Attempts int
	// Delays[i] is the wait before try i+2 (so Delays[0] precedes the 2nd
	// try). If there are fewer delays than waits, the last one is reused.
	Delays []time.Duration
	// Jitter is the fraction each wait may vary by, in either direction
	// (0.2 means ±20%).
	Jitter float64
}

// DefaultRetryPolicy is exactly NFR-3: 3 attempts, waiting 500 ms before the
// 2nd and 1 s before the 3rd, each ±20%.
var DefaultRetryPolicy = RetryPolicy{
	Attempts: 3,
	Delays:   []time.Duration{500 * time.Millisecond, time.Second},
	Jitter:   0.2,
}

// retrying decorates a Client with NFR-3's retries. Only List and Open are
// retried; a failure while reading an already opened stream is never retried
// (re-reading would print the object's matches twice).
type retrying struct {
	inner  Client
	policy RetryPolicy
	// sleep and random are fields so tests can run without real waits.
	sleep  func(ctx context.Context, d time.Duration) error
	random func() float64 // in [0, 1)
}

// WithRetries returns inner wrapped with the retry policy p.
func WithRetries(inner Client, p RetryPolicy) Client {
	return &retrying{inner: inner, policy: p, sleep: sleepContext, random: rand.Float64}
}

func (r *retrying) List(ctx context.Context, bucket, prefix string) ([]ObjectInfo, error) {
	var out []ObjectInfo
	err := r.do(ctx, func() error {
		var err error
		out, err = r.inner.List(ctx, bucket, prefix)
		return err
	})
	return out, err
}

func (r *retrying) Open(ctx context.Context, bucket, object string) (io.ReadCloser, error) {
	var out io.ReadCloser
	err := r.do(ctx, func() error {
		var err error
		out, err = r.inner.Open(ctx, bucket, object)
		return err
	})
	return out, err
}

// do runs op until it succeeds, fails permanently, or the attempts run out.
func (r *retrying) do(ctx context.Context, op func() error) error {
	attempts := max(r.policy.Attempts, 1)
	var err error
	for attempt := 1; attempt <= attempts; attempt++ {
		if attempt > 1 {
			if sleepErr := r.sleep(ctx, r.delayBefore(attempt)); sleepErr != nil {
				return sleepErr
			}
		}
		err = op()
		if err == nil || !errors.Is(err, ErrTransient) {
			return err
		}
	}
	return &RetriesExhaustedError{Attempts: attempts, Err: err}
}

// delayBefore is the jittered wait before the given attempt (2 or more).
func (r *retrying) delayBefore(attempt int) time.Duration {
	if len(r.policy.Delays) == 0 {
		return 0
	}
	base := r.policy.Delays[min(attempt-2, len(r.policy.Delays)-1)]
	factor := 1 + r.policy.Jitter*(2*r.random()-1)
	return time.Duration(float64(base) * factor)
}

func sleepContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
