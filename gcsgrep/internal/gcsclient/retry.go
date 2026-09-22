package gcsclient

import (
	"context"
	"errors"
	"io"
	"math/rand/v2"
	"net"
	"time"

	"cloud.google.com/go/storage"
)

// retryDelays are NFR-3's waits between attempts: 3 attempts in total, so
// 500ms before the 2nd and 1s before the 3rd.
var retryDelays = []time.Duration{500 * time.Millisecond, time.Second}

// retryJitter is the ± fraction applied to each delay, so many clients
// retrying at once don't hit GCS in lockstep.
const retryJitter = 0.2

// retryingClient retries transient failures of List and Open. It never
// retries a read that is already in progress: by then matches from the
// object may be printed, and reading it again from the start would print
// them twice (NFR-3). Nothing has been printed yet when List or Open
// fails, so retrying those is always safe.
type retryingClient struct {
	next  Client
	sleep func(ctx context.Context, d time.Duration) error
	// jitter returns a factor in [1-retryJitter, 1+retryJitter].
	jitter func() float64
}

// WithRetries wraps c with NFR-3's retry policy.
func WithRetries(c Client) Client {
	return &retryingClient{next: c, sleep: sleepContext, jitter: randomJitter}
}

func (r *retryingClient) List(ctx context.Context, bucket, prefix string) ([]ObjectInfo, error) {
	var out []ObjectInfo
	err := r.retry(ctx, func() error {
		var err error
		out, err = r.next.List(ctx, bucket, prefix)
		return err
	})
	return out, err
}

func (r *retryingClient) Open(ctx context.Context, bucket, object string) (io.ReadCloser, error) {
	var out io.ReadCloser
	err := r.retry(ctx, func() error {
		var err error
		out, err = r.next.Open(ctx, bucket, object)
		return err
	})
	return out, err
}

func (r *retryingClient) retry(ctx context.Context, call func() error) error {
	err := call()
	for _, delay := range retryDelays {
		if err == nil || !isTransient(err) || ctx.Err() != nil {
			return err
		}
		if sleepErr := r.sleep(ctx, time.Duration(float64(delay)*r.jitter())); sleepErr != nil {
			return err
		}
		err = call()
	}
	return err
}

// isTransient reports whether err is worth retrying: 5xx, 408, 429, and
// connection-level failures (per the SDK's own classification), plus
// network timeouts, which the SDK's check misses when they come wrapped
// in a *url.Error. 403, 404, and any other 4xx are permanent.
func isTransient(err error) bool {
	if storage.ShouldRetry(err) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
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

func randomJitter() float64 {
	return 1 - retryJitter + rand.Float64()*2*retryJitter
}
