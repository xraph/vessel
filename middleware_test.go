package vessel

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/xraph/go-utils/di"
)

func TestMiddleware_BeforeAfterResolve(t *testing.T) {
	c := New().(*containerImpl)

	// Track middleware calls
	var calls []string

	mw := &FuncMiddleware{
		BeforeResolveFunc: func(ctx context.Context, name string) error {
			calls = append(calls, "before:"+name)
			return nil
		},
		AfterResolveFunc: func(ctx context.Context, name string, service any, err error) error {
			calls = append(calls, "after:"+name)
			return nil
		},
	}

	c.Use(mw)

	// Register a simple service
	err := RegisterSingleton(c, "test", func(c Vessel) (*testService, error) {
		return &testService{value: "test"}, nil
	})
	assert.NoError(t, err)

	// Resolve the service
	svc, err := Resolve[*testService](c, "test")
	assert.NoError(t, err)
	assert.NotNil(t, svc)

	// Check middleware was called
	assert.Equal(t, []string{"before:test", "after:test"}, calls)
}

func TestMiddleware_BeforeResolveError(t *testing.T) {
	c := New().(*containerImpl)

	expectedErr := errors.New("access denied")

	mw := &FuncMiddleware{
		BeforeResolveFunc: func(ctx context.Context, name string) error {
			return expectedErr
		},
	}

	c.Use(mw)

	// Register a service
	err := RegisterSingleton(c, "test", func(c Vessel) (*testService, error) {
		return &testService{value: "test"}, nil
	})
	assert.NoError(t, err)

	// Resolve should fail due to middleware
	_, err = Resolve[*testService](c, "test")
	assert.ErrorIs(t, err, expectedErr)
}

func TestMiddleware_AfterResolveError(t *testing.T) {
	c := New().(*containerImpl)

	expectedErr := errors.New("post-resolve validation failed")

	mw := &FuncMiddleware{
		AfterResolveFunc: func(ctx context.Context, name string, service any, err error) error {
			return expectedErr
		},
	}

	c.Use(mw)

	// Register a service
	err := RegisterSingleton(c, "test", func(c Vessel) (*testService, error) {
		return &testService{value: "test"}, nil
	})
	assert.NoError(t, err)

	// Resolve should fail due to middleware
	_, err = Resolve[*testService](c, "test")
	assert.ErrorIs(t, err, expectedErr)
}

func TestMiddleware_BeforeAfterStart(t *testing.T) {
	c := New().(*containerImpl)

	// Track middleware calls
	var calls []string

	mw := &FuncMiddleware{
		BeforeStartFunc: func(ctx context.Context, name string) error {
			calls = append(calls, "beforeStart:"+name)
			return nil
		},
		AfterStartFunc: func(ctx context.Context, name string, err error) error {
			calls = append(calls, "afterStart:"+name)
			return nil
		},
	}

	c.Use(mw)

	// Register a service that implements di.Service
	err := RegisterSingleton(c, "svc", func(c Vessel) (di.Service, error) {
		return &mockService{name: "svc"}, nil
	})
	assert.NoError(t, err)

	// Resolve the service (should auto-start)
	svc, err := c.Resolve("svc")
	assert.NoError(t, err)
	assert.NotNil(t, svc)

	// Check middleware was called for both resolve and start
	assert.Contains(t, calls, "beforeStart:svc")
	assert.Contains(t, calls, "afterStart:svc")
}

func TestMiddleware_MultipleMiddleware(t *testing.T) {
	c := New().(*containerImpl)

	// Track middleware calls
	var calls []string

	mw1 := &FuncMiddleware{
		BeforeResolveFunc: func(ctx context.Context, name string) error {
			calls = append(calls, "mw1:before")
			return nil
		},
		AfterResolveFunc: func(ctx context.Context, name string, service any, err error) error {
			calls = append(calls, "mw1:after")
			return nil
		},
	}

	mw2 := &FuncMiddleware{
		BeforeResolveFunc: func(ctx context.Context, name string) error {
			calls = append(calls, "mw2:before")
			return nil
		},
		AfterResolveFunc: func(ctx context.Context, name string, service any, err error) error {
			calls = append(calls, "mw2:after")
			return nil
		},
	}

	c.Use(mw1)
	c.Use(mw2)

	// Register and resolve a service
	err := RegisterSingleton(c, "test", func(c Vessel) (*testService, error) {
		return &testService{value: "test"}, nil
	})
	assert.NoError(t, err)

	_, err = Resolve[*testService](c, "test")
	assert.NoError(t, err)

	// Middleware should be called in order (FIFO for before, FIFO for after)
	assert.Equal(t, []string{
		"mw1:before",
		"mw2:before",
		"mw1:after",
		"mw2:after",
	}, calls)
}

func TestMiddleware_AfterResolveReceivesError(t *testing.T) {
	c := New().(*containerImpl)

	var capturedErr error

	mw := &FuncMiddleware{
		AfterResolveFunc: func(ctx context.Context, name string, service any, err error) error {
			capturedErr = err
			return nil // Don't block the error
		},
	}

	c.Use(mw)

	// Register a service that fails
	expectedErr := errors.New("factory failed")
	err := RegisterSingleton(c, "failing", func(c Vessel) (*testService, error) {
		return nil, expectedErr
	})
	assert.NoError(t, err)

	// Resolve should fail
	_, err = Resolve[*testService](c, "failing")
	assert.Error(t, err)

	// Middleware should have captured the error
	assert.NotNil(t, capturedErr)
	assert.Contains(t, capturedErr.Error(), "factory failed")
}
