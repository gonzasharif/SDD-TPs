// Package cli parses gcsgrep's argv into a structured Args value. Iteration
// 1's surface is: gcsgrep [-i] [-n] [--max N] PATTERN gs://bucket[/prefix].
package cli

import (
	"flag"
	"fmt"
	"net/url"
	"strings"

	"gcsgrep/internal/scanner"
)

// Args is gcsgrep's parsed command line.
type Args struct {
	Pattern    string
	Bucket     string
	Prefix     string
	IgnoreCase bool
	MaxObjects int
}

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
	maxObjects := fs.Int("max", scanner.DefaultMaxObjects, "cap on the number of objects to scan under the prefix (0 disables the guardrail)")

	if err := fs.Parse(argv); err != nil {
		return Args{}, fmt.Errorf("usage: gcsgrep [-i] [--max N] PATTERN gs://bucket/prefix (%v)", err)
	}

	rest := fs.Args()
	if len(rest) != 2 {
		return Args{}, fmt.Errorf("usage: gcsgrep [-i] [--max N] PATTERN gs://bucket/prefix")
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
