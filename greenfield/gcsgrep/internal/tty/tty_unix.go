//go:build unix

package tty

import "golang.org/x/sys/unix"

// isTerminal asks for the window size, which only a terminal can answer.
func isTerminal(fd uintptr) bool {
	_, err := unix.IoctlGetWinsize(int(fd), unix.TIOCGWINSZ)
	return err == nil
}
