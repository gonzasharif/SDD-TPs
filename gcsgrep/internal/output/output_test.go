package output

import (
	"bytes"
	"regexp"
	"strconv"
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

// VC-10 (a): on a terminal, the bar is redrawn in place with \r and its
// percentage grows monotonically up to 100%.
func TestProgress_BarRedrawsMonotonicallyTo100(t *testing.T) {
	var stderr bytes.Buffer
	w := New(&bytes.Buffer{}, &stderr)
	w.Progress = ProgressBar

	w.StartProgress(50)
	for range 50 {
		w.AdvanceProgress()
	}
	w.FinishProgress()

	out := stderr.String()
	if !strings.Contains(out, "\r") {
		t.Fatalf("expected \\r redraws: %q", out)
	}
	percents := percentsIn(out)
	if len(percents) < 50 || percents[len(percents)-1] != 100 {
		t.Fatalf("expected a redraw per object ending at 100%%, got %v", percents)
	}
	for i := 1; i < len(percents); i++ {
		if percents[i] < percents[i-1] {
			t.Fatalf("percentage went backwards: %v", percents)
		}
	}
	if !strings.HasSuffix(out, "\n") {
		t.Errorf("the finished bar should end with a newline")
	}
}

// VC-10 (b): redirected, progress is plain lines — at least 2 before the
// final one — and there is no \r anywhere.
func TestProgress_LinesWithoutCarriageReturn(t *testing.T) {
	var stderr bytes.Buffer
	w := New(&bytes.Buffer{}, &stderr)
	w.Progress = ProgressLines

	w.StartProgress(50)
	for range 50 {
		w.AdvanceProgress()
	}
	w.FinishProgress()

	out := stderr.String()
	if strings.Contains(out, "\r") {
		t.Errorf("redirected progress must not contain \\r: %q", out)
	}
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) < 3 {
		t.Fatalf("expected at least 2 progress lines before the final one, got %q", lines)
	}
	if last := lines[len(lines)-1]; last != "gcsgrep: progress: 50/50 objects (100%)" {
		t.Errorf("final line = %q", last)
	}
}

// A run cut short (BR-5) still ends with a line showing where it stopped.
func TestProgress_LinesShowWhereAnEarlyStopHappened(t *testing.T) {
	var stderr bytes.Buffer
	w := New(&bytes.Buffer{}, &stderr)
	w.Progress = ProgressLines

	w.StartProgress(50)
	for range 7 {
		w.AdvanceProgress()
	}
	w.FinishProgress()

	if !strings.HasSuffix(stderr.String(), "gcsgrep: progress: 7/50 objects (14%)\n") {
		t.Errorf("expected a final line at 7/50: %q", stderr.String())
	}
}

// A warning printed mid-run erases the bar first and redraws it after, so
// the two never share a terminal line.
func TestProgress_WarningDoesNotGlueToTheBar(t *testing.T) {
	var stderr bytes.Buffer
	w := New(&bytes.Buffer{}, &stderr)
	w.Progress = ProgressBar

	w.StartProgress(10)
	w.AdvanceProgress()
	w.Warning("logs/x: skipped")

	out := stderr.String()
	i := strings.Index(out, "gcsgrep: warning: logs/x: skipped\n")
	if i < 0 {
		t.Fatalf("warning missing: %q", out)
	}
	if !strings.HasSuffix(out[:i], "\r\x1b[K") {
		t.Errorf("the bar should be erased right before the warning: %q", out)
	}
	if !strings.Contains(out[i:], "(1/10 objects)") {
		t.Errorf("the bar should be redrawn after the warning: %q", out)
	}
}

// A single object is not worth a progress display (FR-10: "más de un
// objeto").
func TestProgress_NothingForASingleObject(t *testing.T) {
	var stderr bytes.Buffer
	w := New(&bytes.Buffer{}, &stderr)
	w.Progress = ProgressLines

	w.StartProgress(1)
	w.AdvanceProgress()
	w.FinishProgress()

	if stderr.Len() != 0 {
		t.Errorf("expected no progress for a single object: %q", stderr.String())
	}
}

// percentsIn extracts every "NN%" in order.
func percentsIn(s string) []int {
	var out []int
	for _, m := range regexp.MustCompile(`(\d+)%`).FindAllStringSubmatch(s, -1) {
		n, _ := strconv.Atoi(m[1])
		out = append(out, n)
	}
	return out
}
