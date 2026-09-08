package gateway

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/cellp/cellp/internal/registry"
)

type listenerEntry struct {
	srv  *http.Server
	ln   net.Listener
	done chan struct{}
}

// ListenerManager owns dedicated 127.0.0.1 ingress listeners (INGRESS-PORT P5c).
type ListenerManager struct {
	mu      sync.Mutex
	gw      *Gateway
	store   registry.Store
	cfg     GatewayConfig
	servers map[int]*listenerEntry
}

// NewListenerManager creates a manager for per-port gateway listeners.
func NewListenerManager(gw *Gateway, store registry.Store, cfg GatewayConfig) *ListenerManager {
	return &ListenerManager{
		gw:      gw,
		store:   store,
		cfg:     cfg,
		servers: make(map[int]*listenerEntry),
	}
}

// ReconcileIngressListeners aligns TCP listeners with active ingress_listen ledger rows.
func (lm *ListenerManager) ReconcileIngressListeners(ctx context.Context) error {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	return lm.reconcileLocked(ctx)
}

func (lm *ListenerManager) reconcileLocked(ctx context.Context) error {
	selfGW := strings.TrimSpace(lm.cfg.GatewayID)
	active, err := lm.store.ListActivePortAllocations(ctx, registry.PortPurposeIngressListen)
	if err != nil {
		return err
	}

	desired := make(map[int]struct{})
	for _, pa := range active {
		if pa.GatewayID == nil || strings.TrimSpace(*pa.GatewayID) != selfGW {
			continue
		}
		if selfGW == "" {
			continue
		}
		b, err := lm.store.GetIngressBinding(ctx, pa.OwnerID)
		if err != nil {
			return err
		}
		if b != nil && b.Active && b.ListenPort != nil && *b.ListenPort == pa.Port {
			desired[pa.Port] = struct{}{}
			continue
		}
		log.Printf("ingress listeners: orphan allocation port=%d allocation_id=%s owner=%s action=release reason=orphan_reconcile",
			pa.Port, pa.AllocationID, pa.OwnerID)
		if err := lm.store.ReleasePort(ctx, registry.ReleasePortInput{
			AllocationID:  pa.AllocationID,
			ReleaseReason: "orphan_reconcile",
		}); err != nil && !errors.Is(err, registry.ErrPortAllocationNotFound) {
			return fmt.Errorf("release orphan port %d: %w", pa.Port, err)
		}
	}

	for port := range desired {
		if _, ok := lm.servers[port]; ok {
			continue
		}
		if err := lm.startLocked(port); err != nil {
			return err
		}
	}
	for port, ent := range lm.servers {
		if _, ok := desired[port]; ok {
			continue
		}
		if err := lm.shutdownEntryLocked(ctx, port, ent); err != nil {
			return err
		}
	}
	return nil
}

func (lm *ListenerManager) handlerForPort(port int) http.Handler {
	base := lm.gw.Handler()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(WithLocalListenPort(r.Context(), port))
		base.ServeHTTP(w, r)
	})
}

func (lm *ListenerManager) startLocked(port int) error {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}
	srv := &http.Server{Handler: lm.handlerForPort(port)}
	ent := &listenerEntry{srv: srv, ln: ln, done: make(chan struct{})}
	lm.servers[port] = ent
	go func() {
		defer close(ent.done)
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("ingress listener port=%d: %v", port, err)
		}
	}()
	log.Printf("ingress listener: listening on http://127.0.0.1:%d", port)
	return nil
}

func (lm *ListenerManager) shutdownEntryLocked(ctx context.Context, port int, ent *listenerEntry) error {
	if ent == nil {
		return nil
	}
	if err := ent.srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("ingress listener port %d shutdown: %w", port, err)
	}
	if err := waitIngressListener(ctx, ent.done); err != nil {
		return fmt.Errorf("ingress listener port %d serve: %w", port, err)
	}
	delete(lm.servers, port)
	log.Printf("ingress listener: closed http://127.0.0.1:%d", port)
	return nil
}

func waitIngressListener(ctx context.Context, done <-chan struct{}) error {
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// CloseAll shuts down dedicated ingress listeners (cellpd shutdown).
func (lm *ListenerManager) CloseAll(ctx context.Context) error {
	lm.mu.Lock()
	ports := make([]int, 0, len(lm.servers))
	entries := make([]*listenerEntry, 0, len(lm.servers))
	for port, ent := range lm.servers {
		ports = append(ports, port)
		entries = append(entries, ent)
	}
	lm.mu.Unlock()
	var errs []error
	for i, port := range ports {
		lm.mu.Lock()
		ent := lm.servers[port]
		if ent != entries[i] {
			lm.mu.Unlock()
			continue
		}
		err := lm.shutdownEntryLocked(ctx, port, ent)
		lm.mu.Unlock()
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
