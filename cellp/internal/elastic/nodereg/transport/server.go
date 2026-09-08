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
	"time"

	agenttransport "github.com/cellp/cellp/internal/elastic/agent/transport"
	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/elastic/nodereg"
	"github.com/cellp/cellp/internal/elastic/registrywire"
)

// ServerConfig configures the controller node-registration HTTPS listener.
type ServerConfig struct {
	BindAddr              string
	ControllerIdentityURI string
	AllowedNodeURIs       []string
	TLS                   agenttransport.TLSMaterials
	MaxBodyBytes          int64
	ReplayMaxEntries      int
}

// Server serves node registration RPCs to authenticated runtime nodes.
type Server struct {
	cfg     ServerConfig
	handler *nodereg.Handler
	replay  *agenttransport.ReplayCache
	mux     *http.ServeMux

	httpServer *http.Server
	listener   net.Listener

	mu        sync.Mutex
	closed    bool
	running   bool
	ready     chan struct{}
	readyOnce sync.Once
}

// NewServer builds the registration server.
func NewServer(cfg ServerConfig, h *nodereg.Handler) (*Server, error) {
	if h == nil {
		return nil, errors.New("handler required")
	}
	if err := nodereg.ValidateHandlerConfig(h); err != nil {
		return nil, err
	}
	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = 1 << 20
	}
	if cfg.ReplayMaxEntries <= 0 {
		cfg.ReplayMaxEntries = 4096
	}
	s := &Server{
		cfg:     cfg,
		handler: h,
		replay:  agenttransport.NewReplayCache(cfg.ReplayMaxEntries),
		mux:     http.NewServeMux(),
		ready:   make(chan struct{}),
	}
	s.mux.HandleFunc(routeActivate, s.handleActivate)
	s.mux.HandleFunc(routeHeartbeat, s.handleHeartbeat)
	s.mux.HandleFunc(routeCordon, s.handleCordon)
	s.mux.HandleFunc(routeRelease, s.handleRelease)
	s.mux.HandleFunc(routeStatus, s.handleStatus)
	return s, nil
}

// Handler returns the HTTP handler (for composition on a shared mTLS listener).
func (s *Server) Handler() http.Handler {
	return s.mux
}

// Run listens until ctx is canceled.
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
	tlsCfg := agenttransport.BuildServerTLSConfig(s.cfg.TLS, s.cfg.ControllerIdentityURI)
	hs := &http.Server{
		Handler:           s.mux,
		TLSConfig:         tlsCfg,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		MaxHeaderBytes:    8 << 10,
	}
	s.mu.Lock()
	s.listener = ln
	s.httpServer = hs
	s.mu.Unlock()

	errCh := make(chan error, 1)
	go func() { errCh <- hs.Serve(tls.NewListener(ln, tlsCfg)) }()
	s.readyOnce.Do(func() { close(s.ready) })

	select {
	case <-ctx.Done():
		shCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = hs.Shutdown(shCtx)
		err := <-errCh
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

// Ready closes after the listener is active.
func (s *Server) Ready() <-chan struct{} { return s.ready }

// Addr returns the bound address.
func (s *Server) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return s.cfg.BindAddr
}

// Shutdown stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	hs := s.httpServer
	ln := s.listener
	s.closed = true
	s.mu.Unlock()
	if ln != nil {
		_ = ln.Close()
	}
	if hs != nil {
		return hs.Shutdown(ctx)
	}
	return nil
}

var errReplayRejected = errors.New("replay rejected")

func (s *Server) handleActivate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeWireError(w, http.StatusMethodNotAllowed, contract.ReasonAuthFailed)
		return
	}
	peer, err := s.authenticate(r)
	if err != nil {
		writeWireError(w, http.StatusUnauthorized, contract.ReasonAuthFailed)
		return
	}
	var req contract.ActivateNodeRequest
	if err := decodeJSON(w, r, s.cfg.MaxBodyBytes, &req); err != nil {
		writeDecodeError(w, err)
		return
	}
	if err := s.consumeReplay(peer, req.Scope); err != nil {
		writeHandlerError(w, err)
		return
	}
	out, err := s.handler.Activate(r.Context(), peer, req)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeWireError(w, http.StatusMethodNotAllowed, contract.ReasonAuthFailed)
		return
	}
	peer, err := s.authenticate(r)
	if err != nil {
		writeWireError(w, http.StatusUnauthorized, contract.ReasonAuthFailed)
		return
	}
	var req contract.HeartbeatNodeRequest
	if err := decodeJSON(w, r, s.cfg.MaxBodyBytes, &req); err != nil {
		writeDecodeError(w, err)
		return
	}
	if err := s.consumeReplay(peer, req.Scope); err != nil {
		writeHandlerError(w, err)
		return
	}
	if err := s.handler.Heartbeat(r.Context(), peer, req); err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleCordon(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeWireError(w, http.StatusMethodNotAllowed, contract.ReasonAuthFailed)
		return
	}
	peer, err := s.authenticate(r)
	if err != nil {
		writeWireError(w, http.StatusUnauthorized, contract.ReasonAuthFailed)
		return
	}
	var req contract.CordonNodeRequest
	if err := decodeJSON(w, r, s.cfg.MaxBodyBytes, &req); err != nil {
		writeDecodeError(w, err)
		return
	}
	if err := s.consumeReplay(peer, req.Scope); err != nil {
		writeHandlerError(w, err)
		return
	}
	if err := s.handler.Cordon(r.Context(), peer, req); err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleRelease(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeWireError(w, http.StatusMethodNotAllowed, contract.ReasonAuthFailed)
		return
	}
	peer, err := s.authenticate(r)
	if err != nil {
		writeWireError(w, http.StatusUnauthorized, contract.ReasonAuthFailed)
		return
	}
	var req contract.ReleaseNodeRequest
	if err := decodeJSON(w, r, s.cfg.MaxBodyBytes, &req); err != nil {
		writeDecodeError(w, err)
		return
	}
	if err := s.consumeReplay(peer, req.Scope); err != nil {
		writeHandlerError(w, err)
		return
	}
	if err := s.handler.Release(r.Context(), peer, req); err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeWireError(w, http.StatusMethodNotAllowed, contract.ReasonAuthFailed)
		return
	}
	peer, err := s.authenticate(r)
	if err != nil {
		writeWireError(w, http.StatusUnauthorized, contract.ReasonAuthFailed)
		return
	}
	var req contract.StatusNodeRequest
	if err := decodeJSON(w, r, s.cfg.MaxBodyBytes, &req); err != nil {
		writeDecodeError(w, err)
		return
	}
	if err := s.consumeReplay(peer, req.Scope); err != nil {
		writeHandlerError(w, err)
		return
	}
	out, err := s.handler.Status(r.Context(), peer, req)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) consumeReplay(peer string, scope contract.NodeRegistrationScope) error {
	key := agenttransport.ReplayKey(peer, scope.NodeID, "", "", "", "node_reg", scope.Generation, scope.Nonce)
	replay, err := s.replay.Consume(key, scope.ExpiresAt)
	if agenttransport.IsReplayCacheAtCapacity(err) {
		return agenttransport.ErrReplayCacheAtCapacity{}
	}
	if err != nil || replay {
		return errReplayRejected
	}
	return nil
}

func (s *Server) authenticate(r *http.Request) (string, error) {
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		return "", errors.New("no peer certificate")
	}
	peer := r.TLS.PeerCertificates[0]
	if err := agenttransport.ValidateLeafCertificateValidity(peer, time.Now().UTC()); err != nil {
		return "", err
	}
	return agenttransport.MatchPeerURIPrincipal(peer, s.cfg.AllowedNodeURIs)
}

type wireError struct {
	Reason contract.ReasonCode `json:"reason"`
}

func decodeJSON(w http.ResponseWriter, r *http.Request, maxBytes int64, dst interface{}) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); err != nil && err != io.EOF {
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeWireError(w http.ResponseWriter, status int, reason contract.ReasonCode) {
	writeJSON(w, status, wireError{Reason: reason})
}

func writeDecodeError(w http.ResponseWriter, err error) {
	if strings.Contains(err.Error(), "too large") {
		writeWireError(w, http.StatusRequestEntityTooLarge, contract.ReasonRequestTooLarge)
		return
	}
	writeWireError(w, http.StatusBadRequest, contract.ReasonAuthFailed)
}

func writeWireErrorWithRetry(w http.ResponseWriter, status int, reason contract.ReasonCode, retryAfter time.Duration) {
	if retryAfter > 0 {
		w.Header().Set("Retry-After", strconvItoa(int(retryAfter.Seconds())))
	}
	writeWireError(w, status, reason)
}

func strconvItoa(v int) string {
	if v <= 0 {
		return "1"
	}
	return fmtInt(v)
}

func fmtInt(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [16]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func writeHandlerError(w http.ResponseWriter, err error) {
	if errors.Is(err, nodereg.ErrRegistryUnavailable) || errors.Is(err, registrywire.ErrRegistryUnavailable) {
		writeWireError(w, http.StatusServiceUnavailable, contract.ReasonRegistryUnavailable)
		return
	}
	if agenttransport.IsReplayCacheAtCapacity(err) {
		writeWireErrorWithRetry(w, http.StatusServiceUnavailable, contract.ReasonCapacityExhausted, 5*time.Second)
		return
	}
	if errors.Is(err, errReplayRejected) {
		writeWireError(w, http.StatusConflict, contract.ReasonReplayRejected)
		return
	}
	var he *nodereg.HandlerError
	if errors.As(err, &he) && he.Reason != "" {
		writeWireError(w, nodereg.HandlerHTTPStatus(he), he.Reason)
		return
	}
	mapped := nodereg.MapHandlerError(err)
	if errors.As(mapped, &he) && he.Reason != "" {
		writeWireError(w, nodereg.HandlerHTTPStatus(he), he.Reason)
		return
	}
	writeWireError(w, http.StatusServiceUnavailable, contract.ReasonRegistryUnavailable)
}
