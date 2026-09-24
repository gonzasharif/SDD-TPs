// Package scanner lists the objects under a bucket/prefix, applies the
// object-count guardrail (BR-3) before reading any content, processes each
// object within the size guardrails (BR-4, BR-5) while reporting progress
// (FR-10), and decides the process exit code (FR-8). Objects are processed
// by a pool of workers (FR-13) that pull from one shared queue.
package scanner

import (
	"context"
	"sync"
	"sync/atomic"

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

// Concurrency bounds (FR-13, FR-14, BR-6). One worker is the sequential
// default; MaxConcurrency is a hard ceiling, enforced by the CLI before any
// GCS call, so the flag can't be used to bypass the cost and load
// guardrails.
const (
	DefaultConcurrency = 1
	MaxConcurrency     = 32
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

	// Concurrency is FR-13's number of workers. Zero (or less) means one:
	// objects are then processed strictly in listing order. Range
	// validation (BR-6) is the CLI's job; Run just never starts more
	// workers than there are objects.
	Concurrency int
}

// Run lists objects under cfg.Bucket/cfg.Prefix (FR-1, FR-2), applies the
// BR-3 guardrail, processes each object against m within the BR-4 and BR-5
// size limits using cfg.Concurrency workers (FR-13), writes results,
// diagnostics, and progress (FR-10) through w, and returns the process
// exit code.
//
// With more than one worker, objects finish in any order, so their output
// is not in listing order; each object's output is still printed as one
// contiguous block.
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

	// The queue is filled up front and closed: workers take the next
	// object as soon as they finish one, so a big object never holds up
	// the small ones behind it (no fixed partition of the list).
	queue := make(chan gcsclient.ObjectInfo, len(objects))
	for _, obj := range objects {
		queue <- obj
	}
	close(queue)

	var state runState
	w.StartProgress(len(objects))

	var wg sync.WaitGroup
	for range workerCount(cfg.Concurrency, len(objects)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for obj := range queue {
				processQueued(ctx, client, cfg, obj, m, w, budget, &state)
			}
		}()
	}
	wg.Wait()

	w.FinishProgress()

	if state.anyError.Load() {
		return ExitError
	}
	if state.matchFound.Load() {
		return ExitMatch
	}
	return ExitNoMatch
}

// workerCount clamps the requested concurrency to [1, objects].
func workerCount(requested, objects int) int {
	return max(1, min(requested, objects))
}

// runState is what the workers share. Every field is atomic: a worker
// records its object's outcome without taking any lock, and Run reads the
// totals only after all workers have finished.
type runState struct {
	matchFound atomic.Bool // at least one match anywhere (FR-8 exit 0)
	anyError   atomic.Bool // something was skipped or cut short (FR-8 exit 2)
	// stopped is BR-5: the run's budget ran out, so no worker opens
	// another object. Once set, the queue is drained without reading.
	stopped atomic.Bool
}

// processQueued handles one object taken from the queue.
func processQueued(ctx context.Context, client gcsclient.Client, cfg Config, obj gcsclient.ObjectInfo, m *match.Matcher, w *output.Writer, budget *reader.Budget, state *runState) {
	if state.stopped.Load() {
		return
	}
	// BR-5: once the run's budget is spent, no further object is
	// opened — not even one that might turn out to be empty. Only the
	// worker that flips `stopped` reports it, so the warning appears once.
	if budget.Exhausted() {
		if state.stopped.CompareAndSwap(false, true) {
			w.Warning("total size limit of %d bytes reached; not reading the remaining objects, results are incomplete (use --max-total-size to raise it)", cfg.MaxTotalSize)
		}
		state.anyError.Store(true)
		return
	}

	out := scanObject(ctx, client, cfg, obj, m, w, budget)
	w.AdvanceProgress()
	if out.matched {
		state.matchFound.Store(true)
	}
	if out.failed {
		state.anyError.Store(true)
	}
	if out.stop {
		state.stopped.Store(true)
	}
}

// objectOutcome is what one object contributes to the run's result.
type objectOutcome struct {
	matched bool // at least one match (FR-8 exit 0)
	failed  bool // searched only partially or not at all (FR-8 exit 2)
	stop    bool // BR-5's budget ran out: open no further object
}

// scanObject opens, searches, and reports a single object.
func scanObject(ctx context.Context, client gcsclient.Client, cfg Config, obj gcsclient.ObjectInfo, m *match.Matcher, w *output.Writer, budget *reader.Budget) objectOutcome {
	// BR-4, before opening: a listed size already over the limit means
	// reading it would cross the limit, compressed or not.
	if cfg.MaxObjectSize > 0 && obj.Size > cfg.MaxObjectSize {
		w.Warning("%s: skipped, its size (%d bytes) exceeds the per-object limit of %d bytes (use --max-object-size to raise it)", obj.Name, obj.Size, cfg.MaxObjectSize)
		return objectOutcome{failed: true}
	}

	stream, err := client.Open(ctx, cfg.Bucket, obj.Name)
	if err != nil {
		w.Warning("%s: could not open (%v)", obj.Name, err)
		return objectOutcome{failed: true}
	}

	res := reader.ProcessObject(stream, obj.Name, m, reader.Options{
		MaxLineSize:   cfg.MaxLineSize,
		Mode:          cfg.Mode,
		MaxObjectSize: cfg.MaxObjectSize,
		Budget:        budget,
	})
	stream.Close()

	// Everything this object produced is printed inside one Do, so with
	// several workers its lines and warnings stay together.
	w.Do(func(s *output.Section) {
		reportObject(s, cfg, res)
	})

	return objectOutcome{
		matched: res.MatchCount > 0,
		failed:  res.Incomplete(),
		stop:    res.TotalSizeLimitHit,
	}
}

// reportObject writes the results and warnings for one processed object.
func reportObject(s *output.Section, cfg Config, res reader.ObjectResult) {
	if res.Skipped {
		s.Warning("%s: skipped (%s)", res.Object, res.SkipReason)
		return
	}

	// Matches found before an object was cut short are still correct, so
	// they are printed; what's missing is the rest of the object.
	switch cfg.Mode {
	case reader.ModeList:
		if res.MatchCount > 0 {
			s.ObjectName(res.Object)
		}
	case reader.ModeCount:
		// A partial count would look like a real one, so none is printed
		// for an incomplete object.
		if !res.Incomplete() {
			s.Count(res.Object, res.MatchCount)
		}
	default:
		for _, lm := range res.Matches {
			s.Match(res.Object, lm.LineNum, lm.Text, lm.Spans)
		}
	}

	if res.LongLineWarn {
		s.Warning("%s: at least one line exceeded the buffer and was skipped without matching", res.Object)
	}
	switch {
	case res.Failed:
		s.Warning("%s: %s", res.Object, res.FailReason)
	case res.ObjectSizeLimitHit:
		s.Warning("%s: stopped reading at the per-object limit of %d bytes, results for it are incomplete (use --max-object-size to raise it)", res.Object, cfg.MaxObjectSize)
	case res.TotalSizeLimitHit:
		s.Warning("%s: total size limit of %d bytes reached mid-object; not reading the remaining objects, results are incomplete (use --max-total-size to raise it)", res.Object, cfg.MaxTotalSize)
	}
}
