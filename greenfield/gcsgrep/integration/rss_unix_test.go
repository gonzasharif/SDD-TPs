//go:build integration && unix

package integration

import (
	"os"
	"runtime"
	"syscall"
)

// maxRSSBytes returns the peak resident set size of a finished process.
// getrusage reports it in bytes on macOS and in KiB on Linux.
func maxRSSBytes(ps *os.ProcessState) (int64, bool) {
	ru, ok := ps.SysUsage().(*syscall.Rusage)
	if !ok {
		return 0, false
	}
	if runtime.GOOS == "darwin" {
		return int64(ru.Maxrss), true
	}
	return int64(ru.Maxrss) * 1024, true
}
