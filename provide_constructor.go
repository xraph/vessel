package vessel

import (
	"fmt"
	"reflect"
)

// ConstructorOption configures how a constructor is registered
type ConstructorOption interface {
	applyConstructor(*constructorConfig)
}

// constructorConfig holds configuration for constructor registration
type constructorConfig struct {
	name      string         // Optional name for disambiguation
	aliases   []string       // Additional names to register under
	group     string         // Add to a value group
	asTypes   []reflect.Type // Register as additional interface types
	lifecycle string         // Service lifecycle (default: "singleton")
}

// constructorOptionFunc is a function adapter for ConstructorOption
type constructorOptionFunc func(*constructorConfig)

func (f constructorOptionFunc) applyConstructor(c *constructorConfig) { f(c) }

// WithName gives the constructor result a name for disambiguation.
// Use this when you have multiple implementations of the same type.
//
// Example:
//
//	ProvideConstructor(c, NewPrimaryDB, WithName("primary"))
//	ProvideConstructor(c, NewReplicaDB, WithName("replica"))
func WithName(name string) ConstructorOption {
	return constructorOptionFunc(func(c *constructorConfig) {
		c.name = name
	})
}

// WithAliases registers the constructor result under additional names.
// This allows retrieving the same service instance using different names.
// Use empty string ("") as an alias to also register without a name.
//
// Example:
//
//	// Register with primary name "manager" and also accessible without name
//	ProvideConstructor(c, NewDatabaseManager, WithName("manager"), WithAliases(""))
//
//	// Can now resolve both ways:
//	manager1, _ := InjectNamed[*DatabaseManager](c, "manager")
//	manager2, _ := InjectType[*DatabaseManager](c)  // Same instance
//
//	// Multiple aliases
//	ProvideConstructor(c, NewCache, WithName("primary"), WithAliases("default", "main"))
func WithAliases(names ...string) ConstructorOption {
	return constructorOptionFunc(func(c *constructorConfig) {
		c.aliases = append(c.aliases, names...)
	})
}

// AsGroup adds the constructor result to a value group.
// Services in the same group can be resolved together as a slice.
//
// Example:
//
//	ProvideConstructor(c, NewUserHandler, AsGroup("handlers"))
//	ProvideConstructor(c, NewProductHandler, AsGroup("handlers"))
//	handlers := InjectGroup[Handler](c, "handlers") // Returns []Handler
func AsGroup(group string) ConstructorOption {
	return constructorOptionFunc(func(c *constructorConfig) {
		c.group = group
	})
}

// As registers the constructor result as additional interface types.
// This enables resolving the service by its interface types.
//
// Example:
//
//	ProvideConstructor(c, NewMyService, As(new(Reader), new(Writer)))
func As(ifaces ...any) ConstructorOption {
	return constructorOptionFunc(func(c *constructorConfig) {
		for _, iface := range ifaces {
			t := reflect.TypeOf(iface)
			if t.Kind() == reflect.Ptr {
				t = t.Elem()
			}
			c.asTypes = append(c.asTypes, t)
		}
	})
}

// AsSingleton makes the constructor result a singleton (default).
func AsSingleton() ConstructorOption {
	return constructorOptionFunc(func(c *constructorConfig) {
		c.lifecycle = "singleton"
	})
}

// AsTransient makes the constructor create a new instance on each resolve.
func AsTransient() ConstructorOption {
	return constructorOptionFunc(func(c *constructorConfig) {
		c.lifecycle = "transient"
	})
}

// AsScoped makes the constructor result scoped to request lifetime.
func AsScoped() ConstructorOption {
	return constructorOptionFunc(func(c *constructorConfig) {
		c.lifecycle = "scoped"
	})
}

// ProvideConstructor registers a constructor function with automatic dependency resolution.
// Dependencies are inferred from function parameters and all return types (except error)
// are registered as services.
//
// This follows the Uber dig pattern for constructor-based dependency injection:
//   - Function parameters become dependencies (resolved by type)
//   - Return types become provided services
//   - Error return type is handled for construction failures
//
// Example:
//
//	// Simple constructor
//	func NewUserService(db *Database, logger *Logger) *UserService {
//	    return &UserService{db: db, logger: logger}
//	}
//	ProvideConstructor(c, NewUserService)
//
//	// Constructor with error
//	func NewDatabase(config *Config) (*Database, error) {
//	    return sql.Open(config.Driver, config.DSN)
//	}
//	ProvideConstructor(c, NewDatabase)
//
//	// Using In struct for many dependencies
//	type ServiceParams struct {
//	    vessel.In
//	    DB     *Database
//	    Logger *Logger `optional:"true"`
//	}
//	func NewService(p ServiceParams) *Service {
//	    return &Service{db: p.DB, logger: p.Logger}
//	}
//	ProvideConstructor(c, NewService)
func ProvideConstructor(c Vessel, constructor any, opts ...ConstructorOption) error {
	// Analyze the constructor
	info, err := analyzeConstructor(constructor)
	if err != nil {
		return fmt.Errorf("invalid constructor: %w", err)
	}

	// Apply options
	config := &constructorConfig{
		lifecycle: "singleton", // Default to singleton like dig
	}
	for _, opt := range opts {
		opt.applyConstructor(config)
	}

	// Get the container implementation
	impl, ok := c.(*containerImpl)
	if !ok {
		return fmt.Errorf("ProvideConstructor requires *containerImpl, got %T", c)
	}

	// Ensure type registry exists
	if impl.typeRegistry == nil {
		impl.typeRegistry = newTypeRegistry()
	}

	// Create factory function that auto-resolves dependencies
	factory := createAutoResolveFactory(info, impl)

	// Register each result type
	results := info.flattenResults()
	for _, result := range results {
		// Use configured name or result-specific name
		name := config.name
		if result.name != "" {
			name = result.name
		}

		key := typeKey{typ: result.typ, name: name}

		// Determine groups for this result
		groups := []string{}
		if config.group != "" {
			groups = append(groups, config.group)
		}
		if result.group != "" {
			groups = append(groups, result.group)
		}

		// Create wrapper factory for multi-result constructors (Out structs)
		resultFactory := factory
		if len(results) > 1 && result.fieldName != "" {
			resultFactory = createMultiResultFactory(factory, result.fieldName, result.typ)
		}

		reg := &typeRegistration{
			key:         key,
			constructor: info,
			factory:     resultFactory,
			lifecycle:   config.lifecycle,
			groups:      groups,
		}

		if err := impl.typeRegistry.register(key, reg); err != nil {
			return err
		}

		// Also register as additional interface types
		for _, asType := range config.asTypes {
			asKey := typeKey{typ: asType, name: name}
			asReg := &typeRegistration{
				key:         asKey,
				constructor: info,
				factory:     resultFactory,
				lifecycle:   config.lifecycle,
				groups:      groups,
			}
			if err := impl.typeRegistry.register(asKey, asReg); err != nil {
				return err
			}
		}

		// Register under additional aliases
		// NOTE: Aliases point to the SAME registration object to share singleton instances
		for _, alias := range config.aliases {
			aliasKey := typeKey{typ: result.typ, name: alias}
			if err := impl.typeRegistry.register(aliasKey, reg); err != nil {
				return fmt.Errorf("failed to register alias %q: %w", alias, err)
			}

			// Also register aliases for additional interface types
			// These share the same registration as the corresponding asType registration
			for i, asType := range config.asTypes {
				aliasAsKey := typeKey{typ: asType, name: alias}
				// Find the corresponding asType registration to share
				asKey := typeKey{typ: asType, name: name}
				asReg, ok := impl.typeRegistry.get(asKey)
				if !ok {
					// This shouldn't happen since we just registered it above
					return fmt.Errorf("failed to find registration for type %s", asType)
				}
				if err := impl.typeRegistry.register(aliasAsKey, asReg); err != nil {
					return fmt.Errorf("failed to register alias %q for type %s: %w", alias, asType, err)
				}
				_ = i // Unused but kept for clarity
			}
		}
	}

	return nil
}

// createAutoResolveFactory creates a factory that automatically resolves
// constructor parameters from the container
func createAutoResolveFactory(info *constructorInfo, impl *containerImpl) Factory {
	return func(container Vessel) (any, error) {
		// Build arguments for the constructor call
		args := make([]reflect.Value, len(info.params))

		for i, param := range info.params {
			if param.isIn {
				// Create In struct and populate fields
				inValue, err := resolveInStruct(param, impl)
				if err != nil {
					return nil, err
				}
				args[i] = inValue
			} else {
				// Resolve single parameter by type
				resolved, err := resolveParam(param, impl)
				if err != nil {
					return nil, err
				}
				args[i] = reflect.ValueOf(resolved)
			}
		}

		// Call the constructor
		results := info.fn.Call(args)

		// Handle error return
		if info.hasError {
			errResult := results[len(results)-1]
			if !errResult.IsNil() {
				return nil, errResult.Interface().(error)
			}
			results = results[:len(results)-1]
		}

		// Return primary result
		if len(results) == 0 {
			return nil, fmt.Errorf("constructor returned no results")
		}

		// For Out structs, return the struct itself (extraction happens in multi-result factory)
		return results[0].Interface(), nil
	}
}

// resolveInStruct creates and populates an In struct with resolved dependencies
func resolveInStruct(param paramInfo, impl *containerImpl) (reflect.Value, error) {
	structType := param.typ
	isPtr := structType.Kind() == reflect.Ptr
	if isPtr {
		structType = structType.Elem()
	}

	structValue := reflect.New(structType).Elem()

	for _, field := range param.inFields {
		var resolved any
		var err error

		if field.group {
			// Resolve group as slice
			resolved, err = resolveGroup(field, impl)
		} else {
			// Resolve single dependency
			resolved, err = resolveParam(field, impl)
		}

		if err != nil {
			if field.optional {
				// Leave as zero value for optional dependencies
				continue
			}
			return reflect.Value{}, err
		}

		if resolved != nil {
			structValue.Field(field.index).Set(reflect.ValueOf(resolved))
		}
	}

	if isPtr {
		ptrValue := reflect.New(structType)
		ptrValue.Elem().Set(structValue)
		return ptrValue, nil
	}

	return structValue, nil
}

// resolveParam resolves a single parameter from the type registry
func resolveParam(param paramInfo, impl *containerImpl) (any, error) {
	key := typeKey{typ: param.typ, name: param.name}

	// Try type registry first
	if impl.typeRegistry != nil {
		if reg, ok := impl.typeRegistry.get(key); ok {
			return reg.resolve(impl)
		}
	}

	// If not found and optional, return nil
	if param.optional {
		return nil, nil
	}

	return nil, fmt.Errorf("no provider for type %s", key)
}

// resolveGroup resolves all services in a group as a slice
func resolveGroup(param paramInfo, impl *containerImpl) (any, error) {
	if impl.typeRegistry == nil {
		if param.optional {
			return nil, nil
		}
		return nil, fmt.Errorf("no providers for group %s", param.groupKey)
	}

	regs := impl.typeRegistry.getGroup(param.groupKey)
	if len(regs) == 0 {
		if param.optional {
			return nil, nil
		}
		return nil, fmt.Errorf("no providers for group %s", param.groupKey)
	}

	// Create slice of the element type
	elemType := param.typ.Elem() // param.typ is a slice, get element type
	sliceValue := reflect.MakeSlice(param.typ, 0, len(regs))

	for _, reg := range regs {
		instance, err := reg.resolve(impl)
		if err != nil {
			return nil, err
		}
		sliceValue = reflect.Append(sliceValue, reflect.ValueOf(instance).Convert(elemType))
	}

	return sliceValue.Interface(), nil
}

// createMultiResultFactory wraps a factory to extract a specific result from Out struct
func createMultiResultFactory(baseFactory Factory, fieldName string, resultType reflect.Type) Factory {
	return func(container Vessel) (any, error) {
		result, err := baseFactory(container)
		if err != nil {
			return nil, err
		}

		// Extract the specific field from Out struct by name
		resultValue := reflect.ValueOf(result)
		if resultValue.Kind() == reflect.Ptr {
			resultValue = resultValue.Elem()
		}

		if resultValue.Kind() != reflect.Struct {
			// Not an Out struct, return as-is (single result)
			return result, nil
		}

		// Find the field by name
		fieldValue := resultValue.FieldByName(fieldName)
		if !fieldValue.IsValid() {
			return nil, fmt.Errorf("field %s not found in result struct", fieldName)
		}
		return fieldValue.Interface(), nil
	}
}

// InjectType resolves a service by its type.
// This is the type-based counterpart to Resolve[T].
//
// Example:
//
//	db, err := InjectType[*Database](c)
func InjectType[T any](c Vessel) (T, error) {
	var zero T
	t := reflect.TypeOf((*T)(nil)).Elem() // Get the type even for interfaces

	impl, ok := c.(*containerImpl)
	if !ok {
		return zero, fmt.Errorf("InjectType requires *containerImpl, got %T", c)
	}

	if impl.typeRegistry == nil {
		return zero, fmt.Errorf("no type registry available")
	}

	key := typeKey{typ: t}
	instance, err := impl.typeRegistry.resolve(key, c)
	if err != nil {
		return zero, err
	}

	typed, ok := instance.(T)
	if !ok {
		return zero, fmt.Errorf("type mismatch: expected %T, got %T", zero, instance)
	}

	return typed, nil
}

// MustInjectType resolves a service by its type, panicking on error.
func MustInjectType[T any](c Vessel) T {
	result, err := InjectType[T](c)
	if err != nil {
		panic(fmt.Sprintf("MustInjectType failed: %v", err))
	}
	return result
}

// InjectNamed resolves a named service by its type.
// Use this when you have multiple implementations of the same type.
//
// Example:
//
//	primaryDB, err := InjectNamed[*Database](c, "primary")
//	replicaDB, err := InjectNamed[*Database](c, "replica")
func InjectNamed[T any](c Vessel, name string) (T, error) {
	var zero T
	t := reflect.TypeOf((*T)(nil)).Elem() // Get the type even for interfaces

	impl, ok := c.(*containerImpl)
	if !ok {
		return zero, fmt.Errorf("InjectNamed requires *containerImpl, got %T", c)
	}

	if impl.typeRegistry == nil {
		return zero, fmt.Errorf("no type registry available")
	}

	key := typeKey{typ: t, name: name}
	instance, err := impl.typeRegistry.resolve(key, c)
	if err != nil {
		return zero, err
	}

	typed, ok := instance.(T)
	if !ok {
		return zero, fmt.Errorf("type mismatch: expected %T, got %T", zero, instance)
	}

	return typed, nil
}

// MustInjectNamed resolves a named service by its type, panicking on error.
func MustInjectNamed[T any](c Vessel, name string) T {
	result, err := InjectNamed[T](c, name)
	if err != nil {
		panic(fmt.Sprintf("MustInjectNamed failed: %v", err))
	}
	return result
}

// InjectGroup resolves all services in a group as a slice.
//
// Example:
//
//	handlers, err := InjectGroup[Handler](c, "http")
func InjectGroup[T any](c Vessel, group string) ([]T, error) {
	impl, ok := c.(*containerImpl)
	if !ok {
		return nil, fmt.Errorf("InjectGroup requires *containerImpl, got %T", c)
	}

	if impl.typeRegistry == nil {
		return nil, fmt.Errorf("no type registry available")
	}

	regs := impl.typeRegistry.getGroup(group)
	if len(regs) == 0 {
		return nil, nil // Empty slice for empty groups
	}

	result := make([]T, 0, len(regs))
	for _, reg := range regs {
		instance, err := reg.resolve(c)
		if err != nil {
			return nil, err
		}
		typed, ok := instance.(T)
		if !ok {
			return nil, fmt.Errorf("type mismatch in group %s: expected %T, got %T", group, *new(T), instance)
		}
		result = append(result, typed)
	}

	return result, nil
}

// MustInjectGroup resolves all services in a group, panicking on error.
func MustInjectGroup[T any](c Vessel, group string) []T {
	result, err := InjectGroup[T](c, group)
	if err != nil {
		panic(fmt.Sprintf("MustInjectGroup failed: %v", err))
	}
	return result
}

// HasType checks if a service of the given type is registered.
func HasType[T any](c Vessel) bool {
	t := reflect.TypeOf((*T)(nil)).Elem() // Get the type even for interfaces

	impl, ok := c.(*containerImpl)
	if !ok {
		return false
	}

	if impl.typeRegistry == nil {
		return false
	}

	return impl.typeRegistry.has(typeKey{typ: t})
}

// HasTypeNamed checks if a named service of the given type is registered.
func HasTypeNamed[T any](c Vessel, name string) bool {
	t := reflect.TypeOf((*T)(nil)).Elem() // Get the type even for interfaces

	impl, ok := c.(*containerImpl)
	if !ok {
		return false
	}

	if impl.typeRegistry == nil {
		return false
	}

	return impl.typeRegistry.has(typeKey{typ: t, name: name})
}
