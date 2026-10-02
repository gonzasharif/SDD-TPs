package scanner

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"gcsgrep/internal/gcsclient/gcsclienttest"
	"gcsgrep/internal/output"
)

// runWithProgress runs the scanner with progress on, like production does.
func runWithProgress(t *testing.T, fake *gcsclienttest.Fake, c Config, terminal bool, pattern string) result {
	t.Helper()
	var stdout, stderr bytes.Buffer
	w := output.NewWithOptions(&stdout, &stderr, output.Options{Progress: true, StderrTerminal: terminal})
	code := Run(context.Background(), fake, c, mustMatcher(t, pattern), w)
	return result{code, stdout.String(), stderr.String()}
}

func fiftyObjects() *gcsclienttest.Fake {
	fake := &gcsclienttest.Fake{}
	for i := 0; i < 50; i++ {
		fake.Objects = append(fake.Objects, gcsclienttest.Object{Name: fmt.Sprintf("prog/%02d.log", i), Content: "INFO ok\n"})
	}
	return fake
}

// VC-10.1: 50 objects on a terminal.
func TestRun_Progress_Terminal(t *testing.T) {
	got := runWithProgress(t, fiftyObjects(), cfg("prog/"), true, "timeout")

	if got.code != ExitNoMatch {
		t.Errorf("exit = %d, want 1", got.code)
	}
	redraws := strings.Split(strings.TrimSuffix(got.stderr, "\n"), "\r")[1:]
	if len(redraws) != 50 || !strings.HasSuffix(got.stderr, "\n") {
		t.Fatalf("got %d redraws, want 50, ending in \\n: %q", len(redraws), got.stderr)
	}
	for i, r := range redraws {
		want := fmt.Sprintf("gcsgrep: progress: %d/50 (%d%%)", i+1, (i+1)*2)
		if r != want {
			t.Errorf("redraw %d = %q, want %q", i+1, r, want)
		}
	}
}

// VC-10.2: 50 objects with stderr redirected.
func TestRun_Progress_Redirected(t *testing.T) {
	got := runWithProgress(t, fiftyObjects(), cfg("prog/"), false, "timeout")

	lines := strings.Split(strings.TrimSuffix(got.stderr, "\n"), "\n")
	if len(lines) != 10 || strings.Contains(got.stderr, "\r") {
		t.Fatalf("got %d lines, want 10 and no \\r: %q", len(lines), got.stderr)
	}
	for i, l := range lines {
		want := fmt.Sprintf("gcsgrep: progress: %d/50 (%d%%)", (i+1)*5, (i+1)*10)
		if l != want {
			t.Errorf("line %d = %q, want %q", i+1, l, want)
		}
	}
}

// FR-10: no progress when no object is processed (VC-26, VC-17.1, ...).
func TestRun_Progress_NotWhenNoObjectIsProcessed(t *testing.T) {
	empty := runWithProgress(t, &gcsclienttest.Fake{}, cfg("none/"), true, "timeout")
	expect(t, empty, ExitNoMatch, "", "gcsgrep: warning: no objects under gs://b/none/\n")

	tooMany := cfg("prog/")
	tooMany.MaxObjects = 5
	got := runWithProgress(t, fiftyObjects(), tooMany, true, "timeout")
	if strings.Contains(got.stderr, "progress") || got.code != ExitError {
		t.Errorf("code = %d, stderr = %q; want exit 2 with no progress", got.code, got.stderr)
	}
}

// FR-10: skipped and failed objects count as processed.
func TestRun_Progress_CountsSkippedAndFailedObjects(t *testing.T) {
	fake := &gcsclienttest.Fake{Objects: []gcsclienttest.Object{
		{Name: "p/1.png", Content: "\x00\x01"},
		{Name: "p/2.log", OpenErr: fmt.Errorf("boom")},
	}}
	got := runWithProgress(t, fake, cfg("p/"), false, "timeout")

	want := "gcsgrep: progress: 1/2 (50%)\n" +
		"gcsgrep: warning: p/2.log: read failed: boom\n" +
		"gcsgrep: progress: 2/2 (100%)\n"
	// The skip warning of p/1.png comes before its progress line.
	want = "gcsgrep: warning: p/1.png: skipped (binary object)\n" + want
	if got.stderr != want {
		t.Errorf("stderr = %q,\nwant %q", got.stderr, want)
	}
}

// FR-10 / BR-5: when the total limit cuts the run, the cut object counts as
// processed, progress stays at its last value, and the error follows.
func TestRun_Progress_AfterTotalLimit(t *testing.T) {
	c := cfg("tot/")
	c.MaxTotalSize = 2621440
	got := runWithProgress(t, totData(), c, false, "timeout")

	want := "gcsgrep: progress: 1/5 (20%)\n" +
		"gcsgrep: progress: 2/5 (40%)\n" +
		"gcsgrep: progress: 3/5 (60%)\n" +
		"gcsgrep: error: total size limit of 2621440 bytes reached, scan incomplete\n"
	if got.stderr != want {
		t.Errorf("stderr = %q,\nwant %q", got.stderr, want)
	}
}
