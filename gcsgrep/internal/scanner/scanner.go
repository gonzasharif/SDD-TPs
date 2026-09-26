// Package scanner runs one gcsgrep corrida: it lists the objects under a
// bucket/prefix, applies the object-count guardrail (BR-3) before reading
// any content, processes each object in listing order (FR-19.1), and decides
// the process exit code (FR-8). Iteration 1 processes objects sequentially;
// the worker pool (FR-13) is Iteration 3 scope.
package scanner

import (
	"context"
	"errors"
	"fmt"

	"gcsgrep/internal/gcsclient"
	"gcsgrep/internal/match"
	"gcsgrep/internal/output"
	"gcsgrep/internal/reader"
)

// Exit codes follow grep's convention (FR-8): 0 means at least one match
// and no errors, 1 means no matches and no errors, 2 means some error
// occurred regardless of whether other objects matched.
const (
	ExitMatch   = 0
	ExitNoMatch = 1
	ExitError   = 2
)

// DefaultMaxObjects is BR-3's default object-count guardrail.
const DefaultMaxObjects = 1000

// Config controls a single scan run.
type Config struct {
	Bucket string
	Prefix string

	// MaxObjects is BR-3's guardrail: if the prefix lists more than this
	// many objects, the run aborts before reading any content. Zero
	// disables the guardrail entirely (the documented `--max 0`).
	MaxObjects int
}

// Run lists objects under cfg.Bucket/cfg.Prefix (FR-1, FR-2, FR-18),
// applies the BR-3 guardrail, processes each object against m, writes
// results and diagnostics through w, and returns the process exit code.
func Run(ctx context.Context, client gcsclient.Client, cfg Config, m *match.Matcher, w *output.Writer) int {
	objects, err := client.List(ctx, cfg.Bucket, cfg.Prefix)
	if err != nil {
		w.Error("%s", describeListError(err, cfg)) // FR-16.3, FR-16.4
		return ExitError
	}

	if len(objects) == 0 {
		w.Warning("no objects under gs://%s/%s", cfg.Bucket, cfg.Prefix) // FR-17
		return ExitNoMatch
	}

	if cfg.MaxObjects > 0 && len(objects) > cfg.MaxObjects {
		w.Error(
			"the prefix has %d objects, which exceeds the limit of %d (use --max to raise it, or --max 0 to disable it)",
			len(objects), cfg.MaxObjects,
		) // BR-3
		return ExitError
	}

	matchFound := false
	anyError := false
	for _, obj := range objects {
		matched, failed := scanObject(ctx, client, cfg.Bucket, obj.Name, m, w)
		matchFound = matchFound || matched
		anyError = anyError || failed
	}

	switch {
	case anyError:
		return ExitError
	case matchFound:
		return ExitMatch
	default:
		return ExitNoMatch
	}
}

// scanObject opens and processes one object, printing its matches as they
// are found and its avisos afterwards. It reports whether the object had at
// least one match and whether it ended as an objeto fallido (FR-9, NFR-3).
func scanObject(ctx context.Context, client gcsclient.Client, bucket, name string, m *match.Matcher, w *output.Writer) (matched, failed bool) {
	stream, err := client.Open(ctx, bucket, name)
	if err != nil {
		w.Warning("%s: %s", name, describeOpenError(err)) // FR-9.1, FR-9.3, FR-9.4
		return false, true
	}
	defer stream.Close()

	res := reader.ProcessObject(stream, m, reader.Options{}, func(lm reader.LineMatch) {
		w.Match(name, lm.LineNum, lm.Text)
	})

	if res.Skipped {
		w.Warning("%s: skipped (%s)", name, res.SkipReason) // FR-11
		return false, false
	}
	if res.LongLineWarn {
		w.Warning("%s: skipped lines longer than 1 MiB", name) // FR-15
	}
	if res.Failed {
		w.Warning("%s: %s", name, res.FailReason) // NFR-3: read interrupted
		return res.MatchCount > 0, true
	}
	return res.MatchCount > 0, false
}

// describeListError builds the FR-16 message for a failed listing.
func describeListError(err error, cfg Config) string {
	switch {
	case errors.Is(err, gcsclient.ErrPermissionDenied):
		return fmt.Sprintf("permission denied listing gs://%s/%s", cfg.Bucket, cfg.Prefix)
	case errors.Is(err, gcsclient.ErrBucketNotFound):
		return fmt.Sprintf("bucket %s does not exist", cfg.Bucket)
	default:
		return fmt.Sprintf("could not list gs://%s/%s: %v", cfg.Bucket, cfg.Prefix, err)
	}
}

// describeOpenError builds the <causa> of the FR-9 aviso for an object that
// could not be opened.
func describeOpenError(err error) string {
	switch {
	case errors.Is(err, gcsclient.ErrPermissionDenied):
		return "permission denied"
	case errors.Is(err, gcsclient.ErrObjectNotFound):
		return "object not found"
	default:
		return fmt.Sprintf("read failed: %v", err)
	}
}
