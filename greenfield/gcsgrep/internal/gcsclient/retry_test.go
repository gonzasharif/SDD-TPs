package gcsclient

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

// scripted is a Client whose List and Open return the next error of a
// script on each call (nil means success).
type scripted struct {
	listScript, openScript []error
	listCalls, openCalls   int
}

func (s *scripted) List(context.Context, string, string) ([]ObjectInfo, error) {
	s.listCalls++
	if s.listCalls <= len(s.listScript) && s.listScript[s.listCalls-1] != nil {
		return nil, s.listScript[s.listCalls-1]
	}
	return []ObjectInfo{{Name: "a.log"}}, nil
}

func (s *scripted) Open(context.Context, string, string) (io.ReadCloser, error) {
	s.openCalls++
	if s.openCalls <= len(s.openScript) && s.openScript[s.openCalls-1] != nil {
		return nil, s.openScript[s.openCalls-1]
	}
	return io.NopCloser(strings.NewReader("x")), nil
}

// testRetrying builds the decorator with recorded waits and a fixed jitter
// draw (0.5 means no jitter, 0 is -20%, just under 1 is +20%).
func testRetrying(inner Client, draw float64) (*retrying, *[]time.Duration) {
	var waits []time.Duration
	r := &retrying{
		inner:  inner,
		policy: DefaultRetryPolicy,
		sleep: func(_ context.Context, d time.Duration) error {
			waits = append(waits, d)
			return nil
		},
		random: func() float64 { return draw },
	}
	return r, &waits
}

var errBoom = errors.New("HTTP 503")

func TestOpen_RecoversAfterTransientErrors(t *testing.T) {
	inner := &scripted{openScript: []error{Transient(errBoom), Transient(errBoom), nil}}
	r, waits := testRetrying(inner, 0.5)

	if _, err := r.Open(context.Background(), "b", "a.log"); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if inner.openCalls != 3 {
		t.Errorf("attempts = %d, want 3", inner.openCalls)
	}
	want := []time.Duration{500 * time.Millisecond, time.Second}
	if len(*waits) != 2 || (*waits)[0] != want[0] || (*waits)[1] != want[1] {
		t.Errorf("waits = %v, want %v", *waits, want)
	}
}

func TestOpen_ExhaustsThreeAttempts(t *testing.T) {
	inner := &scripted{openScript: []error{Transient(errBoom), Transient(errBoom), Transient(errBoom), nil}}
	r, _ := testRetrying(inner, 0.5)

	_, err := r.Open(context.Background(), "b", "a.log")

	var exhausted *RetriesExhaustedError
	if !errors.As(err, &exhausted) || exhausted.Attempts != 3 {
		t.Fatalf("err = %v, want RetriesExhaustedError with 3 attempts", err)
	}
	if err.Error() != "HTTP 503" {
		t.Errorf("detail = %q, want the last attempt's error", err.Error())
	}
	if inner.openCalls != 3 {
		t.Errorf("attempts = %d, want exactly 3", inner.openCalls)
	}
}

func TestOpen_PermanentErrorIsNotRetried(t *testing.T) {
	inner := &scripted{openScript: []error{ErrPermissionDenied}}
	r, waits := testRetrying(inner, 0.5)

	_, err := r.Open(context.Background(), "b", "a.log")

	if !errors.Is(err, ErrPermissionDenied) {
		t.Errorf("err = %v, want ErrPermissionDenied", err)
	}
	if inner.openCalls != 1 || len(*waits) != 0 {
		t.Errorf("attempts = %d, waits = %v; want 1 attempt and no waits", inner.openCalls, *waits)
	}
}

func TestList_ExhaustsThreeAttempts(t *testing.T) {
	inner := &scripted{listScript: []error{Transient(errBoom), Transient(errBoom), Transient(errBoom)}}
	r, _ := testRetrying(inner, 0.5)

	_, err := r.List(context.Background(), "b", "p/")

	var exhausted *RetriesExhaustedError
	if !errors.As(err, &exhausted) || inner.listCalls != 3 {
		t.Errorf("err = %v, attempts = %d; want RetriesExhaustedError after exactly 3", err, inner.listCalls)
	}
}

func TestList_RecoversAfterTransientError(t *testing.T) {
	inner := &scripted{listScript: []error{Transient(errBoom), nil}}
	r, _ := testRetrying(inner, 0.5)

	objs, err := r.List(context.Background(), "b", "p/")

	if err != nil || len(objs) != 1 || inner.listCalls != 2 {
		t.Errorf("objs = %v, err = %v, attempts = %d; want 1 object after 2 attempts", objs, err, inner.listCalls)
	}
}

// NFR-3: waits are 500 ms and 1 s, each within ±20%.
func TestDelays_StayWithinTwentyPercent(t *testing.T) {
	for _, tc := range []struct {
		name     string
		draw     float64
		min, max time.Duration
	}{
		{"lowest jitter", 0, 400 * time.Millisecond, 800 * time.Millisecond},
		{"highest jitter", 0.999999, 600 * time.Millisecond, 1200 * time.Millisecond},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inner := &scripted{openScript: []error{Transient(errBoom), Transient(errBoom), nil}}
			r, waits := testRetrying(inner, tc.draw)
			if _, err := r.Open(context.Background(), "b", "a.log"); err != nil {
				t.Fatalf("Open: %v", err)
			}
			first, second := (*waits)[0], (*waits)[1]
			if first < 400*time.Millisecond || first > 600*time.Millisecond {
				t.Errorf("first wait = %v, want 400ms-600ms", first)
			}
			if second < 800*time.Millisecond || second > 1200*time.Millisecond {
				t.Errorf("second wait = %v, want 800ms-1200ms", second)
			}
		})
	}
}

func TestCanceledContextStopsRetrying(t *testing.T) {
	inner := &scripted{openScript: []error{Transient(errBoom), Transient(errBoom), Transient(errBoom)}}
	ctx, cancel := context.WithCancel(context.Background())
	r := &retrying{
		inner:  inner,
		policy: DefaultRetryPolicy,
		sleep:  sleepContext,
		random: func() float64 { return 0.5 },
	}
	cancel()

	_, err := r.Open(ctx, "b", "a.log")

	if !errors.Is(err, context.Canceled) || inner.openCalls != 1 {
		t.Errorf("err = %v, attempts = %d; want context.Canceled after 1 attempt", err, inner.openCalls)
	}
}
