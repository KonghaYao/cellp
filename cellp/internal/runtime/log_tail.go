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
	path string
}

// TailFileFromEnd opens a log file for tailing; missing files yield empty reads.
func TailFileFromEnd(path string, _ int64) *LogTail {
	return &LogTail{path: path}
}

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
		offset = 0
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
