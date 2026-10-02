package reader

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"strings"
	"testing"
)

func gz(t *testing.T, content string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write([]byte(content)); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return buf.Bytes()
}

// FR-12 / VC-12.1: a .gz stream is searched on its decompressed content.
func TestProcessObject_Gzip_SearchesDecompressedContent(t *testing.T) {
	res, got := process(t, bytes.NewReader(gz(t, "INFO start\nERROR timeout\n")), "timeout", Options{Gzip: true})

	if res.Failed || res.Skipped || res.Cut {
		t.Fatalf("unexpected outcome: %+v", res)
	}
	if len(got) != 1 || got[0].LineNum != 2 || got[0].Text != "ERROR timeout" {
		t.Errorf("matches = %+v, want line 2 'ERROR timeout'", got)
	}
}

// FR-12 / VC-12.2: binary detection looks at the decompressed bytes.
func TestProcessObject_Gzip_BinaryContentIsSkipped(t *testing.T) {
	png := "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR timeout"
	res, got := process(t, bytes.NewReader(gz(t, png)), "timeout", Options{Gzip: true})

	if !res.Skipped || res.SkipReason != "binary object" || len(got) != 0 {
		t.Errorf("res = %+v, matches = %+v; want a skipped binary object with no matches", res, got)
	}
}

// Text inside a .gz is not skipped: detection looks at the decompressed
// bytes, never at the compressed ones (which can contain any byte).
func TestProcessObject_Gzip_TextIsNotSkipped(t *testing.T) {
	res, _ := process(t, bytes.NewReader(gz(t, "timeout\n")), "timeout", Options{Gzip: true})
	if res.Skipped {
		t.Errorf("text content was skipped: %+v", res)
	}
}

// FR-9.2 / VC-9.2: plain text with a .gz name has no gzip header.
func TestProcessObject_Gzip_InvalidHeader(t *testing.T) {
	res, got := process(t, strings.NewReader("esto no es gzip\n"), "timeout", Options{Gzip: true})

	if !res.Failed || res.FailReason != "corrupt gzip data" || len(got) != 0 {
		t.Errorf("res = %+v, matches = %+v; want a failed object, 'corrupt gzip data'", res, got)
	}
}

// FR-9.2: data that goes bad after a valid start keeps the matches printed
// so far, and the object is reported as corrupt.
func TestProcessObject_Gzip_CorruptMidStreamKeepsEarlierMatches(t *testing.T) {
	content := strings.Repeat("timeout\n", 50000)
	data := gz(t, content)
	truncated := data[:len(data)/2]

	res, got := process(t, bytes.NewReader(truncated), "timeout", Options{Gzip: true})

	if !res.Failed || res.FailReason != "corrupt gzip data" {
		t.Errorf("res = %+v, want failed with 'corrupt gzip data'", res)
	}
	if len(got) == 0 {
		t.Errorf("matches before the corruption should have been emitted")
	}
}

// FR-9.2: a checksum mismatch at the end is corrupt data too.
func TestProcessObject_Gzip_ChecksumMismatch(t *testing.T) {
	data := gz(t, "timeout\n")
	data[len(data)-5] ^= 0xff // inside the CRC32 trailer

	res, _ := process(t, bytes.NewReader(data), "timeout", Options{Gzip: true})

	if !res.Failed || res.FailReason != "corrupt gzip data" {
		t.Errorf("res = %+v, want failed with 'corrupt gzip data'", res)
	}
}

// A connection that breaks while a .gz is read is a read interruption
// (NFR-3), not corrupt data.
func TestProcessObject_Gzip_BrokenConnectionIsNotCorruption(t *testing.T) {
	data := gz(t, strings.Repeat("timeout\n", 50000))
	stream := io.MultiReader(bytes.NewReader(data[:len(data)/2]), errReader{errors.New("connection reset")})

	res, _ := process(t, stream, "timeout", Options{Gzip: true})

	if !res.Failed || !strings.HasPrefix(res.FailReason, "read interrupted: ") {
		t.Errorf("res = %+v, want failed with 'read interrupted: ...'", res)
	}
}

// FR-21.1 applies to compressed objects too: nothing to decompress is an
// object without lines, not an error.
func TestProcessObject_Gzip_EmptyObject(t *testing.T) {
	res, got := process(t, bytes.NewReader(nil), "timeout", Options{Gzip: true})

	if res.Failed || res.Skipped || len(got) != 0 {
		t.Errorf("res = %+v, matches = %+v; want a clean empty result", res, got)
	}
}
