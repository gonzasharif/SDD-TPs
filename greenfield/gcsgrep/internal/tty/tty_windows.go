//go:build windows

package tty

import "golang.org/x/sys/windows"

// isTerminal reports whether fd is a console, enabling its ANSI escape
// processing (colors and cursor movement) on the way.
func isTerminal(fd uintptr) bool {
	handle := windows.Handle(fd)
	var mode uint32
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		return false
	}
	if mode&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING != 0 {
		return true
	}
	return windows.SetConsoleMode(handle, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) == nil
}
