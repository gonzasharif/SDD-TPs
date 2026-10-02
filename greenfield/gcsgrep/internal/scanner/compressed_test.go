package scanner

import (
	"bytes"
	"compress/gzip"
	"strings"
	"testing"

	"gcsgrep/internal/gcsclient/gcsclienttest"
)

const miB = 1 << 20

func gzipped(t *testing.T, content string) string {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// VC-12.1: a .gz object is searched on its decompressed content and reported
// under its original name.
func TestRun_Gzip_MatchesDecompressedContent(t *testing.T) {
	client := &gcsclienttest.Fake{Objects: []gcsclienttest.Object{
		{Name: "gz/app.log.gz", Content: gzipped(t, "INFO start\nERROR timeout\n")},
	}}
	expect(t, run(t, client, cfg("gz/"), "timeout"), ExitMatch, "gz/app.log.gz:2:ERROR timeout\n", "")
}

// VC-12.2: a binary inside a .gz is a skipped object, not an error.
func TestRun_Gzip_BinaryContentIsSkipped(t *testing.T) {
	png := "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"
	client := &gcsclienttest.Fake{Objects: []gcsclienttest.Object{
		{Name: "gzb/icon.png.gz", Content: gzipped(t, png)},
	}}
	expect(t, run(t, client, cfg("gzb/"), "timeout"), ExitNoMatch, "",
		"gcsgrep: warning: gzb/icon.png.gz: skipped (binary object)\n")
}

// VC-9.2: a corrupt .gz is an objeto fallido; the rest of the run goes on.
func TestRun_Gzip_CorruptObjectDoesNotStopTheRun(t *testing.T) {
	client := &gcsclienttest.Fake{Objects: []gcsclienttest.Object{
		{Name: "gzbad/bad.gz", Content: "esto no es gzip\n"},
		{Name: "gzbad/ok.log", Content: "timeout\n"},
	}}
	expect(t, run(t, client, cfg("gzbad/"), "timeout"), ExitError, "gzbad/ok.log:1:timeout\n",
		"gcsgrep: warning: gzbad/bad.gz: corrupt gzip data\n")
}

// Only names ending in .gz are decompressed (FR-12).
func TestRun_Gzip_OnlyByNameSuffix(t *testing.T) {
	client := &gcsclienttest.Fake{Objects: []gcsclienttest.Object{
		{Name: "x/notgz.log", Content: gzipped(t, "timeout\n")},
	}}
	// Compressed bytes are not text: the object is not decompressed, so the
	// raw gzip bytes are what gets searched (and they contain a NUL, so it
	// is a binary object).
	got := run(t, client, cfg("x/"), "timeout")
	if got.code != ExitNoMatch || got.stdout != "" {
		t.Errorf("code = %d, stdout = %q; want no match without decompressing", got.code, got.stdout)
	}
}

// VC-18: an object that expands past --max-object-size is cut; the rest of
// the run continues and the exit code is 2.
func TestRun_ObjectSizeLimit(t *testing.T) {
	client := &gcsclienttest.Fake{Objects: []gcsclienttest.Object{
		{Name: "big/app.log.gz", Content: gzipped(t, strings.Repeat("INFO\n", 2*miB/5+1))},
		{Name: "big/ok.log", Content: "timeout\n"},
	}}
	c := cfg("big/")
	c.MaxObjectSize = miB

	expect(t, run(t, client, c, "timeout"), ExitError, "big/ok.log:1:timeout\n",
		"gcsgrep: warning: big/app.log.gz: object size limit of 1048576 bytes reached, rest of the object not read\n")
}

// BR-4: matches printed before the cut are kept.
func TestRun_ObjectSizeLimit_KeepsMatchesBeforeTheCut(t *testing.T) {
	client := &gcsclienttest.Fake{Objects: []gcsclienttest.Object{
		{Name: "big/a.log", Content: "timeout\n" + strings.Repeat("INFO\n", 2*miB/5)},
	}}
	c := cfg("big/")
	c.MaxObjectSize = miB

	expect(t, run(t, client, c, "timeout"), ExitError, "big/a.log:1:timeout\n",
		"gcsgrep: warning: big/a.log: object size limit of 1048576 bytes reached, rest of the object not read\n")
}

// `--max-object-size 0` reads the object completely.
func TestRun_ObjectSizeLimit_ZeroDisablesIt(t *testing.T) {
	client := &gcsclienttest.Fake{Objects: []gcsclienttest.Object{
		{Name: "big/a.log", Content: strings.Repeat("INFO\n", 2*miB/5) + "timeout\n"},
	}}
	expect(t, run(t, client, cfg("big/"), "timeout"), ExitMatch, "big/a.log:419431:timeout\n", "")
}

func totData() *gcsclienttest.Fake {
	fake := &gcsclienttest.Fake{}
	for _, name := range []string{"tot/1.log", "tot/2.log", "tot/3.log", "tot/4.log", "tot/5.log"} {
		// 1 MiB exactly: "timeout\n" then 'INFO' lines and padding.
		body := "timeout\n" + strings.Repeat("x", miB-len("timeout\n")-1) + "\n"
		fake.Objects = append(fake.Objects, gcsclienttest.Object{Name: name, Content: body})
	}
	return fake
}

// VC-19: the cumulative limit cuts the object in progress, opens nothing
// else, and ends the run with the literal error and exit 2.
func TestRun_TotalSizeLimit(t *testing.T) {
	fake := totData()
	c := cfg("tot/")
	c.MaxTotalSize = 2621440 // 2.5 MiB

	expect(t, run(t, fake, c, "timeout"), ExitError,
		"tot/1.log:1:timeout\ntot/2.log:1:timeout\ntot/3.log:1:timeout\n",
		"gcsgrep: error: total size limit of 2621440 bytes reached, scan incomplete\n")
	if got := fake.Opened(); len(got) != 3 {
		t.Errorf("opened %v, want exactly 3 objects", got)
	}
}

// `--max-total-size 0` reads every object.
func TestRun_TotalSizeLimit_ZeroDisablesIt(t *testing.T) {
	fake := totData()

	got := run(t, fake, cfg("tot/"), "timeout")

	if got.code != ExitMatch || strings.Count(got.stdout, "\n") != 5 || got.stderr != "" {
		t.Errorf("code = %d, stdout = %q, stderr = %q; want the 5 matches and no error", got.code, got.stdout, got.stderr)
	}
}

// BR-5 counts decompressed bytes.
func TestRun_TotalSizeLimit_CountsDecompressedBytes(t *testing.T) {
	client := &gcsclienttest.Fake{Objects: []gcsclienttest.Object{
		{Name: "g/a.gz", Content: gzipped(t, strings.Repeat("INFO\n", 2*miB/5))},
		{Name: "g/b.log", Content: "timeout\n"},
	}}
	c := cfg("g/")
	c.MaxTotalSize = miB

	expect(t, run(t, client, c, "timeout"), ExitError, "",
		"gcsgrep: error: total size limit of 1048576 bytes reached, scan incomplete\n")
	if got := client.Opened(); len(got) != 1 {
		t.Errorf("opened %v, want only g/a.gz", got)
	}
}
