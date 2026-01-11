package vessel

import "github.com/xraph/go-utils/di"

// RegisterOption is a configuration option for service registration.
type RegisterOption = di.RegisterOption

// Singleton makes the service a singleton (default).
func Singleton() RegisterOption {
	return di.Singleton()
}

// Transient makes the service created on each resolve.
func Transient() RegisterOption {
	return di.Transient()
}

// Scoped makes the service live for the duration of a scope.
func Scoped() RegisterOption {
	return di.Scoped()
}

// WithDependencies declares explicit dependencies.
func WithDependencies(deps ...string) RegisterOption {
	return di.WithDependencies(deps...)
}

// WithDIMetadata adds diagnostic metadata to DI service registration.
func WithDIMetadata(key, value string) RegisterOption {
	return di.WithDIMetadata(key, value)
}

// WithGroup adds service to a named group.
func WithGroup(group string) RegisterOption {
	return di.WithGroup(group)
}

// merge combines multiple options.
func mergeOptions(opts []RegisterOption) RegisterOption {
	return di.MergeOptions(opts)
}
