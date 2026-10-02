package app

import (
	"strings"
	"testing"

	"gcsgrep/internal/gcsclient/gcsclienttest"
)

// BR-4 and BR-5: the flags reach the scanner, and without them the defaults
// (250 MiB, 2 GiB) apply.
func TestRun_SizeLimitFlagsAreApplied(t *testing.T) {
	content := "timeout\n" + strings.Repeat("INFO\n", 2<<20/5)
	fake := func() *gcsclienttest.Fake {
		return retryData(gcsclienttest.Object{Name: "big/a.log", Content: content})
	}

	cut := runWith(fake(), nil, "--max-object-size", "1048576", "timeout", "gs://b/big/")
	if cut.code != 2 || !strings.Contains(cut.stderr, "object size limit of 1048576 bytes reached") {
		t.Errorf("--max-object-size: code = %d, stderr = %q; want the cut aviso and exit 2", cut.code, cut.stderr)
	}

	incomplete := runWith(fake(), nil, "--max-total-size", "1048576", "timeout", "gs://b/big/")
	if incomplete.code != 2 || incomplete.stderr != "gcsgrep: error: total size limit of 1048576 bytes reached, scan incomplete\n" {
		t.Errorf("--max-total-size: code = %d, stderr = %q; want the scan incomplete error and exit 2", incomplete.code, incomplete.stderr)
	}

	defaults := runWith(fake(), nil, "timeout", "gs://b/big/")
	if defaults.code != 0 || defaults.stderr != "" {
		t.Errorf("defaults: code = %d, stderr = %q; want a complete read", defaults.code, defaults.stderr)
	}
}

// VC-7 / FR-7: -l with -c is a usage error that never reaches GCS.
func TestRun_FilesWithMatchesAndCountNeverTouchGCS(t *testing.T) {
	got := runWith(logsData(), nil, "-l", "-c", "timeout", "gs://b/logs/")

	if got.code != 2 || got.stdout != "" || got.stderr != "gcsgrep: error: -l and -c cannot be used together\n" {
		t.Errorf("code=%d stdout=%q stderr=%q; want 2, empty stdout and the literal error", got.code, got.stdout, got.stderr)
	}
	if got.factoryCalls != 0 || got.gcsCalls != 0 {
		t.Errorf("client created %d times, GCS called %d times; want 0 and 0", got.factoryCalls, got.gcsCalls)
	}
}

// FR-5, FR-6: the flags reach the scanner.
func TestRun_OutputModes(t *testing.T) {
	if got := runWith(logsData(), nil, "-l", "timeout", "gs://b/logs/"); got.stdout != "logs/a.log\n" || got.code != 0 {
		t.Errorf("-l: code=%d stdout=%q; want 0 and %q", got.code, got.stdout, "logs/a.log\n")
	}
	if got := runWith(logsData(), nil, "-c", "timeout", "gs://b/logs/"); got.stdout != "logs/a.log:1\nlogs/b.log:0\n" || got.code != 0 {
		t.Errorf("-c: code=%d stdout=%q; want 0 and both counts", got.code, got.stdout)
	}
}
