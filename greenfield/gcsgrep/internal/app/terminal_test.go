package app

import (
	"bytes"
	"context"
	"testing"

	"gcsgrep/internal/gcsclient"
)

func runOnTerminals(term Terminals, argv ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	factory := func(context.Context) (gcsclient.Client, error) { return logsData(), nil }
	code = RunWithTerminals(context.Background(), argv, &out, &errOut, term, factory)
	return code, out.String(), errOut.String()
}

// FR-3.2: with stdout on a terminal the matching portion is highlighted.
func TestRunWithTerminals_ColorWhenStdoutIsATerminal(t *testing.T) {
	_, stdout, _ := runOnTerminals(Terminals{Stdout: true}, "timeout", "gs://b/logs/")
	if want := "logs/a.log:2:ERROR \x1b[1;31mtimeout\x1b[0m\n"; stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

// FR-3.3: redirected stdout carries no escape byte, even if stderr is a
// terminal.
func TestRunWithTerminals_NoColorWhenStdoutIsRedirected(t *testing.T) {
	_, stdout, _ := runOnTerminals(Terminals{Stderr: true}, "timeout", "gs://b/logs/")
	if want := "logs/a.log:2:ERROR timeout\n"; stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

// FR-5, FR-6: -l and -c print no color on a terminal.
func TestRunWithTerminals_NoColorInFilesAndCountModes(t *testing.T) {
	_, stdout, _ := runOnTerminals(Terminals{Stdout: true}, "-l", "timeout", "gs://b/logs/")
	if stdout != "logs/a.log\n" {
		t.Errorf("-l stdout = %q", stdout)
	}
	_, stdout, _ = runOnTerminals(Terminals{Stdout: true}, "-c", "timeout", "gs://b/logs/")
	if stdout != "logs/a.log:1\nlogs/b.log:0\n" {
		t.Errorf("-c stdout = %q", stdout)
	}
}

// FR-10.1, FR-10.2: the progress style follows stderr.
func TestRunWithTerminals_ProgressStyleFollowsStderr(t *testing.T) {
	_, _, stderr := runOnTerminals(Terminals{Stderr: true}, "timeout", "gs://b/logs/")
	if want := "\rgcsgrep: progress: 1/2 (50%)\rgcsgrep: progress: 2/2 (100%)\n"; stderr != want {
		t.Errorf("terminal stderr = %q, want %q", stderr, want)
	}
	_, _, stderr = runOnTerminals(Terminals{}, "timeout", "gs://b/logs/")
	if want := "gcsgrep: progress: 1/2 (50%)\ngcsgrep: progress: 2/2 (100%)\n"; stderr != want {
		t.Errorf("redirected stderr = %q, want %q", stderr, want)
	}
}
