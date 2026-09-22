// Package output writes gcsgrep's results and diagnostics.
// Matches go to stdout so a script can pipe/parse them (FR-3, NFR-3 in the
// spec: stdout stays clean). Warnings and errors go to stderr, never mixed
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
// object:line:text (FR-3, simplified for Iteration 1 — always plain text,
// no TTY color detection yet; -n's line number is always included).
func (w *Writer) Match(object string, lineNum int, line string) {
	fmt.Fprintf(w.Stdout, "%s:%d:%s\n", object, lineNum, line)
}

// ObjectName prints just the name of an object that matched, once per
// object (FR-5, -l).
func (w *Writer) ObjectName(object string) {
	fmt.Fprintln(w.Stdout, object)
}

// Count prints an object's number of matching lines as object:count
// (FR-6, -c), including objects with zero matches.
func (w *Writer) Count(object string, count int) {
	fmt.Fprintf(w.Stdout, "%s:%d\n", object, count)
}

// Warning reports a recoverable, per-object condition (an unreadable
// object, a binary skip, a long line skip) that must not abort the run
// (FR-9, FR-11, FR-15).
func (w *Writer) Warning(format string, args ...any) {
	fmt.Fprintf(w.Stderr, "gcsgrep: warning: "+format+"\n", args...)
}

// Error reports a condition that aborts the run entirely (a usage error, a
// guardrail hit before any content was read, or a fatal setup failure).
func (w *Writer) Error(format string, args ...any) {
	fmt.Fprintf(w.Stderr, "gcsgrep: error: "+format+"\n", args...)
}
