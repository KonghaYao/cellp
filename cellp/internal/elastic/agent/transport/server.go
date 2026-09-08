package transport

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cellp/cellp/internal/elastic/agent"
	"github.com/cellp/cellp/internal/elastic/contract"
)

// Server serves versioned internal Node Agent commands over HTTPS+mTLS.
type Server struct {
	cfg     ServerConfig
	handler *agent.Handler
	replay  *ReplayCache
	mux     *http.ServeMux

	httpServer *http.Server
	listener   net.Listener

	mu              sync.Mutex
	closed          bool
	running         bool
	ready           chan struct{}
	readyOnce       sync.Once
	shutdownStarted bool
	shutdownDone    chan struct{}
	shutdownErr     error

	accepting atomic.Bool
}

// NewServer builds an HTTPS listener for the given handler.
func NewServer(cfg ServerConfig, h *agent.Handler) (*Server, error) {
	if h == nil {
		return nil, errors.New("handler required")
	}
	cfg = normalizeServerConfig(cfg)
	if err := ValidateServerConfig(cfg); err != nil {
		return nil, err
	}
	s := &Server{
		cfg:          cfg,
		handler:      h,
		replay:       NewReplayCache(cfg.ReplayMaxEntries),
		mux:          http.NewServeMux(),
		ready:        make(chan struct{}),
		shutdownDone: make(chan struct{}),
	}
	s.registerRoutes()
	s.accepting.Store(true)
	return s, nil
}

// SetAccepting gates lifecycle command handling after mTLS authentication.
// Default is accepting. Standalone agentrun disables accepting until boot reconcile completes.
// Enabling accepting after Shutdown is ignored (fail-closed).
// Shutdown sets accepting to false for new requests; handlers that already passed the gate
// (including replay admission) may complete normally while http.Server.Shutdown drains them.
func (s *Server) SetAccepting(on bool) {
	if !on {
		s.accepting.Store(false)
		return
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.accepting.Store(true)
	s.mu.Unlock()
}

func (s *Server) acceptingCommands() bool {
	return s.accepting.Load()
}

func (s *Server) writeNotAccepting(w http.ResponseWriter) {
	writeWireError(w, http.StatusServiceUnavailable, contract.ReasonColdActivating)
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc(routeStart, s.handleStart)
	s.mux.HandleFunc(routeProbe, s.handleProbe)
	s.mux.HandleFunc(routeStop, s.handleStop)
	s.mux.HandleFunc(routeDrain, s.handleDrain)
	s.mux.HandleFunc(routeList, s.handleList)
}

// Run listens until ctx is canceled, then shuts down gracefully.
func (s *Server) Run(ctx context.Context) error {
	if !s.cfg.Enabled {
		return errors.New("transport disabled")
	}
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
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.running = false
		s.listener = nil
		s.httpServer = nil
		s.mu.Unlock()
	}()

	ln, err := net.Listen("tcp", s.cfg.BindAddr)
	if err != nil {
		return err
	}
	tlsCfg := buildServerTLSConfig(s.cfg.TLS, s.cfg.NodeIdentityURI)
	hs := &http.Server{
		Handler:           s.mux,
		TLSConfig:         tlsCfg,
		ReadTimeout:       s.cfg.ReadTimeout,
		WriteTimeout:      s.cfg.WriteTimeout,
		IdleTimeout:       s.cfg.IdleTimeout,
		ReadHeaderTimeout: 5 * time.Second,
		MaxHeaderBytes:    8 << 10,
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		_ = ln.Close()
		return errors.New("server closed")
	}
	s.listener = ln
	s.httpServer = hs
	s.mu.Unlock()

	errCh := make(chan error, 1)
	go func() {
		errCh <- hs.Serve(tls.NewListener(ln, tlsCfg))
	}()
	s.readyOnce.Do(func() { close(s.ready) })

	select {
	case <-ctx.Done():
		shCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		shutdownErr := s.Shutdown(shCtx)
		cancel()
		var serveErr error
		select {
		case serveErr = <-errCh:
			serveErr = normalizeServeErr(serveErr, true)
		case <-time.After(15 * time.Second):
			serveErr = errors.New("agent server did not stop")
		}
		return errors.Join(shutdownErr, serveErr)
	case err := <-errCh:
		stopping := s.shutdownStartedUnderLock()
		return normalizeServeErr(err, stopping)
	}
}

// Shutdown stops the server. Returns ctx.Err() when the deadline expires before quiescence.
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	if s.shutdownStarted {
		done := s.shutdownDone
		s.mu.Unlock()
		return s.waitShutdown(ctx, done)
	}
	s.shutdownStarted = true
	s.closed = true
	s.accepting.Store(false)
	hs := s.httpServer
	ln := s.listener
	done := s.shutdownDone
	s.mu.Unlock()
	go func() {
		var errs []error
		if ln != nil {
			if err := ln.Close(); err != nil {
				if norm := normalizeListenerCloseErr(err); norm != nil {
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

func (s *Server) shutdownStartedUnderLock() bool {
	s.mu.Lock()
	stopping := s.shutdownStarted
	s.mu.Unlock()
	return stopping
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

// Ready is closed after the listener is bound and the Serve goroutine is started.
func (s *Server) Ready() <-chan struct{} { return s.ready }

// Addr returns the bound listen address after Run starts.
func (s *Server) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return s.cfg.BindAddr
}

func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeWireError(w, http.StatusMethodNotAllowed, contract.ReasonAuthFailed)
		return
	}
	principal, err := s.authenticate(r)
	if err != nil {
		writeWireError(w, http.StatusUnauthorized, contract.ReasonAuthFailed)
		return
	}
	if !s.acceptingCommands() {
		s.writeNotAccepting(w)
		return
	}
	var req startRequest
	if err := decodeJSON(w, r, s.cfg.MaxBodyBytes, &req); err != nil {
		writeDecodeError(w, err)
		return
	}
	if err := validateIdempotencyKey(r.Header.Get(headerIdempotencyKey)); err != nil {
		writeDecodeError(w, err)
		return
	}
	if err := s.authorizeScope(principal, req.Spec.Scope, contract.ActionStartReplica); err != nil {
		writeAuthzError(w, err)
		return
	}
	if err := validateSecretRefs(req.Spec.SecretRefs); err != nil {
		writeDecodeError(w, err)
		return
	}
	idem := strings.TrimSpace(r.Header.Get(headerIdempotencyKey))
	rep, err := s.handler.StartReplica(r.Context(), req.Spec, idem)
	s.audit(principal, req.Spec.Scope, reasonForError(err))
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, startResponse{Replica: rep})
}

func (s *Server) handleProbe(w http.ResponseWriter, r *http.Request) {
	s.handleScope(w, r, contract.ActionProbeReplica, func(ctx context.Context, scope contract.CommandScope) (interface{}, error) {
		res, err := s.handler.ProbeReplica(ctx, scope)
		if err != nil {
			return nil, err
		}
		return probeResponse{Result: probeWire{ReplicaID: res.ReplicaID, State: res.State, Generation: res.Generation}}, nil
	})
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	s.handleScope(w, r, contract.ActionStopReplica, func(ctx context.Context, scope contract.CommandScope) (interface{}, error) {
		rep, err := s.handler.StopReplica(ctx, scope)
		if err != nil {
			return nil, err
		}
		return stopResponse{Replica: rep}, nil
	})
}

func (s *Server) handleDrain(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeWireError(w, http.StatusMethodNotAllowed, contract.ReasonAuthFailed)
		return
	}
	principal, err := s.authenticate(r)
	if err != nil {
		writeWireError(w, http.StatusUnauthorized, contract.ReasonAuthFailed)
		return
	}
	if !s.acceptingCommands() {
		s.writeNotAccepting(w)
		return
	}
	var req drainRequest
	if err := decodeJSON(w, r, s.cfg.MaxBodyBytes, &req); err != nil {
		writeDecodeError(w, err)
		return
	}
	if err := s.authorizeScope(principal, req.Scope, contract.ActionDrainReplica); err != nil {
		writeAuthzError(w, err)
		return
	}
	deadline, err := parseOptionalDrainDeadline(req.Deadline, time.Now().UTC())
	if err != nil {
		writeDecodeError(w, err)
		return
	}
	rep, err := s.handler.DrainReplica(r.Context(), req.Scope, deadline)
	s.audit(principal, req.Scope, reasonForError(err))
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, drainResponse{Replica: rep})
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	s.handleScope(w, r, contract.ActionListReplicas, func(ctx context.Context, scope contract.CommandScope) (interface{}, error) {
		reps, err := s.handler.ListReplicas(ctx, scope)
		if err != nil {
			return nil, err
		}
		return listResponse{Replicas: reps}, nil
	})
}

func (s *Server) handleScope(w http.ResponseWriter, r *http.Request, action contract.LifecycleAction, fn func(context.Context, contract.CommandScope) (interface{}, error)) {
	if r.Method != http.MethodPost {
		writeWireError(w, http.StatusMethodNotAllowed, contract.ReasonAuthFailed)
		return
	}
	principal, err := s.authenticate(r)
	if err != nil {
		writeWireError(w, http.StatusUnauthorized, contract.ReasonAuthFailed)
		return
	}
	if !s.acceptingCommands() {
		s.writeNotAccepting(w)
		return
	}
	var req scopeRequest
	if err := decodeJSON(w, r, s.cfg.MaxBodyBytes, &req); err != nil {
		writeDecodeError(w, err)
		return
	}
	if err := s.authorizeScope(principal, req.Scope, action); err != nil {
		writeAuthzError(w, err)
		return
	}
	out, err := fn(r.Context(), req.Scope)
	s.audit(principal, req.Scope, reasonForError(err))
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) audit(principal string, scope contract.CommandScope, reason contract.ReasonCode) {
	if s.cfg.Audit == nil {
		return
	}
	s.cfg.Audit(AuditEvent{Action: scope.Action, Reason: reason, Principal: principal, NodeID: scope.NodeID, ReplicaID: scope.ReplicaID, Generation: scope.Generation})
}

func reasonForError(err error) contract.ReasonCode {
	if err == nil {
		return ""
	}
	var commandErr *agent.CommandError
	if errors.As(err, &commandErr) {
		return commandErr.Reason
	}
	return contract.ReasonAuthFailed
}

func (s *Server) authenticate(r *http.Request) (string, error) {
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		return "", errors.New("no peer certificate")
	}
	peer := r.TLS.PeerCertificates[0]
	if err := ValidateLeafCertificateValidity(peer, time.Now().UTC()); err != nil {
		return "", err
	}
	if s.cfg.TLS.VerifyPeerConnection != nil {
		if err := s.cfg.TLS.VerifyPeerConnection(*r.TLS); err != nil {
			return "", err
		}
	}
	return MatchPeerURIPrincipal(peer, s.cfg.AllowedControllerURIs)
}

func (s *Server) authorizeScope(principal string, scope contract.CommandScope, routeAction contract.LifecycleAction) error {
	if !s.handler.Enabled() {
		return &agent.CommandError{Reason: contract.ReasonElasticDisabled}
	}
	if scope.Action != routeAction {
		return errWire{reason: contract.ReasonAuthFailed}
	}
	if strings.TrimSpace(scope.NodeID) != s.cfg.NodeID {
		return errWire{reason: contract.ReasonAuthFailed}
	}
	if err := contract.ValidateCommandScope(scope); err != nil {
		return errWire{reason: contract.ReasonAuthFailed}
	}
	now := time.Now().UTC()
	if !scope.LeaseExpiry.After(now) {
		return &agent.CommandError{Reason: contract.ReasonGenerationStale}
	}
	replicaID := scope.ReplicaID
	if routeAction == contract.ActionListReplicas {
		replicaID = ""
	}
	key := ReplayKey(principal, scope.NodeID, scope.ProjectID, scope.VersionID, replicaID, string(scope.Action), scope.Generation, scope.Nonce)
	replay, err := s.replay.Consume(key, scope.LeaseExpiry)
	if err != nil {
		return &agent.CommandError{Reason: contract.ReasonReplayRejected}
	}
	if replay {
		return &agent.CommandError{Reason: contract.ReasonReplayRejected}
	}
	return nil
}

func decodeJSON(w http.ResponseWriter, r *http.Request, maxBytes int64, dst interface{}) error {
	if ct := r.Header.Get("Content-Type"); ct != "" {
		mediaType, err := parseMediaType(ct)
		if err != nil {
			return errBadContentType
		}
		if mediaType != "application/json" {
			return errBadContentType
		}
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) || isOversize(err) {
			return errRequestTooLarge
		}
		return errBadJSON
	}
	if err := dec.Decode(&struct{}{}); err != nil {
		if err != io.EOF {
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) || isOversize(err) {
				return errRequestTooLarge
			}
			return errTrailingJSON
		}
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeWireError(w http.ResponseWriter, status int, reason contract.ReasonCode) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(wireError{Reason: reason})
}

func writeDecodeError(w http.ResponseWriter, err error) {
	var ew errWire
	if errors.As(err, &ew) {
		status := http.StatusBadRequest
		if ew.reason == contract.ReasonRequestTooLarge {
			status = http.StatusRequestEntityTooLarge
		}
		writeWireError(w, status, ew.reason)
		return
	}
	writeWireError(w, http.StatusBadRequest, contract.ReasonAuthFailed)
}

func writeAuthzError(w http.ResponseWriter, err error) {
	writeHandlerError(w, err)
}

func writeHandlerError(w http.ResponseWriter, err error) {
	var cmd *agent.CommandError
	if errors.As(err, &cmd) && cmd.Reason != "" {
		writeWireError(w, reasonHTTPStatus(cmd.Reason), cmd.Reason)
		return
	}
	var ew errWire
	if errors.As(err, &ew) {
		writeWireError(w, reasonHTTPStatus(ew.reason), ew.reason)
		return
	}
	writeWireError(w, http.StatusInternalServerError, contract.ReasonAuthFailed)
}

func reasonHTTPStatus(reason contract.ReasonCode) int {
	switch reason {
	case contract.ReasonRequestTooLarge:
		return http.StatusRequestEntityTooLarge
	case contract.ReasonElasticDisabled, contract.ReasonColdActivating:
		return http.StatusServiceUnavailable
	case contract.ReasonGenerationStale, contract.ReasonReplayRejected:
		return http.StatusConflict
	case contract.ReasonAuthFailed:
		return http.StatusUnauthorized
	default:
		return http.StatusInternalServerError
	}
}

func normalizeListenerCloseErr(err error) error {
	if err == nil {
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
		if opErr.Op == "close" || opErr.Op == "accept" {
			if opErr.Err != nil && opErr.Err.Error() == "use of closed network connection" {
				return nil
			}
		}
	}
	if err.Error() == "use of closed network connection" {
		return nil
	}
	return err
}
