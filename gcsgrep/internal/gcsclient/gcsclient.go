// Package gcsclient is the only place in gcsgrep that imports the GCS SDK.
// The Client interface it exposes has no write, delete, or permission
// methods — that omission is what makes BR-1 ("gcsgrep never writes to
// GCS") a structural property of the code instead of a rule someone has to
// remember to follow.
package gcsclient

import (
	"context"
	"io"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
)

// ObjectInfo describes a listed object without its content.
type ObjectInfo struct {
	Name string
	Size int64
}

// Client is the read-only surface gcsgrep uses to talk to GCS.
type Client interface {
	// List returns every object under bucket/prefix. It never reads object
	// content — it's the cheap listing call the BR-3 count guardrail relies
	// on before any content is read.
	List(ctx context.Context, bucket, prefix string) ([]ObjectInfo, error)
	// Open returns a streaming reader for a single object's content. The
	// caller is responsible for closing it.
	Open(ctx context.Context, bucket, object string) (io.ReadCloser, error)
}

type gcsClient struct {
	sc *storage.Client
}

// New builds a Client authenticated via Application Default Credentials,
// with no retries of its own (wrap it with WithRetries for NFR-3) —
// gcsgrep never accepts a service-account key file (decision recorded in
// gcsgrep-requirements.md).
func New(ctx context.Context) (Client, error) {
	sc, err := storage.NewClient(ctx)
	if err != nil {
		return nil, err
	}
	// The SDK's own retries are turned off so that the only retry policy
	// in effect is NFR-3's (WithRetries). Otherwise the two would stack,
	// and the SDK would also transparently reopen a reader mid-read.
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
			return nil, err
		}
		out = append(out, ObjectInfo{Name: attrs.Name, Size: attrs.Size})
	}
	return out, nil
}

func (c *gcsClient) Open(ctx context.Context, bucket, object string) (io.ReadCloser, error) {
	return c.sc.Bucket(bucket).Object(object).NewReader(ctx)
}
