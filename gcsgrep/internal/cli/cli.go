// Package cli parses gcsgrep's argv into a structured Args value. The
// surface is: gcsgrep [-i] [-n] [-l | -c] [-j N | --concurrency N]
// [--max N] [--max-object-size SIZE] [--max-total-size SIZE]
// [--max-line-size SIZE] PATTERN gs://bucket[/prefix].
package cli

import (
	"flag"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"gcsgrep/internal/reader"
	"gcsgrep/internal/scanner"
)

// Args is gcsgrep's parsed command line.
type Args struct {
	Pattern    string
	Bucket     string
	Prefix     string
	IgnoreCase bool
	MaxObjects int
	// ListOnly (-l, FR-5) and CountOnly (-c, FR-6) are mutually exclusive
	// (FR-7): Parse never returns both set.
	ListOnly  bool
	CountOnly bool
	// Size limits in bytes: BR-4 per object, BR-5 per run (0 disables
	// either), and FR-15's line buffer (always > 0).
	MaxObjectSize int64
	MaxTotalSize  int64
	MaxLineSize   int64
	// Concurrency is FR-13's worker count: 1 (the default) up to
	// scanner.MaxConcurrency. Parse never returns a value outside that
	// range (FR-14, BR-6).
	Concurrency int
}

const usage = "usage: gcsgrep [-i] [-l | -c] [-j N | --concurrency N] [--max N] [--max-object-size SIZE] [--max-total-size SIZE] [--max-line-size SIZE] PATTERN gs://bucket/prefix"

// Parse parses argv (os.Args[1:]) into Args. On a usage error it returns a
// message already meant for the user, matching the rest of gcsgrep's
// diagnostics.
func Parse(argv []string) (Args, error) {
	fs := flag.NewFlagSet("gcsgrep", flag.ContinueOnError)
	fs.SetOutput(discard{})

	ignoreCase := fs.Bool("i", false, "case-insensitive search")
	// -n is accepted as a silent no-op: the default output format already
	// always includes the line number (decision recorded in
	// gcsgrep-requirements.md, "Decisiones tomadas" #4).
	fs.Bool("n", true, "show line numbers (always on)")
	listOnly := fs.Bool("l", false, "print only the names of objects with at least one match")
	countOnly := fs.Bool("c", false, "print the number of matching lines per object")
	maxObjects := fs.Int("max", scanner.DefaultMaxObjects, "cap on the number of objects to scan under the prefix (0 disables the guardrail)")
	maxObjectSize := sizeValue(scanner.DefaultMaxObjectSize)
	fs.Var(&maxObjectSize, "max-object-size", "cap on the bytes read from a single object, decompressed (0 disables it)")
	maxTotalSize := sizeValue(scanner.DefaultMaxTotalSize)
	fs.Var(&maxTotalSize, "max-total-size", "cap on the bytes read in the whole run, decompressed (0 disables it)")
	maxLineSize := sizeValue(reader.DefaultMaxLineSize)
	fs.Var(&maxLineSize, "max-line-size", "longest line that is matched; longer lines are skipped")
	// -j is an alias of --concurrency: both write the same variable.
	concurrency := scanner.DefaultConcurrency
	fs.IntVar(&concurrency, "concurrency", scanner.DefaultConcurrency, "number of objects read in parallel (1-32)")
	fs.IntVar(&concurrency, "j", scanner.DefaultConcurrency, "alias of --concurrency")

	if err := fs.Parse(argv); err != nil {
		return Args{}, fmt.Errorf("%s (%v)", usage, err)
	}

	// FR-7: rejected here, before main ever builds a GCS client, so a
	// usage error can never cost a single API call (VC-7).
	if *listOnly && *countOnly {
		return Args{}, fmt.Errorf("-l and -c are mutually exclusive; %s", usage)
	}

	// FR-14 / BR-6: like FR-7, rejected before main builds a GCS client,
	// so an out-of-range value never costs an API call (VC-14).
	if concurrency < 1 || concurrency > scanner.MaxConcurrency {
		return Args{}, fmt.Errorf("--concurrency must be between 1 and %d, got %d; %s", scanner.MaxConcurrency, concurrency, usage)
	}

	// A zero line buffer can't hold any line, so unlike the other two
	// limits it can't mean "disabled".
	if maxLineSize <= 0 {
		return Args{}, fmt.Errorf("--max-line-size must be greater than 0; %s", usage)
	}

	rest := fs.Args()
	if len(rest) != 2 {
		return Args{}, fmt.Errorf("%s", usage)
	}

	bucket, prefix, err := parseLocation(rest[1])
	if err != nil {
		return Args{}, err
	}

	return Args{
		Pattern:    rest[0],
		Bucket:     bucket,
		Prefix:     prefix,
		IgnoreCase: *ignoreCase,
		MaxObjects: *maxObjects,
		ListOnly:   *listOnly,
		CountOnly:  *countOnly,

		MaxObjectSize: int64(maxObjectSize),
		MaxTotalSize:  int64(maxTotalSize),
		MaxLineSize:   int64(maxLineSize),
		Concurrency:   concurrency,
	}, nil
}

// parseLocation parses gs://bucket/prefix (FR-1, FR-2). Only the gs://
// scheme is accepted — no bucket/prefix shorthand (decision #3 in
// gcsgrep-requirements.md, to leave the door open for other providers
// later without ambiguity). A path with no trailing slash is a name
// prefix, not a "folder" — GCS has no real folders, and gcsclient.List
// forwards it as-is to the Objects query.
func parseLocation(location string) (bucket, prefix string, err error) {
	if !strings.HasPrefix(location, "gs://") {
		return "", "", fmt.Errorf("invalid location %q: must start with gs://", location)
	}
	u, err := url.Parse(location)
	if err != nil {
		return "", "", fmt.Errorf("invalid location %q: %v", location, err)
	}
	if u.Host == "" {
		return "", "", fmt.Errorf("invalid location %q: missing bucket name", location)
	}
	return u.Host, strings.TrimPrefix(u.Path, "/"), nil
}

// discard suppresses flag's default usage output to stderr; cli.Parse
// returns its own error instead.
type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

// sizeValue is a flag.Value for byte sizes: a plain number of bytes, or a
// number with a binary suffix (k/KiB, m/MiB, g/GiB, case-insensitive),
// e.g. "250MiB" or "10k".
type sizeValue int64

var sizeSuffixes = []struct {
	suffix     string
	multiplier int64
}{
	// Longest suffixes first, so "kib" is not read as "k" + "ib".
	{"kib", 1 << 10}, {"mib", 1 << 20}, {"gib", 1 << 30},
	{"k", 1 << 10}, {"m", 1 << 20}, {"g", 1 << 30},
}

func (v *sizeValue) String() string { return strconv.FormatInt(int64(*v), 10) }

func (v *sizeValue) Set(s string) error {
	number, multiplier := strings.ToLower(strings.TrimSpace(s)), int64(1)
	for _, sfx := range sizeSuffixes {
		if strings.HasSuffix(number, sfx.suffix) {
			number, multiplier = strings.TrimSuffix(number, sfx.suffix), sfx.multiplier
			break
		}
	}
	n, err := strconv.ParseInt(number, 10, 64)
	if err != nil || n < 0 {
		return fmt.Errorf("invalid size %q: want a number of bytes, optionally with a KiB/MiB/GiB suffix", s)
	}
	if n > (1<<63-1)/multiplier {
		return fmt.Errorf("invalid size %q: too large", s)
	}
	*v = sizeValue(n * multiplier)
	return nil
}
