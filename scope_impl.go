package vessel

import (
	"fmt"
	"sync"

	"github.com/xraph/go-utils/di"
)

// scope implements Scope.
type scope struct {
	parent    *containerImpl
	instances map[string]any
	context   map[string]any // Context storage for request-specific data
	mu        sync.RWMutex
	ended     bool
}

// newScope creates a new scope.
func newScope(parent *containerImpl) *scope {
	return &scope{
		parent:    parent,
		instances: make(map[string]any),
		context:   make(map[string]any),
	}
}

// Resolve returns a service by name from this scope.
func (s *scope) Resolve(name string) (any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.ended {
		return nil, ErrScopeEnded
	}

	// Get registration from parent
	s.parent.mu.RLock()
	reg, exists := s.parent.services[name]
	s.parent.mu.RUnlock()

	if !exists {
		return nil, ErrServiceNotFound(name)
	}

	// Singleton services: resolve from parent
	if reg.singleton {
		return s.parent.Resolve(name)
	}

	// Scoped services: cache in this scope
	if reg.scoped {
		if instance, ok := s.instances[name]; ok {
			return instance, nil
		}

		// Create new instance for this scope
		instance, err := reg.factory(s.parent)
		if err != nil {
			return nil, NewServiceError(name, "resolve", err)
		}

		s.instances[name] = instance

		return instance, nil
	}

	// Transient services: always create new
	instance, err := reg.factory(s.parent)
	if err != nil {
		return nil, NewServiceError(name, "resolve", err)
	}

	return instance, nil
}

// End cleans up all scoped services in this scope.
func (s *scope) End() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.ended {
		return ErrScopeEnded
	}

	// Dispose of scoped instances in reverse order
	var errs []error

	for name, instance := range s.instances {
		if disposable, ok := instance.(di.Disposable); ok {
			if err := disposable.Dispose(); err != nil {
				errs = append(errs, fmt.Errorf("failed to dispose %s: %w", name, err))
			}
		}
	}

	s.instances = nil
	s.context = nil
	s.ended = true

	if len(errs) > 0 {
		return fmt.Errorf("scope cleanup errors: %v", errs)
	}

	return nil
}

// Has checks if a service is registered (delegates to parent container).
func (s *scope) Has(name string) bool {
	return s.parent.Has(name)
}

// IsEnded returns true if the scope has been ended.
func (s *scope) IsEnded() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ended
}

// Services returns a list of services resolved in this scope.
func (s *scope) Services() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	names := make([]string, 0, len(s.instances))
	for name := range s.instances {
		names = append(names, name)
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

	if s.ended {
		return // Silently ignore if scope ended
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
