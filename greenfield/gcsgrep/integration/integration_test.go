//go:build integration

package integration

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gcsgrep/internal/app"
	"gcsgrep/internal/gcsclient"
)

var (
	bucket   = os.Getenv("GCSGREP_TEST_BUCKET")
	credsDir = os.Getenv("GCSGREP_TEST_CREDS")
)

type result struct {
	code           int
	stdout, stderr string
}

// loc is gs://<bucket>/<prefix>.
func loc(prefix string) string { return "gs://" + bucket + "/" + prefix }

// run executes gcsgrep in-process through the same entry point as
// cmd/gcsgrep (app.Run with the real GCS client) and logs the full outcome,
// so `go test -v` doubles as the verification evidence.
func run(t *testing.T, args ...string) result {
	t.Helper()
	if bucket == "" {
		t.Skip("GCSGREP_TEST_BUCKET is not set")
	}
	var stdout, stderr bytes.Buffer
	code := app.Run(context.Background(), args, &stdout, &stderr, gcsclient.New)
	r := result{code, stdout.String(), stderr.String()}
	t.Logf("gcsgrep %s\n--- exit %d\n--- stdout\n%s--- stderr\n%s", strings.Join(args, " "), r.code, r.stdout, r.stderr)
	return r
}

// useCreds makes the next runs use the impersonated ADC of a test service
// account (<sa-viewer>, <sa-restringida>, <sa-sin-rol>).
func useCreds(t *testing.T, name string) {
	t.Helper()
	path := filepath.Join(credsDir, name+".json")
	if credsDir == "" {
		t.Skipf("GCSGREP_TEST_CREDS is not set (needs %s.json)", name)
	}
	if _, err := os.Stat(path); err != nil {
		t.Skipf("missing credentials %s: %v", path, err)
	}
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", path)
}

// useUserADC makes the next runs use the invoking user's own ADC.
func useUserADC(t *testing.T) {
	t.Helper()
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
}

func expect(t *testing.T, r result, code int, stdout, stderr string) {
	t.Helper()
	if r.code != code {
		t.Errorf("exit code = %d, want %d", r.code, code)
	}
	if r.stdout != stdout {
		t.Errorf("stdout = %q, want %q", r.stdout, stdout)
	}
	if r.stderr != stderr {
		t.Errorf("stderr = %q, want %q", r.stderr, stderr)
	}
}

// expectSingleLinePrefix checks that s is exactly one line starting with prefix.
func expectSingleLinePrefix(t *testing.T, name, s, prefix string) {
	t.Helper()
	if !strings.HasPrefix(s, prefix) || strings.Count(s, "\n") != 1 {
		t.Errorf("%s = %q, want exactly one line starting with %q", name, s, prefix)
	}
}

func lines(format string, from, to int) string {
	var b strings.Builder
	for i := from; i <= to; i++ {
		fmt.Fprintf(&b, format+"\n", i)
	}
	return b.String()
}

// countObjects counts every object in the bucket with gcloud, independently
// of gcsgrep's own listing code.
func countObjects(t *testing.T) int {
	t.Helper()
	gcloud, err := exec.LookPath("gcloud")
	if err != nil {
		t.Skip("gcloud is not in PATH")
	}
	out, err := exec.Command(gcloud, "storage", "ls", "gs://"+bucket+"/**").Output()
	if err != nil {
		t.Fatalf("gcloud storage ls: %v", err)
	}
	n := 0
	for _, l := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(l) != "" {
			n++
		}
	}
	return n
}

func TestVC1_1_Search(t *testing.T) {
	expect(t, run(t, "timeout", loc("logs/")), 0, "logs/a.log:2:ERROR timeout\n", "")
}

func TestVC1_3_PatternIsRE2(t *testing.T) {
	expect(t, run(t, `version 1\.[0-9]`, loc("v/")), 0, "v/a.log:1:version 1.2\n", "")
}

func TestVC1_4_InvalidPattern(t *testing.T) {
	r := run(t, "(", loc("logs/"))
	if r.code != 2 || r.stdout != "" {
		t.Errorf("exit %d, stdout %q; want 2 and empty", r.code, r.stdout)
	}
	expectSingleLinePrefix(t, "stderr", r.stderr, `gcsgrep: error: invalid pattern "(": `)
}

func TestVC2_2_WholeBucketListing(t *testing.T) {
	if bucket == "" {
		t.Skip("GCSGREP_TEST_BUCKET is not set")
	}
	n := countObjects(t)
	t.Logf("gcloud storage ls gs://%s/** -> %d objects", bucket, n)
	expect(t, run(t, "--max", "1", "timeout", "gs://"+bucket+"/"), 2, "",
		fmt.Sprintf("gcsgrep: error: the prefix has %d objects, which exceeds the limit of 1 (use --max to raise it, or --max 0 to disable it)\n", n))
}

func TestVC3_1_OutputFormat(t *testing.T) {
	r := run(t, "timeout", loc("logs/"))
	if r.stdout != "logs/a.log:2:ERROR timeout\n" {
		t.Errorf("stdout = %q", r.stdout)
	}
}

func TestVC3_3_NoANSIWhenRedirected(t *testing.T) {
	r := run(t, "timeout", loc("logs/"))
	if strings.Contains(r.stdout, "\x1b") {
		t.Errorf("stdout contains ESC (0x1b): %q", r.stdout)
	}
}

func TestVC4_1_IgnoreCase(t *testing.T) {
	expect(t, run(t, "-i", "timeout", loc("case/")), 0, "case/a.log:1:TIMEOUT error\n", "")
}

func TestVC4_2_CaseSensitiveByDefault(t *testing.T) {
	expect(t, run(t, "timeout", loc("case/")), 1, "", "")
}

func TestVC8_1_ExitMatch(t *testing.T) {
	if r := run(t, "timeout", loc("logs/")); r.code != 0 {
		t.Errorf("exit code = %d, want 0", r.code)
	}
}

func TestVC8_2_ExitNoMatch(t *testing.T) {
	expect(t, run(t, "patron_inexistente_xyz", loc("logs/")), 1, "", "")
}

// VC-8.3 and VC-9.1 are the same run.
func TestVC8_3_And_VC9_1_UnreadableObject(t *testing.T) {
	useCreds(t, "restringida")
	expect(t, run(t, "timeout", loc("acl/")), 2,
		lines("acl/%d.log:1:timeout", 1, 5),
		"gcsgrep: warning: acl/denied.log: permission denied\n")
}

func TestVC9_4_OtherPermanentError(t *testing.T) {
	r := run(t, "timeout", loc("x/"))
	if r.code != 2 || r.stdout != "x/ok.log:1:timeout\n" {
		t.Errorf("exit %d, stdout %q; want 2 and %q", r.code, r.stdout, "x/ok.log:1:timeout\n")
	}
	expectSingleLinePrefix(t, "stderr", r.stderr, "gcsgrep: warning: x/csek.log: read failed: ")
}

func TestVC11_BinarySkipped(t *testing.T) {
	expect(t, run(t, "timeout", loc("bin/")), 0, "bin/a.log:1:timeout\n",
		"gcsgrep: warning: bin/icon.png: skipped (binary object)\n")
}

func TestVC15_1_ReadOnlyCredentials(t *testing.T) {
	cases := [][]string{
		{"timeout", loc("logs/")},
		{"patron_inexistente_xyz", loc("logs/")},
		{"timeout", loc("bin/")},
	}
	for _, args := range cases {
		useUserADC(t)
		user := run(t, args...)
		useCreds(t, "viewer")
		viewer := run(t, args...)
		if user.stdout != viewer.stdout || user.code != viewer.code {
			t.Errorf("gcsgrep %v: user (exit %d, %q) vs <sa-viewer> (exit %d, %q)",
				args, user.code, user.stdout, viewer.code, viewer.stdout)
		}
	}
}

func TestVC16_NoAccessAmplification(t *testing.T) {
	useCreds(t, "restringida")
	expect(t, run(t, "secreto", loc("acl/")), 2, "",
		"gcsgrep: warning: acl/denied.log: permission denied\n")
}

func TestVC17_1_ObjectCountGuardrail(t *testing.T) {
	expect(t, run(t, "--max", "5", "timeout", loc("max/")), 2, "",
		"gcsgrep: error: the prefix has 10 objects, which exceeds the limit of 5 (use --max to raise it, or --max 0 to disable it)\n")
}

func TestVC17_2_GuardrailRaised(t *testing.T) {
	expect(t, run(t, "--max", "20", "timeout", loc("max/")), 0, lines("max/%02d.log:1:timeout", 0, 9), "")
}

func TestVC17_3_GuardrailDisabled(t *testing.T) {
	expect(t, run(t, "--max", "0", "timeout", loc("max/")), 0, lines("max/%02d.log:1:timeout", 0, 9), "")
}

func TestVC24_LongLinesSkipped(t *testing.T) {
	expect(t, run(t, "timeout", loc("long/")), 0,
		"long/x.log:1:timeout antes\nlong/x.log:3:timeout despues\n",
		"gcsgrep: warning: long/x.log: skipped lines longer than 1 MiB\n")
}

func TestVC25_1_LocationWithoutScheme(t *testing.T) {
	expect(t, run(t, "timeout", "mybucket/logs/"), 2, "",
		"gcsgrep: error: invalid location \"mybucket/logs/\": must start with gs://\n")
}

func TestVC25_2_NoCredentials(t *testing.T) {
	empty := t.TempDir()
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
	t.Setenv("HOME", empty)
	t.Setenv("APPDATA", empty)
	t.Setenv("CLOUDSDK_CONFIG", empty)
	r := run(t, "timeout", loc("logs/"))
	if r.code != 2 || r.stdout != "" {
		t.Errorf("exit %d, stdout %q; want 2 and empty", r.code, r.stdout)
	}
	expectSingleLinePrefix(t, "stderr", r.stderr, "gcsgrep: error: no Application Default Credentials found")
}

func TestVC25_3_ListingDenied(t *testing.T) {
	useCreds(t, "sin-rol")
	expect(t, run(t, "timeout", loc("logs/")), 2, "",
		fmt.Sprintf("gcsgrep: error: permission denied listing gs://%s/logs/\n", bucket))
}

func TestVC25_4_BucketDoesNotExist(t *testing.T) {
	expect(t, run(t, "timeout", "gs://gcsgrep-bucket-inexistente-xyz/"), 2, "",
		"gcsgrep: error: bucket gcsgrep-bucket-inexistente-xyz does not exist\n")
}

func TestVC26_LocationWithoutObjects(t *testing.T) {
	expect(t, run(t, "timeout", loc("prefijo-sin-objetos/")), 1, "",
		fmt.Sprintf("gcsgrep: warning: no objects under gs://%s/prefijo-sin-objetos/\n", bucket))
}

func TestVC27_PrefixWithoutTrailingSlash(t *testing.T) {
	expect(t, run(t, "timeout", loc("pfx/logs")), 0,
		"pfx/logs-old/b.log:1:timeout\npfx/logs/a.log:1:timeout\n", "")
}

func TestVC28_1_SequentialOrder(t *testing.T) {
	expect(t, run(t, "match", loc("ord/")), 0,
		"ord/a.log:1:match 1\nord/a.log:2:match 2\n"+
			"ord/b.log:1:match 1\nord/b.log:2:match 2\n"+
			"ord/c.log:1:match 1\nord/c.log:2:match 2\n", "")
}

func TestVC29_NFlagHasNoEffect(t *testing.T) {
	without := run(t, "timeout", loc("logs/"))
	with := run(t, "-n", "timeout", loc("logs/"))
	if with != without || with.code != 0 {
		t.Errorf("-n changed the result: %+v vs %+v", with, without)
	}
}

func TestVC30_1_EmptyObject(t *testing.T) {
	expect(t, run(t, "timeout", loc("e/")), 0, "e/a.log:1:timeout\n", "")
}

func TestVC30_2_LastLineWithoutNewline(t *testing.T) {
	expect(t, run(t, "timeout", loc("n/")), 0, "n/last.log:2:dos timeout\n", "")
}

// openCounter wraps the real client and records which objects are opened
// (VC-19 asserts the exact number of openings).
type openCounter struct {
	gcsclient.Client
	opened []string
}

func (c *openCounter) Open(ctx context.Context, bucket, object string) (io.ReadCloser, error) {
	c.opened = append(c.opened, object)
	return c.Client.Open(ctx, bucket, object)
}

// runCounting is run with a client that counts object openings.
func runCounting(t *testing.T, args ...string) (result, *openCounter) {
	t.Helper()
	if bucket == "" {
		t.Skip("GCSGREP_TEST_BUCKET is not set")
	}
	counter := &openCounter{}
	factory := func(ctx context.Context) (gcsclient.Client, error) {
		real, err := gcsclient.New(ctx)
		counter.Client = real
		return counter, err
	}
	var stdout, stderr bytes.Buffer
	code := app.Run(context.Background(), args, &stdout, &stderr, factory)
	r := result{code, stdout.String(), stderr.String()}
	t.Logf("gcsgrep %s\n--- exit %d\n--- stdout\n%s--- stderr\n%s--- opened %v", strings.Join(args, " "), r.code, r.stdout, r.stderr, counter.opened)
	return r, counter
}

// expectStderrLine checks that stderr contains line as one of its lines.
func expectStderrLine(t *testing.T, r result, line string) {
	t.Helper()
	for _, l := range strings.Split(r.stderr, "\n") {
		if l == line {
			return
		}
	}
	t.Errorf("stderr = %q, want it to contain the line %q", r.stderr, line)
}

func TestVC9_2_CorruptGzip(t *testing.T) {
	r := run(t, "timeout", loc("gzbad/"))
	if r.code != 2 || r.stdout != "gzbad/ok.log:1:timeout\n" {
		t.Errorf("exit %d, stdout %q; want 2 and the match of ok.log", r.code, r.stdout)
	}
	expectStderrLine(t, r, "gcsgrep: warning: gzbad/bad.gz: corrupt gzip data")
}

func TestVC12_1_GzipDecompressed(t *testing.T) {
	expect(t, run(t, "timeout", loc("gz/")), 0, "gz/app.log.gz:2:ERROR timeout\n", "")
}

func TestVC12_2_BinaryGzipSkipped(t *testing.T) {
	r := run(t, "timeout", loc("gzb/"))
	if r.code != 1 || r.stdout != "" {
		t.Errorf("exit %d, stdout %q; want 1 and empty", r.code, r.stdout)
	}
	expectStderrLine(t, r, "gcsgrep: warning: gzb/icon.png.gz: skipped (binary object)")
}

func TestVC18_ObjectSizeGuardrail(t *testing.T) {
	r := run(t, "--max-object-size", "1048576", "timeout", loc("big/"))
	if r.code != 2 || r.stdout != "big/ok.log:1:timeout\n" {
		t.Errorf("exit %d, stdout %q; want 2 and the match of ok.log", r.code, r.stdout)
	}
	expectStderrLine(t, r, "gcsgrep: warning: big/app.log.gz: object size limit of 1048576 bytes reached, rest of the object not read")
}

func TestVC19_TotalSizeGuardrail(t *testing.T) {
	r, counter := runCounting(t, "--max-total-size", "2621440", "timeout", loc("tot/"))
	if r.code != 2 || r.stdout != "tot/1.log:1:timeout\ntot/2.log:1:timeout\ntot/3.log:1:timeout\n" {
		t.Errorf("exit %d, stdout %q; want 2 and the matches of tot/1..3", r.code, r.stdout)
	}
	if len(counter.opened) != 3 {
		t.Errorf("openings = %v, want exactly 3", counter.opened)
	}
	expectStderrLine(t, r, "gcsgrep: error: total size limit of 2621440 bytes reached, scan incomplete")
}
