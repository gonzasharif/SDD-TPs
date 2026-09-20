package reader

import (
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
