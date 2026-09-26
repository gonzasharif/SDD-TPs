package reader

import (
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

// process runs ProcessObject collecting every emitted match.
func process(t *testing.T, r io.Reader, pattern string, opts Options) (ObjectResult, []LineMatch) {
	t.Helper()
	var got []LineMatch
	res := ProcessObject(r, mustMatcher(t, pattern, false), opts, func(lm LineMatch) {
		got = append(got, lm)
	})
	if res.MatchCount != len(got) {
		t.Errorf("MatchCount = %d but %d matches were emitted", res.MatchCount, len(got))
	}
	return res, got
}

// VC-1.1 (unit-level slice): basic line-by-line matching.
func TestProcessObject_BasicMatching(t *testing.T) {
	res, got := process(t, strings.NewReader("INFO start\nERROR timeout\n"), "timeout", Options{})

	if res.Failed || res.Skipped {
		t.Fatalf("should not fail or be skipped: %+v", res)
	}
	if len(got) != 1 || got[0].LineNum != 2 || got[0].Text != "ERROR timeout" {
		t.Fatalf("expected exactly {2, \"ERROR timeout\"}, got %+v", got)
	}
}

func TestProcessObject_NoMatch(t *testing.T) {
	res, got := process(t, strings.NewReader("INFO ok\n"), "timeout", Options{})
	if res.Failed || res.Skipped || len(got) != 0 {
		t.Errorf("expected a clean object with no matches: %+v %+v", res, got)
	}
}

// VC-28.1 (unit-level slice): matches are emitted in ascending line order.
func TestProcessObject_EmitsInLineOrder(t *testing.T) {
	_, got := process(t, strings.NewReader("match 1\nINFO\nmatch 2\nmatch 3\n"), "match", Options{})
	want := []int{1, 3, 4}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i, lm := range got {
		if lm.LineNum != want[i] {
			t.Errorf("match %d on line %d, want %d", i, lm.LineNum, want[i])
		}
	}
}

// VC-30.1 / FR-21.1: a 0-byte object has no lines, no matches, and is
// neither skipped nor failed.
func TestProcessObject_EmptyObject(t *testing.T) {
	res, got := process(t, strings.NewReader(""), "timeout", Options{})
	if res.Failed || res.Skipped || res.LongLineWarn || len(got) != 0 {
		t.Errorf("an empty object should process with no matches and no avisos: %+v", res)
	}
}

// VC-30.2 / FR-21.2: the bytes after the last \n are one more line.
func TestProcessObject_LastLineWithoutNewline(t *testing.T) {
	res, got := process(t, strings.NewReader("uno\ndos timeout"), "timeout", Options{})
	if res.Failed || len(got) != 1 || got[0].LineNum != 2 || got[0].Text != "dos timeout" {
		t.Fatalf("expected {2, \"dos timeout\"}, got %+v (res %+v)", got, res)
	}
}

// VC-11: an object with a null byte in its first 8 KiB is skipped whole,
// never matched against.
func TestProcessObject_SkipsBinary(t *testing.T) {
	binaryContent := "\x89PNG\r\n\x1a\n\x00\x00\x00timeout-looking-but-binary"
	res, got := process(t, strings.NewReader(binaryContent), "timeout", Options{})

	if !res.Skipped || res.SkipReason != "binary object" {
		t.Fatalf("expected the object to be skipped as \"binary object\": %+v", res)
	}
	if res.Failed {
		t.Errorf("a skipped binary is not an error (FR-8 should not count it as one)")
	}
	if len(got) != 0 {
		t.Errorf("a skipped binary object should not report matches: %+v", got)
	}
}

func TestProcessObject_TextWithNoNullBytesIsNotBinary(t *testing.T) {
	res, got := process(t, strings.NewReader("perfectly normal text\nwith a timeout in it\n"), "timeout", Options{})
	if res.Skipped {
		t.Fatalf("normal text should not be skipped as binary: %+v", res)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 match, got %+v", got)
	}
}

// VC-24: a line longer than MaxLineSize is skipped whole — not truncated
// and matched partially — even if it contains a real match past the cut
// point.
func TestProcessObject_LongLineSkippedNotTruncated(t *testing.T) {
	maxLineSize := 1024
	longLine := strings.Repeat("x", maxLineSize*3) + "timeout" + strings.Repeat("y", 100)
	content := "timeout antes\n" + longLine + "\ntimeout despues\n"

	res, got := process(t, strings.NewReader(content), "timeout", Options{MaxLineSize: maxLineSize})

	if res.Failed || res.Skipped {
		t.Fatalf("should not fail or skip the whole object: %+v", res)
	}
	if !res.LongLineWarn {
		t.Errorf("expected LongLineWarn=true for the long line")
	}
	if len(got) != 2 || got[0].LineNum != 1 || got[1].LineNum != 3 {
		t.Fatalf("expected matches on lines 1 and 3 only, got %+v", got)
	}
}

func TestProcessObject_LongLineAtEndOfFileNoTrailingNewline(t *testing.T) {
	maxLineSize := 16
	content := "ok\n" + strings.Repeat("z", maxLineSize*2) // no trailing \n
	res, got := process(t, strings.NewReader(content), "z+", Options{MaxLineSize: maxLineSize})

	if res.Failed || res.Skipped {
		t.Fatalf("should not fail: %+v", res)
	}
	if !res.LongLineWarn {
		t.Errorf("expected LongLineWarn=true for the trailing long line with no newline")
	}
	if len(got) != 0 {
		t.Errorf("the trailing long line must not match, not even partially: %+v", got)
	}
}

func TestProcessObject_LineExactlyAtLimitIsNotTooLong(t *testing.T) {
	maxLineSize := 10
	line := strings.Repeat("a", maxLineSize)
	res, got := process(t, strings.NewReader(line+"\nok\n"), "^a+$", Options{MaxLineSize: maxLineSize})

	if res.LongLineWarn {
		t.Errorf("a line of exactly MaxLineSize should not be considered long")
	}
	if len(got) != 1 || got[0].LineNum != 1 || got[0].Text != line {
		t.Errorf("expected the line at the limit to match normally and in full: %+v", got)
	}
}

// failingReader returns data, then a non-EOF error, like a connection reset
// mid-stream.
type failingReader struct {
	data string
	err  error
	done bool
}

func (f *failingReader) Read(p []byte) (int, error) {
	if !f.done {
		f.done = true
		return copy(p, f.data), nil
	}
	return 0, f.err
}

// VC-23.5 (unit-level slice) / NFR-3: a stream that breaks mid-read is a
// failure, not a silent end of object; matches found before the break stay
// emitted exactly once.
func TestProcessObject_ReadErrorMidStreamFails(t *testing.T) {
	r := &failingReader{data: "timeout uno\nINFO\n", err: errors.New("connection reset")}
	res, got := process(t, r, "timeout", Options{})

	if !res.Failed {
		t.Fatalf("a broken stream must mark the object as failed, got %+v", res)
	}
	if res.FailReason != "read interrupted: connection reset" {
		t.Errorf("FailReason = %q", res.FailReason)
	}
	if len(got) != 1 || got[0].LineNum != 1 {
		t.Errorf("the match before the break should be emitted once, got %+v", got)
	}
}

// A stream that breaks in the middle of a line must not return the partial
// line as if the object had ended there.
func TestProcessObject_ReadErrorMidLineIsNotTreatedAsEOF(t *testing.T) {
	r := &failingReader{data: "ok\npartial timeout", err: errors.New("connection reset")}
	res, got := process(t, r, "timeout", Options{})

	if !res.Failed {
		t.Fatalf("expected Failed, got %+v", res)
	}
	if len(got) != 0 {
		t.Errorf("a partial line cut by an error must not be matched: %+v", got)
	}
}
