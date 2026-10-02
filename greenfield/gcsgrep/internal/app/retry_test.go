package app

import (
	"errors"
	"strings"
	"testing"
	"time"

	"gcsgrep/internal/gcsclient"
	"gcsgrep/internal/gcsclient/gcsclienttest"
)

func http503() error { return gcsclient.Transient(errors.New("HTTP 503 Service Unavailable")) }

func gap(t *testing.T, times []time.Time, i int) time.Duration {
	t.Helper()
	if len(times) <= i+1 {
		t.Fatalf("only %d attempts recorded, need %d", len(times), i+2)
	}
	return times[i+1].Sub(times[i])
}

func between(t *testing.T, name string, got, lo, hi time.Duration) {
	t.Helper()
	if got < lo || got > hi {
		t.Errorf("%s = %v, want %v-%v", name, got, lo, hi)
	}
}

func retryData(objects ...gcsclienttest.Object) *gcsclienttest.Fake {
	return &gcsclienttest.Fake{Objects: objects}
}

// VC-23.1: two 503s then success.
func TestVC23_1_OpenRecoversAfterTwoFailures(t *testing.T) {
	fake := retryData(gcsclienttest.Object{
		Name: "r/a.log", Content: "timeout\n", OpenFailures: []error{http503(), http503()},
	})

	got := runWith(fake, nil, "timeout", "gs://b/r/")

	if got.code != 0 || got.stdout != "r/a.log:1:timeout\n" || got.stderr != "" {
		t.Errorf("code=%d stdout=%q stderr=%q; want 0, the match, and empty stderr", got.code, got.stdout, got.stderr)
	}
	times := fake.OpenTimes("r/a.log")
	if len(times) != 3 {
		t.Fatalf("open attempts = %d, want 3", len(times))
	}
	between(t, "wait 1→2", gap(t, times, 0), 400*time.Millisecond, 600*time.Millisecond)
	between(t, "wait 2→3", gap(t, times, 1), 800*time.Millisecond, 1200*time.Millisecond)
}

// VC-23.2: three 503s leave an objeto fallido.
func TestVC23_2_OpenExhaustsThreeAttempts(t *testing.T) {
	fake := retryData(gcsclienttest.Object{
		Name: "r/a.log", Content: "timeout\n", OpenFailures: []error{http503(), http503(), http503()},
	})

	got := runWith(fake, nil, "timeout", "gs://b/r/")

	if n := len(fake.Opened()); n != 3 {
		t.Errorf("open attempts = %d, want 3", n)
	}
	if !strings.Contains(got.stderr, "gcsgrep: warning: r/a.log: network error after 3 attempts: HTTP 503 Service Unavailable\n") {
		t.Errorf("stderr = %q, want the network error aviso", got.stderr)
	}
	if got.code != 2 || got.stdout != "" {
		t.Errorf("code=%d stdout=%q; want 2 and empty stdout", got.code, got.stdout)
	}
}

// VC-23.3: HTTP 403 is permanent, so a single attempt.
func TestVC23_3_PermanentErrorIsNotRetried(t *testing.T) {
	fake := retryData(gcsclienttest.Object{
		Name: "r/a.log", Content: "timeout\n", OpenErr: gcsclient.ErrPermissionDenied,
	})

	got := runWith(fake, nil, "timeout", "gs://b/r/")

	if n := len(fake.Opened()); n != 1 {
		t.Errorf("open attempts = %d, want 1", n)
	}
	if !strings.Contains(got.stderr, "gcsgrep: warning: r/a.log: permission denied\n") || got.code != 2 {
		t.Errorf("code=%d stderr=%q; want 2 and the permission denied aviso", got.code, got.stderr)
	}
}

// VC-23.4: listing fails three times, so nothing is opened.
func TestVC23_4_ListExhaustsThreeAttempts(t *testing.T) {
	fake := retryData(gcsclienttest.Object{Name: "r/a.log", Content: "timeout\n"})
	fake.ListFailures = []error{http503(), http503(), http503()}

	got := runWith(fake, nil, "timeout", "gs://b/r/")

	if n := fake.ListCalls(); n != 3 {
		t.Errorf("list attempts = %d, want 3", n)
	}
	if n := len(fake.Opened()); n != 0 {
		t.Errorf("open attempts = %d, want 0", n)
	}
	want := "gcsgrep: error: could not list gs://b/r/ after 3 attempts: HTTP 503 Service Unavailable\n"
	if got.stderr != want || got.code != 2 || got.stdout != "" {
		t.Errorf("code=%d stdout=%q stderr=%q; want 2, empty stdout, %q", got.code, got.stdout, got.stderr, want)
	}
}

// VC-23.5: a connection reset after line 2 is not retried and the matches
// already printed are not printed again.
func TestVC23_5_MidReadErrorIsNotRetried(t *testing.T) {
	fake := retryData(gcsclienttest.Object{
		Name:    "r/mid.log",
		Content: "timeout uno\nINFO\n", // the stream breaks after line 2
		ReadErr: errors.New("read tcp: connection reset by peer"),
	})

	got := runWith(fake, nil, "timeout", "gs://b/r/")

	if got.stdout != "r/mid.log:1:timeout uno\n" {
		t.Errorf("stdout = %q, want the line-1 match exactly once", got.stdout)
	}
	if !strings.Contains(got.stderr, "gcsgrep: warning: r/mid.log: read interrupted: ") {
		t.Errorf("stderr = %q, want the read interrupted aviso", got.stderr)
	}
	if n := len(fake.Opened()); n != 1 || got.code != 2 {
		t.Errorf("open attempts = %d, code = %d; want 1 and 2", n, got.code)
	}
}
