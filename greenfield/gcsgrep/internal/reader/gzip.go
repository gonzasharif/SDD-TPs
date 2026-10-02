package reader

import (
	"compress/flate"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
)

// errCorruptGzip marks a read error that comes from invalid gzip data (as
// opposed to a broken connection), so it can be reported as FR-9.2's
// "corrupt gzip data".
var errCorruptGzip = errors.New("corrupt gzip data")

// lazyGzipReader decompresses src by streaming. The gzip header is read on
// the first Read instead of at construction, so a bad header is reported
// through the same Read error path as every other failure.
type lazyGzipReader struct {
	src io.Reader
	zr  *gzip.Reader
	err error
}

func newLazyGzipReader(src io.Reader) *lazyGzipReader {
	return &lazyGzipReader{src: src}
}

func (l *lazyGzipReader) Read(p []byte) (int, error) {
	if l.err != nil {
		return 0, l.err
	}
	if l.zr == nil {
		zr, err := gzip.NewReader(l.src)
		if err != nil {
			l.err = classifyGzipError(err)
			return 0, l.err
		}
		l.zr = zr
	}
	n, err := l.zr.Read(p)
	if err != nil && err != io.EOF {
		l.err = classifyGzipError(err)
		return n, l.err
	}
	return n, err
}

// classifyGzipError wraps err with errCorruptGzip when it means the data is
// not valid gzip: a bad header, a checksum mismatch, invalid compressed
// data, or a stream that ends early. Any other error (a broken connection,
// say) is returned as it is, and io.EOF (an empty object) stays io.EOF.
func classifyGzipError(err error) error {
	var corruptInput flate.CorruptInputError
	switch {
	case errors.Is(err, gzip.ErrHeader), errors.Is(err, gzip.ErrChecksum),
		errors.Is(err, io.ErrUnexpectedEOF), errors.As(err, &corruptInput):
		return fmt.Errorf("%w: %v", errCorruptGzip, err)
	default:
		return err
	}
}
