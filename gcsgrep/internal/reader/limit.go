package reader

import (
	"errors"
	"io"
	"sync/atomic"
)

// ErrObjectSizeLimit and ErrTotalSizeLimit are returned by a limitedReader
// when the next byte would cross BR-4's per-object limit or BR-5's run-wide
// limit, respectively.
var (
	ErrObjectSizeLimit = errors.New("per-object size limit reached")
	ErrTotalSizeLimit  = errors.New("total size limit for the run reached")
)

// Budget is BR-5's run-wide byte allowance, shared by every object a run
// reads, and safe for concurrent use by the scanner's workers.
//
// Workers never read first and subtract afterwards: that would let N
// workers all see the same remaining allowance and each read past it. They
// reserve bytes before reading (reserve) and hand back what a read did not
// use (refund), so the sum of bytes actually read can never exceed the
// limit, however the workers interleave.
//
// One consequence: while a worker holds a reservation that a small final
// read will partly refund, the allowance looks smaller than it really is.
// That can only make a run stop one read early, never late, and only within
// a single chunk of the limit.
type Budget struct {
	remaining atomic.Int64
}

// NewBudget returns a Budget allowing limit bytes in total. A nil *Budget
// means no run-wide limit, which is what callers pass for --max-total-size 0.
func NewBudget(limit int64) *Budget {
	b := &Budget{}
	b.remaining.Store(limit)
	return b
}

// Exhausted reports whether no bytes are left to reserve.
func (b *Budget) Exhausted() bool {
	return b != nil && b.remaining.Load() <= 0
}

// reserve takes up to n bytes of the allowance and returns how many it got
// (0 when the budget is spent). The compare-and-swap loop makes the check
// and the subtraction a single atomic step.
func (b *Budget) reserve(n int64) int64 {
	for {
		cur := b.remaining.Load()
		if cur <= 0 {
			return 0
		}
		take := min(n, cur)
		if b.remaining.CompareAndSwap(cur, cur-take) {
			return take
		}
	}
}

// refund gives back n reserved bytes that were not read.
func (b *Budget) refund(n int64) {
	if n > 0 {
		b.remaining.Add(n)
	}
}

// limitedReader passes bytes through until reading more would cross the
// per-object limit (objRemaining, negative = unlimited) or the run's
// budget (nil = unlimited). Hitting a limit exactly at the end of the
// object is not an error; only content past the limit is.
//
// A limitedReader belongs to a single object and a single goroutine; only
// the Budget it draws from is shared.
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
	if allowed <= 0 && len(p) > 0 {
		return 0, l.limitReached()
	}

	if l.budget != nil {
		allowed = l.budget.reserve(allowed)
		if allowed <= 0 && len(p) > 0 {
			return 0, l.limitReached()
		}
	}

	n, err := l.r.Read(p[:allowed])
	if l.objRemaining >= 0 {
		l.objRemaining -= int64(n)
	}
	if l.budget != nil {
		l.budget.refund(allowed - int64(n))
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
