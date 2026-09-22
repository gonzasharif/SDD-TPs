// Package reader processes a single GCS object by streaming: it never reads
// the whole object into memory (NFR-2), decompresses gzip on the fly
// (FR-12), enforces the per-object and run-wide size limits (BR-4, BR-5),
// detects binaries before matching anything (FR-11), and refuses to partially match a line that exceeds the
// configured buffer (FR-15) — see the package-level comment on readLine for
// why partial matching would be a correctness bug, not just a memory one.
package reader

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"errors"
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

// Mode selects what ProcessObject collects from an object.
type Mode int

const (
	// ModeLines collects every matching line (the default output format).
	ModeLines Mode = iota
	// ModeList stops reading at the first match (FR-5, -l): only whether
	// the object matched matters, so reading the rest would be wasted
	// bytes billed by GCS.
	ModeList
	// ModeCount reads the whole object and counts matching lines without
	// keeping their text (FR-6, -c).
	ModeCount
)

// LineMatch is one matching line found in an object.
type LineMatch struct {
	LineNum int
	Text    string
}

// ObjectResult is the outcome of processing a single object. Skipped means
// nothing was searched; Failed, ObjectSizeLimitHit, and TotalSizeLimitHit
// mean the object was searched only partially (see Incomplete); otherwise
// it was searched in full.
type ObjectResult struct {
	Object string

	// Matches holds every matching line in ModeLines. It stays empty in
	// ModeList and ModeCount, which only need MatchCount.
	Matches []LineMatch
	// MatchCount is the number of matching lines found. In ModeList it is
	// at most 1, since reading stops at the first match.
	MatchCount int

	// Skipped means the object was deliberately not searched (it's binary).
	// This is expected filtering, not an error — FR-8 does not treat it as
	// a reason to exit 2.
	Skipped    bool
	SkipReason string

	// Failed means the object could not be read in full (corruption, an
	// I/O error, a connection dropped mid-read). Matches found before the
	// failure are still reported. FR-9 requires the run to continue past
	// it; FR-8 requires it to still force exit code 2 at the end.
	Failed     bool
	FailReason string

	// ObjectSizeLimitHit means reading stopped at BR-4's per-object limit.
	ObjectSizeLimitHit bool
	// TotalSizeLimitHit means reading stopped because the run's BR-5
	// budget ran out; the caller must not open any further object.
	TotalSizeLimitHit bool

	// LongLineWarn is true if at least one line exceeded MaxLineSize and
	// was skipped whole rather than matched partially (FR-15).
	LongLineWarn bool
}

// Incomplete reports whether the object was only partially searched, so
// its matches may be missing some (FR-8 exit 2; no -c count is printed).
func (r ObjectResult) Incomplete() bool {
	return r.Failed || r.ObjectSizeLimitHit || r.TotalSizeLimitHit
}

// Options configures how a single object is processed.
type Options struct {
	// MaxLineSize caps how many bytes of a single line are buffered before
	// it's treated as "too long" and skipped whole. Zero means
	// DefaultMaxLineSize.
	MaxLineSize int
	// Mode selects what is collected per object. The zero value is
	// ModeLines.
	Mode Mode
	// MaxObjectSize is BR-4's cap on the (decompressed) bytes read from
	// this object. Zero means no limit.
	MaxObjectSize int64
	// Budget is BR-5's run-wide allowance, shared across objects. Nil
	// means no limit.
	Budget *Budget
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

	content, err := decompressIfGzip(stream)
	if err != nil {
		res.setReadError(err)
		return res
	}
	// Limits count decompressed bytes and binary detection looks at
	// decompressed content: compressed gzip bytes always contain nulls.
	limited := newLimitedReader(content, opts.MaxObjectSize, opts.Budget)

	isBinary, combined := sniffBinary(limited)
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
			res.setReadError(err)
			return res
		}
		lineNum++
		if truncated {
			res.LongLineWarn = true
			continue
		}
		if !m.MatchString(line) {
			continue
		}
		res.MatchCount++
		switch opts.Mode {
		case ModeList:
			return res
		case ModeLines:
			res.Matches = append(res.Matches, LineMatch{LineNum: lineNum, Text: line})
		}
	}
	return res
}

// setReadError records why reading stopped early: one of the size limits,
// or an actual read failure.
func (r *ObjectResult) setReadError(err error) {
	switch {
	case errors.Is(err, ErrObjectSizeLimit):
		r.ObjectSizeLimitHit = true
	case errors.Is(err, ErrTotalSizeLimit):
		r.TotalSizeLimitHit = true
	default:
		r.Failed = true
		r.FailReason = fmt.Sprintf("error reading the object: %v", err)
	}
}

// decompressIfGzip returns a reader over the decompressed content if
// stream starts with the gzip magic bytes (1f 8b), or over stream as-is
// otherwise. Detection goes by content, not by a .gz name: GCS already
// decompresses objects stored with Content-Encoding: gzip before handing
// them over (FR-12).
func decompressIfGzip(stream io.Reader) (io.Reader, error) {
	br := bufio.NewReaderSize(stream, chunkSize)
	magic, err := br.Peek(2)
	if err != nil && err != io.EOF {
		return nil, err
	}
	if !bytes.Equal(magic, []byte{0x1f, 0x8b}) {
		return br, nil
	}
	gz, err := gzip.NewReader(br)
	if err != nil {
		return nil, fmt.Errorf("invalid gzip content: %w", err)
	}
	return gz, nil
}

// sniffBinary peeks at the first sniffSize bytes of r looking for a null
// byte, then reconstructs the full stream (peeked bytes + the rest of r) so
// the caller can still read from the beginning regardless of the verdict.
//
// If reading fails within those first bytes (a size limit, a dropped
// connection), the verdict uses what was read, and the error is replayed
// right after the peeked bytes: the complete lines before it still get
// searched, and the line reader still learns why the object ended early.
func sniffBinary(r io.Reader) (isBinary bool, combined io.Reader) {
	buf := make([]byte, sniffSize)
	n, err := io.ReadFull(r, buf)
	peeked := buf[:n]
	isBinary = bytes.IndexByte(peeked, 0) >= 0

	rest := r
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		rest = errReader{err}
	}
	return isBinary, io.MultiReader(bytes.NewReader(peeked), rest)
}

// errReader is a reader whose every Read fails with err.
type errReader struct{ err error }

func (e errReader) Read([]byte) (int, error) { return 0, e.err }

// readLine returns the next line from br (without its trailing newline).
//
// If the line is longer than maxLineSize, it is NOT truncated and matched
// against the buffered prefix — that would risk splitting a real match
// right at the cut point and silently missing it (the false-negative bug
// FR-15 exists to prevent). Instead every byte of that line is consumed and
// discarded, and readLine reports truncated=true with an empty line: the
// caller must not attempt to match it.
//
// io.EOF is returned once there is no more data at all; any other read
// error is returned as-is, never mistaken for the end of the object. A final line with
// no trailing newline is still returned as a complete line, matching
// grep's behavior.
func readLine(br *bufio.Reader, maxLineSize int) (line string, truncated bool, err error) {
	var buf []byte
	sawAnyByte := false

	for {
		b, readErr := br.ReadByte()
		if readErr != nil {
			// Anything but io.EOF means the object was cut short: the
			// partial line is dropped (never matched) and the caller
			// learns why reading stopped.
			if readErr != io.EOF {
				return "", false, readErr
			}
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
