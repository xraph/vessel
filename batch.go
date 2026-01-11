package vessel

// ServiceRegistration holds configuration for a service to be registered.
type ServiceRegistration struct {
	Name    string
	Factory Factory
	Options []RegisterOption
}

// Service creates a ServiceRegistration for batch registration.
// This is a convenience function for creating ServiceRegistration structs.
//
// Example:
//
//	vessel.RegisterServices(c,
//	    vessel.Service("db", NewDatabase, vessel.Singleton()),
//	    vessel.Service("cache", NewCache, vessel.Singleton()),
//	)
func Service(name string, factory Factory, opts ...RegisterOption) ServiceRegistration {
	return ServiceRegistration{
		Name:    name,
		Factory: factory,
		Options: opts,
	}
}

// RegisterServices registers multiple services in a single call.
// Returns error if any service registration fails.
//
// Example:
//
//	err := vessel.RegisterServices(c,
//	    vessel.Service("db", NewDatabase, vessel.Singleton()),
//	    vessel.Service("cache", NewCache, vessel.Singleton()),
//	    vessel.Service("logger", NewLogger, vessel.Singleton()),
//	)
func RegisterServices(c Vessel, services ...ServiceRegistration) error {
	for _, svc := range services {
		if err := c.Register(svc.Name, svc.Factory, svc.Options...); err != nil {
			return err
		}
	}
	return nil
}

// TypedServiceRegistration holds configuration for a typed service to be registered.
type TypedServiceRegistration[T any] struct {
	Name    string
	Factory func(Vessel) (T, error)
	Options []RegisterOption
}

// TypedService creates a TypedServiceRegistration for batch typed registration.
func TypedService[T any](name string, factory func(Vessel) (T, error), opts ...RegisterOption) TypedServiceRegistration[T] {
	return TypedServiceRegistration[T]{
		Name:    name,
		Factory: factory,
		Options: opts,
	}
}

// RegisterTypedServices registers multiple typed services in a single call.
// This version provides type safety for the factory functions.
//
// Example:
//
//	err := vessel.RegisterTypedServices(c,
//	    vessel.TypedService("db", NewDatabase, vessel.Singleton()),
//	    vessel.TypedService("cache", NewCache, vessel.Singleton()),
//	)
func RegisterTypedServices[T any](c Vessel, services ...TypedServiceRegistration[T]) error {
	for _, svc := range services {
		// Wrap typed factory in untyped factory
		wrappedFactory := func(c Vessel) (any, error) {
			return svc.Factory(c)
		}
		if err := c.Register(svc.Name, wrappedFactory, svc.Options...); err != nil {
			return err
		}
	}
	return nil
}

// KeyedServiceRegistration holds configuration for a keyed service to be registered.
type KeyedServiceRegistration[T any] struct {
	Key     ServiceKey[T]
	Factory func(Vessel) (T, error)
	Options []RegisterOption
}

// KeyedService creates a KeyedServiceRegistration for batch registration with service keys.
func KeyedService[T any](key ServiceKey[T], factory func(Vessel) (T, error), opts ...RegisterOption) KeyedServiceRegistration[T] {
	return KeyedServiceRegistration[T]{
		Key:     key,
		Factory: factory,
		Options: opts,
	}
}

// RegisterKeyedServices registers multiple keyed services in a single call.
// This version provides type safety via ServiceKeys.
//
// Example:
//
//	var (
//	    DatabaseKey = vessel.NewServiceKey[*Database]("database")
//	    CacheKey    = vessel.NewServiceKey[*Cache]("cache")
//	)
//
//	err := vessel.RegisterKeyedServices(c,
//	    vessel.KeyedService(DatabaseKey, NewDatabase, vessel.Singleton()),
//	    vessel.KeyedService(CacheKey, NewCache, vessel.Singleton()),
//	)
func RegisterKeyedServices[T any](c Vessel, services ...KeyedServiceRegistration[T]) error {
	for _, svc := range services {
		if err := RegisterWithKey(c, svc.Key, svc.Factory, svc.Options...); err != nil {
			return err
		}
	}
	return nil
}
