package scanner

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"gcsgrep/internal/gcsclient"
	"gcsgrep/internal/match"
	"gcsgrep/internal/output"
)

// fakeClient is an in-memory gcsclient.Client used to unit-test scanner
// without touching real GCS. Objects map to their content; a name present
// in unreadable causes Open to fail, simulating a permission error (FR-9).
type fakeClient struct {
	objects     map[string]string // name -> content
	unreadable  map[string]bool
	listErr     error
	listedNames []string // controls listing order for deterministic tests
}

func (f *fakeClient) List(ctx context.Context, bucket, prefix string) ([]gcsclient.ObjectInfo, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	var out []gcsclient.ObjectInfo
	names := f.listedNames
	if names == nil {
		for name := range f.objects {
			names = append(names, name)
		}
	}
	for _, name := range names {
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		out = append(out, gcsclient.ObjectInfo{Name: name, Size: int64(len(f.objects[name]))})
	}
	return out, nil
}

func (f *fakeClient) Open(ctx context.Context, bucket, object string) (io.ReadCloser, error) {
	if f.unreadable[object] {
		return nil, errors.New("permission denied")
	}
	content, ok := f.objects[object]
	if !ok {
		return nil, errors.New("not found")
	}
	return io.NopCloser(strings.NewReader(content)), nil
}

func mustMatcher(t *testing.T, pattern string) *match.Matcher {
	t.Helper()
	m, err := match.New(pattern, false)
	if err != nil {
		t.Fatalf("match.New: %v", err)
	}
	return m
}

// VC-8 (a): a run with a guaranteed match and every object readable exits 0.
func TestRun_ExitMatchWhenSomethingMatches(t *testing.T) {
	client := &fakeClient{objects: map[string]string{
		"logs/app1.log": "connection timeout after 30s\n",
		"logs/app2.log": "all good\n",
	}}
	var stdout, stderr bytes.Buffer
	w := output.New(&stdout, &stderr)

	code := Run(context.Background(), client, Config{Bucket: "b", MaxObjects: DefaultMaxObjects}, mustMatcher(t, "timeout"), w)

	if code != ExitMatch {
		t.Errorf("exit code = %d, want %d (ExitMatch)", code, ExitMatch)
	}
	if !strings.Contains(stdout.String(), "logs/app1.log:1:connection timeout after 30s") {
		t.Errorf("stdout does not contain the expected result: %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("expected nothing on stderr, got: %q", stderr.String())
	}
}

// VC-8 (b): no matches and everything readable, exit 1.
func TestRun_ExitNoMatchWhenNothingMatches(t *testing.T) {
	client := &fakeClient{objects: map[string]string{
		"logs/app2.log": "all good\n",
	}}
	var stdout, stderr bytes.Buffer
	w := output.New(&stdout, &stderr)

	code := Run(context.Background(), client, Config{Bucket: "b", MaxObjects: DefaultMaxObjects}, mustMatcher(t, "timeout"), w)

	if code != ExitNoMatch {
		t.Errorf("exit code = %d, want %d (ExitNoMatch)", code, ExitNoMatch)
	}
	if stdout.Len() != 0 {
		t.Errorf("expected no results on stdout: %q", stdout.String())
	}
}

// VC-8 (c) / VC-9: an unreadable object forces exit 2 even though other
// objects do match, and the run keeps processing the rest (FR-9) instead
// of aborting as soon as it hits the problematic object.
func TestRun_ExitErrorOnUnreadableObjectButKeepsGoing(t *testing.T) {
	client := &fakeClient{
		objects: map[string]string{
			"logs/app1.log": "connection timeout after 30s\n",
			"logs/secret":   "content should not matter",
		},
		unreadable: map[string]bool{"logs/secret": true},
	}
	var stdout, stderr bytes.Buffer
	w := output.New(&stdout, &stderr)

	code := Run(context.Background(), client, Config{Bucket: "b", MaxObjects: DefaultMaxObjects}, mustMatcher(t, "timeout"), w)

	if code != ExitError {
		t.Fatalf("exit code = %d, want %d (ExitError) even though another object matched", code, ExitError)
	}
	if !strings.Contains(stdout.String(), "logs/app1.log:1:connection timeout after 30s") {
		t.Errorf("the readable object should still be processed and appear on stdout: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "logs/secret") {
		t.Errorf("stderr should mention the unreadable object: %q", stderr.String())
	}
}

// BR-3 / VC-17: if the object count exceeds the limit, the run aborts
// BEFORE reading any content (zero calls to Open).
func TestRun_ObjectCountGuardrailAbortsBeforeReadingContent(t *testing.T) {
	opened := map[string]bool{}
	client := &countingClient{
		fakeClient: fakeClient{objects: map[string]string{
			"a": "x", "b": "x", "c": "x", "d": "x",
		}},
		opened: opened,
	}
	var stdout, stderr bytes.Buffer
	w := output.New(&stdout, &stderr)

	code := Run(context.Background(), client, Config{Bucket: "b", MaxObjects: 2}, mustMatcher(t, "x"), w)

	if code != ExitError {
		t.Fatalf("exit code = %d, want %d (ExitError) for exceeding the count guardrail", code, ExitError)
	}
	if len(opened) != 0 {
		t.Errorf("should not have opened any object after exceeding the guardrail, opened: %v", opened)
	}
	if stdout.Len() != 0 {
		t.Errorf("expected no results on stdout: %q", stdout.String())
	}
}

// With --max 0 (or any sufficiently high value) the guardrail does not
// block the run and every object is processed.
func TestRun_ObjectCountGuardrailDisabledWithZero(t *testing.T) {
	client := &fakeClient{objects: map[string]string{
		"a": "match here", "b": "match here", "c": "match here", "d": "match here",
	}}
	var stdout, stderr bytes.Buffer
	w := output.New(&stdout, &stderr)

	code := Run(context.Background(), client, Config{Bucket: "b", MaxObjects: 0}, mustMatcher(t, "match"), w)

	if code != ExitMatch {
		t.Fatalf("exit code = %d, want %d (ExitMatch) with the guardrail disabled", code, ExitMatch)
	}
	if strings.Count(stdout.String(), "\n") != 4 {
		t.Errorf("expected 4 result lines (one per object), got: %q", stdout.String())
	}
}

// countingClient wraps fakeClient and records every object name passed to
// Open, so guardrail tests can assert zero content reads happened.
type countingClient struct {
	fakeClient
	opened map[string]bool
}

func (c *countingClient) Open(ctx context.Context, bucket, object string) (io.ReadCloser, error) {
	c.opened[object] = true
	return c.fakeClient.Open(ctx, bucket, object)
}
