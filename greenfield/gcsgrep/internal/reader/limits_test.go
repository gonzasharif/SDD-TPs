package reader

import (
	"bytes"
	"strings"
	"testing"
)

const miB = 1 << 20

// infoLines is n bytes of 'INFO\n' lines (n must be a multiple of 5).
func infoLines(n int) string { return strings.Repeat("INFO\n", n/5) }

// BR-4: reading stops at the limit; matches already emitted stay.
func TestProcessObject_ObjectSizeLimit_CutsTheObject(t *testing.T) {
	content := "timeout\n" + infoLines(2*miB)

	res, got := process(t, strings.NewReader(content), "timeout", Options{MaxObjectSize: miB})

	if !res.Cut || res.Failed || res.ScanIncomplete {
		t.Errorf("res = %+v, want only Cut", res)
	}
	if len(got) != 1 || got[0].Text != "timeout" {
		t.Errorf("matches = %+v, want the line-1 match kept", got)
	}
}

// BR-4: content that is exactly as large as the limit is read completely.
func TestProcessObject_ObjectSizeLimit_ExactSizeIsNotCut(t *testing.T) {
	content := strings.Repeat("x", miB-1) + "\n" // exactly 1 MiB

	res, _ := process(t, strings.NewReader(content), "timeout", Options{MaxObjectSize: int64(len(content))})

	if res.Cut || res.Failed {
		t.Errorf("res = %+v, want the object read completely", res)
	}
}

// BR-4: one byte more than the limit is a cut.
func TestProcessObject_ObjectSizeLimit_OneByteOverIsCut(t *testing.T) {
	content := "timeout\nINFO\n"

	res, _ := process(t, strings.NewReader(content), "timeout", Options{MaxObjectSize: int64(len(content)) - 1})

	if !res.Cut {
		t.Errorf("res = %+v, want Cut", res)
	}
}

// `--max-object-size 0` disables BR-4.
func TestProcessObject_ObjectSizeLimit_ZeroDisablesIt(t *testing.T) {
	res, _ := process(t, strings.NewReader(infoLines(2*miB)), "timeout", Options{MaxObjectSize: 0})

	if res.Cut || res.Failed {
		t.Errorf("res = %+v, want no limit", res)
	}
}

// BR-4 / VC-18: the limit counts decompressed bytes, so a small .gz that
// expands is cut.
func TestProcessObject_ObjectSizeLimit_CountsDecompressedBytes(t *testing.T) {
	data := gz(t, infoLines(2*miB))
	if len(data) >= miB {
		t.Fatalf("test needs a .gz smaller than the limit, got %d bytes", len(data))
	}

	res, _ := process(t, bytes.NewReader(data), "timeout", Options{Gzip: true, MaxObjectSize: miB})

	if !res.Cut {
		t.Errorf("res = %+v, want Cut", res)
	}
}

// BR-5: the budget is shared by every object of the run, and the object
// that runs it out is cut and reported as ScanIncomplete.
func TestProcessObject_TotalBudget_SharedAcrossObjects(t *testing.T) {
	budget := NewBudget(5 * miB / 2) // 2.5 MiB
	obj := func() string { return "timeout\n" + infoLines(miB-8-(miB-8)%5) }

	var results []ObjectResult
	for i := 0; i < 3; i++ {
		res, _ := process(t, strings.NewReader(obj()), "timeout", Options{Budget: budget})
		results = append(results, res)
	}

	if results[0].ScanIncomplete || results[1].ScanIncomplete {
		t.Errorf("the first two objects fit in the budget: %+v, %+v", results[0], results[1])
	}
	if !results[2].ScanIncomplete || results[2].Cut {
		t.Errorf("third object = %+v, want ScanIncomplete (and not Cut)", results[2])
	}
	if results[2].MatchCount != 1 {
		t.Errorf("the match on line 1 of the third object should have been emitted")
	}
}

// BR-5: a zero limit disables the budget; so does having none.
func TestProcessObject_TotalBudget_ZeroDisablesIt(t *testing.T) {
	for _, b := range []*Budget{NewBudget(0), nil} {
		res, _ := process(t, strings.NewReader(infoLines(2*miB)), "timeout", Options{Budget: b})
		if res.ScanIncomplete || res.Failed {
			t.Errorf("budget %v: res = %+v, want no limit", b, res)
		}
	}
}

// BR-5: using exactly the budget is not exceeding it.
func TestProcessObject_TotalBudget_ExactUseIsNotIncomplete(t *testing.T) {
	content := "timeout\nINFO\n"

	res, _ := process(t, strings.NewReader(content), "timeout", Options{Budget: NewBudget(int64(len(content)))})

	if res.ScanIncomplete || res.Failed {
		t.Errorf("res = %+v, want the object read completely", res)
	}
}

// BR-5 wins when both limits are used up at the same byte: it also stops the
// rest of the run.
func TestProcessObject_TotalBudgetWinsOverObjectLimit(t *testing.T) {
	content := "timeout\nINFO\n"
	limit := int64(len(content)) - 1

	res, _ := process(t, strings.NewReader(content), "timeout", Options{MaxObjectSize: limit, Budget: NewBudget(limit)})

	if !res.ScanIncomplete || res.Cut {
		t.Errorf("res = %+v, want ScanIncomplete only", res)
	}
}
