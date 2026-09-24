package output

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"
)

// FR-13: workers share one Writer. Lines written at the same time must come
// out whole, never with bytes from two lines mixed together. (Run with
// -race: the buffers are plain bytes.Buffer, so any write that skipped the
// lock would be reported.)
func TestWriter_ConcurrentMatchesKeepLinesIntact(t *testing.T) {
	var stdout, stderr bytes.Buffer
	w := New(&stdout, &stderr)

	const goroutines, perGoroutine = 16, 200
	var wg sync.WaitGroup
	for g := range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range perGoroutine {
				w.Match(fmt.Sprintf("obj%d", g), i+1, fmt.Sprintf("text %d", i), nil)
				w.Warning("warning %d from %d", i, g)
			}
		}()
	}
	wg.Wait()

	lineRe := regexp.MustCompile(`^obj\d+:\d+:text \d+$`)
	lines := strings.Split(strings.TrimSuffix(stdout.String(), "\n"), "\n")
	if len(lines) != goroutines*perGoroutine {
		t.Fatalf("got %d lines on stdout, want %d", len(lines), goroutines*perGoroutine)
	}
	for _, l := range lines {
		if !lineRe.MatchString(l) {
			t.Fatalf("garbled line on stdout: %q", l)
		}
	}

	warnRe := regexp.MustCompile(`^gcsgrep: warning: warning \d+ from \d+$`)
	for _, l := range strings.Split(strings.TrimSuffix(stderr.String(), "\n"), "\n") {
		if !warnRe.MatchString(l) {
			t.Fatalf("garbled line on stderr: %q", l)
		}
	}
}

// Do keeps one object's output together: while a worker is inside Do, no
// other worker's lines can land in the middle of it.
func TestWriter_DoKeepsAnObjectsOutputContiguous(t *testing.T) {
	var stdout bytes.Buffer
	w := New(&stdout, &bytes.Buffer{})

	const objects, linesPerObject = 12, 50
	var wg sync.WaitGroup
	for o := range objects {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.Do(func(s *Section) {
				for i := range linesPerObject {
					s.Match(fmt.Sprintf("obj%d", o), i+1, "hit", nil)
				}
			})
		}()
	}
	wg.Wait()

	lines := strings.Split(strings.TrimSuffix(stdout.String(), "\n"), "\n")
	if len(lines) != objects*linesPerObject {
		t.Fatalf("got %d lines, want %d", len(lines), objects*linesPerObject)
	}
	for block := range objects {
		first := strings.SplitN(lines[block*linesPerObject], ":", 2)[0]
		for i := range linesPerObject {
			line := lines[block*linesPerObject+i]
			if want := fmt.Sprintf("%s:%d:hit", first, i+1); line != want {
				t.Fatalf("object %s was interleaved: line %d is %q, want %q", first, block*linesPerObject+i, line, want)
			}
		}
	}
}

// FR-10 under concurrency: the shared progress counter loses no update, so
// 100 objects finishing at once still produce exactly one line per 10% and
// end at 100%.
func TestProgress_ConcurrentAdvancesAreNeverLost(t *testing.T) {
	var stderr bytes.Buffer
	w := New(&bytes.Buffer{}, &stderr)
	w.Progress = ProgressLines

	const total = 100
	w.StartProgress(total)
	var wg sync.WaitGroup
	for range total {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.AdvanceProgress()
		}()
	}
	wg.Wait()
	w.FinishProgress()

	lines := strings.Split(strings.TrimSuffix(stderr.String(), "\n"), "\n")
	if len(lines) != 10 {
		t.Errorf("got %d progress lines, want 10 (one per 10%%): %q", len(lines), lines)
	}
	if last := lines[len(lines)-1]; !strings.HasSuffix(last, "100/100 objects (100%)") {
		t.Errorf("last progress line = %q, want it to end at 100/100", last)
	}
}
