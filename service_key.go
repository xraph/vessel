package vessel

// ServiceKey provides type-safe service identification.
// Use NewServiceKey to create typed keys for your services.
type ServiceKey[T any] struct {
	name string
}

// NewServiceKey creates a new typed service key.
// The type parameter T ensures type safety when registering and resolving services.
//
// Example:
//
//	var DatabaseKey = NewServiceKey[*Database]("database")
//	var UserServiceKey = NewServiceKey[*UserService]("userService")
func NewServiceKey[T any](name string) ServiceKey[T] {
	return ServiceKey[T]{name: name}
}

// Name returns the string name of the service key.
func (k ServiceKey[T]) Name() string {
	return k.name
}

// RegisterWithKey registers a service using a typed service key.
// This provides type safety and autocomplete support compared to string-based registration.
//
// Example:
//
//	var DatabaseKey = NewServiceKey[*Database]("database")
//	RegisterWithKey(c, DatabaseKey, func(c Vessel) (*Database, error) {
//	    return &Database{}, nil
//	}, Singleton())
func RegisterWithKey[T any](c Vessel, key ServiceKey[T], factory func(Vessel) (T, error), opts ...RegisterOption) error {
	// Wrap the typed factory in an untyped factory
	wrappedFactory := func(c Vessel) (any, error) {
		return factory(c)
	}
	return c.Register(key.name, wrappedFactory, opts...)
}

// ResolveWithKey resolves a service using a typed service key.
// This provides type safety and autocomplete support compared to string-based resolution.
//
// Example:
//
//	db, err := ResolveWithKey(c, DatabaseKey)
func ResolveWithKey[T any](c Vessel, key ServiceKey[T]) (T, error) {
	service, err := c.Resolve(key.name)
	if err != nil {
		var zero T
		return zero, err
	}

	result, ok := service.(T)
	if !ok {
		var zero T
		return zero, ErrTypeMismatch(key.name, result)
	}

	return result, nil
}

// MustWithKey resolves a service using a typed service key and panics on error.
//
// Example:
//
//	db := MustWithKey(c, DatabaseKey)
func MustWithKey[T any](c Vessel, key ServiceKey[T]) T {
	result, err := ResolveWithKey(c, key)
	if err != nil {
		panic(err)
	}
	return result
}

// HasKey checks if a service is registered using a typed service key.
func HasKey[T any](c Vessel, key ServiceKey[T]) bool {
	return c.Has(key.name)
}

// IsStartedKey checks if a service has been started using a typed service key.
func IsStartedKey[T any](c Vessel, key ServiceKey[T]) bool {
	return c.IsStarted(key.name)
}

// InspectKey returns diagnostic information about a service using a typed service key.
func InspectKey[T any](c Vessel, key ServiceKey[T]) ServiceInfo {
	return c.Inspect(key.name)
}
