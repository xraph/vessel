package vessel

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/xraph/go-utils/di"
)

// containerImpl implements Container.
type containerImpl struct {
	// byName is read on every Resolve without locking. Writes happen under
	// mu, together with ordered and graph.
	byName       snapMap[string, *serviceRegistration]
	ordered      []*serviceRegistration // registration order, append-only
	graph        *DependencyGraph
	middleware   middlewareChain
	typeRegistry *typeRegistry // Type-based registry for dig-like constructor injection
	started      bool
	mu           sync.RWMutex

	// startOrder records every singleton in the order it was actually started,
	// across both registries. A service always finishes starting after the
	// dependencies it resolved, so stopping in reverse is dependency-safe.
	startOrder []startedEntry
	lifeMu     sync.Mutex
}

// startedEntry is one started singleton from either registry.
type startedEntry struct {
	named *serviceRegistration
	typed *typeRegistration
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
	mu           sync.RWMutex // guards instance and started
	initMu       sync.Mutex   // held while building and starting the singleton

	// ready is set once the singleton is built and started, and cleared by
	// Stop. When set, instance can be read without locking: it is written
	// before ready is stored and never changes afterwards.
	ready atomic.Bool
}

// newContainerImpl creates a new DI container implementation.
func newContainerImpl() Vessel {
	return &containerImpl{
		graph:        NewDependencyGraph(),
		typeRegistry: newTypeRegistry(),
	}
}

// Register adds a service factory to the container.
func (c *containerImpl) Register(name string, factory Factory, opts ...RegisterOption) error {
	merged := mergeOptions(opts)

	if name == "" {
		return errors.New("service name cannot be empty")
	}

	if factory == nil {
		return ErrInvalidFactory
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	allDeps := merged.GetAllDeps()

	reg := &serviceRegistration{
		name:         name,
		factory:      factory,
		singleton:    merged.Lifecycle == "singleton",
		scoped:       merged.Lifecycle == "scoped",
		dependencies: merged.GetAllDepNames(),
		deps:         allDeps,
		groups:       merged.Groups,
		metadata:     merged.Metadata,
	}

	if !c.byName.StoreNew(name, reg) {
		return ErrServiceAlreadyExists(name)
	}

	c.ordered = append(c.ordered, reg)

	if len(allDeps) > 0 {
		c.graph.AddNodeWithDeps(name, allDeps)
	} else {
		c.graph.AddNode(name, nil)
	}

	return nil
}

// Resolve returns a service by name.
// For singleton services that implement di.Starter, the service is automatically
// started when first resolved.
func (c *containerImpl) Resolve(name string) (any, error) {
	return c.resolveNamed(resolveCtx{}, name)
}

// resolveNamed runs middleware around a named resolution.
func (c *containerImpl) resolveNamed(rc resolveCtx, name string) (any, error) {
	mws := c.middleware.list()
	if len(mws) == 0 {
		return c.resolveInternal(rc, name)
	}

	ctx := rc.context()

	if err := beforeResolve(mws, ctx, name); err != nil {
		return nil, err
	}

	service, err := c.resolveInternal(rc, name)

	if mwErr := afterResolve(mws, ctx, name, service, err); mwErr != nil {
		return nil, mwErr
	}

	return service, err
}

func (c *containerImpl) lookup(name string) (*serviceRegistration, bool) {
	return c.byName.Load(name)
}

// registrations returns every named registration in registration order.
// The slice is append-only, so the snapshot is safe to read without copying.
func (c *containerImpl) registrations() []*serviceRegistration {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.ordered
}

// resolveInternal performs the actual service resolution without middleware.
func (c *containerImpl) resolveInternal(rc resolveCtx, name string) (any, error) {
	reg, exists := c.lookup(name)
	if !exists {
		return nil, ErrServiceNotFound(name)
	}

	// Fast paths: a ready singleton or an already-built scoped instance
	// needs no cycle check and no allocation.
	if reg.singleton && reg.ready.Load() {
		return reg.instance, nil
	}

	if reg.scoped && rc.scope != nil {
		if v, ok, err := rc.scope.cached(reg); ok || err != nil {
			return v, err
		}
	}

	if err := rc.check(reg); err != nil {
		return nil, err
	}

	switch {
	case reg.singleton:
		return c.resolveSingleton(rc.withoutScope(), reg)

	case reg.scoped:
		if rc.scope == nil {
			return nil, fmt.Errorf("scoped service %s must be resolved from a scope", name)
		}

		return rc.scope.instance(reg, name, func() (any, error) {
			return c.callFactory(rc, reg)
		})

	default:
		instance, err := c.callFactory(rc, reg)
		if err != nil {
			return nil, err
		}

		// Transient instances are started but not tracked: the caller owns them.
		if err := c.startInstance(rc.context(), name, instance); err != nil {
			return nil, err
		}

		return instance, nil
	}
}

// resolveSingleton builds and starts a singleton exactly once.
func (c *containerImpl) resolveSingleton(rc resolveCtx, reg *serviceRegistration) (any, error) {
	// Cycles were ruled out by rc.enter, so this only waits for another
	// goroutine that is building the same singleton.
	reg.initMu.Lock()
	defer reg.initMu.Unlock()

	reg.mu.RLock()
	instance, started := reg.instance, reg.started
	reg.mu.RUnlock()

	if instance == nil {
		built, err := c.callFactory(rc, reg)
		if err != nil {
			return nil, err
		}

		instance = built

		reg.mu.Lock()
		reg.instance = instance
		reg.mu.Unlock()
	}

	if !started {
		if err := c.startInstance(rc.context(), reg.name, instance); err != nil {
			return nil, err
		}

		reg.mu.Lock()
		reg.started = true
		reg.mu.Unlock()

		c.recordStarted(startedEntry{named: reg})
	}

	reg.ready.Store(true)

	return instance, nil
}

// callFactory invokes a named factory with a container bound to rc.
func (c *containerImpl) callFactory(rc resolveCtx, reg *serviceRegistration) (any, error) {
	bound := c.bind(rc, reg)
	defer bound.done.Store(true)

	instance, err := reg.factory(bound)
	if err != nil {
		return nil, NewServiceError(reg.name, "resolve", err)
	}

	return instance, nil
}

// startInstance calls Start on instance if it implements di.Starter,
// running the start middleware around it.
func (c *containerImpl) startInstance(ctx context.Context, name string, instance any) error {
	starter, ok := instance.(di.Starter)
	if !ok {
		return nil
	}

	mws := c.middleware.list()

	if err := beforeStart(mws, ctx, name); err != nil {
		return err
	}

	startErr := starter.Start(ctx)

	if mwErr := afterStart(mws, ctx, name, startErr); mwErr != nil {
		return mwErr
	}

	if startErr != nil {
		return NewServiceError(name, "auto_start", startErr)
	}

	return nil
}

func (c *containerImpl) recordStarted(e startedEntry) {
	c.lifeMu.Lock()
	c.startOrder = append(c.startOrder, e)
	c.lifeMu.Unlock()
}

// Use adds middleware to the container.
// Middleware is called in the order they are added.
func (c *containerImpl) Use(middleware Middleware) {
	c.middleware.add(middleware)
}

// Has checks if a service is registered.
func (c *containerImpl) Has(name string) bool {
	_, exists := c.lookup(name)

	return exists
}

// IsStarted checks if a service has been started.
// Returns false if service doesn't exist or hasn't been started.
func (c *containerImpl) IsStarted(name string) bool {
	reg, exists := c.lookup(name)
	if !exists {
		return false
	}

	reg.mu.RLock()
	defer reg.mu.RUnlock()

	return reg.started
}

// ResolveReady resolves a service, ensuring it is started first.
// ctx is passed to the service's Start method and to middleware.
func (c *containerImpl) ResolveReady(ctx context.Context, name string) (any, error) {
	return c.resolveReady(resolveCtx{ctx: ctx}, name)
}

func (c *containerImpl) resolveReady(rc resolveCtx, name string) (any, error) {
	if _, exists := c.lookup(name); !exists {
		return nil, ErrServiceNotFound(name)
	}

	if !c.IsStarted(name) {
		if err := c.startService(rc, name); err != nil {
			return nil, NewServiceError(name, "start", err)
		}
	}

	return c.resolveNamed(rc, name)
}

// Services returns all registered service names, sorted.
// Named-registry services are always included (developer explicitly named them).
// Type-registry services are only included if they implement di.HealthChecker,
// which is the minimum contract for a type-registered component to be
// considered a "service" for dashboard and inspection purposes.
func (c *containerImpl) Services() []string {
	nameSet := make(map[string]bool)

	for _, reg := range c.registrations() {
		nameSet[reg.name] = true
	}

	if c.typeRegistry != nil {
		for _, name := range c.typeRegistry.healthCapableNames() {
			nameSet[name] = true
		}
	}

	names := make([]string, 0, len(nameSet))
	for name := range nameSet {
		names = append(names, name)
	}

	sort.Strings(names)

	return names
}

// BeginScope creates a new scope for request-scoped services.
func (c *containerImpl) BeginScope() Scope {
	return newScope(c)
}

// Start starts every singleton in dependency order: named singletons first,
// following their declared eager dependencies, then any type-registered
// singleton that has already been built. Type-registered services that have
// not been built yet stay lazy and start on first Inject.
//
// If a service fails to start, everything already started is stopped again
// in reverse order before the error is returned. Start is idempotent.
func (c *containerImpl) Start(ctx context.Context) error {
	c.mu.Lock()

	if c.started {
		c.mu.Unlock()

		return nil
	}

	// Lazy dependencies are resolved on demand, so they don't constrain order.
	order, err := c.graph.TopologicalSortEagerOnly()

	c.mu.Unlock()

	if err != nil {
		return err
	}

	rc := resolveCtx{ctx: ctx}

	for _, name := range order {
		if err := c.startService(rc, name); err != nil {
			_ = c.stopAll(ctx)

			return NewServiceError(name, "start", err)
		}
	}

	if c.typeRegistry != nil {
		for _, reg := range c.typeRegistry.registrations() {
			reg.mu.RLock()
			pending := reg.lifecycle == lifecycleSingleton && reg.instance != nil && !reg.started
			reg.mu.RUnlock()

			if !pending {
				continue
			}

			if _, err := reg.resolve(c, rc); err != nil {
				_ = c.stopAll(ctx)

				return NewServiceError(reg.displayName(), "start", err)
			}
		}
	}

	c.mu.Lock()
	c.started = true
	c.mu.Unlock()

	return nil
}

// Stop stops every started singleton in the reverse of the order it was
// started, including services that were auto-started by Resolve or Inject
// without a call to Start. It keeps going when a service fails to stop and
// returns all failures joined together.
func (c *containerImpl) Stop(ctx context.Context) error {
	err := c.stopAll(ctx)

	c.mu.Lock()
	c.started = false
	c.mu.Unlock()

	return err
}

// stopAll stops every recorded started singleton in reverse start order.
func (c *containerImpl) stopAll(ctx context.Context) error {
	c.lifeMu.Lock()
	entries := c.startOrder
	c.startOrder = nil
	c.lifeMu.Unlock()

	var errs []error

	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]

		switch {
		case e.named != nil:
			if err := stopRegistration(ctx, &e.named.mu, &e.named.ready, &e.named.instance, &e.named.started); err != nil {
				errs = append(errs, NewServiceError(e.named.name, "stop", err))
			}
		case e.typed != nil:
			if err := stopRegistration(ctx, &e.typed.mu, &e.typed.ready, &e.typed.instance, &e.typed.started); err != nil {
				errs = append(errs, NewServiceError(e.typed.displayName(), "stop", err))
			}
		}
	}

	return errors.Join(errs...)
}

// stopRegistration calls Stop on a started instance and marks it stopped.
// The service is marked stopped even if Stop fails, so Start can retry it.
func stopRegistration(ctx context.Context, mu *sync.RWMutex, ready *atomic.Bool, instance *any, started *bool) error {
	mu.Lock()
	inst, wasStarted := *instance, *started
	*started = false
	ready.Store(false)
	mu.Unlock()

	if !wasStarted || inst == nil {
		return nil
	}

	if stopper, ok := inst.(di.Stopper); ok {
		return stopper.Stop(ctx)
	}

	return nil
}

// Health checks all built singletons that implement di.HealthChecker, in
// registration order. It returns the first failure.
func (c *containerImpl) Health(ctx context.Context) error {
	for _, reg := range c.registrations() {
		if !reg.singleton {
			continue
		}

		reg.mu.RLock()
		instance := reg.instance
		reg.mu.RUnlock()

		if checker, ok := instance.(di.HealthChecker); ok {
			if err := checker.Health(ctx); err != nil {
				return NewServiceError(reg.name, "health", err)
			}
		}
	}

	if c.typeRegistry != nil {
		for _, reg := range c.typeRegistry.registrations() {
			reg.mu.RLock()
			instance := reg.instance
			reg.mu.RUnlock()

			if checker, ok := instance.(di.HealthChecker); ok {
				if err := checker.Health(ctx); err != nil {
					return NewServiceError(di.ServiceName(instance), "health", err)
				}
			}
		}
	}

	return nil
}

// Inspect returns diagnostic information about a service.
// If the service has been built and implements di.HealthChecker, its Health
// method is called to fill in Healthy.
func (c *containerImpl) Inspect(name string) ServiceInfo {
	reg, exists := c.lookup(name)
	if !exists {
		return c.inspectTypeRegistry(name)
	}

	lifecycle := lifecycleTransient
	if reg.singleton {
		lifecycle = lifecycleSingleton
	} else if reg.scoped {
		lifecycle = lifecycleScoped
	}

	reg.mu.RLock()
	instance, started := reg.instance, reg.started
	reg.mu.RUnlock()

	typeName := "unknown"
	if instance != nil {
		typeName = fmt.Sprintf("%T", instance)
	}

	metadata := make(map[string]string, len(reg.metadata)+1)
	for k, v := range reg.metadata {
		metadata[k] = v
	}

	if len(reg.groups) > 0 {
		metadata[groupsMetadataKey] = strings.Join(reg.groups, ",")
	}

	return ServiceInfo{
		Name:         name,
		Type:         typeName,
		Lifecycle:    lifecycle,
		Dependencies: reg.dependencies,
		Deps:         reg.deps,
		Started:      started,
		Healthy:      isHealthy(instance),
		Metadata:     metadata,
	}
}

// inspectTypeRegistry returns diagnostic info for a type-registry service.
func (c *containerImpl) inspectTypeRegistry(name string) ServiceInfo {
	if c.typeRegistry == nil {
		return ServiceInfo{Name: name}
	}

	reg, key, ok := c.typeRegistry.findByName(name)
	if !ok {
		return ServiceInfo{Name: name}
	}

	reg.mu.RLock()
	instance, started := reg.instance, reg.started
	reg.mu.RUnlock()

	metadata := make(map[string]string)
	if len(reg.groups) > 0 {
		metadata[groupsMetadataKey] = strings.Join(reg.groups, ",")
	}

	return ServiceInfo{
		Name:      name,
		Type:      key.typ.String(),
		Lifecycle: reg.lifecycle,
		Started:   started,
		Healthy:   isHealthy(instance),
		Metadata:  metadata,
	}
}

// isHealthy runs a health check if instance supports one. No locks are held
// while it runs, so a health check may safely use the container.
func isHealthy(instance any) bool {
	checker, ok := instance.(di.HealthChecker)
	if !ok {
		return false
	}

	return checker.Health(context.Background()) == nil
}

// startService starts a single named singleton if it isn't started yet.
// Scoped and transient services are skipped: they have no app-wide instance.
func (c *containerImpl) startService(rc resolveCtx, name string) error {
	reg, exists := c.lookup(name)
	if !exists || !reg.singleton {
		return nil
	}

	_, err := c.resolveNamed(rc, name)

	return err
}

// stopService stops a single named service if it was started.
func (c *containerImpl) stopService(ctx context.Context, name string) error {
	reg, exists := c.lookup(name)
	if !exists {
		return nil
	}

	return stopRegistration(ctx, &reg.mu, &reg.ready, &reg.instance, &reg.started)
}
