package vessel

import (
	"context"
	"fmt"
	"sync"

	"github.com/xraph/go-utils/di"
)

// containerImpl implements Container.
type containerImpl struct {
	services     map[string]*serviceRegistration
	instances    map[string]any
	graph        *DependencyGraph
	middleware   *middlewareChain
	typeRegistry *typeRegistry // Type-based registry for dig-like constructor injection
	started      bool
	mu           sync.RWMutex
}

// serviceRegistration holds service registration details.
type serviceRegistration struct {
	name         string
	factory      Factory
	singleton    bool
	scoped       bool
	dependencies []string // Backward compat: just names
	deps         []di.Dep // New: full dependency specs with modes
	groups       []string
	metadata     map[string]string
	instance     any
	started      bool
	mu           sync.RWMutex
}

// newContainerImpl creates a new DI container implementation.
func newContainerImpl() Vessel {
	return &containerImpl{
		services:     make(map[string]*serviceRegistration),
		instances:    make(map[string]any),
		graph:        NewDependencyGraph(),
		middleware:   newMiddlewareChain(),
		typeRegistry: newTypeRegistry(),
	}
}

// Register adds a service factory to the container.
func (c *containerImpl) Register(name string, factory Factory, opts ...RegisterOption) error {
	// Merge options
	merged := mergeOptions(opts)

	if name == "" {
		return fmt.Errorf("service name cannot be empty")
	}

	if factory == nil {
		return ErrInvalidFactory
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.services[name]; exists {
		return ErrServiceAlreadyExists(name)
	}

	// Get all dependency specs (merges string-based and Dep-based)
	allDeps := merged.GetAllDeps()
	allDepNames := merged.GetAllDepNames()

	// Create registration from merged options
	reg := &serviceRegistration{
		name:         name,
		factory:      factory,
		singleton:    merged.Lifecycle == "singleton",
		scoped:       merged.Lifecycle == "scoped",
		dependencies: allDepNames,
		deps:         allDeps,
		groups:       merged.Groups,
		metadata:     merged.Metadata,
	}

	// Add to services map
	c.services[name] = reg

	// Add to dependency graph with full Dep specs
	if len(allDeps) > 0 {
		c.graph.AddNodeWithDeps(name, allDeps)
	} else {
		c.graph.AddNode(name, nil)
	}

	return nil
}

// Resolve returns a service by name.
// For singleton services that implement di.Service, the service is automatically
// started when first resolved. This enables Angular-like dependency injection where
// dependencies are fully ready when resolved.
func (c *containerImpl) Resolve(name string) (any, error) {
	ctx := context.Background()

	// Call middleware before resolve
	if err := c.middleware.beforeResolve(ctx, name); err != nil {
		return nil, err
	}

	// Perform actual resolution
	service, err := c.resolveInternal(name)

	// Call middleware after resolve
	if mwErr := c.middleware.afterResolve(ctx, name, service, err); mwErr != nil {
		return nil, mwErr
	}

	return service, err
}

// resolveInternal performs the actual service resolution without middleware.
func (c *containerImpl) resolveInternal(name string) (any, error) {
	c.mu.RLock()
	reg, exists := c.services[name]
	c.mu.RUnlock()

	if !exists {
		return nil, ErrServiceNotFound(name)
	}

	// Singleton: return cached instance
	if reg.singleton {
		// Fast path: check if already created AND started (read lock)
		reg.mu.RLock()

		if reg.instance != nil && reg.started {
			instance := reg.instance
			reg.mu.RUnlock()

			return instance, nil
		}
		// Check if instance exists but not started
		existingInstance := reg.instance
		reg.mu.RUnlock()

		// Slow path: create and/or start instance (write lock)
		reg.mu.Lock()
		defer reg.mu.Unlock()

		// Double-check after acquiring write lock
		if reg.instance != nil && reg.started {
			return reg.instance, nil
		}

		// Create instance if needed
		if reg.instance == nil {
			// Call factory while holding lock (container lock is separate, so no deadlock)
			// Note: factory may call c.Resolve() which uses c.mu (different lock)
			instance, err := reg.factory(c)
			if err != nil {
				return nil, NewServiceError(name, "resolve", err)
			}

			reg.instance = instance
			existingInstance = instance
		}

		// Auto-start if service implements di.Service and not yet started
		if !reg.started {
			if svc, ok := existingInstance.(di.Service); ok {
				ctx := context.Background()

				// Call middleware before start
				if err := c.middleware.beforeStart(ctx, name); err != nil {
					return nil, err
				}

				startErr := svc.Start(ctx)

				// Call middleware after start
				if mwErr := c.middleware.afterStart(ctx, name, startErr); mwErr != nil {
					return nil, mwErr
				}

				if startErr != nil {
					return nil, NewServiceError(name, "auto_start", startErr)
				}
			}

			reg.started = true
		}

		return reg.instance, nil
	}

	// Scoped services should be resolved from scope, not container
	if reg.scoped {
		return nil, fmt.Errorf("scoped service %s must be resolved from a scope", name)
	}

	// Transient: create new instance each time
	instance, err := reg.factory(c)
	if err != nil {
		return nil, NewServiceError(name, "resolve", err)
	}

	// Auto-start transient services that implement di.Service
	if svc, ok := instance.(di.Service); ok {
		ctx := context.Background()

		// Call middleware before start
		if err := c.middleware.beforeStart(ctx, name); err != nil {
			return nil, err
		}

		startErr := svc.Start(ctx)

		// Call middleware after start
		if mwErr := c.middleware.afterStart(ctx, name, startErr); mwErr != nil {
			return nil, mwErr
		}

		if startErr != nil {
			return nil, NewServiceError(name, "auto_start", startErr)
		}
	}

	return instance, nil
}

// Use adds middleware to the container.
// Middleware is called in the order they are added.
func (c *containerImpl) Use(middleware Middleware) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.middleware.add(middleware)
}

// Has checks if a service is registered.
func (c *containerImpl) Has(name string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	_, exists := c.services[name]

	return exists
}

// IsStarted checks if a service has been started.
// Returns false if service doesn't exist or hasn't been started.
func (c *containerImpl) IsStarted(name string) bool {
	c.mu.RLock()
	reg, exists := c.services[name]
	c.mu.RUnlock()

	if !exists {
		return false
	}

	reg.mu.RLock()
	defer reg.mu.RUnlock()

	return reg.started
}

// ResolveReady resolves a service, ensuring it and its dependencies are started first.
// This is useful during extension Register() phase when you need a dependency
// to be fully initialized before use.
func (c *containerImpl) ResolveReady(ctx context.Context, name string) (any, error) {
	c.mu.RLock()
	reg, exists := c.services[name]
	c.mu.RUnlock()

	if !exists {
		return nil, ErrServiceNotFound(name)
	}

	// Check if already started
	reg.mu.RLock()
	started := reg.started
	reg.mu.RUnlock()

	// If not started, start the service (and its dependencies via startService)
	if !started {
		if err := c.startService(ctx, name); err != nil {
			return nil, NewServiceError(name, "start", err)
		}
	}

	// Now resolve the service
	return c.Resolve(name)
}

// Services returns all registered service names.
func (c *containerImpl) Services() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	names := make([]string, 0, len(c.services))
	for name := range c.services {
		names = append(names, name)
	}

	return names
}

// BeginScope creates a new scope for request-scoped services.
func (c *containerImpl) BeginScope() Scope {
	return newScope(c)
}

// Start initializes all services in dependency order.
// This method is idempotent - it will skip already-started services and
// won't error if the container is already marked as started.
func (c *containerImpl) Start(ctx context.Context) error {
	c.mu.Lock()

	// Idempotent: if already started, just return success
	if c.started {
		c.mu.Unlock()

		return nil
	}

	// Get services in dependency order
	order, err := c.graph.TopologicalSort()
	if err != nil {
		c.mu.Unlock()

		return err
	}

	c.mu.Unlock()

	// Start services in order (without holding container lock)
	// Services that are already started (via auto-start on Resolve) will be skipped
	for _, name := range order {
		if err := c.startService(ctx, name); err != nil {
			// Rollback: stop already started services
			c.stopServices(ctx, order)

			return NewServiceError(name, "start", err)
		}
	}

	c.mu.Lock()
	c.started = true
	c.mu.Unlock()

	return nil
}

// Stop shuts down all services in reverse order.
func (c *containerImpl) Stop(ctx context.Context) error {
	c.mu.Lock()

	if !c.started {
		c.mu.Unlock()

		return nil // Not an error, just no-op
	}

	// Get services in dependency order, then reverse
	order, err := c.graph.TopologicalSort()
	if err != nil {
		c.mu.Unlock()

		return err
	}

	c.mu.Unlock()

	// Stop in reverse order (without holding container lock)
	for i := len(order) - 1; i >= 0; i-- {
		name := order[i]
		if err := c.stopService(ctx, name); err != nil {
			// Continue stopping other services, but collect error
			return NewServiceError(name, "stop", err)
		}
	}

	c.mu.Lock()
	c.started = false
	c.mu.Unlock()

	return nil
}

// Health checks all services.
func (c *containerImpl) Health(ctx context.Context) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for name, reg := range c.services {
		// Only check singleton services that have been instantiated
		if !reg.singleton || reg.instance == nil {
			continue
		}

		if checker, ok := reg.instance.(di.HealthChecker); ok {
			if err := checker.Health(ctx); err != nil {
				return NewServiceError(name, "health", err)
			}
		}
	}

	return nil
}

// Inspect returns diagnostic information about a service.
func (c *containerImpl) Inspect(name string) ServiceInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()

	reg, exists := c.services[name]
	if !exists {
		return ServiceInfo{Name: name}
	}

	reg.mu.RLock()
	defer reg.mu.RUnlock()

	lifecycle := "transient"
	if reg.singleton {
		lifecycle = "singleton"
	} else if reg.scoped {
		lifecycle = "scoped"
	}

	typeName := "unknown"
	if reg.instance != nil {
		typeName = fmt.Sprintf("%T", reg.instance)
	}

	healthy := false
	if checker, ok := reg.instance.(di.HealthChecker); ok {
		healthy = checker.Health(context.Background()) == nil
	}

	// Copy metadata and add groups
	metadata := make(map[string]string)
	for k, v := range reg.metadata {
		metadata[k] = v
	}

	// Store groups in metadata for query purposes
	if len(reg.groups) > 0 {
		// Store groups as comma-separated string
		metadata["__groups"] = joinStrings(reg.groups, ",")
	}

	return ServiceInfo{
		Name:         name,
		Type:         typeName,
		Lifecycle:    lifecycle,
		Dependencies: reg.dependencies,
		Deps:         reg.deps,
		Started:      reg.started,
		Healthy:      healthy,
		Metadata:     metadata,
	}
}

// startService starts a single service.
// This is idempotent - if the service is already started (via auto-start on Resolve),
// it will be skipped.
func (c *containerImpl) startService(ctx context.Context, name string) error {
	c.mu.RLock()
	reg, exists := c.services[name]
	c.mu.RUnlock()

	if !exists {
		return nil // Service not registered, skip
	}

	// Check if already started
	reg.mu.RLock()
	started := reg.started
	reg.mu.RUnlock()

	if started {
		return nil // Already started (via auto-start on Resolve), skip
	}

	// Resolve the service instance (creates and auto-starts if needed)
	// Since Resolve() now auto-starts services, this should handle everything
	_, err := c.Resolve(name)
	if err != nil {
		return err
	}

	return nil
}

// stopService stops a single service.
func (c *containerImpl) stopService(ctx context.Context, name string) error {
	reg := c.services[name]

	reg.mu.RLock()
	instance := reg.instance
	started := reg.started
	reg.mu.RUnlock()

	if !started || instance == nil {
		return nil
	}

	// Call Stop if service implements Service interface
	if svc, ok := instance.(di.Service); ok {
		if err := svc.Stop(ctx); err != nil {
			return err
		}

		reg.mu.Lock()
		reg.started = false
		reg.mu.Unlock()
	}

	return nil
}

// stopServices stops multiple services (for rollback).
func (c *containerImpl) stopServices(ctx context.Context, names []string) {
	for i := len(names) - 1; i >= 0; i-- {
		_ = c.stopService(ctx, names[i])
	}
}

// joinStrings is a helper to join strings.
func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}
