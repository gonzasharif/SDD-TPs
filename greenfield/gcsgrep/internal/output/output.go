// Package output writes gcsgrep's results and diagnostics.
// Matches go to stdout so a script can pipe/parse them (FR-3). Avisos and
// mensajes de error go to stderr with their literal prefixes, never mixed
// into stdout, so FR-9's "continue past a bad object" behavior never
// corrupts the results a downstream consumer parses.
package output

import (
	"fmt"
	"io"
)

// Writer routes matches to stdout and diagnostics to stderr.
type Writer struct {
	Stdout io.Writer
	Stderr io.Writer
}

// New builds a Writer over the given streams.
func New(stdout, stderr io.Writer) *Writer {
	return &Writer{Stdout: stdout, Stderr: stderr}
}

// Match prints one matching line in gcsgrep's default format:
// object:line:text (FR-3.1). Iteration 1 always prints plain text (FR-3.3);
// TTY color (FR-3.2) is Iteration 2. The line number is always included,
// which is why -n has no effect (FR-20).
func (w *Writer) Match(object string, lineNum int, line string) {
	fmt.Fprintf(w.Stdout, "%s:%d:%s\n", object, lineNum, line)
}

// ObjectName prints just an object's name: `-l` output for an object with at
// least one match (FR-5).
func (w *Writer) ObjectName(object string) {
	fmt.Fprintf(w.Stdout, "%s\n", object)
}

// Count prints `object:count`: `-c` output for an object read completely
// (FR-6).
func (w *Writer) Count(object string, count int) {
	fmt.Fprintf(w.Stdout, "%s:%d\n", object, count)
}

// Warning prints an aviso ("gcsgrep: warning: ..."): a condition the run
// recovers from and continues past (FR-9, FR-11, FR-15, FR-17).
func (w *Writer) Warning(format string, args ...any) {
	fmt.Fprintf(w.Stderr, "gcsgrep: warning: "+format+"\n", args...)
}

// Error prints a mensaje de error ("gcsgrep: error: ..."): a condition that
// ends the run with exit code 2 (FR-1.4, FR-16, FR-22, BR-3).
func (w *Writer) Error(format string, args ...any) {
	fmt.Fprintf(w.Stderr, "gcsgrep: error: "+format+"\n", args...)
}
