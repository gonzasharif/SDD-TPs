// Package scanner lists the objects under a bucket/prefix, applies the
// object-count guardrail (BR-3) before reading any content, processes each
// object, and decides the process exit code (FR-8). Iteration 1 processes
// objects sequentially; the worker pool (FR-13) is Iteration 3 scope.
package scanner

import (
	"context"

	"gcsgrep/internal/gcsclient"
	"gcsgrep/internal/match"
	"gcsgrep/internal/output"
	"gcsgrep/internal/reader"
)

// Exit codes follow grep's convention (FR-8): 0 means at least one match
// and no errors, 1 means no matches and no errors, 2 means some error
// occurred (an unreadable object, a guardrail hit, or a usage error)
// regardless of whether other objects matched.
const (
	ExitMatch   = 0
	ExitNoMatch = 1
	ExitError   = 2
)

// Default guardrails: BR-3 (object count), BR-4 (bytes per object), and
// BR-5 (bytes per run). Sizes count decompressed bytes.
const (
	DefaultMaxObjects    = 1000
	DefaultMaxObjectSize = 250 << 20
	DefaultMaxTotalSize  = 2 << 30
)

// Config controls a single scan run.
type Config struct {
	Bucket string
	Prefix string

	// MaxObjects is BR-3's guardrail: if the prefix lists more than this
	// many objects, the run aborts before reading any content. Zero
	// disables the guardrail entirely (the documented `--max 0`).
	MaxObjects int

	// MaxLineSize is forwarded to reader.Options (FR-15). Zero uses the
	// reader's default.
	MaxLineSize int

	// Mode selects the output mode: every matching line (default), only
	// the names of matching objects (-l), or a per-object count (-c).
	Mode reader.Mode

	// MaxObjectSize is BR-4's per-object limit in bytes; MaxTotalSize is
	// BR-5's limit for the whole run. Zero disables either one.
	MaxObjectSize int64
	MaxTotalSize  int64
}

// Run lists objects under cfg.Bucket/cfg.Prefix (FR-1, FR-2), applies the
// BR-3 guardrail, processes each object against m within the BR-4 and BR-5
// size limits, writes results and
// diagnostics through w, and returns the process exit code.
func Run(ctx context.Context, client gcsclient.Client, cfg Config, m *match.Matcher, w *output.Writer) int {
	objects, err := client.List(ctx, cfg.Bucket, cfg.Prefix)
	if err != nil {
		w.Error("could not list gs://%s/%s: %v", cfg.Bucket, cfg.Prefix, err)
		return ExitError
	}

	if cfg.MaxObjects > 0 && len(objects) > cfg.MaxObjects {
		w.Error(
			"the prefix has %d objects, which exceeds the limit of %d (use --max to raise it, or --max 0 to disable it)",
			len(objects), cfg.MaxObjects,
		)
		return ExitError
	}

	var budget *reader.Budget
	if cfg.MaxTotalSize > 0 {
		budget = reader.NewBudget(cfg.MaxTotalSize)
	}

	matchFound := false
	anyError := false

	for _, obj := range objects {
		// BR-5: once the run's budget is spent, no further object is
		// opened — not even one that might turn out to be empty.
		if budget.Exhausted() {
			w.Warning("total size limit of %d bytes reached; not reading the remaining objects, results are incomplete (use --max-total-size to raise it)", cfg.MaxTotalSize)
			anyError = true
			break
		}
		// BR-4, before opening: a listed size already over the limit
		// means reading it would cross the limit, compressed or not.
		if cfg.MaxObjectSize > 0 && obj.Size > cfg.MaxObjectSize {
			w.Warning("%s: skipped, its size (%d bytes) exceeds the per-object limit of %d bytes (use --max-object-size to raise it)", obj.Name, obj.Size, cfg.MaxObjectSize)
			anyError = true
			continue
		}

		stream, err := client.Open(ctx, cfg.Bucket, obj.Name)
		if err != nil {
			w.Warning("%s: could not open (%v)", obj.Name, err)
			anyError = true
			continue
		}

		res := reader.ProcessObject(stream, obj.Name, m, reader.Options{
			MaxLineSize:   cfg.MaxLineSize,
			Mode:          cfg.Mode,
			MaxObjectSize: cfg.MaxObjectSize,
			Budget:        budget,
		})
		stream.Close()

		if res.Skipped {
			w.Warning("%s: skipped (%s)", res.Object, res.SkipReason)
			continue
		}
		if res.MatchCount > 0 {
			matchFound = true
		}
		// Matches found before an object was cut short are still correct,
		// so they are printed; what's missing is the rest of the object.
		switch cfg.Mode {
		case reader.ModeList:
			if res.MatchCount > 0 {
				w.ObjectName(res.Object)
			}
		case reader.ModeCount:
			// A partial count would look like a real one, so none is
			// printed for an incomplete object.
			if !res.Incomplete() {
				w.Count(res.Object, res.MatchCount)
			}
		default:
			for _, lm := range res.Matches {
				w.Match(res.Object, lm.LineNum, lm.Text)
			}
		}

		if res.LongLineWarn {
			w.Warning("%s: at least one line exceeded the buffer and was skipped without matching", res.Object)
		}
		if res.Incomplete() {
			anyError = true
		}
		switch {
		case res.Failed:
			w.Warning("%s: %s", res.Object, res.FailReason)
		case res.ObjectSizeLimitHit:
			w.Warning("%s: stopped reading at the per-object limit of %d bytes, results for it are incomplete (use --max-object-size to raise it)", res.Object, cfg.MaxObjectSize)
		case res.TotalSizeLimitHit:
			w.Warning("%s: total size limit of %d bytes reached mid-object; not reading the remaining objects, results are incomplete (use --max-total-size to raise it)", res.Object, cfg.MaxTotalSize)
		}
		if res.TotalSizeLimitHit {
			break
		}
	}

	if anyError {
		return ExitError
	}
	if matchFound {
		return ExitMatch
	}
	return ExitNoMatch
}
