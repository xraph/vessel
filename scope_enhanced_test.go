package vessel

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScope_Has(t *testing.T) {
	c := New()
	scopeImpl := c.BeginScope().(*scope)
	defer func() { _ = scopeImpl.End() }()

	// Register a service
	err := RegisterSingleton(c, "test", func(c Vessel) (*testService, error) {
		return &testService{value: "test"}, nil
	})
	require.NoError(t, err)

	// Scope should delegate to parent container
	assert.True(t, scopeImpl.Has("test"))
	assert.False(t, scopeImpl.Has("nonexistent"))
}

func TestScope_IsEnded(t *testing.T) {
	c := New()
	scopeImpl := c.BeginScope().(*scope)

	// Not ended initially
	assert.False(t, scopeImpl.IsEnded())

	// End the scope
	err := scopeImpl.End()
	require.NoError(t, err)

	// Now ended
	assert.True(t, scopeImpl.IsEnded())
}

func TestScope_Services(t *testing.T) {
	c := New()
	scopeImpl := c.BeginScope().(*scope)
	defer func() { _ = scopeImpl.End() }()

	// Register scoped services
	err := RegisterScoped(c, "svc1", func(c Vessel) (*testService, error) {
		return &testService{value: "svc1"}, nil
	})
	require.NoError(t, err)

	err = RegisterScoped(c, "svc2", func(c Vessel) (*testService, error) {
		return &testService{value: "svc2"}, nil
	})
	require.NoError(t, err)

	// No services resolved yet
	assert.Empty(t, scopeImpl.Services())

	// Resolve first service
	_, err = scopeImpl.Resolve("svc1")
	require.NoError(t, err)
	assert.Len(t, scopeImpl.Services(), 1)
	assert.Contains(t, scopeImpl.Services(), "svc1")

	// Resolve second service
	_, err = scopeImpl.Resolve("svc2")
	require.NoError(t, err)
	assert.Len(t, scopeImpl.Services(), 2)
	assert.Contains(t, scopeImpl.Services(), "svc1")
	assert.Contains(t, scopeImpl.Services(), "svc2")
}

func TestScope_Parent(t *testing.T) {
	c := New()
	scopeImpl := c.BeginScope().(*scope)
	defer func() { _ = scopeImpl.End() }()

	// Parent should be the original container
	assert.Same(t, c, scopeImpl.Parent())
}

func TestScope_SetGet(t *testing.T) {
	c := New()
	scopeImpl := c.BeginScope().(*scope)
	defer func() { _ = scopeImpl.End() }()

	// Set and get values
	scopeImpl.Set("key1", "value1")
	scopeImpl.Set("key2", 42)

	val1, ok1 := scopeImpl.Get("key1")
	assert.True(t, ok1)
	assert.Equal(t, "value1", val1)

	val2, ok2 := scopeImpl.Get("key2")
	assert.True(t, ok2)
	assert.Equal(t, 42, val2)

	// Get nonexistent key
	_, ok3 := scopeImpl.Get("nonexistent")
	assert.False(t, ok3)
}

func TestScope_SetAfterEnd(t *testing.T) {
	c := New()
	scopeImpl := c.BeginScope().(*scope)

	scopeImpl.Set("key", "value")

	// End the scope
	err := scopeImpl.End()
	require.NoError(t, err)

	// Set after end should be silently ignored
	scopeImpl.Set("key2", "value2")

	// Original value should still be accessible
	_, ok := scopeImpl.Get("key")
	assert.False(t, ok) // Context is cleared on End

	// New value should not be set
	_, ok2 := scopeImpl.Get("key2")
	assert.False(t, ok2)
}

func TestResolveScope(t *testing.T) {
	c := New()
	scope := c.BeginScope()
	defer func() { _ = scope.End() }()

	// Register scoped service
	err := RegisterScoped(c, "test", func(c Vessel) (*testService, error) {
		return &testService{value: "hello"}, nil
	})
	require.NoError(t, err)

	// Resolve with type safety
	svc, err := ResolveScope[*testService](scope, "test")
	require.NoError(t, err)
	assert.Equal(t, "hello", svc.value)
}

func TestMustScope(t *testing.T) {
	c := New()
	scope := c.BeginScope()
	defer func() { _ = scope.End() }()

	// Register scoped service
	err := RegisterScoped(c, "test", func(c Vessel) (*testService, error) {
		return &testService{value: "hello"}, nil
	})
	require.NoError(t, err)

	// MustScope should not panic
	svc := MustScope[*testService](scope, "test")
	assert.Equal(t, "hello", svc.value)
}

func TestMustScopePanics(t *testing.T) {
	c := New()
	scope := c.BeginScope()
	defer func() { _ = scope.End() }()

	// Don't register the service

	// MustScope should panic
	assert.Panics(t, func() {
		MustScope[*testService](scope, "test")
	})
}

func TestSetScoped_GetScoped(t *testing.T) {
	c := New()
	scope := c.BeginScope()
	defer func() { _ = scope.End() }()

	// Set typed values
	SetScoped(scope, "string", "hello")
	SetScoped(scope, "int", 42)
	SetScoped(scope, "struct", &testService{value: "world"})

	// Get typed values
	str, ok := GetScoped[string](scope, "string")
	assert.True(t, ok)
	assert.Equal(t, "hello", str)

	num, ok := GetScoped[int](scope, "int")
	assert.True(t, ok)
	assert.Equal(t, 42, num)

	svc, ok := GetScoped[*testService](scope, "struct")
	assert.True(t, ok)
	assert.Equal(t, "world", svc.value)

	// Get nonexistent key
	_, ok = GetScoped[string](scope, "nonexistent")
	assert.False(t, ok)
}

func TestGetScoped_TypeMismatch(t *testing.T) {
	c := New()
	scope := c.BeginScope()
	defer func() { _ = scope.End() }()

	// Set a string value
	SetScoped(scope, "key", "hello")

	// Try to get as int (type mismatch)
	_, ok := GetScoped[int](scope, "key")
	assert.False(t, ok)
}

func TestScope_ContextIsolation(t *testing.T) {
	c := New()

	// Create two scopes
	scope1 := c.BeginScope()
	scope2 := c.BeginScope()
	defer func() { _ = scope1.End() }()
	defer func() { _ = scope2.End() }()

	// Set values in each scope
	SetScoped(scope1, "key", "scope1")
	SetScoped(scope2, "key", "scope2")

	// Values should be isolated
	val1, ok1 := GetScoped[string](scope1, "key")
	assert.True(t, ok1)
	assert.Equal(t, "scope1", val1)

	val2, ok2 := GetScoped[string](scope2, "key")
	assert.True(t, ok2)
	assert.Equal(t, "scope2", val2)
}
