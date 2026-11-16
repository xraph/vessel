package vessel

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"
)

// Middleware provides hooks for intercepting container operations.
// Middleware can be used for logging, metrics, security, testing, etc.
type Middleware interface {
	// BeforeResolve is called before resolving a service.
	// Return error to abort resolution.
	BeforeResolve(ctx context.Context, name string) error

	// AfterResolve is called after resolving a service.
	// Called even if resolution failed (service and err may both be set).
	AfterResolve(ctx context.Context, name string, service any, err error) error

	// BeforeStart is called before starting a service.
	// Return error to abort start.
	BeforeStart(ctx context.Context, name string) error

	// AfterStart is called after starting a service.
	// Called even if start failed.
	AfterStart(ctx context.Context, name string, err error) error
}

// middlewareChain manages multiple middleware.
// The list is copy-on-write: Use replaces it, and every resolve reads the
// current list with a single atomic load and no copying.
type middlewareChain struct {
	middleware atomic.Pointer[[]Middleware]
	mu         sync.Mutex // serializes writers
}

// add appends middleware to the chain.
func (m *middlewareChain) add(middleware Middleware) {
	m.mu.Lock()
	defer m.mu.Unlock()

	next := append(slices.Clone(m.list()), middleware)
	m.middleware.Store(&next)
}

// list returns the current middleware. Callers must not modify it.
func (m *middlewareChain) list() []Middleware {
	if p := m.middleware.Load(); p != nil {
		return *p
	}

	return nil
}

// beforeResolve calls BeforeResolve on each middleware in mws.
func beforeResolve(mws []Middleware, ctx context.Context, name string) error {
	for _, mw := range mws {
		if err := mw.BeforeResolve(ctx, name); err != nil {
			return err
		}
	}
	return nil
}

// afterResolve calls AfterResolve on each middleware in mws.
func afterResolve(mws []Middleware, ctx context.Context, name string, service any, err error) error {
	for _, mw := range mws {
		if mwErr := mw.AfterResolve(ctx, name, service, err); mwErr != nil {
			return mwErr
		}
	}
	return nil
}

// beforeStart calls BeforeStart on each middleware in mws.
func beforeStart(mws []Middleware, ctx context.Context, name string) error {
	for _, mw := range mws {
		if err := mw.BeforeStart(ctx, name); err != nil {
			return err
		}
	}
	return nil
}

// afterStart calls AfterStart on each middleware in mws.
func afterStart(mws []Middleware, ctx context.Context, name string, err error) error {
	for _, mw := range mws {
		if mwErr := mw.AfterStart(ctx, name, err); mwErr != nil {
			return mwErr
		}
	}
	return nil
}

// FuncMiddleware wraps functions as Middleware.
type FuncMiddleware struct {
	BeforeResolveFunc func(ctx context.Context, name string) error
	AfterResolveFunc  func(ctx context.Context, name string, service any, err error) error
	BeforeStartFunc   func(ctx context.Context, name string) error
	AfterStartFunc    func(ctx context.Context, name string, err error) error
}

// BeforeResolve implements Middleware.
func (f *FuncMiddleware) BeforeResolve(ctx context.Context, name string) error {
	if f.BeforeResolveFunc != nil {
		return f.BeforeResolveFunc(ctx, name)
	}
	return nil
}

// AfterResolve implements Middleware.
func (f *FuncMiddleware) AfterResolve(ctx context.Context, name string, service any, err error) error {
	if f.AfterResolveFunc != nil {
		return f.AfterResolveFunc(ctx, name, service, err)
	}
	return nil
}

// BeforeStart implements Middleware.
func (f *FuncMiddleware) BeforeStart(ctx context.Context, name string) error {
	if f.BeforeStartFunc != nil {
		return f.BeforeStartFunc(ctx, name)
	}
	return nil
}

// AfterStart implements Middleware.
func (f *FuncMiddleware) AfterStart(ctx context.Context, name string, err error) error {
	if f.AfterStartFunc != nil {
		return f.AfterStartFunc(ctx, name, err)
	}
	return nil
}
