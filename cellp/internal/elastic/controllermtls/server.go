// Package controllermtls serves nodereg and registryrelay on one HTTPS+mTLS listener.
package controllermtls

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	agenttransport "github.com/cellp/cellp/internal/elastic/agent/transport"
	noderegtransport "github.com/cellp/cellp/internal/elastic/nodereg/transport"
	registryrelaytransport "github.com/cellp/cellp/internal/elastic/registryrelay/transport"
)

// Config wires a combined controller internal mTLS listener.
type Config struct {
	BindAddr              string
	ControllerIdentityURI string
	TLS                   agenttransport.TLSMaterials
	Nodereg               *noderegtransport.Server
	Relay                 *registryrelaytransport.Server
}

// Server exposes nodereg and registry relay RPCs on a single TLS listener.
type Server struct {
	cfg Config

	httpServer *http.Server
	listener   net.Listener

	mu              sync.Mutex
	closed          bool
	running         bool
	ready           chan struct{}
	shutdownStarted bool
	shutdownDone    chan struct{}
	shutdownErr     error
}

// NewServer builds a combined listener. Sub-servers are used for routing only (Run is not called on them).
func NewServer(cfg Config) (*Server, error) {
	if cfg.Nodereg == nil || cfg.Relay == nil {
		return nil, errors.New("nodereg and relay servers required")
	}
	if strings.TrimSpace(cfg.BindAddr) == "" {
		return nil, errors.New("bind address required")
	}
	return &Server{cfg: cfg, ready: make(chan struct{}), shutdownDone: make(chan struct{})}, nil
}

func (s *Server) handler() http.Handler {
	nd := s.cfg.Nodereg.Handler()
	rl := s.cfg.Relay.Handler()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.HasPrefix(path, noderegtransport.RoutePrefix):
			nd.ServeHTTP(w, r)
		case strings.HasPrefix(path, registryrelaytransport.RoutePrefix):
			rl.ServeHTTP(w, r)
		default:
			http.NotFound(w, r)
		}
	})
}

// Run listens until ctx is canceled, then shuts down gracefully via the shared shutdown state machine.
func (s *Server) Run(ctx context.Context) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return errors.New("server closed")
	}
	if s.running {
		s.mu.Unlock()
		return errors.New("server already running")
	}
	s.running = true
	readyCh := make(chan struct{})
	s.ready = readyCh
	var readySignaled bool
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.running = false
		s.listener = nil
		s.httpServer = nil
		s.mu.Unlock()
	}()

	signalReady := func() {
		s.mu.Lock()
		if !readySignaled {
			readySignaled = true
			close(readyCh)
		}
		s.mu.Unlock()
	}

	ln, err := net.Listen("tcp", s.cfg.BindAddr)
	if err != nil {
		return err
	}
	tlsCfg := agenttransport.BuildServerTLSConfig(s.cfg.TLS, s.cfg.ControllerIdentityURI)
	hs := &http.Server{
		Handler:           s.handler(),
		TLSConfig:         tlsCfg,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		MaxHeaderBytes:    8 << 10,
	}

	s.mu.Lock()
	if s.closed || s.shutdownStarted {
		s.mu.Unlock()
		_ = ln.Close()
		return errors.New("server closed")
	}
	s.listener = ln
	s.httpServer = hs
	s.mu.Unlock()

	errCh := make(chan error, 1)
	go func() { errCh <- hs.Serve(tls.NewListener(ln, tlsCfg)) }()

	s.mu.Lock()
	aborted := s.closed || s.shutdownStarted
	s.mu.Unlock()
	if aborted {
		_ = ln.Close()
		select {
		case <-errCh:
		case <-time.After(5 * time.Second):
		}
		return errors.New("server closed")
	}
	signalReady()

	select {
	case <-ctx.Done():
		shCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		shutdownErr := s.Shutdown(shCtx)
		var serveErr error
		select {
		case serveErr = <-errCh:
			serveErr = normalizeServeErr(serveErr, true)
		case <-time.After(15 * time.Second):
			serveErr = errors.New("controller mTLS server did not stop")
		}
		return errors.Join(shutdownErr, serveErr)
	case err := <-errCh:
		stopping := s.shutdownStartedUnderLock()
		return normalizeServeErr(err, stopping)
	}
}

func (s *Server) shutdownStartedUnderLock() bool {
	s.mu.Lock()
	stopping := s.shutdownStarted
	s.mu.Unlock()
	return stopping
}

// Ready closes after the listener is active.
func (s *Server) Ready() <-chan struct{} {
	s.mu.Lock()
	ch := s.ready
	s.mu.Unlock()
	return ch
}

// Addr returns the bound address.
func (s *Server) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return s.cfg.BindAddr
}

// Shutdown stops the server idempotently. Concurrent callers wait on the same shutdown completion.
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	if s.shutdownStarted {
		done := s.shutdownDone
		s.mu.Unlock()
		return s.waitShutdown(ctx, done)
	}
	s.shutdownStarted = true
	s.closed = true
	hs := s.httpServer
	ln := s.listener
	done := s.shutdownDone
	s.mu.Unlock()

	go func() {
		var errs []error
		if ln != nil {
			if err := ln.Close(); err != nil {
				if norm := normalizeCloseErr(err); norm != nil {
					errs = append(errs, norm)
				}
			}
		}
		if hs != nil {
			if err := hs.Shutdown(ctx); err != nil {
				if norm := normalizeServeErr(err, true); norm != nil {
					errs = append(errs, norm)
				}
			}
		}
		s.mu.Lock()
		s.shutdownErr = errors.Join(errs...)
		close(done)
		s.mu.Unlock()
	}()
	return s.waitShutdown(ctx, done)
}

func (s *Server) waitShutdown(ctx context.Context, done <-chan struct{}) error {
	select {
	case <-done:
		s.mu.Lock()
		err := s.shutdownErr
		s.mu.Unlock()
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
