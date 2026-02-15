package vessel

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xraph/go-utils/di"
)

func TestScope_Has(t *testing.T) {
	c := New()
	scopeImpl := c.BeginScope().(*scope)
	defer func() { _ = scopeImpl.End() }()

	// Register a service
	err := c.Register("test", func(c Vessel) (any, error) {
		return &testService{value: "test"}, nil
	}, di.Singleton())
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
	err := c.Register("svc1", func(c Vessel) (any, error) {
		return &testService{value: "svc1"}, nil
	}, di.Scoped())
	require.NoError(t, err)

	err = c.Register("svc2", func(c Vessel) (any, error) {
		return &testService{value: "svc2"}, nil
	}, di.Scoped())
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
	s := c.BeginScope()
	defer func() { _ = s.End() }()

	// Register scoped service
	err := c.Register("test", func(c Vessel) (any, error) {
		return &testService{value: "hello"}, nil
	}, di.Scoped())
	require.NoError(t, err)

	// Resolve with type assertion
	raw, err := s.Resolve("test")
	require.NoError(t, err)
	svc := raw.(*testService)
	assert.Equal(t, "hello", svc.value)
}

func TestMustScope(t *testing.T) {
	c := New()
	s := c.BeginScope()
	defer func() { _ = s.End() }()

	// Register scoped service
	err := c.Register("test", func(c Vessel) (any, error) {
		return &testService{value: "hello"}, nil
	}, di.Scoped())
	require.NoError(t, err)

	// Resolve should not panic
	raw, err := s.Resolve("test")
	require.NoError(t, err)
	svc := raw.(*testService)
	assert.Equal(t, "hello", svc.value)
}

func TestMustScopePanics(t *testing.T) {
	c := New()
	s := c.BeginScope()
	defer func() { _ = s.End() }()

	// Don't register the service

	// Resolve should fail
	_, err := s.Resolve("test")
	assert.Error(t, err)
}

func TestSetScoped_GetScoped(t *testing.T) {
	c := New()
	scopeImpl := c.BeginScope().(*scope)
	defer func() { _ = scopeImpl.End() }()

	// Set typed values
	scopeImpl.Set("string", "hello")
	scopeImpl.Set("int", 42)
	scopeImpl.Set("struct", &testService{value: "world"})

	// Get typed values
	str, ok := scopeImpl.Get("string")
	assert.True(t, ok)
	assert.Equal(t, "hello", str)

	num, ok := scopeImpl.Get("int")
	assert.True(t, ok)
	assert.Equal(t, 42, num)

	svcRaw, ok := scopeImpl.Get("struct")
	assert.True(t, ok)
	svc := svcRaw.(*testService)
	assert.Equal(t, "world", svc.value)

	// Get nonexistent key
	_, ok = scopeImpl.Get("nonexistent")
	assert.False(t, ok)
}

func TestGetScoped_TypeMismatch(t *testing.T) {
	c := New()
	scopeImpl := c.BeginScope().(*scope)
	defer func() { _ = scopeImpl.End() }()

	// Set a string value
	scopeImpl.Set("key", "hello")

	// Try to get - value is there but is a string, not int
	val, ok := scopeImpl.Get("key")
	assert.True(t, ok)
	_, isInt := val.(int)
	assert.False(t, isInt)
}

func TestScope_ContextIsolation(t *testing.T) {
	c := New()

	// Create two scopes
	scope1 := c.BeginScope().(*scope)
	scope2 := c.BeginScope().(*scope)
	defer func() { _ = scope1.End() }()
	defer func() { _ = scope2.End() }()

	// Set values in each scope
	scope1.Set("key", "scope1")
	scope2.Set("key", "scope2")

	// Values should be isolated
	val1, ok1 := scope1.Get("key")
	assert.True(t, ok1)
	assert.Equal(t, "scope1", val1)

	val2, ok2 := scope2.Get("key")
	assert.True(t, ok2)
	assert.Equal(t, "scope2", val2)
}
