package vessel

import (
	"github.com/xraph/go-utils/di"
)

// Vessel provides dependency injection with lifecycle management.
type Vessel = di.Container

// Scope represents a lifetime scope for scoped services
// Typically used for HTTP requests or other bounded operations.
type Scope = di.Scope

// Factory creates a service instance.
type Factory = di.Factory

// ServiceInfo contains diagnostic information.
type ServiceInfo = di.ServiceInfo

// RegisterOption is a configuration option for service registration.
type RegisterOption = di.RegisterOption

// Lifecycle names, as reported in ServiceInfo.Lifecycle and accepted by
// ServiceQuery.Lifecycle.
const (
	lifecycleSingleton = "singleton"
	lifecycleTransient = "transient"
	lifecycleScoped    = "scoped"
)

// groupsMetadataKey is the ServiceInfo.Metadata key holding a service's
// comma-separated groups.
const groupsMetadataKey = "__groups"

// mergeOptions combines multiple options.
func mergeOptions(opts []RegisterOption) RegisterOption {
	return di.MergeOptions(opts)
}

// New creates a new DI container.
func New() Vessel {
	return newContainerImpl()
}
