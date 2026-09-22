package output

import (
	"bytes"
	"strings"
	"testing"
)

// VC-3, plain branch: without Color (stdout redirected), the output is
// object:line:text with no ANSI escape at all.
func TestMatch_PlainHasNoEscapeCodes(t *testing.T) {
	var stdout bytes.Buffer
	w := New(&stdout, &bytes.Buffer{})

	w.Match("logs/app.log", 7, "connection timeout here", [][]int{{11, 18}})

	if want := "logs/app.log:7:connection timeout here\n"; stdout.String() != want {
		t.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
	if strings.Contains(stdout.String(), "\x1b") {
		t.Errorf("plain output must contain no ANSI escapes: %q", stdout.String())
	}
}

// VC-3, TTY branch: with Color, each match is wrapped in the match color,
// and removing the escape codes leaves exactly the plain format.
func TestMatch_ColorHighlightsEveryMatch(t *testing.T) {
	var stdout bytes.Buffer
	w := New(&stdout, &bytes.Buffer{})
	w.Color = true

	w.Match("logs/app.log", 7, "timeout, then timeout", [][]int{{0, 7}, {14, 21}})

	out := stdout.String()
	if got := strings.Count(out, colorMatch+"timeout"+colorReset); got != 2 {
		t.Errorf("expected both matches highlighted, found %d in %q", got, out)
	}
	if plain := stripANSI(out); plain != "logs/app.log:7:timeout, then timeout\n" {
		t.Errorf("without escape codes, output = %q, want the plain format", plain)
	}
}

// A pattern that matches the empty string yields empty spans; they must
// not produce empty color sequences or break the line.
func TestMatch_ColorSkipsEmptySpans(t *testing.T) {
	var stdout bytes.Buffer
	w := New(&stdout, &bytes.Buffer{})
	w.Color = true

	w.Match("o", 1, "abc", [][]int{{0, 0}, {1, 1}, {2, 2}, {3, 3}})

	if strings.Contains(stdout.String(), colorMatch) {
		t.Errorf("empty spans should not be highlighted: %q", stdout.String())
	}
	if plain := stripANSI(stdout.String()); plain != "o:1:abc\n" {
		t.Errorf("output = %q, want %q", plain, "o:1:abc\n")
	}
}

// stripANSI removes every "\x1b[...m" SGR sequence.
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\x1b' {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
