// Package reader processes a single GCS object by streaming: it never reads
// the whole object into memory (NFR-2), detects binaries before matching
// anything (FR-11), refuses to partially match a line that exceeds the line
// buffer (FR-15), and hands every match to the caller as soon as it is found,
// so the caller can print it right away (VC-21.3) and keep it even if the
// stream fails later (NFR-3).
package reader

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"

	"gcsgrep/internal/match"
)

const (
	// sniffSize is how many leading bytes are inspected for a null byte
	// before deciding an object is binary (FR-11).
	sniffSize = 8192
	// chunkSize is the underlying read buffer size; memory use per object
	// stays bounded by this plus MaxLineSize, never by the object's total
	// size (NFR-2).
	chunkSize = 64 * 1024
	// DefaultMaxLineSize is the line buffer cap: lines longer than this are
	// skipped whole (FR-15, 1 MiB fixed in v1).
	DefaultMaxLineSize = 1 << 20
)

// LineMatch is one matching line found in an object.
type LineMatch struct {
	LineNum int
	Text    string
}

// ObjectResult is the outcome of processing a single object. Skipped and
// Failed are mutually exclusive; when neither is set the object was
// processed completely.
type ObjectResult struct {
	// MatchCount is how many matching lines were handed to emit.
	MatchCount int

	// Skipped means the object was deliberately not searched (it is a
	// binary object, FR-11). This is not an error (FR-8).
	Skipped    bool
	SkipReason string

	// Failed means the stream broke before the object was fully read
	// (NFR-3: "read interrupted"). Matches emitted before the failure stay
	// emitted; FR-8 counts the object as an error.
	Failed     bool
	FailReason string

	// Cut means the object has more content than the per-object limit
	// allows (BR-4), so reading stopped there: an objeto cortado. Matches
	// emitted before the cut stay emitted. It is an error (FR-8).
	Cut bool

	// ScanIncomplete means the run has read more content than the total
	// limit allows (BR-5), so reading stopped in this object and the run must
	// not open any more objects.
	ScanIncomplete bool

	// LongLineWarn is true if at least one line exceeded MaxLineSize and
	// was skipped whole rather than matched partially (FR-15).
	LongLineWarn bool
}

// Options configures how a single object is processed.
type Options struct {
	// MaxLineSize caps how many bytes of a single line are buffered before
	// it's treated as too long and skipped whole. Zero means
	// DefaultMaxLineSize. Only tests override it.
	MaxLineSize int

	// Gzip means the stream is gzip-compressed (FR-12): it is decompressed by
	// streaming, and every other option and check applies to the
	// decompressed bytes.
	Gzip bool

	// MaxObjectSize is BR-4's limit in bytes (0 means none).
	MaxObjectSize int64

	// Budget is the bytes counter of the run, for BR-5 (nil means none).
	Budget *Budget
}

// ProcessObject reads stream line by line, skips it whole if it looks
// binary, matches every line against m, and calls emit for each matching
// line in ascending line order, as soon as it is found. It never returns an
// error for per-object problems — those are reported through the returned
// ObjectResult so the caller (scanner) can apply FR-9's "continue past a bad
// object" policy uniformly.
func ProcessObject(stream io.Reader, m *match.Matcher, opts Options, emit func(LineMatch)) ObjectResult {
	var res ObjectResult

	maxLineSize := opts.MaxLineSize
	if maxLineSize <= 0 {
		maxLineSize = DefaultMaxLineSize
	}

	var content io.Reader = stream
	if opts.Gzip {
		content = newLazyGzipReader(content)
	}
	content = newLimitedReader(content, opts.MaxObjectSize, opts.Budget)

	isBinary, combined := sniffBinary(content)
	if isBinary {
		res.Skipped = true
		res.SkipReason = "binary object"
		return res
	}

	br := bufio.NewReaderSize(combined, chunkSize)
	lineNum := 0
	for {
		line, tooLong, err := readLine(br, maxLineSize)
		if err == io.EOF {
			break
		}
		if err != nil {
			recordReadError(&res, err)
			return res
		}
		lineNum++
		if tooLong {
			res.LongLineWarn = true
			continue
		}
		if m.MatchString(line) {
			res.MatchCount++
			if emit != nil {
				emit(LineMatch{LineNum: lineNum, Text: line})
			}
		}
	}
	return res
}

// recordReadError sets the outcome that a read error means: a guardrail was
// hit (BR-4, BR-5), the gzip data is invalid (FR-9.2), or the stream broke
// mid-read (NFR-3).
func recordReadError(res *ObjectResult, err error) {
	switch {
	case errors.Is(err, errTotalLimit):
		res.ScanIncomplete = true
	case errors.Is(err, errObjectLimit):
		res.Cut = true
	case errors.Is(err, errCorruptGzip):
		res.Failed = true
		res.FailReason = "corrupt gzip data"
	default:
		res.Failed = true
		res.FailReason = fmt.Sprintf("read interrupted: %v", err)
	}
}

// sniffBinary peeks at the first sniffSize bytes of r looking for a null
// byte, then reconstructs the full stream (peeked bytes + the rest of r) so
// the caller can still read from the beginning regardless of the verdict.
// A 0-byte object is not binary (FR-21.1). If the stream breaks while
// peeking, the bytes already received are still processed and the error is
// replayed right after them, so the lines received before the break are
// matched and printed (NFR-3) instead of being lost.
func sniffBinary(r io.Reader) (isBinary bool, combined io.Reader) {
	buf := make([]byte, sniffSize)
	n, err := io.ReadFull(r, buf)
	peeked := buf[:n]
	isBinary = bytes.IndexByte(peeked, 0) >= 0
	switch err {
	case nil:
		combined = io.MultiReader(bytes.NewReader(peeked), r)
	case io.EOF, io.ErrUnexpectedEOF:
		combined = bytes.NewReader(peeked)
	default:
		combined = io.MultiReader(bytes.NewReader(peeked), errReader{err})
	}
	return isBinary, combined
}

// errReader always fails with err.
type errReader struct{ err error }

func (e errReader) Read([]byte) (int, error) { return 0, e.err }

// readLine returns the next line from br (without its trailing newline).
//
// If the line is longer than maxLineSize, it is NOT truncated and matched
// against the buffered prefix — that would risk splitting a real match
// right at the cut point and silently missing it (the false negative FR-15
// exists to prevent). Instead every byte of that line is consumed and
// discarded, and readLine reports tooLong=true with an empty line: the
// caller must not attempt to match it.
//
// io.EOF is returned once there is no more data at all. A final line with no
// trailing newline is still returned as a complete line (FR-21.2). Any other
// read error is returned as-is, even mid-line, so a broken stream is never
// mistaken for the end of the object (NFR-3).
func readLine(br *bufio.Reader, maxLineSize int) (line string, tooLong bool, err error) {
	var buf []byte
	sawAnyByte := false

	for {
		b, readErr := br.ReadByte()
		if readErr != nil {
			if readErr != io.EOF {
				return "", false, readErr
			}
			if !sawAnyByte {
				return "", false, io.EOF
			}
			if tooLong {
				return "", true, nil
			}
			return string(buf), false, nil
		}
		sawAnyByte = true

		if b == '\n' {
			if tooLong {
				return "", true, nil
			}
			return string(buf), false, nil
		}

		if !tooLong {
			if len(buf) >= maxLineSize {
				tooLong = true
				buf = nil // stop holding the oversized prefix; it's never used
			} else {
				buf = append(buf, b)
			}
		}
	}
}
