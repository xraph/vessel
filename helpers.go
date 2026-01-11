package vessel

import (
	"context"
	"fmt"

	"github.com/xraph/go-utils/di"
	logger "github.com/xraph/go-utils/log"
	"github.com/xraph/go-utils/metrics"
)

// Resolve with type safety.
func Resolve[T any](c Vessel, name string) (T, error) {
	var zero T

	instance, err := c.Resolve(name)
	if err != nil {
		return zero, err
	}

	typed, ok := instance.(T)
	if !ok {
		return zero, fmt.Errorf("service %s: type mismatch, expected %T but got %T", name, zero, instance)
	}

	return typed, nil
}

// Must resolves or panics - use only during startup.
func Must[T any](c Vessel, name string) T {
	instance, err := Resolve[T](c, name)
	if err != nil {
		panic(fmt.Sprintf("failed to resolve %s: %v", name, err))
	}

	return instance
}

// ResolveReady resolves a service with type safety, ensuring it and its dependencies are started first.
// This is useful during extension Register() phase when you need a dependency
// to be fully initialized before use.
func ResolveReady[T any](ctx context.Context, c Vessel, name string) (T, error) {
	var zero T

	instance, err := c.ResolveReady(ctx, name)
	if err != nil {
		return zero, err
	}

	typed, ok := instance.(T)
	if !ok {
		return zero, fmt.Errorf("service %s: type mismatch, expected %T but got %T", name, zero, instance)
	}

	return typed, nil
}

// MustResolveReady resolves or panics, ensuring the service is started first.
// Use only during startup/registration phase.
func MustResolveReady[T any](ctx context.Context, c Vessel, name string) T {
	instance, err := ResolveReady[T](ctx, c, name)
	if err != nil {
		panic(fmt.Sprintf("failed to resolve ready %s: %v", name, err))
	}

	return instance
}

// RegisterSingleton is a convenience wrapper for singleton services.
func RegisterSingleton[T any](c Vessel, name string, factory func(Vessel) (T, error)) error {
	return c.Register(name, func(c Vessel) (any, error) {
		return factory(c)
	}, Singleton())
}

// RegisterTransient is a convenience wrapper for transient services.
func RegisterTransient[T any](c Vessel, name string, factory func(Vessel) (T, error)) error {
	return c.Register(name, func(c Vessel) (any, error) {
		return factory(c)
	}, Transient())
}

// RegisterScoped is a convenience wrapper for request-scoped services.
func RegisterScoped[T any](c Vessel, name string, factory func(Vessel) (T, error)) error {
	return c.Register(name, func(c Vessel) (any, error) {
		return factory(c)
	}, Scoped())
}

// RegisterSingletonWith registers a singleton service with typed dependency injection.
// This is a convenience wrapper around Provide with Singleton lifecycle.
//
// Usage:
//
//	vessel.RegisterSingletonWith[*UserService](c, "userService",
//	    vessel.Inject[*DB]("database"),
//	    func(db *DB) (*UserService, error) {
//	        return &UserService{db: db}, nil
//	    },
//	)
func RegisterSingletonWith[T any](c Vessel, name string, args ...any) error {
	return ProvideWithOpts[T](c, name, []di.RegisterOption{Singleton()}, args...)
}

// RegisterTransientWith registers a transient service with typed dependency injection.
// This is a convenience wrapper around Provide with Transient lifecycle.
//
// Usage:
//
//	vessel.RegisterTransientWith[*Request](c, "request",
//	    vessel.Inject[*Context]("ctx"),
//	    func(ctx *Context) (*Request, error) {
//	        return &Request{ctx: ctx}, nil
//	    },
//	)
func RegisterTransientWith[T any](c Vessel, name string, args ...any) error {
	return ProvideWithOpts[T](c, name, []di.RegisterOption{Transient()}, args...)
}

// RegisterScopedWith registers a scoped service with typed dependency injection.
// This is a convenience wrapper around Provide with Scoped lifecycle.
//
// Usage:
//
//	vessel.RegisterScopedWith[*Session](c, "session",
//	    vessel.Inject[*User]("user"),
//	    func(user *User) (*Session, error) {
//	        return &Session{user: user}, nil
//	    },
//	)
func RegisterScopedWith[T any](c Vessel, name string, args ...any) error {
	return ProvideWithOpts[T](c, name, []di.RegisterOption{Scoped()}, args...)
}

// RegisterInterface registers an implementation as an interface
// Supports all lifecycle options (Singleton, Scoped, Transient).
func RegisterInterface[I, T any](c Vessel, name string, factory func(Vessel) (T, error), opts ...RegisterOption) error {
	return c.Register(name, func(c Vessel) (any, error) {
		impl, err := factory(c)
		if err != nil {
			return nil, err
		}
		// Return as any - the type will be checked at resolve time
		return any(impl), nil
	}, opts...)
}

// RegisterValue registers a pre-built instance (always singleton).
func RegisterValue[T any](c Vessel, name string, instance T) error {
	return c.Register(name, func(c Vessel) (any, error) {
		return instance, nil
	}, Singleton())
}

// RegisterSingletonInterface is a convenience wrapper.
func RegisterSingletonInterface[I, T any](c Vessel, name string, factory func(Vessel) (T, error)) error {
	return RegisterInterface[I, T](c, name, factory, Singleton())
}

// RegisterScopedInterface is a convenience wrapper.
func RegisterScopedInterface[I, T any](c Vessel, name string, factory func(Vessel) (T, error)) error {
	return RegisterInterface[I, T](c, name, factory, Scoped())
}

// RegisterTransientInterface is a convenience wrapper.
func RegisterTransientInterface[I, T any](c Vessel, name string, factory func(Vessel) (T, error)) error {
	return RegisterInterface[I, T](c, name, factory, Transient())
}

// ResolveScope is a helper for resolving from a scope.
func ResolveScope[T any](s Scope, name string) (T, error) {
	var zero T

	instance, err := s.Resolve(name)
	if err != nil {
		return zero, err
	}

	typed, ok := instance.(T)
	if !ok {
		return zero, fmt.Errorf("service %s: type mismatch, expected %T but got %T", name, zero, instance)
	}

	return typed, nil
}

// MustScope resolves from scope or panics.
func MustScope[T any](s Scope, name string) T {
	instance, err := ResolveScope[T](s, name)
	if err != nil {
		panic(fmt.Sprintf("failed to resolve %s from scope: %v", name, err))
	}

	return instance
}

// GetLogger resolves the logger from the container
// This is a convenience function for resolving the logger service
// The logger type is defined in the forge package, so this returns interface{}
// and should be type-asserted to the appropriate logger interface.
func GetLogger(c Vessel) (logger.Logger, error) {
	return Resolve[logger.Logger](c, "logger")
}

// GetMetrics resolves the metrics from the container
// This is a convenience function for resolving the metrics service
// The metrics type is defined in the forge package, so this returns interface{}
// and should be type-asserted to the appropriate metrics interface.
func GetMetrics(c Vessel) (metrics.Metrics, error) {
	m, err := c.Resolve("metrics")
	if err != nil {
		return nil, err
	}

	metrics, ok := m.(metrics.Metrics)
	if !ok {
		return nil, fmt.Errorf("resolved instance is not Metrics, got %T", m)
	}

	return metrics, nil
}
