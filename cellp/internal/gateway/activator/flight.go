package activator

import (
	"context"
	"errors"
	"sync"
)

var errGroupClosed = errors.New("activator: closed")

// Group merges in-flight work by key and lets each caller stop waiting without
// cancelling work shared by other callers.
type Group struct {
	mu     sync.Mutex
	m      map[string]*call
	closed bool
	wg     sync.WaitGroup
}

type call struct {
	done chan struct{}
	val  interface{}
	err  error
}

// Do starts fn once per key and waits for either the shared result or ctx.
func (g *Group) Do(ctx context.Context, key string, fn func() (interface{}, error)) (interface{}, error, bool) {
	c, leader, err := g.start(key, fn)
	if err != nil {
		return nil, err, false
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err(), leader
	case <-c.done:
		return c.val, c.err, leader
	}
}

// Start starts shared work without waiting for it to finish.
func (g *Group) Start(key string, fn func() (interface{}, error)) error {
	_, _, err := g.start(key, fn)
	return err
}

// Has reports whether a flight for key is already in progress.
func (g *Group) Has(key string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.m != nil && g.m[key] != nil
}

func (g *Group) start(key string, fn func() (interface{}, error)) (*call, bool, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return nil, false, errGroupClosed
	}
	if g.m == nil {
		g.m = make(map[string]*call)
	}
	if c, ok := g.m[key]; ok {
		return c, false, nil
	}
	c := &call{done: make(chan struct{})}
	g.m[key] = c
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		c.val, c.err = fn()
		g.mu.Lock()
		delete(g.m, key)
		close(c.done)
		g.mu.Unlock()
	}()
	return c, true, nil
}

// Shutdown rejects new work and waits for every admitted flight to stop.
func (g *Group) Shutdown(ctx context.Context) error {
	g.mu.Lock()
	g.closed = true
	g.mu.Unlock()
	done := make(chan struct{})
	go func() {
		g.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
