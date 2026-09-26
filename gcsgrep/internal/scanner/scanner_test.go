package scanner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"gcsgrep/internal/gcsclient"
	"gcsgrep/internal/gcsclient/gcsclienttest"
	"gcsgrep/internal/match"
	"gcsgrep/internal/output"
)

func mustMatcher(t *testing.T, pattern string) *match.Matcher {
	t.Helper()
	m, err := match.New(pattern, false)
	if err != nil {
		t.Fatalf("match.New: %v", err)
	}
	return m
}

type result struct {
	code           int
	stdout, stderr string
}

func run(t *testing.T, client gcsclient.Client, cfg Config, pattern string) result {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), client, cfg, mustMatcher(t, pattern), output.New(&stdout, &stderr))
	return result{code, stdout.String(), stderr.String()}
}

func cfg(prefix string) Config {
	return Config{Bucket: "b", Prefix: prefix, MaxObjects: DefaultMaxObjects}
}

func expect(t *testing.T, got result, code int, stdout, stderr string) {
	t.Helper()
	if got.code != code {
		t.Errorf("exit code = %d, want %d", got.code, code)
	}
	if got.stdout != stdout {
		t.Errorf("stdout = %q, want %q", got.stdout, stdout)
	}
	if got.stderr != stderr {
		t.Errorf("stderr = %q, want %q", got.stderr, stderr)
	}
}

// VC-1.1, VC-8.1: exact stdout for the logs/ data set, exit 0.
func TestRun_Results(t *testing.T) {
	client := &gcsclienttest.Fake{Objects: []gcsclienttest.Object{
		{Name: "logs/a.log", Content: "INFO start\nERROR timeout\n"},
		{Name: "logs/b.log", Content: "INFO ok\n"},
	}}
	expect(t, run(t, client, cfg("logs/"), "timeout"), ExitMatch, "logs/a.log:2:ERROR timeout\n", "")
}

// VC-2.1 / FR-2: a location without prefix lists the whole bucket (empty
// prefix) and covers objects under every prefix.
func TestRun_WholeBucket(t *testing.T) {
	client := &gcsclienttest.Fake{Objects: []gcsclienttest.Object{
		{Name: "a/1.log", Content: "timeout\n"},
		{Name: "b/2.log", Content: "timeout\n"},
		{Name: "c/d/3.log", Content: "timeout\n"},
		{Name: "raiz.log", Content: "timeout\n"},
	}}
	expect(t, run(t, client, cfg(""), "timeout"), ExitMatch,
		"a/1.log:1:timeout\nb/2.log:1:timeout\nc/d/3.log:1:timeout\nraiz.log:1:timeout\n", "")
	if got := client.ListPrefixes(); len(got) != 1 || got[0] != "" {
		t.Errorf("List prefixes = %q, want exactly one call with the empty prefix", got)
	}
}

// VC-8.2: no matches and no errors, exit 1.
func TestRun_NoMatch(t *testing.T) {
	client := &gcsclienttest.Fake{Objects: []gcsclienttest.Object{
		{Name: "logs/a.log", Content: "INFO start\nERROR timeout\n"},
	}}
	expect(t, run(t, client, cfg("logs/"), "patron_inexistente_xyz"), ExitNoMatch, "", "")
}

// VC-28.1 / FR-19.1: objects in listing order, lines in ascending order.
func TestRun_SequentialOrder(t *testing.T) {
	client := &gcsclienttest.Fake{Objects: []gcsclienttest.Object{
		{Name: "ord/a.log", Content: "match 1\nmatch 2\n"},
		{Name: "ord/b.log", Content: "match 1\nmatch 2\n"},
		{Name: "ord/c.log", Content: "match 1\nmatch 2\n"},
	}}
	want := "ord/a.log:1:match 1\nord/a.log:2:match 2\n" +
		"ord/b.log:1:match 1\nord/b.log:2:match 2\n" +
		"ord/c.log:1:match 1\nord/c.log:2:match 2\n"
	expect(t, run(t, client, cfg("ord/"), "match"), ExitMatch, want, "")
}

// VC-27 / FR-18: a prefix without a trailing slash is passed as-is to the
// listing, so it also covers "logs-old/".
func TestRun_PrefixWithoutTrailingSlash(t *testing.T) {
	client := &gcsclienttest.Fake{Objects: []gcsclienttest.Object{
		{Name: "pfx/logs-old/b.log", Content: "timeout\n"},
		{Name: "pfx/logs/a.log", Content: "timeout\n"},
		{Name: "pfx/other/c.log", Content: "timeout\n"},
	}}
	expect(t, run(t, client, cfg("pfx/logs"), "timeout"), ExitMatch,
		"pfx/logs-old/b.log:1:timeout\npfx/logs/a.log:1:timeout\n", "")
}

// VC-8.3, VC-9.1, VC-9.3, VC-9.4: an object that cannot be opened is an
// objeto fallido with the literal cause; the run continues and exits 2.
func TestRun_ObjectFailsToOpen(t *testing.T) {
	cases := []struct {
		name    string
		openErr error
		cause   string
	}{
		{"FR-9.1 permission denied", fmt.Errorf("%w: googleapi: Error 403", gcsclient.ErrPermissionDenied), "permission denied"},
		{"FR-9.3 object not found", fmt.Errorf("%w: storage: object doesn't exist", gcsclient.ErrObjectNotFound), "object not found"},
		{"FR-9.4 other 4xx", errors.New("googleapi: Error 400: customer-supplied encryption key required"), "read failed: googleapi: Error 400: customer-supplied encryption key required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := &gcsclienttest.Fake{Objects: []gcsclienttest.Object{
				{Name: "x/bad.log", OpenErr: tc.openErr},
				{Name: "x/ok.log", Content: "timeout\n"},
			}}
			expect(t, run(t, client, cfg("x/"), "timeout"), ExitError,
				"x/ok.log:1:timeout\n", "gcsgrep: warning: x/bad.log: "+tc.cause+"\n")
		})
	}
}

// VC-25.3, VC-25.4: listing errors end the run with a mensaje de error and
// no object is opened.
func TestRun_ListFails(t *testing.T) {
	cases := []struct {
		name    string
		listErr error
		stderr  string
	}{
		{"FR-16.3 permission denied", fmt.Errorf("%w: googleapi: Error 403", gcsclient.ErrPermissionDenied), "gcsgrep: error: permission denied listing gs://b/logs/\n"},
		{"FR-16.4 bucket does not exist", fmt.Errorf("%w: storage: bucket doesn't exist", gcsclient.ErrBucketNotFound), "gcsgrep: error: bucket b does not exist\n"},
		{"other error", errors.New("boom"), "gcsgrep: error: could not list gs://b/logs/: boom\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := &gcsclienttest.Fake{ListErr: tc.listErr}
			expect(t, run(t, client, cfg("logs/"), "timeout"), ExitError, "", tc.stderr)
			if n := len(client.Opened()); n != 0 {
				t.Errorf("opened %d objects after a failed listing", n)
			}
		})
	}
}

// VC-26 / FR-17: a location with no objects is an aviso and exit 1, not an
// error.
func TestRun_NoObjects(t *testing.T) {
	client := &gcsclienttest.Fake{}
	expect(t, run(t, client, cfg("prefijo-sin-objetos/"), "timeout"), ExitNoMatch, "",
		"gcsgrep: warning: no objects under gs://b/prefijo-sin-objetos/\n")
}

func tenObjects() *gcsclienttest.Fake {
	f := &gcsclienttest.Fake{}
	for i := 0; i < 10; i++ {
		f.Objects = append(f.Objects, gcsclienttest.Object{Name: fmt.Sprintf("max/%02d.log", i), Content: "timeout\n"})
	}
	return f
}

// VC-17.1 / BR-3: over the limit, the run aborts before opening any object.
func TestRun_ObjectCountGuardrail(t *testing.T) {
	client := tenObjects()
	got := run(t, client, Config{Bucket: "b", Prefix: "max/", MaxObjects: 5}, "timeout")
	expect(t, got, ExitError, "",
		"gcsgrep: error: the prefix has 10 objects, which exceeds the limit of 5 (use --max to raise it, or --max 0 to disable it)\n")
	if client.ListCalls() != 1 || len(client.Opened()) != 0 {
		t.Errorf("want 1 listing and 0 opens, got %d and %d", client.ListCalls(), len(client.Opened()))
	}
}

// VC-17.2, VC-17.3: a raised or disabled limit processes every object.
func TestRun_ObjectCountGuardrailRaisedOrDisabled(t *testing.T) {
	for _, max := range []int{20, 0} {
		client := tenObjects()
		got := run(t, client, Config{Bucket: "b", Prefix: "max/", MaxObjects: max}, "timeout")
		if got.code != ExitMatch || len(client.Opened()) != 10 {
			t.Errorf("--max %d: exit %d, %d opens; want exit 0 and 10 opens", max, got.code, len(client.Opened()))
		}
	}
}

// VC-11 / FR-11: a binary object is skipped with the literal aviso and is
// not an error.
func TestRun_BinarySkipped(t *testing.T) {
	client := &gcsclienttest.Fake{Objects: []gcsclienttest.Object{
		{Name: "bin/a.log", Content: "timeout\n"},
		{Name: "bin/icon.png", Content: "\x89PNG\r\n\x1a\n\x00\x00\x00\x0dIHDR timeout"},
	}}
	expect(t, run(t, client, cfg("bin/"), "timeout"), ExitMatch,
		"bin/a.log:1:timeout\n", "gcsgrep: warning: bin/icon.png: skipped (binary object)\n")
}

// VC-24 / FR-15: lines over 1 MiB are skipped with exactly one aviso per
// object; they are not an error.
func TestRun_LongLinesSkipped(t *testing.T) {
	long5 := strings.Repeat("x", 2*1024*1024) + "timeout" + strings.Repeat("x", 3*1024*1024)
	long2 := strings.Repeat("y", 2*1024*1024)
	client := &gcsclienttest.Fake{Objects: []gcsclienttest.Object{
		{Name: "long/x.log", Content: "timeout antes\n" + long5 + "\ntimeout despues\n" + long2 + "\n"},
	}}
	expect(t, run(t, client, cfg("long/"), "timeout"), ExitMatch,
		"long/x.log:1:timeout antes\nlong/x.log:3:timeout despues\n",
		"gcsgrep: warning: long/x.log: skipped lines longer than 1 MiB\n")
}

// VC-30.1 / FR-21.1: a 0-byte object produces no output at all.
func TestRun_EmptyObject(t *testing.T) {
	client := &gcsclienttest.Fake{Objects: []gcsclienttest.Object{
		{Name: "e/a.log", Content: "timeout\n"},
		{Name: "e/empty.log", Content: ""},
	}}
	expect(t, run(t, client, cfg("e/"), "timeout"), ExitMatch, "e/a.log:1:timeout\n", "")
}

// VC-23.5 (Iteration 1 slice) / NFR-3: a stream that breaks mid-read keeps
// the matches already printed (once), is an objeto fallido, and the object is
// not reopened.
func TestRun_ReadInterrupted(t *testing.T) {
	client := &gcsclienttest.Fake{Objects: []gcsclienttest.Object{
		{Name: "r/mid.log", Content: "timeout uno\nINFO\n", ReadErr: errors.New("connection reset")},
	}}
	expect(t, run(t, client, cfg("r/"), "timeout"), ExitError,
		"r/mid.log:1:timeout uno\n", "gcsgrep: warning: r/mid.log: read interrupted: connection reset\n")
	if n := len(client.Opened()); n != 1 {
		t.Errorf("opened %d times, want exactly 1 (no retry)", n)
	}
}
