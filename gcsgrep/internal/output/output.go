// Package output writes gcsgrep's results, diagnostics, and progress.
// Matches go to stdout so a script can pipe/parse them (FR-3: stdout stays
// clean, and colored only on a terminal). Warnings, errors, and progress
// (FR-10) go to stderr, never mixed into stdout, so FR-9's "continue past a
// bad object" behavior never corrupts the results a downstream consumer
// parses.
//
// A Writer is safe for concurrent use (FR-13's workers share one). Every
// call takes the Writer's lock, so two goroutines never interleave bytes
// on a line or garble the progress bar. Do groups several writes under one
// lock, which is how a worker prints everything an object produced without
// another object's lines landing in the middle.
package output

import (
	"fmt"
	"io"
	"strings"
	"sync"
)

// Writer routes matches to stdout and diagnostics to stderr.
type Writer struct {
	Stdout io.Writer
	Stderr io.Writer

	// Color highlights results with ANSI colors, like grep --color=auto.
	// main sets it only when stdout is a terminal (FR-3): escape codes in
	// a file or a pipe would corrupt what a script parses.
	Color bool

	// Progress selects how FR-10's progress is shown on stderr; main
	// picks the bar or plain lines depending on whether stderr is a
	// terminal.
	Progress ProgressStyle

	// mu guards the streams and progress. Color and Progress are
	// configuration: set them before the run starts, not during it.
	mu       sync.Mutex
	progress progress
}

// ANSI SGR sequences, matching GNU grep's default colors.
const (
	colorObject  = "\x1b[35m"    // magenta
	colorLineNum = "\x1b[32m"    // green
	colorSep     = "\x1b[36m"    // cyan
	colorMatch   = "\x1b[01;31m" // bold red
	colorReset   = "\x1b[m"
)

// New builds a Writer over the given streams.
func New(stdout, stderr io.Writer) *Writer {
	return &Writer{Stdout: stdout, Stderr: stderr}
}

// Section is a Writer whose lock is already held by the caller. It exists
// only inside Do.
type Section struct {
	w *Writer
}

// Do runs fn holding the Writer's lock, so everything fn writes through the
// Section reaches the terminal contiguously. fn must not call the Writer's
// own methods (they would wait on the lock fn already holds) and should not
// block on anything slow.
func (w *Writer) Do(fn func(s *Section)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	fn(&Section{w: w})
}

// Match prints one matching line in gcsgrep's default format:
// object:line:text (FR-3; -n's line number is always included). With
// Color, each span of line (as returned by match.FindAllIndex) is
// highlighted.
func (w *Writer) Match(object string, lineNum int, line string, spans [][]int) {
	w.Do(func(s *Section) { s.Match(object, lineNum, line, spans) })
}

// ObjectName prints just the name of an object that matched, once per
// object (FR-5, -l).
func (w *Writer) ObjectName(object string) {
	w.Do(func(s *Section) { s.ObjectName(object) })
}

// Count prints an object's number of matching lines as object:count
// (FR-6, -c), including objects with zero matches.
func (w *Writer) Count(object string, count int) {
	w.Do(func(s *Section) { s.Count(object, count) })
}

// Warning reports a recoverable, per-object condition (an unreadable
// object, a binary skip, a long line skip) that must not abort the run
// (FR-9, FR-11, FR-15).
func (w *Writer) Warning(format string, args ...any) {
	w.Do(func(s *Section) { s.Warning(format, args...) })
}

// Error reports a condition that aborts the run entirely (a usage error, a
// guardrail hit before any content was read, or a fatal setup failure).
func (w *Writer) Error(format string, args ...any) {
	w.Do(func(s *Section) { s.Error(format, args...) })
}

// Match is Writer.Match inside a Do.
func (s *Section) Match(object string, lineNum int, line string, spans [][]int) {
	w := s.w
	w.clearBar()
	defer w.drawBar()
	if !w.Color {
		fmt.Fprintf(w.Stdout, "%s:%d:%s\n", object, lineNum, line)
		return
	}
	sep := colorSep + ":" + colorReset
	fmt.Fprintf(w.Stdout, "%s%s%s%s%s%d%s%s%s\n",
		colorObject, object, colorReset, sep,
		colorLineNum, lineNum, colorReset, sep,
		highlight(line, spans))
}

// ObjectName is Writer.ObjectName inside a Do.
func (s *Section) ObjectName(object string) {
	s.w.clearBar()
	defer s.w.drawBar()
	fmt.Fprintln(s.w.Stdout, object)
}

// Count is Writer.Count inside a Do.
func (s *Section) Count(object string, count int) {
	s.w.clearBar()
	defer s.w.drawBar()
	fmt.Fprintf(s.w.Stdout, "%s:%d\n", object, count)
}

// Warning is Writer.Warning inside a Do.
func (s *Section) Warning(format string, args ...any) {
	s.w.clearBar()
	defer s.w.drawBar()
	fmt.Fprintf(s.w.Stderr, "gcsgrep: warning: "+format+"\n", args...)
}

// Error is Writer.Error inside a Do.
func (s *Section) Error(format string, args ...any) {
	s.w.clearBar()
	defer s.w.drawBar()
	fmt.Fprintf(s.w.Stderr, "gcsgrep: error: "+format+"\n", args...)
}

// highlight wraps each non-empty span of line in the match color. Empty
// spans (a pattern like "x*" matches the empty string) have nothing to
// color and are skipped.
func highlight(line string, spans [][]int) string {
	var b strings.Builder
	prev := 0
	for _, sp := range spans {
		if sp[0] == sp[1] {
			continue
		}
		b.WriteString(line[prev:sp[0]])
		b.WriteString(colorMatch)
		b.WriteString(line[sp[0]:sp[1]])
		b.WriteString(colorReset)
		prev = sp[1]
	}
	b.WriteString(line[prev:])
	return b.String()
}
