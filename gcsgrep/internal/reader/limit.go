package reader

import (
	"errors"
	"io"
)

// ErrObjectSizeLimit and ErrTotalSizeLimit are returned by a limitedReader
// when the next byte would cross BR-4's per-object limit or BR-5's run-wide
// limit, respectively.
var (
	ErrObjectSizeLimit = errors.New("per-object size limit reached")
	ErrTotalSizeLimit  = errors.New("total size limit for the run reached")
)

// Budget is BR-5's run-wide byte allowance, shared by every object a run
// reads. It is not safe for concurrent use: Iteration 2 reads objects one
// at a time, and Iteration 3's worker pool has to make it atomic.
type Budget struct {
	remaining int64
}

// NewBudget returns a Budget allowing limit bytes in total. A nil *Budget
// means no run-wide limit, which is what callers pass for --max-total-size 0.
func NewBudget(limit int64) *Budget {
	return &Budget{remaining: limit}
}

// Exhausted reports whether no bytes are left to read.
func (b *Budget) Exhausted() bool {
	return b != nil && b.remaining <= 0
}

// limitedReader passes bytes through until reading more would cross the
// per-object limit (objRemaining, negative = unlimited) or the run's
// budget (nil = unlimited). Hitting a limit exactly at the end of the
// object is not an error; only content past the limit is.
type limitedReader struct {
	r            io.Reader
	objRemaining int64
	budget       *Budget
}

func newLimitedReader(r io.Reader, maxObjectSize int64, budget *Budget) *limitedReader {
	objRemaining := int64(-1)
	if maxObjectSize > 0 {
		objRemaining = maxObjectSize
	}
	return &limitedReader{r: r, objRemaining: objRemaining, budget: budget}
}

func (l *limitedReader) Read(p []byte) (int, error) {
	allowed := int64(len(p))
	if l.objRemaining >= 0 {
		allowed = min(allowed, l.objRemaining)
	}
	if l.budget != nil {
		allowed = min(allowed, l.budget.remaining)
	}

	if allowed <= 0 && len(p) > 0 {
		return 0, l.limitReached()
	}

	n, err := l.r.Read(p[:allowed])
	if l.objRemaining >= 0 {
		l.objRemaining -= int64(n)
	}
	if l.budget != nil {
		l.budget.remaining -= int64(n)
	}
	return n, err
}

// limitReached probes the underlying reader for one more byte, to tell an
// object that ends exactly at the limit (io.EOF, fine) from one that goes
// past it (a limit error). The probed byte is discarded: reading stops.
func (l *limitedReader) limitReached() error {
	var probe [1]byte
	for {
		n, err := l.r.Read(probe[:])
		if n > 0 {
			break
		}
		if err != nil {
			return err
		}
	}
	if l.objRemaining == 0 {
		return ErrObjectSizeLimit
	}
	return ErrTotalSizeLimit
}
