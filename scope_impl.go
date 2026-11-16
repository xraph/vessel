package vessel

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/xraph/go-utils/di"
)

// scope implements Scope.
type scope struct {
	parent  *containerImpl
	entries map[any]*onceValue // keyed by registration pointer; created on first use
	created []scopedInstance   // creation order, for disposal and Services
	context map[string]any     // Context storage for request-specific data; created on first Set
	mu      sync.RWMutex
	ended   atomic.Bool // written under mu, read lock-free
}

// scopedInstance is one instance created in this scope.
type scopedInstance struct {
	name  string
	value any
	named bool // registered with Register (listed by Services)
}

// newScope creates a new scope.
func newScope(parent *containerImpl) *scope {
	return &scope{parent: parent}
}

// Resolve returns a service by name from this scope.
// Scoped services are cached for the life of the scope, singletons come from
// the parent container, and transients are built fresh. Factories receive a
// container bound to this scope, so a scoped or transient factory can resolve
// other scoped services from the same scope.
func (s *scope) Resolve(name string) (any, error) {
	if s.IsEnded() {
		return nil, ErrScopeEnded
	}

	return s.parent.resolveNamed(resolveCtx{scope: s}, name)
}

// cached returns the scope's instance for key if it has already been built.
func (s *scope) cached(key any) (any, bool, error) {
	if s.ended.Load() {
		return nil, false, ErrScopeEnded
	}

	s.mu.RLock()
	entry := s.entries[key]
	s.mu.RUnlock()

	if entry == nil {
		return nil, false, nil
	}

	v, ok := entry.load()

	return v, ok, nil
}

// entry returns the build slot for key, creating it if needed.
func (s *scope) entry(key any) (*onceValue, error) {
	if s.ended.Load() {
		return nil, ErrScopeEnded
	}

	s.mu.RLock()
	entry := s.entries[key]
	s.mu.RUnlock()

	if entry != nil {
		return entry, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.ended.Load() {
		return nil, ErrScopeEnded
	}

	if entry = s.entries[key]; entry == nil {
		if s.entries == nil {
			s.entries = make(map[any]*onceValue)
		}

		entry = &onceValue{}
		s.entries[key] = entry
	}

	return entry, nil
}

// instance returns the scope's instance for a registration, building it with
// build on first use. key must uniquely identify the registration.
func (s *scope) instance(key any, name string, build func() (any, error)) (any, error) {
	entry, err := s.entry(key)
	if err != nil {
		return nil, err
	}

	// The entry lock is not s.mu, so the factory can resolve other scoped
	// services from this scope. Cycles were already caught by the caller.
	value, created, err := entry.get(build)
	if err != nil || !created {
		return value, err
	}

	_, named := key.(*serviceRegistration)

	s.mu.Lock()
	s.created = append(s.created, scopedInstance{name: name, value: value, named: named})
	s.mu.Unlock()

	return value, nil
}

// End disposes every scoped instance that implements di.Disposable, in the
// reverse of the order they were created, and returns all failures joined.
func (s *scope) End() error {
	s.mu.Lock()

	if s.ended.Load() {
		s.mu.Unlock()

		return ErrScopeEnded
	}

	created := s.created
	s.created = nil
	s.entries = nil
	s.context = nil
	s.ended.Store(true)

	s.mu.Unlock()

	var errs []error

	for i := len(created) - 1; i >= 0; i-- {
		inst := created[i]
		if disposable, ok := inst.value.(di.Disposable); ok {
			if err := disposable.Dispose(); err != nil {
				errs = append(errs, fmt.Errorf("failed to dispose %s: %w", inst.name, err))
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("scope cleanup errors: %w", errors.Join(errs...))
	}

	return nil
}

// Has checks if a service is registered (delegates to parent container).
func (s *scope) Has(name string) bool {
	return s.parent.Has(name)
}

// IsEnded returns true if the scope has been ended.
func (s *scope) IsEnded() bool {
	return s.ended.Load()
}

// Services returns the names of named services resolved in this scope,
// in the order they were created.
func (s *scope) Services() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	names := make([]string, 0, len(s.created))
	for _, inst := range s.created {
		if inst.named {
			names = append(names, inst.name)
		}
	}

	return names
}

// Parent returns the parent container.
func (s *scope) Parent() Vessel {
	return s.parent
}

// Set stores a value in the scope context.
func (s *scope) Set(key string, value any) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.ended.Load() {
		return // Silently ignore if scope ended
	}

	if s.context == nil {
		s.context = make(map[string]any)
	}

	s.context[key] = value
}

// Get retrieves a value from the scope context.
func (s *scope) Get(key string) (any, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	value, ok := s.context[key]

	return value, ok
}
