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
	"strings"

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

// Default values of the size guardrails (BR-4, BR-5).
const (
	DefaultMaxObjectSize = 250 << 20 // 250 MiB
	DefaultMaxTotalSize  = 2 << 30   // 2 GiB
)

// Mode is what a run prints for each object.
type Mode int

const (
	// ModeLines prints every matching line as object:line:text (FR-3).
	ModeLines Mode = iota
	// ModeFilesWithMatches prints the name of each object with a match and
	// stops reading it at the first match (FR-5, `-l`).
	ModeFilesWithMatches
	// ModeCount prints object:count for each object read completely (FR-6,
	// `-c`).
	ModeCount
)

// Config controls a single scan run.
type Config struct {
	Bucket string
	Prefix string

	// MaxObjects is BR-3's guardrail: if the prefix lists more than this
	// many objects, the run aborts before reading any content. Zero
	// disables the guardrail entirely (the documented `--max 0`).
	MaxObjects int

	// MaxObjectSize is BR-4's limit, in bytes of decompressed content read
	// from one object. Zero disables it (`--max-object-size 0`).
	MaxObjectSize int64

	// MaxTotalSize is BR-5's limit, in bytes of decompressed content read in
	// the whole run. Zero disables it (`--max-total-size 0`).
	MaxTotalSize int64

	// Mode selects the output (the zero value is ModeLines).
	Mode Mode
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

	budget := reader.NewBudget(cfg.MaxTotalSize)
	matchFound := false
	anyError := false
	defer w.EndProgress() // FR-10.1
	for i, obj := range objects {
		res := scanObject(ctx, client, cfg, budget, obj.Name, m, w)
		matchFound = matchFound || res.matched
		anyError = anyError || res.failed
		w.Progress(i+1, len(objects)) // FR-10
		if res.scanIncomplete {
			w.Error("total size limit of %d bytes reached, scan incomplete", cfg.MaxTotalSize) // BR-5
			anyError = true
			break
		}
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

// objectResult is what scanning one object means for the run.
type objectResult struct {
	matched bool
	// failed means the object counts as an error: an objeto fallido (FR-9,
	// NFR-3) or an objeto cortado (BR-4).
	failed bool
	// scanIncomplete means BR-5's limit was reached while reading this
	// object, so the run must stop.
	scanIncomplete bool
}

// scanObject opens and processes one object, printing its matches as they
// are found and its avisos afterwards.
func scanObject(ctx context.Context, client gcsclient.Client, cfg Config, budget *reader.Budget, name string, m *match.Matcher, w *output.Writer) objectResult {
	stream, err := client.Open(ctx, cfg.Bucket, name)
	if err != nil {
		w.Warning("%s: %s", name, describeOpenError(err)) // FR-9.1, FR-9.3, FR-9.4, NFR-3
		return objectResult{failed: true}
	}
	defer stream.Close()

	opts := reader.Options{
		Gzip:          strings.HasSuffix(name, ".gz"), // FR-12
		MaxObjectSize: cfg.MaxObjectSize,
		Budget:        budget,
	}
	var emit func(reader.LineMatch)
	switch cfg.Mode {
	case ModeLines:
		emit = func(lm reader.LineMatch) {
			var spans []match.Span
			if w.Color() {
				spans = m.Spans(lm.Text) // FR-3.2
			}
			w.Match(name, lm.LineNum, lm.Text, spans)
		}
	case ModeFilesWithMatches:
		opts.StopAtFirstMatch = true // FR-5
	}
	res := reader.ProcessObject(stream, m, opts, emit)

	out := objectResult{matched: res.MatchCount > 0}
	if res.Skipped {
		w.Warning("%s: skipped (%s)", name, res.SkipReason) // FR-11
		return out
	}
	if res.LongLineWarn {
		w.Warning("%s: skipped lines longer than 1 MiB", name) // FR-15
	}
	if cfg.Mode == ModeFilesWithMatches && res.MatchCount > 0 {
		w.ObjectName(name) // FR-5
	}
	switch {
	case res.ScanIncomplete:
		out.scanIncomplete = true // BR-5: Run prints the error
	case res.Cut:
		w.Warning(
			"%s: object size limit of %d bytes reached, rest of the object not read",
			name, cfg.MaxObjectSize,
		) // BR-4
		out.failed = true
	case res.Failed:
		w.Warning("%s: %s", name, res.FailReason) // FR-9.2, NFR-3
		out.failed = true
	default:
		if cfg.Mode == ModeCount {
			w.Count(name, res.MatchCount) // FR-6: only for an object read completely
		}
	}
	return out
}

// describeListError builds the FR-16 message for a failed listing.
func describeListError(err error, cfg Config) string {
	var exhausted *gcsclient.RetriesExhaustedError
	switch {
	case errors.As(err, &exhausted):
		return fmt.Sprintf("could not list gs://%s/%s after %d attempts: %v", cfg.Bucket, cfg.Prefix, exhausted.Attempts, exhausted.Err) // NFR-3
	case errors.Is(err, gcsclient.ErrPermissionDenied):
		return fmt.Sprintf("permission denied listing gs://%s/%s", cfg.Bucket, cfg.Prefix)
	case errors.Is(err, gcsclient.ErrBucketNotFound):
		return fmt.Sprintf("bucket %s does not exist", cfg.Bucket)
	default:
		return fmt.Sprintf("could not list gs://%s/%s: %v", cfg.Bucket, cfg.Prefix, err)
	}
}

// describeOpenError builds the <causa> of the FR-9 aviso for an object that
// could not be opened (FR-9, NFR-3).
func describeOpenError(err error) string {
	var exhausted *gcsclient.RetriesExhaustedError
	switch {
	case errors.As(err, &exhausted):
		return fmt.Sprintf("network error after %d attempts: %v", exhausted.Attempts, exhausted.Err) // NFR-3
	case errors.Is(err, gcsclient.ErrPermissionDenied):
		return "permission denied"
	case errors.Is(err, gcsclient.ErrObjectNotFound):
		return "object not found"
	default:
		return fmt.Sprintf("read failed: %v", err)
	}
}
