package reader

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"strings"
	"testing"

	"gcsgrep/internal/match"
)

func mustMatcher(t *testing.T, pattern string, ignoreCase bool) *match.Matcher {
	t.Helper()
	m, err := match.New(pattern, ignoreCase)
	if err != nil {
		t.Fatalf("match.New(%q): %v", pattern, err)
	}
	return m
}

// VC-1 (unit-level slice): basic line-by-line matching over a normal
// object, including a final line with no trailing newline.
func TestProcessObject_BasicMatching(t *testing.T) {
	content := "service started\nconnection timeout after 30s\nrequest handled"
	m := mustMatcher(t, "timeout", false)

	res := ProcessObject(strings.NewReader(content), "logs/app1.log", m, Options{})

	if res.Failed || res.Skipped {
		t.Fatalf("should not fail or be skipped: %+v", res)
	}
	if len(res.Matches) != 1 {
		t.Fatalf("expected 1 match, got %d: %+v", len(res.Matches), res.Matches)
	}
	if res.Matches[0].LineNum != 2 {
		t.Errorf("expected the match on line 2, got line %d", res.Matches[0].LineNum)
	}
	if res.Matches[0].Text != "connection timeout after 30s" {
		t.Errorf("unexpected match text: %q", res.Matches[0].Text)
	}
}

func TestProcessObject_NoMatch(t *testing.T) {
	content := "all good\nnothing to see here\n"
	m := mustMatcher(t, "timeout", false)

	res := ProcessObject(strings.NewReader(content), "logs/app2.log", m, Options{})

	if res.Failed || res.Skipped {
		t.Fatalf("should not fail or be skipped: %+v", res)
	}
	if len(res.Matches) != 0 {
		t.Errorf("expected no matches, got %+v", res.Matches)
	}
}

func TestProcessObject_EmptyObject(t *testing.T) {
	m := mustMatcher(t, "timeout", false)
	res := ProcessObject(strings.NewReader(""), "logs/empty.log", m, Options{})
	if res.Failed || res.Skipped || len(res.Matches) != 0 {
		t.Errorf("an empty object should process with no matches and no errors: %+v", res)
	}
}

// VC-11: an object with a null byte in its first 8 KiB is skipped whole,
// never matched against.
func TestProcessObject_SkipsBinary(t *testing.T) {
	binaryContent := "\x89PNG\r\n\x1a\n\x00\x00\x00timeout-looking-but-binary"
	m := mustMatcher(t, "timeout", false)

	res := ProcessObject(strings.NewReader(binaryContent), "logs/icon.png", m, Options{})

	if !res.Skipped {
		t.Fatalf("expected the object to be skipped as binary: %+v", res)
	}
	if res.Failed {
		t.Errorf("a skipped binary is not an error (FR-8 should not count it as one)")
	}
	if len(res.Matches) != 0 {
		t.Errorf("a skipped binary object should not report matches: %+v", res.Matches)
	}
}

func TestProcessObject_TextWithNoNullBytesIsNotBinary(t *testing.T) {
	content := "perfectly normal text\nwith a timeout in it\n"
	m := mustMatcher(t, "timeout", false)

	res := ProcessObject(strings.NewReader(content), "logs/app.log", m, Options{})

	if res.Skipped {
		t.Fatalf("normal text should not be skipped as binary: %+v", res)
	}
	if len(res.Matches) != 1 {
		t.Errorf("expected 1 match, got %+v", res.Matches)
	}
}

// VC-24: a line longer than MaxLineSize is skipped whole — not truncated
// and matched partially — even if it contains a real match past the cut
// point. This is the false-negative-by-design behavior: the match is
// correctly NOT reported, and exactly one warning fires for the object.
func TestProcessObject_LongLineSkippedNotTruncated(t *testing.T) {
	maxLineSize := 1024
	// A line far longer than maxLineSize, with "timeout" placed well past
	// the cut point so a naive truncate-then-match would miss it anyway —
	// what we're actually verifying is that gcsgrep does NOT attempt to
	// match the truncated prefix at all, and does not crash or hang.
	longLine := strings.Repeat("x", maxLineSize*3) + "timeout" + strings.Repeat("y", 100)
	content := "short line before\n" + longLine + "\nshort line after with timeout\n"

	m := mustMatcher(t, "timeout", false)
	res := ProcessObject(strings.NewReader(content), "logs/biglines.log", m, Options{MaxLineSize: maxLineSize})

	if res.Failed || res.Skipped {
		t.Fatalf("should not fail or skip the whole object: %+v", res)
	}
	if !res.LongLineWarn {
		t.Errorf("expected LongLineWarn=true for the long line")
	}
	if len(res.Matches) != 1 {
		t.Fatalf("expected exactly 1 match (the final short line), got %+v", res.Matches)
	}
	if res.Matches[0].LineNum != 3 {
		t.Errorf("expected the match on line 3 (after the skipped long line), got line %d", res.Matches[0].LineNum)
	}
}

func TestProcessObject_LongLineAtEndOfFileNoTrailingNewline(t *testing.T) {
	maxLineSize := 16
	content := "ok\n" + strings.Repeat("z", maxLineSize*2) // no trailing \n
	m := mustMatcher(t, "z+", false)

	res := ProcessObject(strings.NewReader(content), "logs/tail.log", m, Options{MaxLineSize: maxLineSize})

	if res.Failed || res.Skipped {
		t.Fatalf("should not fail: %+v", res)
	}
	if !res.LongLineWarn {
		t.Errorf("expected LongLineWarn=true for the trailing long line with no newline")
	}
	if len(res.Matches) != 0 {
		t.Errorf("the trailing long line must not match, not even partially: %+v", res.Matches)
	}
}

func TestProcessObject_LineExactlyAtLimitIsNotTruncated(t *testing.T) {
	maxLineSize := 10
	line := strings.Repeat("a", maxLineSize) // exactly at the limit, not over it
	content := line + "\nok\n"               // second line, short and well under the limit
	m := mustMatcher(t, "^a+$", false)       // only matches the full line of "a"s

	res := ProcessObject(strings.NewReader(content), "logs/exact.log", m, Options{MaxLineSize: maxLineSize})

	if res.LongLineWarn {
		t.Errorf("a line of exactly MaxLineSize should not be considered long")
	}
	if len(res.Matches) != 1 || res.Matches[0].LineNum != 1 || res.Matches[0].Text != line {
		t.Errorf("expected the line at the limit to match normally and in full: %+v", res.Matches)
	}
}

// countingReader records how many bytes were pulled from the underlying
// reader, so VC-5 can assert that -l stops reading early.
type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

// VC-5: with -l, a large object whose first line matches is read only up
// to that match (plus whatever the read buffers already pulled in), not
// in full.
func TestProcessObject_ListModeStopsAtFirstMatch(t *testing.T) {
	const objectSize = 10 << 20
	content := "connection timeout after 30s\n" + strings.Repeat("filler line with no match\n", objectSize/26)
	stream := &countingReader{r: strings.NewReader(content)}
	m := mustMatcher(t, "timeout", false)

	res := ProcessObject(stream, "logs/huge.log", m, Options{Mode: ModeList})

	if res.Failed || res.Skipped {
		t.Fatalf("should not fail or be skipped: %+v", res)
	}
	if res.MatchCount != 1 {
		t.Errorf("MatchCount = %d, want 1", res.MatchCount)
	}
	if len(res.Matches) != 0 {
		t.Errorf("-l should not collect line text: %+v", res.Matches)
	}
	// The binary sniff (8 KiB) plus one bufio chunk (64 KiB) is the most
	// that can be read before the first line is matched.
	if limit := int64(sniffSize + chunkSize); stream.n > limit {
		t.Errorf("read %d bytes of a %d-byte object, want at most %d (early cut)", stream.n, len(content), limit)
	}
}

// VC-6: with -c, every matching line is counted and none of their text is
// kept.
func TestProcessObject_CountModeCountsEveryMatch(t *testing.T) {
	content := "timeout 1\nok\ntimeout 2\nok\ntimeout 3"
	m := mustMatcher(t, "timeout", false)

	res := ProcessObject(strings.NewReader(content), "logs/app.log", m, Options{Mode: ModeCount})

	if res.MatchCount != 3 {
		t.Errorf("MatchCount = %d, want 3", res.MatchCount)
	}
	if len(res.Matches) != 0 {
		t.Errorf("-c should not collect line text: %+v", res.Matches)
	}
}

// failingReader returns its content and then a non-EOF error, simulating
// a connection that drops mid-object.
type failingReader struct {
	r   io.Reader
	err error
}

func (f *failingReader) Read(p []byte) (int, error) {
	n, err := f.r.Read(p)
	if err == io.EOF {
		return n, f.err
	}
	return n, err
}

// FR-9 / NFR-3 (d): an error in the middle of an object must mark it as
// failed, not be mistaken for a normal end of object. Matches found before
// the error are kept; the partial line cut by the error is never matched.
func TestProcessObject_MidReadErrorFailsObjectAndKeepsEarlierMatches(t *testing.T) {
	stream := &failingReader{
		// Longer than the 8 KiB binary sniff, so the error hits the
		// line-by-line read and not the sniff.
		r:   strings.NewReader("timeout before the drop\n" + strings.Repeat("filler\n", 2000) + "partial timeout li"),
		err: errors.New("connection reset by peer"),
	}
	m := mustMatcher(t, "timeout", false)

	res := ProcessObject(stream, "logs/app.log", m, Options{})

	if !res.Failed {
		t.Fatalf("a mid-read error must mark the object as failed: %+v", res)
	}
	if len(res.Matches) != 1 || res.Matches[0].LineNum != 1 {
		t.Errorf("expected only the complete line before the error to match: %+v", res.Matches)
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

// VC-12 (unit-level slice): gzip content is decompressed and searched,
// whatever the object is called — detection goes by the 1f 8b magic
// bytes, not the .gz extension.
func TestProcessObject_GzipIsDetectedByContentNotName(t *testing.T) {
	compressed := gzipped(t, "service started\nconnection timeout after 30s\n")
	m := mustMatcher(t, "timeout", false)

	for _, name := range []string{"logs/app.log.gz", "logs/app-without-extension"} {
		res := ProcessObject(strings.NewReader(compressed), name, m, Options{})

		if res.Skipped || res.Incomplete() {
			t.Fatalf("%s: gzip content should be decompressed and searched: %+v", name, res)
		}
		if len(res.Matches) != 1 || res.Matches[0].LineNum != 2 || res.Matches[0].Text != "connection timeout after 30s" {
			t.Errorf("%s: expected the decompressed line 2 to match: %+v", name, res.Matches)
		}
	}
}

// A .gz name on plain text (e.g. GCS already decompressed it because of
// Content-Encoding: gzip) must be searched as text, not fail as bad gzip.
func TestProcessObject_PlainTextNamedGzIsSearchedAsText(t *testing.T) {
	m := mustMatcher(t, "timeout", false)

	res := ProcessObject(strings.NewReader("connection timeout\n"), "logs/app.log.gz", m, Options{})

	if res.Incomplete() || res.Skipped || len(res.Matches) != 1 {
		t.Errorf("plain text named .gz should be searched as text: %+v", res)
	}
}

// FR-11 over FR-12: binary detection looks at the decompressed content.
// Compressed bytes always contain nulls, so gzipped text must not be
// skipped, and gzipped binary must.
func TestProcessObject_BinaryDetectionRunsOnDecompressedContent(t *testing.T) {
	m := mustMatcher(t, "timeout", false)

	text := ProcessObject(strings.NewReader(gzipped(t, "timeout\n")), "a.gz", m, Options{})
	if text.Skipped {
		t.Errorf("gzipped text should not be skipped as binary: %+v", text)
	}

	binary := ProcessObject(strings.NewReader(gzipped(t, "\x89PNG\x00\x00timeout")), "b.gz", m, Options{})
	if !binary.Skipped {
		t.Errorf("gzipped binary should be skipped as binary: %+v", binary)
	}
}

func TestProcessObject_CorruptGzipFails(t *testing.T) {
	m := mustMatcher(t, "timeout", false)
	corrupt := gzipped(t, strings.Repeat("timeout\n", 1000))[:40] // truncated stream

	res := ProcessObject(strings.NewReader(corrupt), "logs/broken.gz", m, Options{})

	if !res.Failed {
		t.Errorf("a truncated gzip stream should fail the object: %+v", res)
	}
}

// VC-18 (b), unit-level: a small gzip that expands past MaxObjectSize is
// cut at the limit; matches before the cut are kept.
func TestProcessObject_ObjectSizeLimitCutsExpandingGzip(t *testing.T) {
	const limit = 64 << 10
	content := "timeout at the start\n" + strings.Repeat("filler line\n", 1<<20/12)
	compressed := gzipped(t, content)
	if len(compressed) >= limit {
		t.Fatalf("test setup: compressed size %d should be under the limit %d", len(compressed), limit)
	}
	m := mustMatcher(t, "timeout", false)

	res := ProcessObject(strings.NewReader(compressed), "logs/bomb.gz", m, Options{MaxObjectSize: limit})

	if !res.ObjectSizeLimitHit || res.Failed {
		t.Fatalf("expected the per-object limit to be hit: %+v", res)
	}
	if len(res.Matches) != 1 {
		t.Errorf("the match before the cut should be kept: %+v", res.Matches)
	}
}

// An object exactly at the limit is read in full: only content past the
// limit counts as crossing it.
func TestProcessObject_ObjectExactlyAtSizeLimitIsNotCut(t *testing.T) {
	content := strings.Repeat("timeout\n", 100)
	m := mustMatcher(t, "timeout", false)

	res := ProcessObject(strings.NewReader(content), "logs/exact.log", m, Options{MaxObjectSize: int64(len(content))})

	if res.Incomplete() || res.MatchCount != 100 {
		t.Errorf("an object exactly at the limit should be read in full: %+v", res)
	}
}

// BR-5, unit-level: the Budget is shared across objects, so the second
// object is cut once the first one has used most of it.
func TestProcessObject_TotalBudgetIsSharedAcrossObjects(t *testing.T) {
	content := strings.Repeat("timeout\n", 1000) // 8000 bytes
	budget := NewBudget(12000)
	m := mustMatcher(t, "timeout", false)

	first := ProcessObject(strings.NewReader(content), "a.log", m, Options{Budget: budget})
	second := ProcessObject(strings.NewReader(content), "b.log", m, Options{Budget: budget})

	if first.Incomplete() || first.MatchCount != 1000 {
		t.Errorf("the first object fits the budget and should be read in full: %+v", first.MatchCount)
	}
	if !second.TotalSizeLimitHit {
		t.Errorf("the second object should hit the run-wide limit: %+v", second)
	}
	if second.MatchCount != 500 {
		t.Errorf("the second object should have matched exactly the 500 complete lines within the budget, got %d", second.MatchCount)
	}
	if !budget.Exhausted() {
		t.Errorf("the budget should be exhausted")
	}
}
