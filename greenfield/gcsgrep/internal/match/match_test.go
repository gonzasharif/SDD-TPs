package match

import "testing"

// VC-4: case-insensitive search.
func TestMatchString_IgnoreCase(t *testing.T) {
	line := "2026-09-20T10:02:05Z ERROR TIMEOUT while waiting for upstream"

	m, err := New("timeout", true)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if !m.MatchString(line) {
		t.Errorf("with -i, %q should match %q", "timeout", line)
	}

	mCase, err := New("timeout", false)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if mCase.MatchString(line) {
		t.Errorf("without -i, %q should NOT match %q", "timeout", line)
	}
}

func TestMatchString_LiteralIsValidRE2(t *testing.T) {
	m, err := New("timeout", false)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if !m.MatchString("connection timeout after 30s") {
		t.Errorf("a literal search should match a plain substring")
	}
	if m.MatchString("no relation here") {
		t.Errorf("should not match a line without the pattern")
	}
}

func TestNew_InvalidRegex(t *testing.T) {
	if _, err := New("(unclosed", false); err == nil {
		t.Errorf("an invalid regex pattern should fail to compile")
	}
}

// FR-3.2: every non-overlapping match is reported, empty ones are not.
func TestSpans(t *testing.T) {
	m, err := New("a+", false)
	if err != nil {
		t.Fatal(err)
	}
	got := m.Spans("baaab a")
	want := []Span{{1, 4}, {6, 7}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("Spans = %v, want %v", got, want)
	}

	empty, err := New("x*", false)
	if err != nil {
		t.Fatal(err)
	}
	if got := empty.Spans("abc"); len(got) != 0 {
		t.Errorf("Spans of an empty match = %v, want none", got)
	}
}
