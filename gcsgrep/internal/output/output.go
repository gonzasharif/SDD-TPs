// Package output writes gcsgrep's results and diagnostics.
// Matches go to stdout so a script can pipe/parse them (FR-3, NFR-3 in the
// spec: stdout stays clean). Warnings and errors go to stderr, never mixed
// into stdout, so FR-9's "continue past a bad object" behavior never
// corrupts the results a downstream consumer parses.
package output

import (
	"fmt"
	"io"
	"strings"
)

// Writer routes matches to stdout and diagnostics to stderr.
type Writer struct {
	Stdout io.Writer
	Stderr io.Writer

	// Color highlights results with ANSI colors, like grep --color=auto.
	// main sets it only when stdout is a terminal (FR-3): escape codes in
	// a file or a pipe would corrupt what a script parses.
	Color bool
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

// Match prints one matching line in gcsgrep's default format:
// object:line:text (FR-3; -n's line number is always included). With
// Color, each span of line (as returned by match.FindAllIndex) is
// highlighted.
func (w *Writer) Match(object string, lineNum int, line string, spans [][]int) {
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
