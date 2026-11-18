package vessel

import (
	"fmt"
	"reflect"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/xraph/go-utils/di"
	"github.com/xraph/go-utils/errs"
)

// typeKey uniquely identifies a service by its type and optional name.
// This enables dig-like type-based resolution where services are resolved
// by their Go type rather than string names.
type typeKey struct {
	typ  reflect.Type
	name string // Empty for unnamed services, or "primary", "readonly" etc.
}

// String returns a human-readable representation of the type key
func (k typeKey) String() string {
	typeName := "<nil>"
	if k.typ != nil {
		typeName = k.typ.String()
	}
	if k.name == "" {
		return typeName
	}
	return fmt.Sprintf("%s[name=%s]", typeName, k.name)
}

// builder constructs an instance, continuing the resolution chain in rc.
type builder func(rc resolveCtx) (any, error)

// typeRegistration holds a type-based service registration. Aliases and
// interface types registered with As point at the same registration, so they
// share one instance.
type typeRegistration struct {
	key           typeKey // primary key
	build         builder
	instance      any
	lifecycle     string // "singleton", "transient", "scoped"
	groups        []string
	healthCapable bool // true if return type implements di.HealthChecker
	started       bool // tracks lifecycle state for Start/Stop

	// value marks a registration whose instance the caller built and handed
	// over (ProvideValue). Resolving it returns the value as it is: the
	// caller decides when it starts, and Container.Start starts it with the
	// rest. Auto-starting it on resolve would run its Start whenever anything
	// first asks for it, which can be long before what it needs is ready.
	value  bool
	mu     sync.RWMutex
	initMu sync.Mutex // held while building and starting the singleton

	// ready is set once the singleton is built and started (or, for a value,
	// first resolved), and cleared by Stop. When set, instance can be read
	// without locking.
	ready atomic.Bool
}

// displayName is the name used in errors and diagnostics.
func (reg *typeRegistration) displayName() string {
	return deriveServiceName(reg.key)
}

// typeRegistry manages type-based service registrations alongside the
// existing name-based registry. This enables dig-like constructor injection.
//
// Lookups by key never take a lock. All writes happen under mu; ordered and
// the group slices are append-only, so readers can use a snapshot of them
// without copying.
type typeRegistry struct {
	services snapMap[typeKey, *typeRegistration]
	ordered  []*typeRegistration            // unique registrations, in registration order
	groups   map[string][]*typeRegistration // group name -> registrations
	mu       sync.RWMutex
}

// newTypeRegistry creates a new type registry
func newTypeRegistry() *typeRegistry {
	return &typeRegistry{}
}

// keyedRegistration pairs a key with the registration it resolves to.
type keyedRegistration struct {
	key   typeKey
	reg   *typeRegistration
	label string // optional, e.g. `alias "db"`, prefixed to conflict errors
}

// registerAll adds several keys atomically: if any key is taken, or appears
// twice, nothing is registered.
func (r *typeRegistry) registerAll(entries []keyedRegistration) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	keys := make([]typeKey, len(entries))
	regs := make([]*typeRegistration, len(entries))
	for i, e := range entries {
		keys[i], regs[i] = e.key, e.reg
	}

	if conflict, ok := r.services.StoreAll(keys, regs); !ok {
		msg := fmt.Sprintf("service already registered for type %s", conflict)
		for _, e := range entries {
			if e.key == conflict && e.label != "" {
				msg = fmt.Sprintf("failed to register %s: %s", e.label, msg)
				break
			}
		}

		return errs.NewError(CodeServiceAlreadyExists, msg, nil).
			WithContext("service", conflict.String()).(*errs.Error)
	}

	for _, e := range entries {
		if slices.Contains(r.ordered, e.reg) {
			continue
		}

		r.ordered = append(r.ordered, e.reg)
		for _, group := range e.reg.groups {
			if r.groups == nil {
				r.groups = make(map[string][]*typeRegistration)
			}
			r.groups[group] = append(r.groups[group], e.reg)
		}
	}

	return nil
}

// get retrieves a type registration by key
func (r *typeRegistry) get(key typeKey) (*typeRegistration, bool) {
	return r.services.Load(key)
}

// has checks if a type is registered
func (r *typeRegistry) has(key typeKey) bool {
	_, ok := r.services.Load(key)
	return ok
}

// getGroup returns all registrations in a group, in registration order.
// The result is a read-only snapshot.
func (r *typeRegistry) getGroup(group string) []*typeRegistration {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.groups[group]
}

// registrations returns every unique registration in registration order.
// The result is a read-only snapshot.
func (r *typeRegistry) registrations() []*typeRegistration {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.ordered
}

// findByName finds the registration whose derived service name matches,
// preferring primary keys over aliases.
func (r *typeRegistry) findByName(name string) (*typeRegistration, typeKey, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, reg := range r.ordered {
		if deriveServiceName(reg.key) == name {
			return reg, reg.key, true
		}
	}

	var (
		found    *typeRegistration
		foundKey typeKey
	)

	r.services.Range(func(key typeKey, reg *typeRegistration) bool {
		if deriveServiceName(key) == name {
			found, foundKey = reg, key
			return false
		}
		return true
	})

	return found, foundKey, found != nil
}

// deriveServiceName returns a display name for a type-registry entry.
// Named services use their name. Unnamed services derive from the type
// using the pattern "{pkgPath}.{TypeName}".
func deriveServiceName(key typeKey) string {
	if key.name != "" {
		return key.name
	}
	t := key.typ
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	pkg := t.PkgPath()
	if pkg != "" {
		return pkg + "." + t.Name()
	}
	return t.String()
}

// healthCapableNames returns the primary service name of every registration
// that implements di.HealthChecker. Only these are considered "services" for
// dashboard and inspection purposes.
func (r *typeRegistry) healthCapableNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.ordered))
	for _, reg := range r.ordered {
		if reg.healthCapable {
			names = append(names, deriveServiceName(reg.key))
		}
	}

	return names
}

// errNoTypeProvider builds a not-found error for a type key.
func errNoTypeProvider(format string, key typeKey) *errs.Error {
	return errs.NewError(CodeServiceNotFound, fmt.Sprintf(format, key), nil).
		WithContext("service", key.String()).(*errs.Error)
}

// resolve resolves a service by type key, instantiating if necessary
func (r *typeRegistry) resolve(key typeKey, c *containerImpl, rc resolveCtx) (any, error) {
	reg, ok := r.get(key)
	if !ok {
		return nil, errNoTypeProvider("no service registered for type %s", key)
	}

	return reg.resolve(c, rc)
}

// resolve returns the instance for this registration according to its
// lifecycle, building and starting it as needed.
func (reg *typeRegistration) resolve(c *containerImpl, rc resolveCtx) (any, error) {
	// Fast paths: a ready singleton or an already-built scoped instance
	// needs no cycle check and no allocation.
	switch reg.lifecycle {
	case lifecycleSingleton:
		if reg.ready.Load() {
			return reg.instance, nil
		}
	case lifecycleScoped:
		if rc.scope == nil {
			return nil, fmt.Errorf("scoped service %s must be resolved from a scope", reg.key)
		}

		if v, ok, err := rc.scope.cached(reg); ok || err != nil {
			return v, err
		}
	}

	rc, err := rc.enter(reg)
	if err != nil {
		return nil, err
	}

	switch reg.lifecycle {
	case lifecycleScoped:
		return rc.scope.instance(reg, reg.key.String(), func() (any, error) {
			return reg.build(rc)
		})

	case lifecycleTransient:
		instance, err := reg.build(rc)
		if err != nil {
			return nil, err
		}

		if err := startTyped(rc, reg, instance); err != nil {
			return nil, err
		}

		return instance, nil

	default:
		return reg.resolveSingleton(c, rc.withoutScope(), false)
	}
}

// resolveSingleton builds and starts the singleton exactly once. Concurrent
// first resolutions wait for each other instead of failing. A value given to
// ProvideValue is started only when fromStart is set, that is when
// Container.Start reaches it, never by a plain resolve.
func (reg *typeRegistration) resolveSingleton(c *containerImpl, rc resolveCtx, fromStart bool) (any, error) {
	reg.initMu.Lock()
	defer reg.initMu.Unlock()

	reg.mu.RLock()
	instance, started := reg.instance, reg.started
	reg.mu.RUnlock()

	if instance == nil {
		built, err := reg.build(rc)
		if err != nil {
			return nil, err
		}

		instance = built

		reg.mu.Lock()
		reg.instance = instance
		reg.mu.Unlock()
	}

	if !started && (fromStart || !reg.value) {
		if err := startTyped(rc, reg, instance); err != nil {
			return nil, err
		}

		reg.mu.Lock()
		reg.started = true
		reg.mu.Unlock()

		c.recordStarted(startedEntry{typed: reg})
	}

	reg.ready.Store(true)

	return instance, nil
}

// startTyped calls Start on instance if it implements di.Starter.
func startTyped(rc resolveCtx, reg *typeRegistration, instance any) error {
	starter, ok := instance.(di.Starter)
	if !ok {
		return nil
	}

	if err := starter.Start(rc.context()); err != nil {
		return fmt.Errorf("auto-start %s: %w", reg.key, err)
	}

	return nil
}
