package cli

import (
	"strings"
	"testing"
)

// FR-13: sequential by default; -j and --concurrency are two spellings of
// the same flag.
func TestParse_ConcurrencyDefaultAndAlias(t *testing.T) {
	cases := []struct {
		name string
		argv []string
		want int
	}{
		{"default is sequential", []string{"x", "gs://b/"}, 1},
		{"long flag", []string{"--concurrency", "8", "x", "gs://b/"}, 8},
		{"short alias", []string{"-j", "8", "x", "gs://b/"}, 8},
		{"lower bound", []string{"-j", "1", "x", "gs://b/"}, 1},
		{"upper bound", []string{"--concurrency", "32", "x", "gs://b/"}, 32},
		{"combined with other flags", []string{"-i", "-l", "-j", "4", "--max", "0", "x", "gs://b/"}, 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args, err := Parse(tc.argv)
			if err != nil {
				t.Fatalf("Parse(%v): %v", tc.argv, err)
			}
			if args.Concurrency != tc.want {
				t.Errorf("Concurrency = %d, want %d", args.Concurrency, tc.want)
			}
		})
	}
}

// FR-14 / BR-6 / VC-14 / VC-20: anything outside 1..32 is a usage error.
// Parse fails before main builds the GCS client, so no API call is made.
func TestParse_ConcurrencyOutOfRangeIsRejected(t *testing.T) {
	for _, value := range []string{"33", "100", "0", "-1", "abc"} {
		for _, flagName := range []string{"--concurrency", "-j"} {
			t.Run(flagName+" "+value, func(t *testing.T) {
				_, err := Parse([]string{flagName, value, "x", "gs://b/"})
				if err == nil {
					t.Fatalf("%s %s should be rejected", flagName, value)
				}
			})
		}
	}
}

// The error names the flag and the allowed range, so the user knows what
// to change.
func TestParse_ConcurrencyErrorExplainsTheRange(t *testing.T) {
	_, err := Parse([]string{"--concurrency", "100", "x", "gs://b/"})
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"--concurrency", "1 and 32", "100"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should mention %q", err, want)
		}
	}
}
