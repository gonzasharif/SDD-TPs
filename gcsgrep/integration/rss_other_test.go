//go:build integration && !unix

package integration

import "os"

// maxRSSBytes is not available outside Unix (VC-22 is skipped there).
func maxRSSBytes(*os.ProcessState) (int64, bool) { return 0, false }
