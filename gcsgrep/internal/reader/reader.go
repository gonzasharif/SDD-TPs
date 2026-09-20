// Package reader processes a single GCS object by streaming: it never reads
// the whole object into memory (NFR-2), detects binaries before matching
// anything (FR-11), and refuses to partially match a line that exceeds the
// configured buffer (FR-15) — see the package-level comment on readLine for
// why partial matching would be a correctness bug, not just a memory one.
package reader

import (
	"bufio"
	"bytes"
	"fmt"
	"io"

	"gcsgrep/internal/match"
)

const (
	// sniffSize is how many leading bytes we inspect for a null byte before
	// deciding an object is binary (BR-4 in the original draft, now FR-11).
	sniffSize = 8192
	// chunkSize is the underlying read buffer size; memory use per object
	// stays bounded by this plus MaxLineSize, never by the object's total
	// size (NFR-2).
	chunkSize = 64 * 1024
	// DefaultMaxLineSize is the line buffer cap used when Options.MaxLineSize
	// is left at zero (FR-15's --max-line-size default of 1 MiB).
	DefaultMaxLineSize = 1 << 20
)

// LineMatch is one matching line found in an object.
type LineMatch struct {
	LineNum int
	Text    string
}

// ObjectResult is the outcome of processing a single object. Exactly one of
// Failed, Skipped, or "matched normally" describes what happened; they are
// mutually exclusive per object.
type ObjectResult struct {
	Object  string
	Matches []LineMatch

	// Skipped means the object was deliberately not searched (it's binary).
	// This is expected filtering, not an error — FR-8 does not treat it as
	// a reason to exit 2.
	Skipped    bool
	SkipReason string

	// Failed means the object could not be read at all (permissions,
	// corruption, I/O error). FR-9 requires the run to continue past it;
	// FR-8 requires it to still force exit code 2 at the end.
	Failed     bool
	FailReason string

	// LongLineWarn is true if at least one line exceeded MaxLineSize and
	// was skipped whole rather than matched partially (FR-15).
	LongLineWarn bool
}

// Options configures how a single object is processed.
type Options struct {
	// MaxLineSize caps how many bytes of a single line are buffered before
	// it's treated as "too long" and skipped whole. Zero means
	// DefaultMaxLineSize.
	MaxLineSize int
}

// ProcessObject reads stream line by line, skips it whole if it looks
// binary, matches every line against m, and reports lines that had to be
// skipped for exceeding the line buffer. It never returns an error for
// per-object problems — those are reported through the returned
// ObjectResult so the caller (scanner) can apply FR-9's "continue past a
// bad object" policy uniformly.
func ProcessObject(stream io.Reader, objectName string, m *match.Matcher, opts Options) ObjectResult {
	res := ObjectResult{Object: objectName}

	maxLineSize := opts.MaxLineSize
	if maxLineSize <= 0 {
		maxLineSize = DefaultMaxLineSize
	}

	isBinary, combined, err := sniffBinary(stream)
	if err != nil {
		res.Failed = true
		res.FailReason = fmt.Sprintf("could not read the object: %v", err)
		return res
	}
	if isBinary {
		res.Skipped = true
		res.SkipReason = "binary object (null byte found in the first 8 KiB)"
		return res
	}

	br := bufio.NewReaderSize(combined, chunkSize)
	lineNum := 0
	for {
		line, truncated, err := readLine(br, maxLineSize)
		if err != nil {
			if err == io.EOF {
				break
			}
			res.Failed = true
			res.FailReason = fmt.Sprintf("error reading the object: %v", err)
			return res
		}
		lineNum++
		if truncated {
			res.LongLineWarn = true
			continue
		}
		if m.MatchString(line) {
			res.Matches = append(res.Matches, LineMatch{LineNum: lineNum, Text: line})
		}
	}
	return res
}

// sniffBinary peeks at the first sniffSize bytes of r looking for a null
// byte, then reconstructs the full stream (peeked bytes + the rest of r) so
// the caller can still read from the beginning regardless of the verdict.
func sniffBinary(r io.Reader) (isBinary bool, combined io.Reader, err error) {
	buf := make([]byte, sniffSize)
	n, err := io.ReadFull(r, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return false, nil, err
	}
	peeked := buf[:n]
	isBinary = bytes.IndexByte(peeked, 0) >= 0
	combined = io.MultiReader(bytes.NewReader(peeked), r)
	return isBinary, combined, nil
}

// readLine returns the next line from br (without its trailing newline).
//
// If the line is longer than maxLineSize, it is NOT truncated and matched
// against the buffered prefix — that would risk splitting a real match
// right at the cut point and silently missing it (the false-negative bug
// FR-15 exists to prevent). Instead every byte of that line is consumed and
// discarded, and readLine reports truncated=true with an empty line: the
// caller must not attempt to match it.
//
// io.EOF is returned once there is no more data at all. A final line with
// no trailing newline is still returned as a complete line, matching
// grep's behavior.
func readLine(br *bufio.Reader, maxLineSize int) (line string, truncated bool, err error) {
	var buf []byte
	sawAnyByte := false

	for {
		b, readErr := br.ReadByte()
		if readErr != nil {
			if !sawAnyByte {
				return "", false, io.EOF
			}
			if truncated {
				return "", true, nil
			}
			return string(buf), false, nil
		}
		sawAnyByte = true

		if b == '\n' {
			if truncated {
				return "", true, nil
			}
			return string(buf), false, nil
		}

		if !truncated {
			if len(buf) >= maxLineSize {
				truncated = true
				buf = nil // stop holding the oversized prefix; it's never used
			} else {
				buf = append(buf, b)
			}
		}
	}
}
