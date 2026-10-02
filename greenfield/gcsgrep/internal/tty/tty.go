// Package tty tells whether a file is a terminal. gcsgrep needs it to decide
// whether stdout gets color (FR-3.2, FR-3.3) and which kind of progress
// stderr gets (FR-10.1, FR-10.2).
package tty

import "os"

// IsTerminal reports whether f is connected to a terminal. On Windows it
// also turns on the console's ANSI escape processing, which color and the
// progress line need; if that cannot be turned on it reports false.
func IsTerminal(f *os.File) bool {
	return isTerminal(f.Fd())
}
