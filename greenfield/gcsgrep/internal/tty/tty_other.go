//go:build !unix && !windows

package tty

// isTerminal is false where there is no way to ask: no color, plain progress.
func isTerminal(uintptr) bool { return false }
