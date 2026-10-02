//go:build integration && !linux

package integration

import (
	"os"
	"testing"
)

// newPty is only implemented on Linux: the VCs that need a terminal (VC-3.2,
// VC-10.1) are skipped elsewhere; run them from WSL.
func newPty(t *testing.T) (master, slave *os.File) {
	t.Helper()
	t.Skip("VCs that need a pseudo-terminal run on Linux (use WSL)")
	return nil, nil
}
