package vessel

import (
	"fmt"
	"reflect"
	"sync"
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

// typeRegistration holds a type-based service registration
type typeRegistration struct {
	key          typeKey
	constructor  *constructorInfo
	factory      Factory
	instance     any
	lifecycle    string // "singleton", "transient", "scoped"
	groups       []string
	constructing bool // Prevent circular instantiation
	mu           sync.RWMutex
}

// typeRegistry manages type-based service registrations alongside the
// existing name-based registry. This enables dig-like constructor injection.
type typeRegistry struct {
	services map[typeKey]*typeRegistration
	groups   map[string][]*typeRegistration // group name -> registrations
	mu       sync.RWMutex
}

// newTypeRegistry creates a new type registry
func newTypeRegistry() *typeRegistry {
	return &typeRegistry{
		services: make(map[typeKey]*typeRegistration),
		groups:   make(map[string][]*typeRegistration),
	}
}

// register adds a new type-based service registration
func (r *typeRegistry) register(key typeKey, reg *typeRegistration) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.services[key]; exists {
		return fmt.Errorf("service already registered for type %s", key)
	}

	r.services[key] = reg

	// Add to groups
	for _, group := range reg.groups {
		r.groups[group] = append(r.groups[group], reg)
	}

	return nil
}

// get retrieves a type registration by key
func (r *typeRegistry) get(key typeKey) (*typeRegistration, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	reg, ok := r.services[key]
	return reg, ok
}

// has checks if a type is registered
func (r *typeRegistry) has(key typeKey) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.services[key]
	return ok
}

// getGroup returns all registrations in a group
func (r *typeRegistry) getGroup(group string) []*typeRegistration {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.groups[group]
}

// resolve resolves a service by type key, instantiating if necessary
func (r *typeRegistry) resolve(key typeKey, container Vessel) (any, error) {
	reg, ok := r.get(key)
	if !ok {
		return nil, fmt.Errorf("no service registered for type %s", key)
	}

	return reg.resolve(container)
}

// resolve resolves the service instance
func (reg *typeRegistration) resolve(container Vessel) (any, error) {
	reg.mu.Lock()

	// Check for circular dependency during construction
	if reg.constructing {
		reg.mu.Unlock()
		return nil, fmt.Errorf("circular dependency detected for type %s", reg.key)
	}

	// Return cached instance for singletons
	if reg.lifecycle == "singleton" && reg.instance != nil {
		instance := reg.instance
		reg.mu.Unlock()
		return instance, nil
	}

	// Mark as constructing to detect cycles
	reg.constructing = true
	reg.mu.Unlock() // Release lock before calling factory to avoid deadlock

	// Call factory (without holding lock)
	instance, err := reg.factory(container)

	// Re-acquire lock to update state
	reg.mu.Lock()
	reg.constructing = false

	if err != nil {
		reg.mu.Unlock()
		return nil, err
	}

	// Cache for singletons
	if reg.lifecycle == "singleton" {
		reg.instance = instance
	}
	reg.mu.Unlock()

	return instance, nil
}
