//go:build integration

package integration

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"testing"
	"time"

	"gcsgrep/internal/app"
	"gcsgrep/internal/gcsclient"
)

// Measurements repeat each run 3 times and take the median (NFR-1, NFR-2).
const repetitions = 3

func requireBench(t *testing.T) {
	t.Helper()
	if bucket == "" {
		t.Skip("GCSGREP_TEST_BUCKET is not set")
	}
	if os.Getenv("GCSGREP_BENCH") != "1" {
		t.Skip("measurement VC: set GCSGREP_BENCH=1 to run it")
	}
}

func medianDuration(ds []time.Duration) time.Duration {
	s := append([]time.Duration(nil), ds...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	return s[len(s)/2]
}

func medianInt(xs []int64) int64 {
	s := append([]int64(nil), xs...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	return s[len(s)/2]
}

// VC-21.1 / NFR-1: sequential throughput over perf/ (500 objects of 1 MiB)
// is at least 0.5 objects/second (median of 3 runs, wall clock).
func TestVC21_1_SequentialThroughput(t *testing.T) {
	requireBench(t)
	var ds []time.Duration
	for i := 0; i < repetitions; i++ {
		var stdout, stderr bytes.Buffer
		start := time.Now()
		code := app.Run(context.Background(), []string{"needle", loc("perf/")}, &stdout, &stderr, gcsclient.New)
		d := time.Since(start)
		if code != 0 || stdout.String() != "perf/000.log:1:needle\n" {
			t.Fatalf("run %d: exit %d, stdout %q, stderr %q", i+1, code, stdout.String(), stderr.String())
		}
		t.Logf("run %d: %.1fs (%.2f objects/s)", i+1, d.Seconds(), 500/d.Seconds())
		ds = append(ds, d)
	}
	m := medianDuration(ds)
	throughput := 500 / m.Seconds()
	t.Logf("median: %.1fs -> %.2f objects/s (threshold >= 0.5)", m.Seconds(), throughput)
	if throughput < 0.5 {
		t.Errorf("sequential throughput %.2f objects/s < 0.5", throughput)
	}
}

// firstByteWriter records when the first byte reaches stdout and cancels the
// run right after: VC-21.3 only measures the first result.
type firstByteWriter struct {
	start  time.Time
	cancel context.CancelFunc
	once   sync.Once
	at     time.Duration
	buf    bytes.Buffer
}

func (w *firstByteWriter) Write(p []byte) (int, error) {
	w.once.Do(func() {
		w.at = time.Since(w.start)
		w.cancel()
	})
	return w.buf.Write(p)
}

// VC-21.3 / NFR-1: with the match on line 1 of the first listed object, the
// first byte of stdout arrives within 2 seconds (median of 3 runs).
func TestVC21_3_FirstResultLatency(t *testing.T) {
	requireBench(t)
	var ds []time.Duration
	for i := 0; i < repetitions; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		w := &firstByteWriter{start: time.Now(), cancel: cancel}
		var stderr bytes.Buffer
		app.Run(ctx, []string{"needle", loc("perf/")}, w, &stderr, gcsclient.New)
		cancel()
		if w.buf.String() != "perf/000.log:1:needle\n" {
			t.Fatalf("run %d: stdout %q, stderr %q", i+1, w.buf.String(), stderr.String())
		}
		t.Logf("run %d: first result after %.3fs", i+1, w.at.Seconds())
		ds = append(ds, w.at)
	}
	m := medianDuration(ds)
	t.Logf("median: %.3fs (threshold <= 2s)", m.Seconds())
	if m > 2*time.Second {
		t.Errorf("first result latency %.3fs > 2s", m.Seconds())
	}
}

// VC-22 / NFR-2: the peak RSS of a run over a 500 MiB object exceeds the one
// over a 5 MiB object by at most 5 MiB (median of 3 runs each). RSS is read
// with getrusage, the same value /usr/bin/time reports; needs Linux or macOS.
// In Iteration 1 the run has no --max-object-size/--max-total-size flags yet.
func TestVC22_ConstantMemory(t *testing.T) {
	requireBench(t)
	if runtime.GOOS == "windows" {
		t.Skip("peak RSS via getrusage needs Linux or macOS")
	}
	bin := filepath.Join(t.TempDir(), "gcsgrep")
	if out, err := exec.Command("go", "build", "-o", bin, "../cmd/gcsgrep").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	peak := func(object string) int64 {
		var rss []int64
		for i := 0; i < repetitions; i++ {
			cmd := exec.Command(bin, "--max-object-size", "0", "--max-total-size", "0", "needle", loc(object))
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			err := cmd.Run()
			if code := cmd.ProcessState.ExitCode(); code != 1 {
				t.Fatalf("%s run %d: exit %d (%v), want 1\n--- stderr\n%s", object, i+1, code, err, stderr.String())
			}
			b, ok := maxRSSBytes(cmd.ProcessState)
			if !ok {
				t.Skip("peak RSS is not available on this platform")
			}
			t.Logf("%s run %d: max RSS %.1f MiB", object, i+1, float64(b)/(1<<20))
			rss = append(rss, b)
		}
		return medianInt(rss)
	}

	small, large := peak("mem/small.log"), peak("mem/large.log")
	diff := large - small
	t.Logf("median max RSS: small %.1f MiB, large %.1f MiB, diff %.1f MiB (threshold <= 5 MiB)",
		float64(small)/(1<<20), float64(large)/(1<<20), float64(diff)/(1<<20))
	if diff > 5<<20 {
		t.Errorf("RSS difference %d bytes > 5 MiB", diff)
	}
}
