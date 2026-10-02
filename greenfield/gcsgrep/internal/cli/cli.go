// Package cli parses gcsgrep's argv into a structured Args value. Iteration
// 1's surface is: gcsgrep [-i] [-n] [--max N] PATTERN gs://bucket[/prefix].
//
// Every error Parse returns is a usage error (exit code 2) whose message is
// meant to follow the "gcsgrep: error: " prefix (FR-16.1, FR-22).
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"strconv"
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
	// MaxObjectSize and MaxTotalSize are BR-4's and BR-5's limits in bytes
	// (0 disables them).
	MaxObjectSize int64
	MaxTotalSize  int64
}

// Parse parses argv (os.Args[1:]) into Args.
func Parse(argv []string) (Args, error) {
	fs := flag.NewFlagSet("gcsgrep", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	ignoreCase := fs.Bool("i", false, "case-insensitive search")
	// -n is accepted with no effect: the output format always includes the
	// line number (FR-20, decision #4 in gcsgrep-design.md).
	fs.Bool("n", true, "show line numbers (always on)")
	maxObjects := &nonNegativeInt{flagName: "--max", value: scanner.DefaultMaxObjects}
	fs.Var(maxObjects, "max", "cap on the number of objects to scan under the prefix (0 disables the guardrail)")
	maxObjectSize := &nonNegativeInt{flagName: "--max-object-size", value: scanner.DefaultMaxObjectSize}
	fs.Var(maxObjectSize, "max-object-size", "cap on the bytes read from one object, in bytes (0 disables the guardrail)")
	maxTotalSize := &nonNegativeInt{flagName: "--max-total-size", value: scanner.DefaultMaxTotalSize}
	fs.Var(maxTotalSize, "max-total-size", "cap on the bytes read in the whole run, in bytes (0 disables the guardrail)")

	if err := fs.Parse(argv); err != nil {
		return Args{}, translateFlagError(err, maxObjects, maxObjectSize, maxTotalSize)
	}

	rest := fs.Args()
	if len(rest) != 2 {
		return Args{}, fmt.Errorf("expected 2 arguments (PATTERN and LOCATION), got %d", len(rest)) // FR-22.2
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
		MaxObjects: int(maxObjects.value),

		MaxObjectSize: maxObjectSize.value,
		MaxTotalSize:  maxTotalSize.value,
	}, nil
}

// parseLocation parses gs://bucket/prefix (FR-1, FR-2). Only the gs://
// scheme is accepted — no bucket/prefix shorthand (FR-16.1, decision #3 in
// gcsgrep-design.md). A prefix with no trailing slash is a name prefix, not
// a "folder" (FR-18): GCS has no real folders, and gcsclient.List forwards
// it as-is to the listing query.
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

// translateFlagError turns the flag package's errors into gcsgrep's usage
// messages (FR-22). Value errors are taken from the flag values themselves,
// because the flag package rewrites the flag name ("-max" instead of the
// "--max" the user typed).
func translateFlagError(err error, values ...*nonNegativeInt) error {
	for _, v := range values {
		if v.err != nil {
			return v.err // FR-22.3, FR-22.4
		}
	}
	if errors.Is(err, flag.ErrHelp) {
		return errors.New("unknown flag -h")
	}
	const undefined = "flag provided but not defined: "
	if msg := err.Error(); strings.HasPrefix(msg, undefined) {
		return fmt.Errorf("unknown flag %s", strings.TrimPrefix(msg, undefined)) // FR-22.1
	}
	return err
}

// nonNegativeInt is a flag.Value for the limit flags (--max,
// --max-object-size, --max-total-size) that accepts integers >= 0 and
// records a spec-formatted error otherwise.
type nonNegativeInt struct {
	flagName string
	value    int64
	err      error
}

func (n *nonNegativeInt) String() string { return strconv.FormatInt(n.value, 10) }

func (n *nonNegativeInt) Set(s string) error {
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		n.err = fmt.Errorf("invalid value %q for %s: must be an integer", s, n.flagName)
		return n.err
	}
	if v < 0 {
		n.err = fmt.Errorf("invalid value %q for %s: must be >= 0", s, n.flagName)
		return n.err
	}
	n.value = v
	return nil
}
