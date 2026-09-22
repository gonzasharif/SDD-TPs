package scanner

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"gcsgrep/internal/gcsclient"
	"gcsgrep/internal/match"
	"gcsgrep/internal/output"
	"gcsgrep/internal/reader"
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

// FR-5: -l prints each matching object's name exactly once, with no line
// number or text, and nothing for objects without a match.
func TestRun_ListModePrintsOnlyMatchingObjectNames(t *testing.T) {
	client := &fakeClient{
		objects: map[string]string{
			"logs/app1.log": "timeout one\ntimeout two\n",
			"logs/app2.log": "all good\n",
			"logs/app3.log": "another timeout\n",
		},
		listedNames: []string{"logs/app1.log", "logs/app2.log", "logs/app3.log"},
	}
	var stdout, stderr bytes.Buffer
	w := output.New(&stdout, &stderr)

	code := Run(context.Background(), client, Config{Bucket: "b", MaxObjects: DefaultMaxObjects, Mode: reader.ModeList}, mustMatcher(t, "timeout"), w)

	if code != ExitMatch {
		t.Errorf("exit code = %d, want %d (ExitMatch)", code, ExitMatch)
	}
	if want := "logs/app1.log\nlogs/app3.log\n"; stdout.String() != want {
		t.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
}

// VC-6: -c prints object:count for every processed object, including
// objects with zero matches, but not for skipped binaries — a 0 there
// would claim the object was searched when it wasn't.
func TestRun_CountModePrintsZeroCountsButNotSkippedObjects(t *testing.T) {
	client := &fakeClient{
		objects: map[string]string{
			"logs/app1.log": "timeout 1\nok\ntimeout 2\ntimeout 3\n",
			"logs/app2.log": "all good\n",
			"logs/icon.png": "\x89PNG\x00\x00timeout",
		},
		listedNames: []string{"logs/app1.log", "logs/app2.log", "logs/icon.png"},
	}
	var stdout, stderr bytes.Buffer
	w := output.New(&stdout, &stderr)

	code := Run(context.Background(), client, Config{Bucket: "b", MaxObjects: DefaultMaxObjects, Mode: reader.ModeCount}, mustMatcher(t, "timeout"), w)

	if code != ExitMatch {
		t.Errorf("exit code = %d, want %d (ExitMatch)", code, ExitMatch)
	}
	if want := "logs/app1.log:3\nlogs/app2.log:0\n"; stdout.String() != want {
		t.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
	if !strings.Contains(stderr.String(), "logs/icon.png") {
		t.Errorf("the skipped binary should still be reported on stderr: %q", stderr.String())
	}
}

// FR-8 under -c: printing only zero counts is still "no match", exit 1.
func TestRun_CountModeWithNoMatchesExitsNoMatch(t *testing.T) {
	client := &fakeClient{objects: map[string]string{"logs/app2.log": "all good\n"}}
	var stdout, stderr bytes.Buffer
	w := output.New(&stdout, &stderr)

	code := Run(context.Background(), client, Config{Bucket: "b", MaxObjects: DefaultMaxObjects, Mode: reader.ModeCount}, mustMatcher(t, "timeout"), w)

	if code != ExitNoMatch {
		t.Errorf("exit code = %d, want %d (ExitNoMatch)", code, ExitNoMatch)
	}
	if want := "logs/app2.log:0\n"; stdout.String() != want {
		t.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
}

func gzipped(t *testing.T, content string) string {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write([]byte(content)); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return buf.String()
}

// VC-12: matches inside a gzip object are reported under the object's own
// name, next to plain-text objects in the same prefix.
func TestRun_GzipObjectIsSearchedAndReportedByItsName(t *testing.T) {
	client := &fakeClient{
		objects: map[string]string{
			"logs/app1.log":    "all good\n",
			"logs/app2.log.gz": gzipped(t, "ok\nconnection timeout\n"),
		},
		listedNames: []string{"logs/app1.log", "logs/app2.log.gz"},
	}
	var stdout, stderr bytes.Buffer
	w := output.New(&stdout, &stderr)

	code := Run(context.Background(), client, Config{Bucket: "b", MaxObjects: DefaultMaxObjects}, mustMatcher(t, "timeout"), w)

	if code != ExitMatch {
		t.Errorf("exit code = %d, want %d (ExitMatch); stderr: %q", code, ExitMatch, stderr.String())
	}
	if want := "logs/app2.log.gz:2:connection timeout\n"; stdout.String() != want {
		t.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
}

// VC-18 (a): a plain object whose listed size is over --max-object-size
// is skipped without being opened; the run goes on and exits 2.
func TestRun_ObjectOverSizeLimitIsSkippedBeforeOpening(t *testing.T) {
	opened := map[string]bool{}
	client := &countingClient{
		fakeClient: fakeClient{
			objects: map[string]string{
				"logs/big.log":   strings.Repeat("timeout\n", 100),
				"logs/small.log": "timeout\n",
			},
			listedNames: []string{"logs/big.log", "logs/small.log"},
		},
		opened: opened,
	}
	var stdout, stderr bytes.Buffer
	w := output.New(&stdout, &stderr)

	code := Run(context.Background(), client, Config{Bucket: "b", MaxObjects: DefaultMaxObjects, MaxObjectSize: 100}, mustMatcher(t, "timeout"), w)

	if code != ExitError {
		t.Errorf("exit code = %d, want %d (ExitError)", code, ExitError)
	}
	if opened["logs/big.log"] {
		t.Errorf("the object over the limit should never be opened")
	}
	if want := "logs/small.log:1:timeout\n"; stdout.String() != want {
		t.Errorf("stdout = %q, want %q (the rest of the run goes on)", stdout.String(), want)
	}
	if !strings.Contains(stderr.String(), "logs/big.log") {
		t.Errorf("stderr should warn about the skipped object: %q", stderr.String())
	}
}

// VC-18 (b): a small gzip that expands past the limit is cut mid-read;
// its earlier matches are printed, the run goes on, and it exits 2.
func TestRun_ExpandingGzipIsCutAtObjectLimit(t *testing.T) {
	client := &fakeClient{
		objects: map[string]string{
			"logs/bomb.gz":   gzipped(t, "timeout early\n"+strings.Repeat("filler line\n", 100000)),
			"logs/small.log": "timeout\n",
		},
		listedNames: []string{"logs/bomb.gz", "logs/small.log"},
	}
	var stdout, stderr bytes.Buffer
	w := output.New(&stdout, &stderr)

	code := Run(context.Background(), client, Config{Bucket: "b", MaxObjects: DefaultMaxObjects, MaxObjectSize: 64 << 10}, mustMatcher(t, "timeout"), w)

	if code != ExitError {
		t.Errorf("exit code = %d, want %d (ExitError)", code, ExitError)
	}
	if want := "logs/bomb.gz:1:timeout early\nlogs/small.log:1:timeout\n"; stdout.String() != want {
		t.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
	if !strings.Contains(stderr.String(), "logs/bomb.gz: stopped reading at the per-object limit") {
		t.Errorf("stderr should warn about the cut: %q", stderr.String())
	}
}

// VC-19: crossing --max-total-size cuts the object being read, opens no
// further object, keeps the matches found so far, and exits 2.
func TestRun_TotalSizeLimitStopsTheRun(t *testing.T) {
	opened := map[string]bool{}
	chunk := strings.Repeat("timeout\n", 1000) // 8000 bytes
	client := &countingClient{
		fakeClient: fakeClient{
			objects:     map[string]string{"a.log": chunk, "b.log": chunk, "c.log": chunk},
			listedNames: []string{"a.log", "b.log", "c.log"},
		},
		opened: opened,
	}
	var stdout, stderr bytes.Buffer
	w := output.New(&stdout, &stderr)

	code := Run(context.Background(), client, Config{Bucket: "b", MaxObjects: DefaultMaxObjects, MaxTotalSize: 12000, Mode: reader.ModeCount}, mustMatcher(t, "timeout"), w)

	if code != ExitError {
		t.Errorf("exit code = %d, want %d (ExitError)", code, ExitError)
	}
	if opened["c.log"] {
		t.Errorf("no object should be opened after the total limit is reached")
	}
	// b.log was cut mid-object, so its partial count is not printed.
	if want := "a.log:1000\n"; stdout.String() != want {
		t.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
	if !strings.Contains(stderr.String(), "total size limit") {
		t.Errorf("stderr should warn that the run was cut: %q", stderr.String())
	}
}

// FR-9 / NFR-3 (d): a connection dropped mid-object prints the matches
// found before the drop exactly once, then fails the object with exit 2.
func TestRun_MidReadFailurePrintsEarlierMatchesOnce(t *testing.T) {
	client := &droppingClient{content: "timeout before the drop\n" + strings.Repeat("filler\n", 2000)}
	var stdout, stderr bytes.Buffer
	w := output.New(&stdout, &stderr)

	code := Run(context.Background(), client, Config{Bucket: "b", MaxObjects: DefaultMaxObjects}, mustMatcher(t, "timeout"), w)

	if code != ExitError {
		t.Errorf("exit code = %d, want %d (ExitError)", code, ExitError)
	}
	if want := "logs/app.log:1:timeout before the drop\n"; stdout.String() != want {
		t.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
	if !strings.Contains(stderr.String(), "connection reset") {
		t.Errorf("stderr should report the failure: %q", stderr.String())
	}
}

// droppingClient lists a single object whose stream fails after content.
type droppingClient struct{ content string }

func (d *droppingClient) List(ctx context.Context, bucket, prefix string) ([]gcsclient.ObjectInfo, error) {
	return []gcsclient.ObjectInfo{{Name: "logs/app.log", Size: int64(len(d.content))}}, nil
}

func (d *droppingClient) Open(ctx context.Context, bucket, object string) (io.ReadCloser, error) {
	return io.NopCloser(io.MultiReader(strings.NewReader(d.content), failingStream{errors.New("connection reset by peer")})), nil
}

type failingStream struct{ err error }

func (e failingStream) Read([]byte) (int, error) { return 0, e.err }
