package gcsclient

import (
	"context"
	"errors"
	"fmt"
	"net"
	"syscall"
	"testing"

	"google.golang.org/api/googleapi"
)

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

var _ net.Error = timeoutErr{}

// NFR-3: which failures are transient and which are permanent.
func TestIsTransient(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"HTTP 408", &googleapi.Error{Code: 408}, true},
		{"HTTP 429", &googleapi.Error{Code: 429}, true},
		{"HTTP 500", &googleapi.Error{Code: 500}, true},
		{"HTTP 503", &googleapi.Error{Code: 503}, true},
		{"HTTP 400", &googleapi.Error{Code: 400}, false},
		{"HTTP 401", &googleapi.Error{Code: 401}, false},
		{"HTTP 403", &googleapi.Error{Code: 403}, false},
		{"HTTP 404", &googleapi.Error{Code: 404}, false},
		{"network timeout", timeoutErr{}, true},
		{"wrapped timeout", fmt.Errorf("Get: %w", timeoutErr{}), true},
		{"connection reset", fmt.Errorf("read: %w", syscall.ECONNRESET), true},
		{"connection reset by text", errors.New("read tcp: connection reset by peer"), true},
		{"deadline exceeded", context.DeadlineExceeded, true},
		{"canceled", context.Canceled, false},
		{"unknown error", errors.New("boom"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isTransient(tc.err); got != tc.want {
				t.Errorf("isTransient(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// Domain errors win over the transient classification, and transient errors
// keep their own message as the <detalle> of NFR-3's messages.
func TestTranslate(t *testing.T) {
	if err := translateOpenError(&googleapi.Error{Code: 403}); !errors.Is(err, ErrPermissionDenied) {
		t.Errorf("open 403 = %v, want ErrPermissionDenied", err)
	}
	if err := translateOpenError(&googleapi.Error{Code: 404}); !errors.Is(err, ErrObjectNotFound) {
		t.Errorf("open 404 = %v, want ErrObjectNotFound", err)
	}
	if err := translateListError(&googleapi.Error{Code: 404}); !errors.Is(err, ErrBucketNotFound) {
		t.Errorf("list 404 = %v, want ErrBucketNotFound", err)
	}
	sdkErr := &googleapi.Error{Code: 503, Message: "backend unavailable"}
	if err := translateOpenError(sdkErr); !errors.Is(err, ErrTransient) || err.Error() != sdkErr.Error() {
		t.Errorf("open 503 = %v, want transient with the SDK's message", err)
	}
	if err := translateListError(&googleapi.Error{Code: 429, Message: "slow down"}); !errors.Is(err, ErrTransient) {
		t.Errorf("list 429 = %v, want transient", err)
	}
	if err := translateOpenError(&googleapi.Error{Code: 400, Message: "csek"}); errors.Is(err, ErrTransient) {
		t.Errorf("open 400 = %v, must not be transient", err)
	}
}
