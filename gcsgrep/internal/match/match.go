// Package match applies gcsgrep's search pattern to a single line of text.
// It has no I/O so it can be tested without touching GCS (FR-4).
package match

import "regexp"

// Matcher tests lines against a compiled pattern.
type Matcher struct {
	re *regexp.Regexp
}

// New compiles pattern as an RE2 regular expression (Go's regexp package is
// RE2-based: no backtracking, matching the "literal + regex básica" decision
// in gcsgrep-requirements.md). A plain literal string like "timeout" is a
// valid RE2 pattern that matches itself, so no separate literal mode is
// needed. If ignoreCase is set, the match is case-insensitive (FR-4, `-i`).
func New(pattern string, ignoreCase bool) (*Matcher, error) {
	p := pattern
	if ignoreCase {
		p = "(?i)" + p
	}
	re, err := regexp.Compile(p)
	if err != nil {
		return nil, err
	}
	return &Matcher{re: re}, nil
}

// MatchString reports whether line contains a match for the pattern.
func (m *Matcher) MatchString(line string) bool {
	return m.re.MatchString(line)
}
