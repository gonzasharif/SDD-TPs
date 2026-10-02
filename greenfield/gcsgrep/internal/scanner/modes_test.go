package scanner

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"gcsgrep/internal/gcsclient"
	"gcsgrep/internal/gcsclient/gcsclienttest"
)

func withMode(prefix string, mode Mode) Config {
	c := cfg(prefix)
	c.Mode = mode
	return c
}

// countingStream counts the bytes handed out by a stream.
type countingStream struct {
	io.Reader
	n int
}

func (c *countingStream) Read(p []byte) (int, error) {
	n, err := c.Reader.Read(p)
	c.n += n
	return n, err
}

// streamClient lists through fake but serves every object from stream, so a
// test can see how many bytes were read from it.
type streamClient struct {
	fake   *gcsclienttest.Fake
	stream io.Reader
}

func (c *streamClient) List(ctx context.Context, bucket, prefix string) ([]gcsclient.ObjectInfo, error) {
	return c.fake.List(ctx, bucket, prefix)
}

func (c *streamClient) Open(context.Context, string, string) (io.ReadCloser, error) {
	return io.NopCloser(c.stream), nil
}

// FR-5: -l prints each object name once, however many lines match, and
// leaves out the objects without a match.
func TestRun_FilesWithMatches(t *testing.T) {
	client := &gcsclienttest.Fake{Objects: []gcsclienttest.Object{
		{Name: "l/a.log", Content: "timeout 1\ntimeout 2\nINFO\n"},
		{Name: "l/b.log", Content: "INFO ok\n"},
		{Name: "l/c.log", Content: "INFO\ntimeout\n"},
	}}
	expect(t, run(t, client, withMode("l/", ModeFilesWithMatches), "timeout"), ExitMatch, "l/a.log\nl/c.log\n", "")
}

// FR-5 / VC-5: reading stops at the first match, long before the end of a
// large object.
func TestRun_FilesWithMatches_StopsReading(t *testing.T) {
	content := "timeout\n" + strings.Repeat("INFO ok\n", 100<<20/8)
	stream := &countingStream{Reader: strings.NewReader(content)}
	client := &streamClient{fake: &gcsclienttest.Fake{Objects: []gcsclienttest.Object{{Name: "l/big.log", Content: content}}}, stream: stream}

	expect(t, run(t, client, withMode("l/", ModeFilesWithMatches), "timeout"), ExitMatch, "l/big.log\n", "")
	if stream.n > 1<<20 {
		t.Errorf("read %d bytes of the object, want <= 1 MiB (1048576)", stream.n)
	}
	if stream.n == 0 {
		t.Errorf("the stream was never read")
	}
}

// FR-5: -l on a run without matches prints nothing and exits 1.
func TestRun_FilesWithMatches_NoMatches(t *testing.T) {
	client := &gcsclienttest.Fake{Objects: []gcsclienttest.Object{{Name: "l/a.log", Content: "INFO\n"}}}
	expect(t, run(t, client, withMode("l/", ModeFilesWithMatches), "timeout"), ExitNoMatch, "", "")
}

// FR-6 / VC-6: -c prints object:count for every object read completely,
// including 0.
func TestRun_Count(t *testing.T) {
	client := &gcsclienttest.Fake{Objects: []gcsclienttest.Object{
		{Name: "c/none.log", Content: "INFO ok\n"},
		{Name: "c/three.log", Content: "timeout 1\nINFO\ntimeout 2\nINFO\ntimeout 3\n"},
	}}
	expect(t, run(t, client, withMode("c/", ModeCount), "timeout"), ExitMatch, "c/none.log:0\nc/three.log:3\n", "")
}

// FR-6: counts are lines, not occurrences; a last line without \n counts.
func TestRun_Count_CountsLines(t *testing.T) {
	client := &gcsclienttest.Fake{Objects: []gcsclienttest.Object{
		{Name: "c/a.log", Content: "timeout timeout\nINFO\ntimeout"},
	}}
	expect(t, run(t, client, withMode("c/", ModeCount), "timeout"), ExitMatch, "c/a.log:2\n", "")
}

// FR-8.2: -c with every count at 0 is a run without matches.
func TestRun_Count_AllZeroExitsOne(t *testing.T) {
	client := &gcsclienttest.Fake{Objects: []gcsclienttest.Object{{Name: "c/none.log", Content: "INFO\n"}}}
	expect(t, run(t, client, withMode("c/", ModeCount), "timeout"), ExitNoMatch, "c/none.log:0\n", "")
}

// FR-6: an objeto salteado, fallido or cortado prints no count line.
func TestRun_Count_NoLineForObjectsNotReadCompletely(t *testing.T) {
	client := &gcsclienttest.Fake{Objects: []gcsclienttest.Object{
		{Name: "c/1-bin.png", Content: "\x89PNG\x00\x00 timeout"},
		{Name: "c/2-gone.log", OpenErr: errors.New("boom")},
		{Name: "c/3-broken.log", Content: "timeout\n", ReadErr: errors.New("connection reset")},
		{Name: "c/4-bad.gz", Content: "no es gzip\n"},
		{Name: "c/5-ok.log", Content: "timeout\n"},
	}}

	got := run(t, client, withMode("c/", ModeCount), "timeout")

	if got.code != ExitError || got.stdout != "c/5-ok.log:1\n" {
		t.Errorf("code = %d, stdout = %q; want 2 and only the count of c/5-ok.log", got.code, got.stdout)
	}
}

// FR-6 / BR-4: the objeto cortado prints no count either.
func TestRun_Count_NoLineForCutObject(t *testing.T) {
	client := &gcsclienttest.Fake{Objects: []gcsclienttest.Object{
		{Name: "big/a.log", Content: "timeout\n" + strings.Repeat("INFO\n", 2*miB/5)},
		{Name: "big/ok.log", Content: "timeout\n"},
	}}
	c := withMode("big/", ModeCount)
	c.MaxObjectSize = miB

	expect(t, run(t, client, c, "timeout"), ExitError, "big/ok.log:1\n",
		"gcsgrep: warning: big/a.log: object size limit of 1048576 bytes reached, rest of the object not read\n")
}

// FR-6 / BR-5: the object in progress when the total limit is reached is
// not complete, so it prints no count.
func TestRun_Count_NoLineWhenTotalLimitCutsTheObject(t *testing.T) {
	fake := totData()
	c := withMode("tot/", ModeCount)
	c.MaxTotalSize = 2621440

	expect(t, run(t, fake, c, "timeout"), ExitError, "tot/1.log:1\ntot/2.log:1\n",
		"gcsgrep: error: total size limit of 2621440 bytes reached, scan incomplete\n")
}
