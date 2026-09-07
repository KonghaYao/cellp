package transport

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	agenttransport "github.com/cellp/cellp/internal/elastic/agent/transport"
	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/elastic/registryrelay"
)

// ServerConfig configures the controller registry-relay HTTPS listener.
type ServerConfig struct {
	BindAddr              string
	ControllerIdentityURI string
	AllowedNodeURIs       []string
	TLS                   agenttransport.TLSMaterials
	MaxBodyBytes          int64
	ReplayMaxEntries      int
	ReadTimeout           time.Duration
	WriteTimeout          time.Duration
}

// Server serves registry relay RPCs to authenticated runtime nodes.
type Server struct {
	cfg     ServerConfig
	handler *registryrelay.Handler
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

// NewServer builds the registry relay server.
func NewServer(cfg ServerConfig, h *registryrelay.Handler) (*Server, error) {
	if h == nil {
		return nil, errors.New("handler required")
	}
	if err := contract.ValidateNodeIdentityAllowlist(h.Allowlist); err != nil {
		return nil, err
	}
	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = 1 << 20
	}
	if cfg.ReplayMaxEntries <= 0 {
		cfg.ReplayMaxEntries = 4096
	}
	if cfg.ReadTimeout <= 0 {
		cfg.ReadTimeout = 15 * time.Second
	}
	if cfg.WriteTimeout <= 0 {
		cfg.WriteTimeout = 15 * time.Second
	}
	s := &Server{
		cfg:     cfg,
		handler: h,
		replay:  agenttransport.NewReplayCache(cfg.ReplayMaxEntries),
		mux:     http.NewServeMux(),
		ready:   make(chan struct{}),
	}
	s.registerRoutes()
	return s, nil
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc(routeGetRuntimeNode, s.handleGetRuntimeNode)
	s.mux.HandleFunc(routeGetRuntimeReplica, s.handleGetRuntimeReplica)
	s.mux.HandleFunc(routeValidateAgentAssignment, s.handleValidateAssignment)
	s.mux.HandleFunc(routeValidateAgentCleanupAssignment, s.handleValidateCleanup)
	s.mux.HandleFunc(routeListRuntimeReplicasByNode, s.handleListByNode)
	s.mux.HandleFunc(routeRecordObservation, s.handleRecordObservation)
	s.mux.HandleFunc(routeClaimAgentCommand, s.handleClaimCommand)
	s.mux.HandleFunc(routeRenewAgentCommandLease, s.handleRenewCommand)
	s.mux.HandleFunc(routeCompleteAgentCommand, s.handleCompleteCommand)
	s.mux.HandleFunc(routeRecordObservationAndComplete, s.handleRecordAndComplete)
	s.mux.HandleFunc(routeWithdrawReplica, s.handleWithdraw)
	s.mux.HandleFunc(routeTerminalizeReplica, s.handleTerminalize)
	s.mux.HandleFunc(routeGetVersionEnv, s.handleGetVersionEnv)
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
		ReadTimeout:       s.cfg.ReadTimeout,
		WriteTimeout:      s.cfg.WriteTimeout,
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

var errReplayRejected = &registryrelay.RelayError{Reason: contract.ReasonReplayRejected, Message: "replay rejected"}

func (s *Server) consumeReplay(peer string, route string, scope contract.RegistryRelayScope) error {
	key := agenttransport.ReplayKey(peer, scope.NodeID, "", "", "", route, 0, scope.Nonce)
	replay, err := s.replay.Consume(key, scope.ExpiresAt)
	if agenttransport.IsReplayCacheAtCapacity(err) {
		return errReplayCacheCapacity
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

func (s *Server) handleGetRuntimeNode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeWireError(w, http.StatusMethodNotAllowed, contract.ReasonAuthFailed)
		return
	}
	peer, err := s.authenticate(r)
	if err != nil {
		writeWireError(w, http.StatusUnauthorized, contract.ReasonAuthFailed)
		return
	}
	var req envelope
	if err := decodeJSON(w, r, s.cfg.MaxBodyBytes, &req); err != nil {
		writeDecodeError(w, err)
		return
	}
	if err := s.consumeReplay(peer, routeGetRuntimeNode, req.Scope); err != nil {
		writeHandlerError(w, err)
		return
	}
	node, err := s.handler.GetRuntimeNode(r.Context(), peer, req.Scope)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, runtimeNodeResponse{Node: runtimeNodeToWire(node)})
}

func (s *Server) handleGetRuntimeReplica(w http.ResponseWriter, r *http.Request) {
	postJSON(s, w, r, func(ctx context.Context, peer string, req getRuntimeReplicaRequest) (interface{}, error) {
		if err := s.consumeReplay(peer, routeGetRuntimeReplica, req.Scope); err != nil {
			return nil, err
		}
		rep, err := s.handler.GetRuntimeReplica(ctx, peer, req.Scope, req.ReplicaID)
		if err != nil {
			return nil, err
		}
		return runtimeReplicaResponse{Replica: runtimeReplicaToWire(rep)}, nil
	})
}

func (s *Server) handleValidateAssignment(w http.ResponseWriter, r *http.Request) {
	postJSON(s, w, r, func(ctx context.Context, peer string, req validateAssignmentRequest) (interface{}, error) {
		if err := s.consumeReplay(peer, routeValidateAgentAssignment, req.Scope); err != nil {
			return nil, err
		}
		rep, err := s.handler.ValidateAgentAssignment(ctx, peer, req.Scope, req.CmdScope, req.Now)
		if err != nil {
			return nil, err
		}
		return runtimeReplicaResponse{Replica: runtimeReplicaToWire(rep)}, nil
	})
}

func (s *Server) handleValidateCleanup(w http.ResponseWriter, r *http.Request) {
	postJSON(s, w, r, func(ctx context.Context, peer string, req validateCleanupRequest) (interface{}, error) {
		if err := s.consumeReplay(peer, routeValidateAgentCleanupAssignment, req.Scope); err != nil {
			return nil, err
		}
		rep, err := s.handler.ValidateAgentCleanupAssignment(ctx, peer, req.Scope, req.CmdScope)
		if err != nil {
			return nil, err
		}
		return runtimeReplicaResponse{Replica: runtimeReplicaToWire(rep)}, nil
	})
}

func (s *Server) handleListByNode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeWireError(w, http.StatusMethodNotAllowed, contract.ReasonAuthFailed)
		return
	}
	peer, err := s.authenticate(r)
	if err != nil {
		writeWireError(w, http.StatusUnauthorized, contract.ReasonAuthFailed)
		return
	}
	var req envelope
	if err := decodeJSON(w, r, s.cfg.MaxBodyBytes, &req); err != nil {
		writeDecodeError(w, err)
		return
	}
	if err := s.consumeReplay(peer, routeListRuntimeReplicasByNode, req.Scope); err != nil {
		writeHandlerError(w, err)
		return
	}
	reps, err := s.handler.ListRuntimeReplicasByNode(r.Context(), peer, req.Scope)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, replicasResponse{Replicas: runtimeReplicasToWire(reps)})
}

func (s *Server) handleRecordObservation(w http.ResponseWriter, r *http.Request) {
	postJSON(s, w, r, func(ctx context.Context, peer string, req recordObservationRequest) (interface{}, error) {
		if err := s.consumeReplay(peer, routeRecordObservation, req.Scope); err != nil {
			return nil, err
		}
		if err := s.handler.RecordObservation(ctx, peer, req.Scope, req.Obs); err != nil {
			return nil, err
		}
		return map[string]string{"status": "ok"}, nil
	})
}

func (s *Server) handleClaimCommand(w http.ResponseWriter, r *http.Request) {
	postJSON(s, w, r, func(ctx context.Context, peer string, req claimCommandRequest) (interface{}, error) {
		if err := s.consumeReplay(peer, routeClaimAgentCommand, req.Scope); err != nil {
			return nil, err
		}
		claim, err := s.handler.ClaimAgentCommand(ctx, peer, req.Scope, req.Command)
		if err != nil {
			return nil, err
		}
		return claimResponse{Claim: claim}, nil
	})
}

func (s *Server) handleRenewCommand(w http.ResponseWriter, r *http.Request) {
	postJSON(s, w, r, func(ctx context.Context, peer string, req renewCommandRequest) (interface{}, error) {
		if err := s.consumeReplay(peer, routeRenewAgentCommandLease, req.Scope); err != nil {
			return nil, err
		}
		if err := s.handler.RenewAgentCommandLease(ctx, peer, req.Scope, req.Command, req.Expiry); err != nil {
			return nil, err
		}
		return map[string]string{"status": "ok"}, nil
	})
}

func (s *Server) handleCompleteCommand(w http.ResponseWriter, r *http.Request) {
	postJSON(s, w, r, func(ctx context.Context, peer string, req completeCommandRequest) (interface{}, error) {
		if err := s.consumeReplay(peer, routeCompleteAgentCommand, req.Scope); err != nil {
			return nil, err
		}
		if err := s.handler.CompleteAgentCommand(ctx, peer, req.Scope, req.Command); err != nil {
			return nil, err
		}
		return map[string]string{"status": "ok"}, nil
	})
}

func (s *Server) handleRecordAndComplete(w http.ResponseWriter, r *http.Request) {
	postJSON(s, w, r, func(ctx context.Context, peer string, req recordAndCompleteRequest) (interface{}, error) {
		if err := s.consumeReplay(peer, routeRecordObservationAndComplete, req.Scope); err != nil {
			return nil, err
		}
		if err := s.handler.RecordObservationAndCompleteAgentCommand(ctx, peer, req.Scope, req.Obs, req.Command); err != nil {
			return nil, err
		}
		return map[string]string{"status": "ok"}, nil
	})
}

func (s *Server) handleWithdraw(w http.ResponseWriter, r *http.Request) {
	postJSON(s, w, r, func(ctx context.Context, peer string, req withdrawRequest) (interface{}, error) {
		if err := s.consumeReplay(peer, routeWithdrawReplica, req.Scope); err != nil {
			return nil, err
		}
		if err := s.handler.WithdrawReplica(ctx, peer, req.Scope, req.ReplicaID, req.ProjectID, req.VersionID, req.Generation); err != nil {
			return nil, err
		}
		return map[string]string{"status": "ok"}, nil
	})
}

func (s *Server) handleTerminalize(w http.ResponseWriter, r *http.Request) {
	postJSON(s, w, r, func(ctx context.Context, peer string, req terminalizeRequest) (interface{}, error) {
		if err := s.consumeReplay(peer, routeTerminalizeReplica, req.Scope); err != nil {
			return nil, err
		}
		if err := s.handler.TerminalizeReplica(ctx, peer, req.Scope, req.ReplicaID, req.Generation, req.State); err != nil {
			return nil, err
		}
		return map[string]string{"status": "ok"}, nil
	})
}

func (s *Server) handleGetVersionEnv(w http.ResponseWriter, r *http.Request) {
	postJSON(s, w, r, func(ctx context.Context, peer string, req getVersionEnvRequest) (interface{}, error) {
		if err := s.consumeReplay(peer, routeGetVersionEnv, req.Scope); err != nil {
			return nil, err
		}
		env, err := s.handler.GetVersionEnv(ctx, peer, req.Scope, req.ProjectID, req.VersionID)
		if err != nil {
			return nil, err
		}
		if env == nil {
			env = map[string]string{}
		}
		return versionEnvResponse{Env: env}, nil
	})
}

func postJSON[T any](s *Server, w http.ResponseWriter, r *http.Request, fn func(context.Context, string, T) (interface{}, error)) {
	if r.Method != http.MethodPost {
		writeWireError(w, http.StatusMethodNotAllowed, contract.ReasonAuthFailed)
		return
	}
	peer, err := s.authenticate(r)
	if err != nil {
		writeWireError(w, http.StatusUnauthorized, contract.ReasonAuthFailed)
		return
	}
	var req T
	if err := decodeJSON(w, r, s.cfg.MaxBodyBytes, &req); err != nil {
		writeDecodeError(w, err)
		return
	}
	out, err := fn(r.Context(), peer, req)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
