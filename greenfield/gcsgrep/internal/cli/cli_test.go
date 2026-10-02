package cli

import "testing"

// FR-1/FR-2/FR-18: gs://bucket/prefix and gs://bucket/ (no prefix) both
// parse; a prefix without a trailing slash is kept as-is (name filter).
func TestParse_LocationVariants(t *testing.T) {
	cases := []struct {
		name       string
		argv       []string
		wantBucket string
		wantPrefix string
	}{
		{"with prefix", []string{"timeout", "gs://logs/app/"}, "logs", "app/"},
		{"whole bucket, no prefix", []string{"timeout", "gs://logs/"}, "logs", ""},
		{"bucket with no trailing slash", []string{"timeout", "gs://logs"}, "logs", ""},
		{"prefix with no trailing slash (name filter)", []string{"timeout", "gs://logs/app"}, "logs", "app"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args, err := Parse(tc.argv)
			if err != nil {
				t.Fatalf("Parse(%v): %v", tc.argv, err)
			}
			if args.Bucket != tc.wantBucket {
				t.Errorf("bucket = %q, want %q", args.Bucket, tc.wantBucket)
			}
			if args.Prefix != tc.wantPrefix {
				t.Errorf("prefix = %q, want %q", args.Prefix, tc.wantPrefix)
			}
		})
	}
}

// FR-16.1 / FR-22: usage errors carry the exact message the spec defines
// (app prints it after "gcsgrep: error: ").
func TestParse_UsageErrorMessages(t *testing.T) {
	cases := []struct {
		name string
		argv []string
		want string
	}{
		{"FR-16.1 location without gs://", []string{"timeout", "mybucket/logs/"}, `invalid location "mybucket/logs/": must start with gs://`},
		{"FR-22.1 unknown flag", []string{"-v", "timeout", "gs://b/logs/"}, "unknown flag -v"},
		{"FR-22.2 one argument", []string{"timeout"}, "expected 2 arguments (PATTERN and LOCATION), got 1"},
		{"FR-22.2 no arguments", []string{}, "expected 2 arguments (PATTERN and LOCATION), got 0"},
		{"FR-22.2 three arguments", []string{"a", "b", "c"}, "expected 2 arguments (PATTERN and LOCATION), got 3"},
		{"FR-22.3 non-integer --max", []string{"--max", "diez", "timeout", "gs://b/logs/"}, `invalid value "diez" for --max: must be an integer`},
		{"FR-22.4 negative --max", []string{"--max", "-5", "timeout", "gs://b/logs/"}, `invalid value "-5" for --max: must be >= 0`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(tc.argv)
			if err == nil {
				t.Fatalf("Parse(%v) should fail", tc.argv)
			}
			if err.Error() != tc.want {
				t.Errorf("error = %q, want %q", err.Error(), tc.want)
			}
		})
	}
}

func TestParse_IgnoreCaseFlag(t *testing.T) {
	args, err := Parse([]string{"-i", "timeout", "gs://logs/"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !args.IgnoreCase {
		t.Errorf("-i should set IgnoreCase")
	}

	args, err = Parse([]string{"timeout", "gs://logs/"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if args.IgnoreCase {
		t.Errorf("without -i, IgnoreCase should stay false")
	}
}

// FR-20: -n is accepted and produces exactly the same Args as without it.
func TestParse_NFlagHasNoEffect(t *testing.T) {
	with, err := Parse([]string{"-n", "timeout", "gs://logs/app/"})
	if err != nil {
		t.Fatalf("Parse with -n: %v", err)
	}
	without, err := Parse([]string{"timeout", "gs://logs/app/"})
	if err != nil {
		t.Fatalf("Parse without -n: %v", err)
	}
	if with != without {
		t.Errorf("-n changed the parsed arguments: %+v vs %+v", with, without)
	}
}

// BR-3: --max defaults to 1000 and can be overridden, including
// --max 0 to disable it.
func TestParse_MaxObjectsDefaultAndOverride(t *testing.T) {
	cases := []struct {
		argv []string
		want int
	}{
		{[]string{"timeout", "gs://logs/"}, 1000},
		{[]string{"--max", "5", "timeout", "gs://logs/"}, 5},
		{[]string{"--max", "0", "timeout", "gs://logs/"}, 0},
	}
	for _, tc := range cases {
		args, err := Parse(tc.argv)
		if err != nil {
			t.Fatalf("Parse(%v): %v", tc.argv, err)
		}
		if args.MaxObjects != tc.want {
			t.Errorf("Parse(%v).MaxObjects = %d, want %d", tc.argv, args.MaxObjects, tc.want)
		}
	}
}

// BR-4, BR-5: defaults (250 MiB and 2 GiB), overrides, and 0 to disable.
func TestParse_SizeLimitsDefaultAndOverride(t *testing.T) {
	cases := []struct {
		argv       []string
		wantObject int64
		wantTotal  int64
	}{
		{[]string{"timeout", "gs://logs/"}, 262144000, 2147483648},
		{[]string{"--max-object-size", "1048576", "--max-total-size", "2621440", "timeout", "gs://logs/"}, 1048576, 2621440},
		{[]string{"--max-object-size", "0", "--max-total-size", "0", "timeout", "gs://logs/"}, 0, 0},
		{[]string{"--max-total-size", "4294967296", "timeout", "gs://logs/"}, 262144000, 4294967296},
	}
	for _, tc := range cases {
		args, err := Parse(tc.argv)
		if err != nil {
			t.Fatalf("Parse(%v): %v", tc.argv, err)
		}
		if args.MaxObjectSize != tc.wantObject || args.MaxTotalSize != tc.wantTotal {
			t.Errorf("Parse(%v) sizes = %d, %d; want %d, %d", tc.argv, args.MaxObjectSize, args.MaxTotalSize, tc.wantObject, tc.wantTotal)
		}
	}
}

// VC-31.3 and VC-31.4 extended to every limit flag (FR-22.3, FR-22.4).
func TestParse_SizeLimitsRejectInvalidValues(t *testing.T) {
	for _, flagName := range []string{"--max-object-size", "--max-total-size"} {
		cases := []struct {
			value string
			want  string
		}{
			{"diez", `invalid value "diez" for ` + flagName + `: must be an integer`},
			{"1.5", `invalid value "1.5" for ` + flagName + `: must be an integer`},
			{"-5", `invalid value "-5" for ` + flagName + `: must be >= 0`},
		}
		for _, tc := range cases {
			_, err := Parse([]string{flagName, tc.value, "timeout", "gs://logs/"})
			if err == nil || err.Error() != tc.want {
				t.Errorf("Parse(%s %s) error = %v, want %q", flagName, tc.value, err, tc.want)
			}
		}
	}
}
