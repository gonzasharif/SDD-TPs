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
