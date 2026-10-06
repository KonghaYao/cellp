package runtime

import (
	"io"
	"os"
)

// CelldLogPath returns the per-version celld stdout log path (exported for telemetry SSE).
func CelldLogPath(project, version string) string {
	return celldLogPath(project, version)
}

// LogTail supports incremental reads for live log SSE.
type LogTail struct {
	path         string
	startOffset  int64
}

// TailFileFromEnd opens a log file for tailing. The returned tail starts at end-of-file
// (or at most maxBytes from the end when maxBytes > 0). Missing files yield empty reads.
func TailFileFromEnd(path string, maxBytes int64) *LogTail {
	t := &LogTail{path: path}
	f, err := os.Open(path)
	if err != nil {
		return t
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return t
	}
	size := info.Size()
	if maxBytes > 0 && size > maxBytes {
		t.startOffset = size - maxBytes
	} else {
		t.startOffset = size
	}
	return t
}

// InitialOffset is the byte offset clients should use when no explicit offset is supplied.
func (t *LogTail) InitialOffset() int64 { return t.startOffset }

func (t *LogTail) Close() error { return nil }

func (t *LogTail) ReadSince(offset int64) ([]byte, int64, error) {
	f, err := os.Open(t.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, offset, nil
		}
		return nil, offset, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, offset, err
	}
	size := info.Size()
	if offset > size {
		offset = size
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, offset, err
	}
	buf, err := io.ReadAll(f)
	if err != nil {
		return nil, offset, err
	}
	return buf, size, nil
}
