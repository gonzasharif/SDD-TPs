package output

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"gcsgrep/internal/match"
)

func newTestWriter(opts Options) (*Writer, *bytes.Buffer, *bytes.Buffer) {
	var stdout, stderr bytes.Buffer
	return NewWithOptions(&stdout, &stderr, opts), &stdout, &stderr
}

// FR-3.1, FR-3.3: plain output has no escape sequences.
func TestMatch_Plain(t *testing.T) {
	w, stdout, _ := newTestWriter(Options{})
	w.Match("logs/a.log", 2, "ERROR timeout", []match.Span{{Start: 6, End: 13}})
	if got := stdout.String(); got != "logs/a.log:2:ERROR timeout\n" {
		t.Errorf("stdout = %q", got)
	}
}

// FR-3.2 / VC-3.2: each matching portion goes between ESC[1;31m and ESC[0m.
func TestMatch_Color(t *testing.T) {
	w, stdout, _ := newTestWriter(Options{Color: true})
	w.Match("logs/a.log", 2, "ERROR timeout", []match.Span{{Start: 6, End: 13}})
	if got, want := stdout.String(), "logs/a.log:2:ERROR \x1b[1;31mtimeout\x1b[0m\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

// FR-3.2: every match of the line is highlighted, the text between them is
// left alone.
func TestMatch_ColorSeveralMatches(t *testing.T) {
	w, stdout, _ := newTestWriter(Options{Color: true})
	w.Match("o", 1, "ab ab", []match.Span{{Start: 0, End: 2}, {Start: 3, End: 5}})
	if got, want := stdout.String(), "o:1:\x1b[1;31mab\x1b[0m \x1b[1;31mab\x1b[0m\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

// FR-5, FR-6: no highlighting in -l and -c output, with or without a terminal.
func TestObjectNameAndCount_NeverColored(t *testing.T) {
	w, stdout, _ := newTestWriter(Options{Color: true})
	w.ObjectName("logs/a.log")
	w.Count("logs/a.log", 1)
	if got, want := stdout.String(), "logs/a.log\nlogs/a.log:1\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

func progressLines(n int) []string {
	var out []string
	for i := 1; i <= n; i++ {
		out = append(out, fmt.Sprintf("gcsgrep: progress: %d/%d (%d%%)", i, n, i*100/n))
	}
	return out
}

// FR-10.1 / VC-10.1: on a terminal every object redraws the text in place
// with \r, and the run ends with a single \n.
func TestProgress_Terminal(t *testing.T) {
	w, _, stderr := newTestWriter(Options{Progress: true, StderrTerminal: true})
	for i := 1; i <= 50; i++ {
		w.Progress(i, 50)
	}
	w.EndProgress()

	var want strings.Builder
	for _, l := range progressLines(50) {
		want.WriteString("\r" + l)
	}
	want.WriteString("\n")
	if got := stderr.String(); got != want.String() {
		t.Errorf("stderr = %q, want %q", got, want.String())
	}
}

// FR-10.2 / VC-10.2: redirected, one line per multiple of 10%, no \r.
func TestProgress_Redirected(t *testing.T) {
	w, _, stderr := newTestWriter(Options{Progress: true})
	for i := 1; i <= 50; i++ {
		w.Progress(i, 50)
	}
	w.EndProgress()

	got := strings.Split(strings.TrimSuffix(stderr.String(), "\n"), "\n")
	if len(got) != 10 {
		t.Fatalf("got %d lines, want 10: %q", len(got), stderr.String())
	}
	for i, l := range got {
		want := fmt.Sprintf("gcsgrep: progress: %d/50 (%d%%)", (i+1)*5, (i+1)*10)
		if l != want {
			t.Errorf("line %d = %q, want %q", i+1, l, want)
		}
	}
	if strings.Contains(stderr.String(), "\r") {
		t.Errorf("redirected progress must not contain \\r")
	}
}

// FR-10.2: an object that crosses several multiples of 10 prints one line,
// with its real percentage.
func TestProgress_Redirected_OneLineWhenSeveralMultiplesAreCrossed(t *testing.T) {
	w, _, stderr := newTestWriter(Options{Progress: true})
	w.Progress(1, 2)
	w.Progress(2, 2)

	want := "gcsgrep: progress: 1/2 (50%)\ngcsgrep: progress: 2/2 (100%)\n"
	if got := stderr.String(); got != want {
		t.Errorf("stderr = %q, want %q", got, want)
	}
}

// FR-10.2: below the first multiple of 10 nothing is printed.
func TestProgress_Redirected_NothingBelowTenPercent(t *testing.T) {
	w, _, stderr := newTestWriter(Options{Progress: true})
	w.Progress(1, 20) // 5%
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want nothing", stderr.String())
	}
}

// FR-10: no progress when it is off or when nothing was processed.
func TestProgress_Off(t *testing.T) {
	w, _, stderr := newTestWriter(Options{})
	w.Progress(1, 1)
	w.EndProgress()
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want nothing", stderr.String())
	}

	w, _, stderr = newTestWriter(Options{Progress: true, StderrTerminal: true})
	w.EndProgress()
	if stderr.Len() != 0 {
		t.Errorf("no progress was shown, yet stderr = %q", stderr.String())
	}
}

// FR-10.1: a message written while the progress text is on screen erases it
// first, and the next redraw starts again.
func TestProgress_Terminal_MessagesReplaceTheProgressText(t *testing.T) {
	w, stdout, stderr := newTestWriter(Options{Progress: true, StderrTerminal: true})
	w.Progress(1, 3)
	w.Warning("a.log: permission denied")
	w.Progress(2, 3)
	w.Match("b.log", 1, "x", nil)
	w.Progress(3, 3)
	w.EndProgress()

	want := "\rgcsgrep: progress: 1/3 (33%)" +
		"\r\x1b[Kgcsgrep: warning: a.log: permission denied\n" +
		"\rgcsgrep: progress: 2/3 (66%)" +
		"\r\x1b[K" +
		"\rgcsgrep: progress: 3/3 (100%)" +
		"\n"
	if got := stderr.String(); got != want {
		t.Errorf("stderr = %q,\nwant %q", got, want)
	}
	if got := stdout.String(); got != "b.log:1:x\n" {
		t.Errorf("stdout = %q", got)
	}
}

// FR-10.2: redirected progress never erases anything.
func TestProgress_Redirected_NoEraseSequences(t *testing.T) {
	w, _, stderr := newTestWriter(Options{Progress: true})
	w.Progress(1, 1)
	w.Warning("x")
	if strings.Contains(stderr.String(), "\x1b") {
		t.Errorf("stderr = %q, want no escape sequences", stderr.String())
	}
}
