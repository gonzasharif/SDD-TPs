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
// in gcsgrep-design.md). A plain literal string like "timeout" is a
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

// Span is the byte range [Start, End) of one match inside a line.
type Span struct {
	Start, End int
}

// Spans returns every non-overlapping, non-empty match of the pattern in
// line, in order (FR-3.2 highlights each of them). An empty match has
// nothing to highlight, so it is left out.
func (m *Matcher) Spans(line string) []Span {
	var spans []Span
	for _, loc := range m.re.FindAllStringIndex(line, -1) {
		if loc[1] > loc[0] {
			spans = append(spans, Span{Start: loc[0], End: loc[1]})
		}
	}
	return spans
}
