package vessel

import (
	"context"
	"fmt"
	"reflect"

	"github.com/xraph/go-utils/di"
)

// Provide registers a service with typed dependency injection.
// It accepts InjectOption arguments followed by a factory function.
//
// The factory function receives the resolved dependencies in order and returns
// the service instance and an optional error.
//
// Usage:
//
//	di.Provide[*UserService](c, "userService",
//	    di.Inject[*bun.DB]("database"),
//	    di.Inject[Logger]("logger"),
//	    di.LazyInject[*Cache]("cache"),
//	    func(db *bun.DB, logger Logger, cache *Lazy[*Cache]) (*UserService, error) {
//	        return &UserService{db, logger, cache}, nil
//	    },
//	)
func Provide[T any](c Vessel, name string, args ...any) error {
	// Separate inject options from the factory function
	var (
		injectOpts []InjectOption
		factoryFn  any
	)

	for _, arg := range args {
		switch v := arg.(type) {
		case InjectOption:
			injectOpts = append(injectOpts, v)
		case di.RegisterOption:
			// Ignore register options here, they're handled separately
		default:
			// Assume it's the factory function
			if factoryFn != nil {
				return fmt.Errorf("provide %s: multiple factory functions provided", name)
			}

			factoryFn = arg
		}
	}

	if factoryFn == nil {
		return fmt.Errorf("provide %s: no factory function provided", name)
	}

	// Extract dependencies for the graph
	deps := ExtractDeps(injectOpts)

	// Create the wrapper factory that resolves dependencies
	factory := func(container Vessel) (any, error) {
		// Resolve all dependencies according to their modes
		resolvedDeps := make([]any, len(injectOpts))

		for i, opt := range injectOpts {
			resolved, err := resolveDep(container, opt)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve dependency %s: %w", opt.Dep.Name, err)
			}

			resolvedDeps[i] = resolved
		}

		// Call the factory function with resolved dependencies
		return callFactory(factoryFn, resolvedDeps)
	}

	// Register with the container using the new deps
	return c.Register(name, factory, di.WithDeps(deps...))
}

// ProvideWithOpts is like Provide but accepts additional RegisterOptions.
func ProvideWithOpts[T any](c Vessel, name string, opts []di.RegisterOption, args ...any) error {
	// Separate inject options from the factory function
	var (
		injectOpts []InjectOption
		factoryFn  any
	)

	for _, arg := range args {
		switch v := arg.(type) {
		case InjectOption:
			injectOpts = append(injectOpts, v)
		default:
			if factoryFn != nil {
				return fmt.Errorf("provide %s: multiple factory functions provided", name)
			}

			factoryFn = arg
		}
	}

	if factoryFn == nil {
		return fmt.Errorf("provide %s: no factory function provided", name)
	}

	// Extract dependencies for the graph
	deps := ExtractDeps(injectOpts)

	// Create the wrapper factory that resolves dependencies
	factory := func(container Vessel) (any, error) {
		resolvedDeps := make([]any, len(injectOpts))

		for i, opt := range injectOpts {
			resolved, err := resolveDep(container, opt)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve dependency %s: %w", opt.Dep.Name, err)
			}

			resolvedDeps[i] = resolved
		}

		return callFactory(factoryFn, resolvedDeps)
	}

	// Merge deps into options
	allOpts := append(opts, di.WithDeps(deps...))

	return c.Register(name, factory, allOpts...)
}

// resolveDep resolves a single dependency based on its mode.
func resolveDep(c Vessel, opt InjectOption) (any, error) {
	switch opt.Dep.Mode {
	case di.DepEager:
		// Resolve immediately, fail if not found
		return c.Resolve(opt.Dep.Name)

	case di.DepLazy:
		// Return a Lazy wrapper
		return createLazyWrapper(c, opt)

	case di.DepOptional:
		// Resolve immediately, return nil if not found
		if !c.Has(opt.Dep.Name) {
			return nil, nil
		}

		return c.Resolve(opt.Dep.Name)

	case di.DepLazyOptional:
		// Return an OptionalLazy wrapper
		return createOptionalLazyWrapper(c, opt)

	default:
		return nil, fmt.Errorf("unknown dependency mode: %v", opt.Dep.Mode)
	}
}

// createLazyWrapper creates a Lazy[T] wrapper for the dependency.
// Since we can't use generics dynamically, we return a LazyAny wrapper.
func createLazyWrapper(c Vessel, opt InjectOption) (*LazyAny, error) {
	return NewLazyAny(c, opt.Dep.Name, opt.TypeInfo), nil
}

// createOptionalLazyWrapper creates an OptionalLazy wrapper.
func createOptionalLazyWrapper(c Vessel, opt InjectOption) (*OptionalLazyAny, error) {
	return NewOptionalLazyAny(c, opt.Dep.Name, opt.TypeInfo), nil
}

// callFactory calls the factory function with the resolved dependencies.
func callFactory(factoryFn any, deps []any) (any, error) {
	fnValue := reflect.ValueOf(factoryFn)
	fnType := fnValue.Type()

	if fnType.Kind() != reflect.Func {
		return nil, fmt.Errorf("factory must be a function, got %T", factoryFn)
	}

	// Verify parameter count matches
	if fnType.NumIn() != len(deps) {
		return nil, fmt.Errorf("factory expects %d parameters, got %d dependencies", fnType.NumIn(), len(deps))
	}

	// Build arguments
	args := make([]reflect.Value, len(deps))
	for i, dep := range deps {
		if dep == nil {
			// For optional deps, use zero value
			args[i] = reflect.Zero(fnType.In(i))
		} else {
			args[i] = reflect.ValueOf(dep)
		}
	}

	// Call the factory
	results := fnValue.Call(args)

	// Handle return values
	switch fnType.NumOut() {
	case 1:
		// Returns only the service
		return results[0].Interface(), nil
	case 2:
		// Returns service and error
		if !results[1].IsNil() {
			return nil, results[1].Interface().(error)
		}

		return results[0].Interface(), nil
	default:
		return nil, fmt.Errorf("factory must return (T) or (T, error), got %d return values", fnType.NumOut())
	}
}

// LazyAny is a non-generic lazy wrapper that can hold any type.
// This is needed because we can't create Lazy[T] dynamically at runtime.
type LazyAny struct {
	container  Vessel
	name       string
	expectedTy reflect.Type
	resolved   bool
	value      any
	err        error
}

// NewLazyAny creates a new lazy wrapper for any type.
func NewLazyAny(c Vessel, name string, expectedType reflect.Type) *LazyAny {
	return &LazyAny{
		container:  c,
		name:       name,
		expectedTy: expectedType,
	}
}

// Get resolves the dependency and returns it.
func (l *LazyAny) Get() (any, error) {
	if l.resolved {
		return l.value, l.err
	}

	instance, err := l.container.Resolve(l.name)
	if err != nil {
		l.err = err
		l.resolved = true

		return nil, err
	}

	l.value = instance
	l.resolved = true

	return l.value, nil
}

// MustGet resolves the dependency and returns it, panicking on error.
func (l *LazyAny) MustGet() any {
	value, err := l.Get()
	if err != nil {
		panic(fmt.Sprintf("lazy dependency %s failed: %v", l.name, err))
	}

	return value
}

// IsResolved returns true if the dependency has been resolved.
func (l *LazyAny) IsResolved() bool {
	return l.resolved
}

// Name returns the name of the dependency.
func (l *LazyAny) Name() string {
	return l.name
}

// OptionalLazyAny is a non-generic optional lazy wrapper.
type OptionalLazyAny struct {
	container  Vessel
	name       string
	expectedTy reflect.Type
	resolved   bool
	found      bool
	value      any
	err        error
}

// NewOptionalLazyAny creates a new optional lazy wrapper.
func NewOptionalLazyAny(c Vessel, name string, expectedType reflect.Type) *OptionalLazyAny {
	return &OptionalLazyAny{
		container:  c,
		name:       name,
		expectedTy: expectedType,
	}
}

// Get resolves the dependency and returns it.
func (l *OptionalLazyAny) Get() (any, error) {
	if l.resolved {
		return l.value, l.err
	}

	if !l.container.Has(l.name) {
		l.resolved = true
		l.found = false

		return nil, nil
	}

	instance, err := l.container.Resolve(l.name)
	if err != nil {
		l.err = err
		l.resolved = true

		return nil, err
	}

	l.value = instance
	l.resolved = true
	l.found = true

	return l.value, nil
}

// MustGet resolves the dependency and returns it, panicking on error.
func (l *OptionalLazyAny) MustGet() any {
	value, err := l.Get()
	if err != nil {
		panic(fmt.Sprintf("optional lazy dependency %s failed: %v", l.name, err))
	}

	return value
}

// IsResolved returns true if the dependency has been resolved.
func (l *OptionalLazyAny) IsResolved() bool {
	return l.resolved
}

// IsFound returns true if the dependency was found.
func (l *OptionalLazyAny) IsFound() bool {
	return l.found
}

// Name returns the name of the dependency.
func (l *OptionalLazyAny) Name() string {
	return l.name
}

// ResolveWithDeps resolves a service and its dependencies according to the Dep specs.
// This is used internally when resolving services that declare dependencies.
func ResolveWithDeps(ctx context.Context, c Vessel, name string, deps []di.Dep) error {
	for _, dep := range deps {
		switch dep.Mode {
		case di.DepEager:
			// Resolve and start the dependency
			if _, err := c.ResolveReady(ctx, dep.Name); err != nil {
				return fmt.Errorf("failed to resolve eager dependency %s: %w", dep.Name, err)
			}
		case di.DepOptional:
			// Resolve if exists, ignore if not found
			if c.Has(dep.Name) {
				if _, err := c.ResolveReady(ctx, dep.Name); err != nil {
					return fmt.Errorf("failed to resolve optional dependency %s: %w", dep.Name, err)
				}
			}
			// Lazy and LazyOptional are resolved on-demand, not here
		}
	}

	return nil
}
