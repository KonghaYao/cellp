package activator

import (
	"bytes"
	"errors"
	"io"
	"net/http"
)

var (
	// ErrBodyTooLarge means the request body exceeded the configured hard limit.
	ErrBodyTooLarge = errors.New("activator: body too large")
	// ErrBodyRead means the request body could not be read completely.
	ErrBodyRead = errors.New("activator: body read failed")
)

// BufferBoundedBody reads up to max+1 bytes to detect actual size, replaces r.Body
// with a replay reader, and returns the byte budget to reserve. Caller must invoke
// cancel to stop background body work when the caller disconnects without waiting
// for shared activation.
func BufferBoundedBody(r *http.Request, max int64) (reserved int64, cancel func(), err error) {
	if r == nil || max <= 0 {
		return 0, func() {}, nil
	}
	method := r.Method
	if method == http.MethodGet || method == http.MethodHead {
		return 0, func() {}, nil
	}
	if r.Body == nil {
		return 0, func() {}, nil
	}
	limited := io.LimitReader(r.Body, max+1)
	data, readErr := io.ReadAll(limited)
	_ = r.Body.Close()
	if readErr != nil {
		return 0, func() {}, ErrBodyRead
	}
	if int64(len(data)) > max {
		return 0, func() {}, ErrBodyTooLarge
	}
	body := io.NopCloser(bytes.NewReader(data))
	r.Body = body
	r.ContentLength = int64(len(data))
	r.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(data)), nil
	}
	return int64(len(data)), func() {
		_ = body.Close()
	}, nil
}
