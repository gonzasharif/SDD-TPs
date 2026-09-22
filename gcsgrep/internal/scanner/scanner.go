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

	// MaxLineSize is forwarded to reader.Options (FR-15). Zero uses the
	// reader's default.
	MaxLineSize int

	// Mode selects the output mode: every matching line (default), only
	// the names of matching objects (-l), or a per-object count (-c).
	Mode reader.Mode
}

// Run lists objects under cfg.Bucket/cfg.Prefix (FR-1, FR-2), applies the
// BR-3 guardrail, processes each object against m, writes results and
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

	matchFound := false
	anyError := false

	for _, obj := range objects {
		stream, err := client.Open(ctx, cfg.Bucket, obj.Name)
		if err != nil {
			w.Warning("%s: could not open (%v)", obj.Name, err)
			anyError = true
			continue
		}

		res := reader.ProcessObject(stream, obj.Name, m, reader.Options{MaxLineSize: cfg.MaxLineSize, Mode: cfg.Mode})
		stream.Close()

		if res.Failed {
			w.Warning("%s: %s", res.Object, res.FailReason)
			anyError = true
			continue
		}
		if res.Skipped {
			w.Warning("%s: skipped (%s)", res.Object, res.SkipReason)
			continue
		}
		if res.LongLineWarn {
			w.Warning("%s: at least one line exceeded the buffer and was skipped without matching", res.Object)
		}
		if res.MatchCount > 0 {
			matchFound = true
		}
		switch cfg.Mode {
		case reader.ModeList:
			if res.MatchCount > 0 {
				w.ObjectName(res.Object)
			}
		case reader.ModeCount:
			w.Count(res.Object, res.MatchCount)
		default:
			for _, lm := range res.Matches {
				w.Match(res.Object, lm.LineNum, lm.Text)
			}
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
