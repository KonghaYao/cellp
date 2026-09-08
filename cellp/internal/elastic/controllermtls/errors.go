package controllermtls

import (
	"errors"
	"net"
	"net/http"
	"syscall"
)

// normalizeServeErr maps expected listener/http shutdown errors to nil when stopping is true.
// Real Serve failures while not shutting down are returned as-is.
func normalizeServeErr(err error, stopping bool) error {
	if err == nil {
		return nil
	}
	if !stopping {
		return err
	}
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	if errors.Is(err, net.ErrClosed) {
		return nil
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if errors.Is(opErr.Err, net.ErrClosed) {
			return nil
		}
		if opErr.Op == "accept" || opErr.Op == "close" {
			if errors.Is(opErr.Err, syscall.ECONNRESET) {
				return nil
			}
			if errors.Is(opErr.Err, syscall.EBADF) {
				return nil
			}
			if isUseOfClosedNetworkConn(err) {
				return nil
			}
		}
	}
	if isUseOfClosedNetworkConn(err) {
		return nil
	}
	return err
}

func normalizeCloseErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, net.ErrClosed) {
		return nil
	}
	if isUseOfClosedNetworkConn(err) {
		return nil
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) && errors.Is(opErr.Err, net.ErrClosed) {
		return nil
	}
	return err
}

func isUseOfClosedNetworkConn(err error) bool {
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if errors.Is(opErr.Err, net.ErrClosed) {
			return true
		}
		// Some platforms surface listener teardown as a plain error string.
		if opErr.Op == "close" || opErr.Op == "accept" {
			return opErr.Err != nil && opErr.Err.Error() == "use of closed network connection"
		}
	}
	return err != nil && err.Error() == "use of closed network connection"
}
