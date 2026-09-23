package output

import (
	"fmt"
	"strings"
)

// ProgressStyle selects how FR-10's progress is shown on stderr.
type ProgressStyle int

const (
	// ProgressOff shows nothing (the zero value, used by tests).
	ProgressOff ProgressStyle = iota
	// ProgressBar redraws a single bar line in place with \r, for when
	// stderr is a terminal.
	ProgressBar
	// ProgressLines prints a plain line every 10% of the objects, for
	// when stderr is a file or a pipe: no \r, so a log stays readable.
	ProgressLines
)

const barWidth = 30

// progress is the Writer's FR-10 state. Like Budget, it is not safe for
// concurrent use until Iteration 3.
type progress struct {
	total, done int
	lastStep    int  // last 10% step printed in ProgressLines
	lastPrinted int  // done count of the last line printed in ProgressLines
	barShown    bool // a bar is currently drawn on stderr's last line
}

// StartProgress begins tracking a run over total objects. With a single
// object (or none) there is nothing worth tracking, so nothing is shown.
func (w *Writer) StartProgress(total int) {
	w.progress = progress{total: total}
	w.drawBar()
}

// AdvanceProgress records one more processed object.
func (w *Writer) AdvanceProgress() {
	if !w.progressActive() {
		return
	}
	w.progress.done++
	switch w.Progress {
	case ProgressBar:
		w.drawBar()
	case ProgressLines:
		if step := w.progress.done * 10 / w.progress.total; step > w.progress.lastStep {
			w.progress.lastStep = step
			w.printProgressLine()
		}
	}
}

// FinishProgress ends the progress display. The bar is left in place,
// completed by a newline; in ProgressLines, a run that stopped early
// (BR-5) still gets a final line showing where it stopped.
func (w *Writer) FinishProgress() {
	if !w.progressActive() {
		return
	}
	switch w.Progress {
	case ProgressBar:
		if w.progress.barShown {
			fmt.Fprintln(w.Stderr)
			w.progress.barShown = false
		}
	case ProgressLines:
		if w.progress.done != w.progress.lastPrinted || w.progress.done == 0 {
			w.printProgressLine()
		}
	}
	w.progress = progress{}
}

func (w *Writer) progressActive() bool {
	return w.Progress != ProgressOff && w.progress.total > 1
}

func (w *Writer) percent() int {
	return w.progress.done * 100 / w.progress.total
}

func (w *Writer) printProgressLine() {
	w.progress.lastPrinted = w.progress.done
	fmt.Fprintf(w.Stderr, "gcsgrep: progress: %d/%d objects (%d%%)\n", w.progress.done, w.progress.total, w.percent())
}

func (w *Writer) drawBar() {
	if w.Progress != ProgressBar || !w.progressActive() {
		return
	}
	filled := w.progress.done * barWidth / w.progress.total
	fmt.Fprintf(w.Stderr, "\r\x1b[Kgcsgrep: [%s%s] %3d%% (%d/%d objects)",
		strings.Repeat("#", filled), strings.Repeat("-", barWidth-filled),
		w.percent(), w.progress.done, w.progress.total)
	w.progress.barShown = true
}

// clearBar erases the bar before anything else is written, so a result on
// stdout or a warning on stderr never ends up glued to it on the same
// terminal line. The caller redraws it afterwards.
func (w *Writer) clearBar() {
	if w.progress.barShown {
		fmt.Fprint(w.Stderr, "\r\x1b[K")
		w.progress.barShown = false
	}
}
