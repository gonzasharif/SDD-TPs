//go:build integration

package integration

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

var (
	binaryOnce sync.Once
	binaryPath string
	binaryErr  error
)

// gcsgrepBinary builds cmd/gcsgrep once: the terminal checks need a real
// process, because the terminal is that process's stdout or stderr.
func gcsgrepBinary(t *testing.T) string {
	t.Helper()
	binaryOnce.Do(func() {
		dir, err := os.MkdirTemp("", "gcsgrep-bin")
		if err != nil {
			binaryErr = err
			return
		}
		binaryPath = filepath.Join(dir, "gcsgrep")
		if out, err := exec.Command("go", "build", "-o", binaryPath, "../cmd/gcsgrep").CombinedOutput(); err != nil {
			binaryErr = fmt.Errorf("go build: %v\n%s", err, out)
		}
	})
	if binaryErr != nil {
		t.Fatalf("%v", binaryErr)
	}
	return binaryPath
}

// ptyRun is the outcome of a process whose stdout or stderr was a terminal.
type ptyRun struct {
	code           int
	stdout, stderr string
}

// runOnTerminal runs gcsgrep with stdout and/or stderr connected to a
// pseudo-terminal; the stream that is not a terminal is captured through a
// pipe. creds, if not empty, is the name of a test service account.
func runOnTerminal(t *testing.T, stdoutTTY, stderrTTY bool, creds string, args ...string) ptyRun {
	t.Helper()
	if bucket == "" {
		t.Skip("GCSGREP_TEST_BUCKET is not set")
	}
	bin := gcsgrepBinary(t)

	cmd := exec.Command(bin, args...)
	cmd.Env = os.Environ()
	if creds != "" {
		useCreds(t, creds)
		cmd.Env = append(cmd.Env, "GOOGLE_APPLICATION_CREDENTIALS="+filepath.Join(credsDir, creds+".json"))
	}

	var master, slave *os.File
	if stdoutTTY || stderrTTY {
		master, slave = newPty(t)
	}
	var stdoutBuf, stderrBuf bytes.Buffer
	if stdoutTTY {
		cmd.Stdout = slave
	} else {
		cmd.Stdout = &stdoutBuf
	}
	if stderrTTY {
		cmd.Stderr = slave
	} else {
		cmd.Stderr = &stderrBuf
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	if slave != nil {
		slave.Close() // the child has its own copy; the master sees EOF when it exits
	}

	var terminalOut chan string
	if master != nil {
		terminalOut = make(chan string, 1)
		go func() {
			b, _ := io.ReadAll(master) // a pty master returns EIO once the slave closes
			terminalOut <- string(b)
		}()
	}

	waitErr := cmd.Wait()
	code := 0
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		code = exitErr.ExitCode()
	} else if waitErr != nil {
		t.Fatalf("wait: %v", waitErr)
	}

	run := ptyRun{code: code, stdout: stdoutBuf.String(), stderr: stderrBuf.String()}
	if master != nil {
		out := <-terminalOut
		switch {
		case stdoutTTY:
			run.stdout = out // when both are terminals they share this one
		default:
			run.stderr = out
		}
	}
	t.Logf("gcsgrep %s (stdout tty=%v, stderr tty=%v)\n--- exit %d\n--- stdout %q\n--- stderr %q",
		strings.Join(args, " "), stdoutTTY, stderrTTY, run.code, run.stdout, run.stderr)
	return run
}

func progressTerminalText(total int, percents func(i int) int) string {
	var b strings.Builder
	for i := 1; i <= total; i++ {
		fmt.Fprintf(&b, "\rgcsgrep: progress: %d/%d (%d%%)", i, total, percents(i))
	}
	return b.String()
}

// VC-3.2: with stdout on a terminal the match is highlighted; -l and -c print
// no color.
func TestVC3_2_ColorOnTerminal(t *testing.T) {
	got := runOnTerminal(t, true, false, "", "timeout", loc("logs/"))
	if got.code != 0 || got.stdout != "logs/a.log:2:ERROR \x1b[1;31mtimeout\x1b[0m\n" {
		t.Errorf("exit %d, stdout %q; want 0 and the highlighted match", got.code, got.stdout)
	}

	files := runOnTerminal(t, true, false, "", "-l", "timeout", loc("logs/a.log"))
	if files.code != 0 || files.stdout != "logs/a.log\n" {
		t.Errorf("-l: exit %d, stdout %q; want 0 and %q", files.code, files.stdout, "logs/a.log\n")
	}
	count := runOnTerminal(t, true, false, "", "-c", "timeout", loc("logs/a.log"))
	if count.code != 0 || count.stdout != "logs/a.log:1\n" {
		t.Errorf("-c: exit %d, stdout %q; want 0 and %q", count.code, count.stdout, "logs/a.log:1\n")
	}
}

// VC-10.1: progress redrawn in place on a terminal; a message replaces it.
func TestVC10_1_ProgressOnTerminal(t *testing.T) {
	got := runOnTerminal(t, false, true, "", "timeout", loc("prog/"))
	want := progressTerminalText(50, func(i int) int { return i * 2 }) + "\n"
	if got.code != 1 || got.stderr != want {
		t.Errorf("exit %d, stderr %q;\nwant 1 and %q", got.code, got.stderr, want)
	}

	// acl/ has 6 objects; the last one (denied.log) cannot be read by
	// <sa-restringida>, so its aviso arrives while the progress is on screen.
	denied := runOnTerminal(t, false, true, "restringida", "secreto", loc("acl/"))
	percents := []int{16, 33, 50, 66, 83}
	var wantDenied strings.Builder
	for i, p := range percents {
		fmt.Fprintf(&wantDenied, "\rgcsgrep: progress: %d/6 (%d%%)", i+1, p)
	}
	wantDenied.WriteString("\r\x1b[Kgcsgrep: warning: acl/denied.log: permission denied\n")
	wantDenied.WriteString("\rgcsgrep: progress: 6/6 (100%)\n")
	if denied.code != 2 || denied.stderr != wantDenied.String() {
		t.Errorf("exit %d, stderr %q;\nwant 2 and %q", denied.code, denied.stderr, wantDenied.String())
	}
}
