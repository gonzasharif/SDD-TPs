package gcsclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"syscall"

	"cloud.google.com/go/storage"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iterator"
)

// gcsClient is the Client backed by the real GCS SDK. It is the only code in
// gcsgrep that imports the SDK.
type gcsClient struct {
	sc *storage.Client
}

// New builds a Client authenticated via Application Default Credentials —
// gcsgrep never accepts a service-account key file (decision #2 in
// gcsgrep-design.md). An error here means no ADC could be resolved
// (FR-16.2).
//
// The SDK's own retries are turned off: the only retry policy is NFR-3's,
// applied by WithRetries, so the SDK's attempts and waits never add to it.
func New(ctx context.Context) (Client, error) {
	sc, err := storage.NewClient(ctx)
	if err != nil {
		return nil, err
	}
	sc.SetRetry(storage.WithPolicy(storage.RetryNever))
	return &gcsClient{sc: sc}, nil
}

func (c *gcsClient) List(ctx context.Context, bucket, prefix string) ([]ObjectInfo, error) {
	it := c.sc.Bucket(bucket).Objects(ctx, &storage.Query{Prefix: prefix})
	var out []ObjectInfo
	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, translateListError(err)
		}
		out = append(out, ObjectInfo{Name: attrs.Name, Size: attrs.Size})
	}
	return out, nil
}

func (c *gcsClient) Open(ctx context.Context, bucket, object string) (io.ReadCloser, error) {
	r, err := c.sc.Bucket(bucket).Object(object).NewReader(ctx)
	if err != nil {
		return nil, translateOpenError(err)
	}
	return r, nil
}

// translateListError maps SDK listing errors to the domain errors of
// FR-16.3 (403) and FR-16.4 (bucket does not exist).
func translateListError(err error) error {
	switch {
	case errors.Is(err, storage.ErrBucketNotExist), httpStatus(err) == http.StatusNotFound:
		return fmt.Errorf("%w: %v", ErrBucketNotFound, err)
	case httpStatus(err) == http.StatusForbidden:
		return fmt.Errorf("%w: %v", ErrPermissionDenied, err)
	case isTransient(err):
		return Transient(err)
	default:
		return err
	}
}

// translateOpenError maps SDK open errors to the domain errors of FR-9.1
// (403) and FR-9.3 (404). Any other error is returned as-is (FR-9.4).
func translateOpenError(err error) error {
	switch {
	case errors.Is(err, storage.ErrObjectNotExist), httpStatus(err) == http.StatusNotFound:
		return fmt.Errorf("%w: %v", ErrObjectNotFound, err)
	case httpStatus(err) == http.StatusForbidden:
		return fmt.Errorf("%w: %v", ErrPermissionDenied, err)
	case isTransient(err):
		return Transient(err)
	default:
		return err
	}
}

// isTransient reports whether err is one of NFR-3's transient failures: a
// timeout, a reset connection, or HTTP 408, 429 or 5xx. A canceled context
// is never transient.
func isTransient(err error) bool {
	if errors.Is(err, context.Canceled) {
		return false
	}
	switch status := httpStatus(err); {
	case status == http.StatusRequestTimeout, status == http.StatusTooManyRequests, status >= 500:
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, syscall.ECONNRESET) ||
		strings.Contains(err.Error(), "connection reset")
}

// httpStatus returns the HTTP status code carried by a GCS API error, or 0.
func httpStatus(err error) int {
	var apiErr *googleapi.Error
	if errors.As(err, &apiErr) {
		return apiErr.Code
	}
	return 0
}
