package vessel

import (
	"fmt"

	"github.com/xraph/go-utils/errs"
)

// =============================================================================
// ERROR CODES
// =============================================================================

const (
	// CodeInvalidFactory indicates a factory function is invalid or nil
	CodeInvalidFactory = "INVALID_FACTORY"

	// CodeServiceAlreadyExists indicates a service is already registered
	CodeServiceAlreadyExists = "SERVICE_ALREADY_EXISTS"

	// CodeServiceNotFound indicates a service was not found in the container
	CodeServiceNotFound = "SERVICE_NOT_FOUND"

	// CodeServiceError indicates an error occurred during service operation
	CodeServiceError = "SERVICE_ERROR"

	// CodeCircularDependency indicates a circular dependency was detected
	CodeCircularDependency = "CIRCULAR_DEPENDENCY"

	// CodeScopeEnded indicates operation on an ended scope
	CodeScopeEnded = "SCOPE_ENDED"

	// CodeTypeMismatch indicates a type mismatch during service resolution
	CodeTypeMismatch = "TYPE_MISMATCH"
)

// =============================================================================
// SENTINEL ERRORS
// =============================================================================

// ErrInvalidFactory is returned when a nil or invalid factory is provided.
var ErrInvalidFactory = errs.NewError(CodeInvalidFactory, "factory cannot be nil", nil)

// ErrServiceNotFoundSentinel is a sentinel error for service not found (for error checking).
var ErrServiceNotFoundSentinel = errs.NewError(CodeServiceNotFound, "service not found", nil)

// ErrCircularDependencySentinel is a sentinel error for circular dependency (for error checking).
var ErrCircularDependencySentinel = errs.NewError(CodeCircularDependency, "circular dependency", nil)

// ErrScopeEnded is returned when operations are attempted on an ended scope.
var ErrScopeEnded = errs.NewError(CodeScopeEnded, "scope has ended", nil)

// ErrTypeMismatchSentinel is a sentinel error for type mismatch during resolution.
var ErrTypeMismatchSentinel = errs.NewError(CodeTypeMismatch, "type mismatch", nil)

// =============================================================================
// ERROR CONSTRUCTORS
// =============================================================================

// ErrServiceAlreadyExists creates an error for when a service is already registered
func ErrServiceAlreadyExists(serviceName string) *errs.Error {
	return errs.NewError(
		CodeServiceAlreadyExists,
		fmt.Sprintf("service '%s' already exists", serviceName),
		nil,
	).WithContext("service", serviceName).(*errs.Error)
}

// ErrServiceNotFound creates an error for when a service is not found
func ErrServiceNotFound(serviceName string) *errs.Error {
	return errs.NewError(
		CodeServiceNotFound,
		fmt.Sprintf("service '%s' not found", serviceName),
		nil,
	).WithContext("service", serviceName).(*errs.Error)
}

// NewServiceError creates an error for service operations
func NewServiceError(serviceName, operation string, cause error) *errs.Error {
	return errs.NewError(
		CodeServiceError,
		fmt.Sprintf("service '%s' error during %s", serviceName, operation),
		cause,
	).WithContext("service", serviceName).
		WithContext("operation", operation).(*errs.Error)
}

// ErrCircularDependency creates an error for circular dependency detection
func ErrCircularDependency(cycle []string) *errs.Error {
	return errs.NewError(
		CodeCircularDependency,
		fmt.Sprintf("circular dependency detected: %v", cycle),
		nil,
	).WithContext("cycle", cycle).(*errs.Error)
}

// ErrTypeMismatch creates an error for type mismatch during resolution
func ErrTypeMismatch(serviceName string, actual any) *errs.Error {
	return errs.NewError(
		CodeTypeMismatch,
		fmt.Sprintf("service '%s' type mismatch: got %T", serviceName, actual),
		nil,
	).WithContext("service", serviceName).
		WithContext("actual_type", fmt.Sprintf("%T", actual)).(*errs.Error)
}
