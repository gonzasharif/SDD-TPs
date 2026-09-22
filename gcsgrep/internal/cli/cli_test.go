package cli

import "testing"

// FR-1/FR-2: gs://bucket/prefix and gs://bucket/ (no prefix) both parse.
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

func TestParse_RejectsNonGsScheme(t *testing.T) {
	_, err := Parse([]string{"timeout", "logs/app/"})
	if err == nil {
		t.Fatalf("a location without the gs:// scheme should be rejected")
	}
}

func TestParse_RejectsMissingArgs(t *testing.T) {
	if _, err := Parse([]string{"only-one-argument"}); err == nil {
		t.Errorf("should fail with a single positional argument")
	}
	if _, err := Parse([]string{}); err == nil {
		t.Errorf("should fail with no arguments")
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

// BR-3: --max defaults to 1000 and can be overridden, including
// --max 0 to disable it.
func TestParse_MaxObjectsDefaultAndOverride(t *testing.T) {
	args, err := Parse([]string{"timeout", "gs://logs/"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if args.MaxObjects != 1000 {
		t.Errorf("--max default = %d, want 1000", args.MaxObjects)
	}

	args, err = Parse([]string{"--max", "5", "timeout", "gs://logs/"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if args.MaxObjects != 5 {
		t.Errorf("--max 5 was not applied, got %d", args.MaxObjects)
	}

	args, err = Parse([]string{"--max", "0", "timeout", "gs://logs/"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if args.MaxObjects != 0 {
		t.Errorf("--max 0 should disable the guardrail (MaxObjects=0), got %d", args.MaxObjects)
	}
}

// FR-5/FR-6: -l and -c each parse on their own.
func TestParse_ListAndCountFlags(t *testing.T) {
	args, err := Parse([]string{"-l", "timeout", "gs://logs/"})
	if err != nil {
		t.Fatalf("Parse -l: %v", err)
	}
	if !args.ListOnly || args.CountOnly {
		t.Errorf("-l should set only ListOnly: %+v", args)
	}

	args, err = Parse([]string{"-c", "timeout", "gs://logs/"})
	if err != nil {
		t.Fatalf("Parse -c: %v", err)
	}
	if !args.CountOnly || args.ListOnly {
		t.Errorf("-c should set only CountOnly: %+v", args)
	}
}

// VC-7: -l and -c together are a usage error. Parse runs before main
// builds the GCS client, so rejecting here means zero API calls.
func TestParse_ListAndCountAreMutuallyExclusive(t *testing.T) {
	for _, argv := range [][]string{
		{"-l", "-c", "timeout", "gs://logs/"},
		{"-c", "-l", "timeout", "gs://logs/"},
	} {
		if _, err := Parse(argv); err == nil {
			t.Errorf("Parse(%v) should reject -l together with -c", argv)
		}
	}
}

// BR-4/BR-5/FR-15: size flags default to the spec's values and accept
// plain bytes or KiB/MiB/GiB suffixes.
func TestParse_SizeFlags(t *testing.T) {
	args, err := Parse([]string{"timeout", "gs://logs/"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if args.MaxObjectSize != 250<<20 || args.MaxTotalSize != 2<<30 || args.MaxLineSize != 1<<20 {
		t.Errorf("defaults = %d/%d/%d, want 250 MiB / 2 GiB / 1 MiB", args.MaxObjectSize, args.MaxTotalSize, args.MaxLineSize)
	}

	cases := []struct {
		value string
		want  int64
	}{
		{"1234", 1234},
		{"10k", 10 << 10},
		{"10KiB", 10 << 10},
		{"250MiB", 250 << 20},
		{"2g", 2 << 30},
		{"0", 0},
	}
	for _, tc := range cases {
		args, err := Parse([]string{"--max-object-size", tc.value, "timeout", "gs://logs/"})
		if err != nil {
			t.Errorf("--max-object-size %s: %v", tc.value, err)
			continue
		}
		if args.MaxObjectSize != tc.want {
			t.Errorf("--max-object-size %s = %d, want %d", tc.value, args.MaxObjectSize, tc.want)
		}
	}
}

func TestParse_RejectsInvalidSizes(t *testing.T) {
	for _, argv := range [][]string{
		{"--max-total-size", "abc", "timeout", "gs://logs/"},
		{"--max-total-size", "-5", "timeout", "gs://logs/"},
		{"--max-object-size", "10TiB", "timeout", "gs://logs/"},
		{"--max-line-size", "0", "timeout", "gs://logs/"},
	} {
		if _, err := Parse(argv); err == nil {
			t.Errorf("Parse(%v) should fail", argv)
		}
	}
}
