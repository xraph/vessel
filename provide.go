package vessel

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/xraph/go-utils/di"
	"github.com/xraph/go-utils/errs"
)

// healthCheckerType is the reflect.Type for di.HealthChecker, used to check
// whether registered types implement health checking at registration time.
var healthCheckerType = reflect.TypeOf((*di.HealthChecker)(nil)).Elem()

// isTypeHealthCapable checks if a reflect.Type implements di.HealthChecker.
func isTypeHealthCapable(t reflect.Type) bool {
	if t.Implements(healthCheckerType) {
		return true
	}
	if t.Kind() == reflect.Pointer {
		return t.Elem().Implements(healthCheckerType)
	}
	// Check pointer-to-type as well (methods may be on *T)
	return reflect.PointerTo(t).Implements(healthCheckerType)
}

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
	eager     bool           // Instantiate immediately after registration
}

// constructorOptionFunc is a function adapter for ConstructorOption
type constructorOptionFunc func(*constructorConfig)

func (f constructorOptionFunc) applyConstructor(c *constructorConfig) { f(c) }

// WithName gives the constructor result a name for disambiguation.
// Use this when you have multiple implementations of the same type.
//
// Example:
//
//	Provide(c, NewPrimaryDB, WithName("primary"))
//	Provide(c, NewReplicaDB, WithName("replica"))
func WithName(name string) ConstructorOption {
	return constructorOptionFunc(func(c *constructorConfig) {
		c.name = name
	})
}

// WithAliases registers the constructor result under additional named variants.
// The primary registration is by TYPE (unnamed), and aliases provide named access.
// This allows the service to be resolved both by type and by name.
//
// Example:
//
//	// Register by type with named aliases
//	Provide(c, NewDatabaseManager, WithAliases("manager", "db-manager"))
//
//	// Can resolve by type (unnamed):
//	mgr1, _ := Inject[*DatabaseManager](c)
//
//	// Or by any alias name:
//	mgr2, _ := InjectNamed[*DatabaseManager](c, "manager")
//	mgr3, _ := InjectNamed[*DatabaseManager](c, "db-manager")
//	// mgr1 == mgr2 == mgr3 (same singleton instance)
//
// Note: If you use WithName(), that becomes the primary name and aliases are additional.
// Using only WithAliases() keeps the service unnamed (type-based) as primary.
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
//	Provide(c, NewUserHandler, AsGroup("handlers"))
//	Provide(c, NewProductHandler, AsGroup("handlers"))
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
//	Provide(c, NewMyService, As(new(Reader), new(Writer)))
func As(ifaces ...any) ConstructorOption {
	return constructorOptionFunc(func(c *constructorConfig) {
		for _, iface := range ifaces {
			t := reflect.TypeOf(iface)
			if t.Kind() == reflect.Pointer {
				t = t.Elem()
			}
			c.asTypes = append(c.asTypes, t)
		}
	})
}

// AsSingleton makes the constructor result a singleton (default).
func AsSingleton() ConstructorOption {
	return constructorOptionFunc(func(c *constructorConfig) {
		c.lifecycle = lifecycleSingleton
	})
}

// AsTransient makes the constructor create a new instance on each resolve.
func AsTransient() ConstructorOption {
	return constructorOptionFunc(func(c *constructorConfig) {
		c.lifecycle = lifecycleTransient
	})
}

// AsScoped makes the constructor result scoped to request lifetime.
// Scoped services are resolved with InjectScoped, or with Inject from inside
// a factory or constructor that is itself running in a scope.
func AsScoped() ConstructorOption {
	return constructorOptionFunc(func(c *constructorConfig) {
		c.lifecycle = lifecycleScoped
	})
}

// WithEager causes the constructor to be invoked immediately after registration,
// rather than waiting for first use (lazy). This is useful for:
//   - Fail-fast behavior: catch construction errors at startup
//   - Services that need to initialize during app startup (servers, connections)
//   - Pre-warming caches
//
// Example:
//
//	// Start database connection immediately, fail if can't connect
//	Provide(c, NewDatabase, WithEager())
//
//	// Lazy (default): Wait until first use
//	Provide(c, NewCache)
func WithEager() ConstructorOption {
	return constructorOptionFunc(func(c *constructorConfig) {
		c.eager = true
	})
}

// newConstructorConfig applies opts over the defaults.
func newConstructorConfig(opts []ConstructorOption) *constructorConfig {
	config := &constructorConfig{lifecycle: lifecycleSingleton} // Default to singleton like dig
	for _, opt := range opts {
		opt.applyConstructor(config)
	}

	return config
}

// containerFor returns the concrete container behind c, making sure it has
// a type registry.
func containerFor(c Vessel, op string) (*containerImpl, resolveCtx, error) {
	impl, rc, ok := unwrapContainer(c)
	if !ok {
		return nil, rc, fmt.Errorf("%s requires a container created by vessel.New, got %T", op, c)
	}

	if impl.typeRegistry == nil {
		impl.typeRegistry = newTypeRegistry()
	}

	return impl, rc, nil
}

// Provide registers a constructor function with automatic dependency resolution.
// Dependencies are inferred from function parameters and all return types (except error)
// are registered as services.
//
// This follows the Uber dig pattern for constructor-based dependency injection:
//   - Function parameters become dependencies (resolved by type)
//   - Return types become provided services
//   - Error return type is handled for construction failures
//
// A constructor with several results, or one returning an Out struct, is
// called once and every result shares that call. Registration is atomic: if
// any key is already taken, nothing is registered.
//
// Example:
//
//	// Simple constructor
//	func NewUserService(db *Database, logger *Logger) *UserService {
//	    return &UserService{db: db, logger: logger}
//	}
//	Provide(c, NewUserService)
//
//	// Constructor with error
//	func NewDatabase(config *Config) (*Database, error) {
//	    return sql.Open(config.Driver, config.DSN)
//	}
//	Provide(c, NewDatabase)
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
//	Provide(c, NewService)
func Provide(c Vessel, constructor any, opts ...ConstructorOption) error {
	info, err := analyzeConstructor(constructor)
	if err != nil {
		return fmt.Errorf("invalid constructor: %w", err)
	}

	config := newConstructorConfig(opts)

	impl, rc, err := containerFor(c, "Provide")
	if err != nil {
		return err
	}

	results := info.flattenResults()
	call := sharedConstructorCall(info, impl, config.lifecycle, len(results) > 1)

	var (
		entries []keyedRegistration
		regs    []*typeRegistration
	)

	for _, result := range results {
		name := config.name
		if result.name != "" {
			name = result.name
		}

		var groups []string
		if config.group != "" {
			groups = append(groups, config.group)
		}
		if result.group != "" {
			groups = append(groups, result.group)
		}

		for _, asType := range config.asTypes {
			if !result.typ.AssignableTo(asType) {
				return fmt.Errorf("invalid constructor: %s does not implement %s", result.typ, asType)
			}
		}

		reg := &typeRegistration{
			key:           typeKey{typ: result.typ, name: name},
			build:         resultBuilder(call, result),
			lifecycle:     config.lifecycle,
			groups:        groups,
			healthCapable: isTypeHealthCapable(result.typ),
		}

		regs = append(regs, reg)
		entries = append(entries, keysFor(reg, result.typ, name, config)...)
	}

	if err := impl.typeRegistry.registerAll(entries); err != nil {
		return err
	}

	if config.eager && config.lifecycle != lifecycleScoped {
		for _, reg := range regs {
			if _, err := reg.resolve(impl, rc); err != nil {
				return fmt.Errorf("eager instantiation failed for %s: %w", reg.key, err)
			}
		}
	}

	return nil
}

// keysFor lists every key a registration answers to: its own type, each As
// type, and every alias of each. All of them share reg, and so one instance.
func keysFor(reg *typeRegistration, typ reflect.Type, name string, config *constructorConfig) []keyedRegistration {
	types := append([]reflect.Type{typ}, config.asTypes...)

	entries := make([]keyedRegistration, 0, len(types)*(1+len(config.aliases)))
	for _, t := range types {
		entries = append(entries, keyedRegistration{key: typeKey{typ: t, name: name}, reg: reg})

		for _, alias := range config.aliases {
			label := fmt.Sprintf("alias %q", alias)
			if t != typ {
				label = fmt.Sprintf("alias %q for type %s", alias, t)
			}

			entries = append(entries, keyedRegistration{key: typeKey{typ: t, name: alias}, reg: reg, label: label})
		}
	}

	return entries
}

// constructorCall runs a constructor and returns its non-error results.
type constructorCall func(rc resolveCtx) ([]reflect.Value, error)

// sharedConstructorCall returns a call that, when the constructor provides
// several services, runs once per lifetime (once per container for
// singletons, once per scope for scoped) and is shared by all its results.
func sharedConstructorCall(info *constructorInfo, impl *containerImpl, lifecycle string, multi bool) constructorCall {
	call := func(rc resolveCtx) ([]reflect.Value, error) {
		return callConstructor(info, impl, rc)
	}

	if !multi || lifecycle == lifecycleTransient {
		return call
	}

	memo := &constructorMemo{label: info.fnType.String()}

	return func(rc resolveCtx) ([]reflect.Value, error) {
		rc, err := rc.enter(memo)
		if err != nil {
			return nil, err
		}

		build := func() (any, error) { return call(rc) }

		var v any
		if lifecycle == lifecycleScoped {
			v, err = rc.scope.instance(memo, memo.label, build)
		} else {
			v, _, err = memo.get(build)
		}

		if err != nil {
			return nil, err
		}

		return v.([]reflect.Value), nil
	}
}

// constructorMemo holds the shared call result for a constructor that
// provides several services.
type constructorMemo struct {
	onceValue
	label string
}

// resultBuilder extracts one result, or one Out struct field, from a call.
func resultBuilder(call constructorCall, result resultInfo) builder {
	return func(rc resolveCtx) (any, error) {
		values, err := call(rc)
		if err != nil {
			return nil, err
		}

		v := values[result.position]

		if result.fieldName != "" {
			if v.Kind() == reflect.Pointer {
				if v.IsNil() {
					return nil, fmt.Errorf("constructor returned a nil %s", v.Type())
				}
				v = v.Elem()
			}

			v = v.FieldByName(result.fieldName)
			if !v.IsValid() {
				return nil, fmt.Errorf("field %s not found in result struct", result.fieldName)
			}
		}

		return v.Interface(), nil
	}
}

// callConstructor resolves the constructor's parameters and calls it.
func callConstructor(info *constructorInfo, impl *containerImpl, rc resolveCtx) ([]reflect.Value, error) {
	args := make([]reflect.Value, len(info.params))

	for i, param := range info.params {
		if param.isIn {
			inValue, err := resolveInStruct(param, impl, rc)
			if err != nil {
				return nil, err
			}
			args[i] = inValue

			continue
		}

		resolved, err := resolveParam(param, impl, rc)
		if err != nil {
			return nil, err
		}
		args[i] = valueOf(resolved, param.typ)
	}

	results := info.fn.Call(args)

	if info.hasError {
		if errResult := results[len(results)-1]; !errResult.IsNil() {
			return nil, errResult.Interface().(error)
		}
		results = results[:len(results)-1]
	}

	return results, nil
}

// valueOf converts a resolved instance to a reflect.Value of type t, using
// the zero value for nil.
func valueOf(v any, t reflect.Type) reflect.Value {
	if v == nil {
		return reflect.Zero(t)
	}

	return reflect.ValueOf(v)
}

// Deprecated: Use Provide instead.
func ProvideConstructor(c Vessel, constructor any, opts ...ConstructorOption) error {
	return Provide(c, constructor, opts...)
}

// Deprecated: Use Inject instead.
func InjectType[T any](c Vessel) (T, error) {
	return Inject[T](c)
}

// Deprecated: Use MustInject instead.
func MustInjectType[T any](c Vessel) T {
	return MustInject[T](c)
}

// ProvideNamed registers a named constructor function with automatic dependency resolution.
// This is equivalent to calling Provide with WithName(name).
//
// Example:
//
//	ProvideNamed(c, "primary", NewPrimaryDB)
//	ProvideNamed(c, "replica", NewReplicaDB)
func ProvideNamed(c Vessel, name string, constructor any, opts ...ConstructorOption) error {
	return Provide(c, constructor, append([]ConstructorOption{WithName(name)}, opts...)...)
}

// ProvideValue registers a pre-built instance as a singleton service.
// The instance is registered under the type parameter T, so pass T
// explicitly to register a value under an interface type. Lifecycle options
// are ignored: a value is always a singleton.
//
// Example:
//
//	cfg := &Config{Port: 8080}
//	ProvideValue(c, cfg)
//
//	// Later:
//	config, _ := Inject[*Config](c)
func ProvideValue[T any](c Vessel, value T, opts ...ConstructorOption) error {
	t := reflect.TypeFor[T]()
	config := newConstructorConfig(opts)

	impl, _, err := containerFor(c, "ProvideValue")
	if err != nil {
		return err
	}

	for _, asType := range config.asTypes {
		if !t.AssignableTo(asType) {
			return fmt.Errorf("ProvideValue: %s does not implement %s", t, asType)
		}
	}

	_, isHealthCapable := any(value).(di.HealthChecker)

	reg := &typeRegistration{
		key:           typeKey{typ: t, name: config.name},
		build:         func(resolveCtx) (any, error) { return value, nil },
		instance:      value,
		lifecycle:     lifecycleSingleton,
		healthCapable: isHealthCapable,
	}

	if config.group != "" {
		reg.groups = []string{config.group}
	}

	return impl.typeRegistry.registerAll(keysFor(reg, t, config.name, config))
}

// resolveInStruct creates and populates an In struct with resolved dependencies
func resolveInStruct(param paramInfo, impl *containerImpl, rc resolveCtx) (reflect.Value, error) {
	structType := param.typ
	isPtr := structType.Kind() == reflect.Pointer
	if isPtr {
		structType = structType.Elem()
	}

	structValue := reflect.New(structType).Elem()

	for _, field := range param.inFields {
		var (
			resolved any
			err      error
		)

		if field.group {
			resolved, err = resolveGroup(field, impl, rc)
		} else {
			resolved, err = resolveParam(field, impl, rc)
		}

		if err != nil {
			if field.optional && isNotFound(err) {
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
		return structValue.Addr(), nil
	}

	return structValue, nil
}

// isNotFound reports whether err means a dependency has no provider.
func isNotFound(err error) bool {
	return errors.Is(err, ErrServiceNotFoundSentinel)
}

// resolveParam resolves a single parameter from the type registry
func resolveParam(param paramInfo, impl *containerImpl, rc resolveCtx) (any, error) {
	key := typeKey{typ: param.typ, name: param.name}

	if impl.typeRegistry != nil {
		if reg, ok := impl.typeRegistry.get(key); ok {
			return reg.resolve(impl, rc)
		}
	}

	if param.optional {
		return nil, nil
	}

	return nil, errNoTypeProvider("no provider for type %s", key)
}

// resolveGroup resolves all services in a group as a slice
func resolveGroup(param paramInfo, impl *containerImpl, rc resolveCtx) (any, error) {
	var regs []*typeRegistration
	if impl.typeRegistry != nil {
		regs = impl.typeRegistry.getGroup(param.groupKey)
	}

	if len(regs) == 0 {
		if param.optional {
			return nil, nil
		}

		return nil, errs.NewError(CodeServiceNotFound, fmt.Sprintf("no providers for group %s", param.groupKey), nil)
	}

	elemType := param.typ.Elem() // param.typ is a slice, get element type
	sliceValue := reflect.MakeSlice(param.typ, 0, len(regs))

	for _, reg := range regs {
		instance, err := reg.resolve(impl, rc)
		if err != nil {
			return nil, err
		}

		v := valueOf(instance, elemType)
		if !v.Type().AssignableTo(elemType) {
			return nil, fmt.Errorf("group %s: %s is not assignable to %s", param.groupKey, v.Type(), elemType)
		}

		sliceValue = reflect.Append(sliceValue, v)
	}

	return sliceValue.Interface(), nil
}

// injectKey resolves key and asserts the result to T.
func injectKey[T any](impl *containerImpl, rc resolveCtx, key typeKey) (T, error) {
	var zero T

	if impl.typeRegistry == nil {
		return zero, errNoTypeProvider("no service registered for type %s", key)
	}

	instance, err := impl.typeRegistry.resolve(key, impl, rc)
	if err != nil {
		return zero, err
	}

	typed, ok := instance.(T)
	if !ok {
		return zero, fmt.Errorf("type mismatch: expected %T, got %T", zero, instance)
	}

	return typed, nil
}

// Inject resolves a service by its type.
//
// Example:
//
//	db, err := Inject[*Database](c)
func Inject[T any](c Vessel) (T, error) {
	return InjectNamed[T](c, "")
}

// MustInject resolves a service by its type, panicking on error.
func MustInject[T any](c Vessel) T {
	result, err := Inject[T](c)
	if err != nil {
		panic(fmt.Sprintf("MustInject failed: %v", err))
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
	impl, rc, ok := unwrapContainer(c)
	if !ok {
		var zero T
		return zero, fmt.Errorf("Inject requires a container created by vessel.New, got %T", c)
	}

	return injectKey[T](impl, rc, typeKey{typ: reflect.TypeFor[T](), name: name})
}

// MustInjectNamed resolves a named service by its type, panicking on error.
func MustInjectNamed[T any](c Vessel, name string) T {
	result, err := InjectNamed[T](c, name)
	if err != nil {
		panic(fmt.Sprintf("MustInjectNamed failed: %v", err))
	}
	return result
}

// InjectScoped resolves a service by its type from a scope. Services
// registered with AsScoped get one instance per scope; singletons come from
// the container and transients are built fresh.
//
// Example:
//
//	scope := c.BeginScope()
//	defer scope.End()
//	session, err := InjectScoped[*Session](scope)
func InjectScoped[T any](s Scope) (T, error) {
	return InjectScopedNamed[T](s, "")
}

// MustInjectScoped resolves a service by its type from a scope, panicking on error.
func MustInjectScoped[T any](s Scope) T {
	result, err := InjectScoped[T](s)
	if err != nil {
		panic(fmt.Sprintf("MustInjectScoped failed: %v", err))
	}
	return result
}

// InjectScopedNamed resolves a named service by its type from a scope.
func InjectScopedNamed[T any](s Scope, name string) (T, error) {
	var zero T

	sc, ok := s.(*scope)
	if !ok {
		return zero, fmt.Errorf("InjectScoped requires a scope created by BeginScope, got %T", s)
	}

	if sc.IsEnded() {
		return zero, ErrScopeEnded
	}

	return injectKey[T](sc.parent, resolveCtx{scope: sc}, typeKey{typ: reflect.TypeFor[T](), name: name})
}

// InjectGroup resolves all services in a group as a slice, in registration order.
// An empty or unknown group returns a nil slice and no error.
//
// Example:
//
//	handlers, err := InjectGroup[Handler](c, "http")
func InjectGroup[T any](c Vessel, group string) ([]T, error) {
	impl, rc, ok := unwrapContainer(c)
	if !ok {
		return nil, fmt.Errorf("InjectGroup requires a container created by vessel.New, got %T", c)
	}

	if impl.typeRegistry == nil {
		return nil, errors.New("no type registry available")
	}

	regs := impl.typeRegistry.getGroup(group)
	if len(regs) == 0 {
		return nil, nil // Empty slice for empty groups
	}

	result := make([]T, 0, len(regs))
	for _, reg := range regs {
		instance, err := reg.resolve(impl, rc)
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
	return HasTypeNamed[T](c, "")
}

// HasTypeNamed checks if a named service of the given type is registered.
func HasTypeNamed[T any](c Vessel, name string) bool {
	impl, _, ok := unwrapContainer(c)
	if !ok || impl.typeRegistry == nil {
		return false
	}

	return impl.typeRegistry.has(typeKey{typ: reflect.TypeFor[T](), name: name})
}
