// Package output writes gcsgrep's results and diagnostics.
// Matches go to stdout so a script can pipe/parse them (FR-3). Avisos and
// mensajes de error go to stderr with their literal prefixes, never mixed
// into stdout, so FR-9's "continue past a bad object" behavior never
// corrupts the results a downstream consumer parses. Progress (FR-10) goes
// to stderr too.
package output

import (
	"fmt"
	"io"

	"gcsgrep/internal/match"
)

const (
	// colorStart and colorEnd wrap each highlighted match (FR-3.2).
	colorStart = "\x1b[1;31m"
	colorEnd   = "\x1b[0m"
	// eraseProgress returns to the start of the line and erases it, so a
	// message replaces the progress text instead of sticking to it
	// (FR-10.1).
	eraseProgress = "\r\x1b[K"
)

// Options says what kind of streams the Writer writes to.
type Options struct {
	// Color highlights the matches on stdout: stdout is a terminal
	// (FR-3.2). Without it the output is plain text (FR-3.3).
	Color bool

	// Progress turns on the progress text (FR-10).
	Progress bool

	// StderrTerminal means stderr is a terminal: progress redraws one line
	// in place (FR-10.1) instead of printing a line per 10% (FR-10.2).
	StderrTerminal bool
}

// Writer routes matches to stdout and diagnostics to stderr.
type Writer struct {
	Stdout io.Writer
	Stderr io.Writer

	opts Options

	// progressOnScreen is true while the last thing written to stderr is a
	// progress text without its line end (terminal mode).
	progressOnScreen bool
	// progressShown is true once any progress text has been written.
	progressShown bool
	// lastDecile is the highest multiple of 10% already printed (redirected
	// mode).
	lastDecile int
}

// New builds a Writer over the given streams, with plain output and no
// progress.
func New(stdout, stderr io.Writer) *Writer {
	return NewWithOptions(stdout, stderr, Options{})
}

// NewWithOptions builds a Writer over the given streams.
func NewWithOptions(stdout, stderr io.Writer, opts Options) *Writer {
	return &Writer{Stdout: stdout, Stderr: stderr, opts: opts}
}

// Color reports whether matches are highlighted, so callers only compute
// the highlighted ranges when they are used.
func (w *Writer) Color() bool { return w.opts.Color }

// Match prints one matching line in gcsgrep's default format:
// object:line:text (FR-3.1). With color on, each of spans (the ranges of
// text that match) is wrapped in ANSI start and end sequences (FR-3.2); the
// rest of the format does not change. The line number is always included,
// which is why -n has no effect (FR-20).
func (w *Writer) Match(object string, lineNum int, line string, spans []match.Span) {
	w.eraseProgressText()
	if !w.opts.Color || len(spans) == 0 {
		fmt.Fprintf(w.Stdout, "%s:%d:%s\n", object, lineNum, line)
		return
	}
	fmt.Fprintf(w.Stdout, "%s:%d:%s\n", object, lineNum, highlight(line, spans))
}

// highlight wraps every span of line in the color sequences.
func highlight(line string, spans []match.Span) string {
	out := make([]byte, 0, len(line)+len(spans)*(len(colorStart)+len(colorEnd)))
	pos := 0
	for _, s := range spans {
		out = append(out, line[pos:s.Start]...)
		out = append(out, colorStart...)
		out = append(out, line[s.Start:s.End]...)
		out = append(out, colorEnd...)
		pos = s.End
	}
	out = append(out, line[pos:]...)
	return string(out)
}

// ObjectName prints just an object's name: `-l` output for an object with at
// least one match (FR-5).
func (w *Writer) ObjectName(object string) {
	w.eraseProgressText()
	fmt.Fprintf(w.Stdout, "%s\n", object)
}

// Count prints `object:count`: `-c` output for an object read completely
// (FR-6).
func (w *Writer) Count(object string, count int) {
	w.eraseProgressText()
	fmt.Fprintf(w.Stdout, "%s:%d\n", object, count)
}

// Warning prints an aviso ("gcsgrep: warning: ..."): a condition the run
// recovers from and continues past (FR-9, FR-11, FR-15, FR-17).
func (w *Writer) Warning(format string, args ...any) {
	w.eraseProgressText()
	fmt.Fprintf(w.Stderr, "gcsgrep: warning: "+format+"\n", args...)
}

// Error prints a mensaje de error ("gcsgrep: error: ..."): a condition that
// ends the run with exit code 2 (FR-1.4, FR-16, FR-22, BR-3).
func (w *Writer) Error(format string, args ...any) {
	w.eraseProgressText()
	fmt.Fprintf(w.Stderr, "gcsgrep: error: "+format+"\n", args...)
}

// Progress reports that processed of total objects are done (FR-10). On a
// terminal it redraws the text in place every time; when stderr is
// redirected it prints a line each time the percentage reaches a new
// multiple of 10, and a single line if one object crosses several.
func (w *Writer) Progress(processed, total int) {
	if !w.opts.Progress || total <= 0 {
		return
	}
	percent := processed * 100 / total
	text := fmt.Sprintf("gcsgrep: progress: %d/%d (%d%%)", processed, total, percent)

	if w.opts.StderrTerminal {
		fmt.Fprintf(w.Stderr, "\r%s", text)
		w.progressOnScreen = true
		w.progressShown = true
		return
	}
	if decile := percent / 10; decile > w.lastDecile {
		w.lastDecile = decile
		fmt.Fprintf(w.Stderr, "%s\n", text)
		w.progressShown = true
	}
}

// EndProgress closes the progress text at the end of the run: on a terminal
// that showed progress it ends the line with a single "\n" (FR-10.1).
func (w *Writer) EndProgress() {
	if w.opts.Progress && w.opts.StderrTerminal && w.progressShown {
		fmt.Fprint(w.Stderr, "\n")
	}
	w.progressOnScreen = false
}

// eraseProgressText clears the progress text still on screen before
// anything else is written, so the message replaces it (FR-10.1).
func (w *Writer) eraseProgressText() {
	if w.progressOnScreen {
		fmt.Fprint(w.Stderr, eraseProgress)
		w.progressOnScreen = false
	}
}
