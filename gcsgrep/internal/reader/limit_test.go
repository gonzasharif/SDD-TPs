package reader

import (
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/iotest"
)

// infiniteReader yields 'x' forever, so a limit is the only thing that can
// stop a read.
type infiniteReader struct{}

func (infiniteReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'x'
	}
	return len(p), nil
}

// BR-5 under concurrency: however many goroutines reserve at once, the
// bytes handed out add up to exactly the limit, never more. An overshoot can
// only happen in the last few reservations, when several workers see the
// same small remainder, so the test repeats a short race many times with all
// goroutines released together.
func TestBudget_ReserveNeverHandsOutMoreThanTheLimit(t *testing.T) {
	const (
		trials     = 2000
		goroutines = 32
		limit      = 1003 // not a multiple of the chunk
		chunk      = 64
	)
	for trial := range trials {
		budget := NewBudget(limit)
		start := make(chan struct{})
		var granted atomic.Int64
		var wg sync.WaitGroup
		for range goroutines {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				for {
					n := budget.reserve(chunk)
					if n == 0 {
						return
					}
					granted.Add(n)
				}
			}()
		}
		close(start)
		wg.Wait()

		if got := granted.Load(); got != limit {
			t.Fatalf("trial %d: granted %d bytes in total, want exactly %d", trial, got, limit)
		}
		if !budget.Exhausted() {
			t.Fatalf("trial %d: the budget should be exhausted once every byte was handed out", trial)
		}
	}
}

// BR-5 under concurrency, through the real reader: many objects drawing on
// one Budget together read exactly the limit and every one of them ends on
// the total-limit error.
func TestLimitedReader_ConcurrentReadersShareOneBudget(t *testing.T) {
	const limit = 2_000_017
	budget := NewBudget(limit)

	var read atomic.Int64
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lr := newLimitedReader(infiniteReader{}, 0, budget)
			buf := make([]byte, 4096)
			for {
				n, err := lr.Read(buf)
				read.Add(int64(n))
				if err != nil {
					if !errors.Is(err, ErrTotalSizeLimit) {
						t.Errorf("stopped with %v, want ErrTotalSizeLimit", err)
					}
					return
				}
			}
		}()
	}
	wg.Wait()

	if got := read.Load(); got != limit {
		t.Errorf("readers consumed %d bytes together, want exactly %d", got, limit)
	}
}

// A read that returns less than what was reserved gives the difference
// back: otherwise short reads would drain the budget far faster than the
// bytes really read.
func TestLimitedReader_ShortReadsRefundTheReservation(t *testing.T) {
	budget := NewBudget(100)
	lr := newLimitedReader(iotest.OneByteReader(strings.NewReader("0123456789")), 0, budget)

	data, err := io.ReadAll(lr)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(data) != "0123456789" {
		t.Errorf("read %q, want the whole object", data)
	}
	if got := budget.remaining.Load(); got != 90 {
		t.Errorf("remaining = %d, want 90 (only the 10 bytes actually read are spent)", got)
	}
	if budget.Exhausted() {
		t.Errorf("the budget must not look exhausted after a short read")
	}
}
