package vessel

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServiceKey_BasicUsage(t *testing.T) {
	c := New()

	// Define typed service keys
	var TestKey = NewServiceKey[*testService]("test")

	// Register using key
	err := RegisterWithKey(c, TestKey, func(c Vessel) (*testService, error) {
		return &testService{value: "hello"}, nil
	}, Singleton())
	require.NoError(t, err)

	// Resolve using key
	svc, err := ResolveWithKey(c, TestKey)
	require.NoError(t, err)
	assert.Equal(t, "hello", svc.value)
}

func TestServiceKey_TypeSafety(t *testing.T) {
	c := New()

	// Define typed service keys
	var TestKey = NewServiceKey[*testService]("test")

	// Register using key
	err := RegisterWithKey(c, TestKey, func(c Vessel) (*testService, error) {
		return &testService{value: "hello"}, nil
	})
	require.NoError(t, err)

	// Resolve using key - type is known at compile time
	svc, err := ResolveWithKey(c, TestKey)
	require.NoError(t, err)

	// Can use the service without type assertion
	assert.Equal(t, "hello", svc.value)
}

func TestServiceKey_MustWithKey(t *testing.T) {
	c := New()

	var TestKey = NewServiceKey[*testService]("test")

	err := RegisterWithKey(c, TestKey, func(c Vessel) (*testService, error) {
		return &testService{value: "hello"}, nil
	})
	require.NoError(t, err)

	// MustWithKey should not panic
	svc := MustWithKey(c, TestKey)
	assert.Equal(t, "hello", svc.value)
}

func TestServiceKey_MustWithKeyPanics(t *testing.T) {
	c := New()

	var TestKey = NewServiceKey[*testService]("test")

	// Don't register the service

	// MustWithKey should panic
	assert.Panics(t, func() {
		MustWithKey(c, TestKey)
	})
}

func TestServiceKey_HasKey(t *testing.T) {
	c := New()

	var TestKey = NewServiceKey[*testService]("test")

	// Should not have the service yet
	assert.False(t, HasKey(c, TestKey))

	// Register the service
	err := RegisterWithKey(c, TestKey, func(c Vessel) (*testService, error) {
		return &testService{value: "hello"}, nil
	})
	require.NoError(t, err)

	// Now it should have the service
	assert.True(t, HasKey(c, TestKey))
}

func TestServiceKey_IsStartedKey(t *testing.T) {
	c := New()

	var TestKey = NewServiceKey[*testService]("test")

	// Register a service
	err := RegisterWithKey(c, TestKey, func(c Vessel) (*testService, error) {
		return &testService{value: "hello"}, nil
	}, Singleton())
	require.NoError(t, err)

	// Not started yet (not resolved)
	assert.False(t, IsStartedKey(c, TestKey))

	// Resolve (singletons are marked as started after first resolution)
	_, err = ResolveWithKey(c, TestKey)
	require.NoError(t, err)

	// Now marked as started (singleton was resolved and cached)
	assert.True(t, IsStartedKey(c, TestKey))
}

func TestServiceKey_InspectKey(t *testing.T) {
	c := New()

	var TestKey = NewServiceKey[*testService]("test")

	// Register a service
	err := RegisterWithKey(c, TestKey, func(c Vessel) (*testService, error) {
		return &testService{value: "hello"}, nil
	}, Singleton())
	require.NoError(t, err)

	// Inspect the service
	info := InspectKey(c, TestKey)
	assert.Equal(t, "test", info.Name)
	assert.Equal(t, "singleton", info.Lifecycle)
}

func TestServiceKey_MultipleKeys(t *testing.T) {
	c := New()

	// Define multiple service keys with different types
	var TestKey1 = NewServiceKey[*testService]("test1")
	var TestKey2 = NewServiceKey[*testImpl]("test2")

	// Register multiple services
	err := RegisterWithKey(c, TestKey1, func(c Vessel) (*testService, error) {
		return &testService{value: "service1"}, nil
	})
	require.NoError(t, err)

	err = RegisterWithKey(c, TestKey2, func(c Vessel) (*testImpl, error) {
		return &testImpl{value: "service2"}, nil
	})
	require.NoError(t, err)

	// Resolve both services with type safety
	svc1, err := ResolveWithKey(c, TestKey1)
	require.NoError(t, err)
	assert.Equal(t, "service1", svc1.value)

	svc2, err := ResolveWithKey(c, TestKey2)
	require.NoError(t, err)
	assert.Equal(t, "service2", svc2.value)
}

func TestServiceKey_WithLifecycles(t *testing.T) {
	c := New()

	var SingletonKey = NewServiceKey[*testService]("singleton")
	var TransientKey = NewServiceKey[*testService]("transient")

	// Register singleton
	err := RegisterWithKey(c, SingletonKey, func(c Vessel) (*testService, error) {
		return &testService{value: "singleton"}, nil
	}, Singleton())
	require.NoError(t, err)

	// Register transient
	err = RegisterWithKey(c, TransientKey, func(c Vessel) (*testService, error) {
		return &testService{value: "transient"}, nil
	}, Transient())
	require.NoError(t, err)

	// Resolve singleton twice - should get same instance
	svc1, err := ResolveWithKey(c, SingletonKey)
	require.NoError(t, err)
	svc2, err := ResolveWithKey(c, SingletonKey)
	require.NoError(t, err)
	assert.Same(t, svc1, svc2)

	// Resolve transient twice - should get different instances
	svc3, err := ResolveWithKey(c, TransientKey)
	require.NoError(t, err)
	svc4, err := ResolveWithKey(c, TransientKey)
	require.NoError(t, err)
	assert.NotSame(t, svc3, svc4)
}

func TestServiceKey_NameMethod(t *testing.T) {
	var TestKey = NewServiceKey[*testService]("myService")
	assert.Equal(t, "myService", TestKey.Name())
}

func TestServiceKey_WithOptions(t *testing.T) {
	c := New()

	var TestKey = NewServiceKey[*testService]("test")

	// Register with multiple options
	err := RegisterWithKey(c, TestKey, func(c Vessel) (*testService, error) {
		return &testService{value: "hello"}, nil
	}, Singleton(), WithGroup("api"), WithDIMetadata("version", "1.0"))
	require.NoError(t, err)

	// Inspect the service
	info := InspectKey(c, TestKey)
	assert.Equal(t, "singleton", info.Lifecycle)
	assert.Equal(t, "1.0", info.Metadata["version"])
}
