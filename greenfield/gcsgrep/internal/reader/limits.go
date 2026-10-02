package reader

import (
	"errors"
	"io"
)

// Errors a limitedReader reports when it stops a read because a size
// guardrail was exceeded.
var (
	// errObjectLimit: the object has more content than BR-4 allows.
	errObjectLimit = errors.New("object size limit reached")
	// errTotalLimit: the run has read more content than BR-5 allows.
	errTotalLimit = errors.New("total size limit reached")
)

// Budget counts the bytes read across a whole run, to enforce BR-5. The zero
// limit disables it. A Budget is not safe for concurrent use: Iteration 3
// (FR-13) makes it thread-safe when it adds workers.
type Budget struct {
	limit int64
	used  int64
}

// NewBudget returns a Budget that allows limit bytes in total (0 means no
// limit).
func NewBudget(limit int64) *Budget {
	return &Budget{limit: limit}
}

// remaining is how many more bytes the run may read, and whether there is a
// limit at all.
func (b *Budget) remaining() (int64, bool) {
	if b == nil || b.limit <= 0 {
		return 0, false
	}
	return max(b.limit-b.used, 0), true
}

func (b *Budget) add(n int64) {
	if b != nil {
		b.used += n
	}
}

// limitedReader passes bytes through until the object limit (BR-4) or the
// run budget (BR-5) is used up. Reaching a limit exactly at the end of the
// content is fine; only content beyond the limit is an error, and it is
// detected by peeking one byte (which is not counted as read).
type limitedReader struct {
	src          io.Reader
	objRemaining int64 // bytes left under BR-4
	objLimited   bool
	budget       *Budget
}

// newLimitedReader wraps src with a per-object limit (0 means none) and the
// run budget (nil means none).
func newLimitedReader(src io.Reader, maxObjectSize int64, budget *Budget) *limitedReader {
	return &limitedReader{
		src:          src,
		objRemaining: maxObjectSize,
		objLimited:   maxObjectSize > 0,
		budget:       budget,
	}
}

func (l *limitedReader) Read(p []byte) (int, error) {
	allowed := int64(len(p))
	totalLeft, totalLimited := l.budget.remaining()
	if totalLimited {
		allowed = min(allowed, totalLeft)
	}
	if l.objLimited {
		allowed = min(allowed, l.objRemaining)
	}

	if allowed == 0 && len(p) > 0 {
		return 0, l.peekBeyondLimit(totalLimited && totalLeft == 0)
	}

	n, err := l.src.Read(p[:allowed])
	l.objRemaining -= int64(n)
	l.budget.add(int64(n))
	return n, err
}

// peekBeyondLimit is called when a limit is used up: it reports which limit
// was exceeded if the source still has content, or the source's own end or
// error if it does not. The run budget wins when both are used up, because
// it also stops the rest of the run.
func (l *limitedReader) peekBeyondLimit(totalUsedUp bool) error {
	var probe [1]byte
	n, err := l.src.Read(probe[:])
	if n > 0 {
		if totalUsedUp {
			return errTotalLimit
		}
		return errObjectLimit
	}
	if err == nil {
		err = io.ErrNoProgress
	}
	return err
}
