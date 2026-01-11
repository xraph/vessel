package vessel

import "context"

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
type middlewareChain struct {
	middleware []Middleware
}

// newMiddlewareChain creates a new middleware chain.
func newMiddlewareChain() *middlewareChain {
	return &middlewareChain{
		middleware: make([]Middleware, 0),
	}
}

// add appends middleware to the chain.
func (m *middlewareChain) add(middleware Middleware) {
	m.middleware = append(m.middleware, middleware)
}

// beforeResolve calls BeforeResolve on all middleware.
func (m *middlewareChain) beforeResolve(ctx context.Context, name string) error {
	for _, mw := range m.middleware {
		if err := mw.BeforeResolve(ctx, name); err != nil {
			return err
		}
	}
	return nil
}

// afterResolve calls AfterResolve on all middleware.
func (m *middlewareChain) afterResolve(ctx context.Context, name string, service any, err error) error {
	for _, mw := range m.middleware {
		if mwErr := mw.AfterResolve(ctx, name, service, err); mwErr != nil {
			return mwErr
		}
	}
	return nil
}

// beforeStart calls BeforeStart on all middleware.
func (m *middlewareChain) beforeStart(ctx context.Context, name string) error {
	for _, mw := range m.middleware {
		if err := mw.BeforeStart(ctx, name); err != nil {
			return err
		}
	}
	return nil
}

// afterStart calls AfterStart on all middleware.
func (m *middlewareChain) afterStart(ctx context.Context, name string, err error) error {
	for _, mw := range m.middleware {
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
