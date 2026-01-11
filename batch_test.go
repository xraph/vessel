package vessel

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterServices_Basic(t *testing.T) {
	c := New()

	// Register multiple services in one call
	err := RegisterServices(c,
		Service("svc1", func(c Vessel) (any, error) {
			return &testService{value: "svc1"}, nil
		}, Singleton()),
		Service("svc2", func(c Vessel) (any, error) {
			return &testService{value: "svc2"}, nil
		}, Transient()),
		Service("svc3", func(c Vessel) (any, error) {
			return &testService{value: "svc3"}, nil
		}),
	)
	require.NoError(t, err)

	// Verify all services are registered
	assert.True(t, c.Has("svc1"))
	assert.True(t, c.Has("svc2"))
	assert.True(t, c.Has("svc3"))

	// Verify they can be resolved
	svc1, err := Resolve[*testService](c, "svc1")
	require.NoError(t, err)
	assert.Equal(t, "svc1", svc1.value)

	svc2, err := Resolve[*testService](c, "svc2")
	require.NoError(t, err)
	assert.Equal(t, "svc2", svc2.value)

	svc3, err := Resolve[*testService](c, "svc3")
	require.NoError(t, err)
	assert.Equal(t, "svc3", svc3.value)
}

func TestRegisterServices_Error(t *testing.T) {
	c := New()

	// Register a service first
	err := RegisterSingleton(c, "existing", func(c Vessel) (*testService, error) {
		return &testService{value: "existing"}, nil
	})
	require.NoError(t, err)

	// Try to register multiple services including a duplicate
	err = RegisterServices(c,
		Service("new1", func(c Vessel) (any, error) {
			return &testService{value: "new1"}, nil
		}),
		Service("existing", func(c Vessel) (any, error) {
			return &testService{value: "duplicate"}, nil
		}),
		Service("new2", func(c Vessel) (any, error) {
			return &testService{value: "new2"}, nil
		}),
	)

	// Should get an error
	assert.Error(t, err)

	// First service should be registered
	assert.True(t, c.Has("new1"))

	// Third service should not be registered (error stops processing)
	assert.False(t, c.Has("new2"))
}

func TestRegisterTypedServices_Basic(t *testing.T) {
	c := New()

	// Register multiple typed services
	err := RegisterTypedServices(c,
		TypedService("svc1", func(c Vessel) (*testService, error) {
			return &testService{value: "svc1"}, nil
		}, Singleton()),
		TypedService("svc2", func(c Vessel) (*testService, error) {
			return &testService{value: "svc2"}, nil
		}, Transient()),
	)
	require.NoError(t, err)

	// Verify services can be resolved
	svc1, err := Resolve[*testService](c, "svc1")
	require.NoError(t, err)
	assert.Equal(t, "svc1", svc1.value)

	svc2, err := Resolve[*testService](c, "svc2")
	require.NoError(t, err)
	assert.Equal(t, "svc2", svc2.value)
}

func TestRegisterKeyedServices_Basic(t *testing.T) {
	c := New()

	// Define service keys
	var (
		Key1 = NewServiceKey[*testService]("svc1")
		Key2 = NewServiceKey[*testService]("svc2")
		Key3 = NewServiceKey[*testImpl]("svc3")
	)

	// Register multiple keyed services
	err := RegisterKeyedServices(c,
		KeyedService(Key1, func(c Vessel) (*testService, error) {
			return &testService{value: "svc1"}, nil
		}, Singleton()),
		KeyedService(Key2, func(c Vessel) (*testService, error) {
			return &testService{value: "svc2"}, nil
		}, Transient()),
	)
	require.NoError(t, err)

	// Register different type
	err = RegisterKeyedServices(c,
		KeyedService(Key3, func(c Vessel) (*testImpl, error) {
			return &testImpl{value: "svc3"}, nil
		}),
	)
	require.NoError(t, err)

	// Verify services can be resolved with type safety
	svc1, err := ResolveWithKey(c, Key1)
	require.NoError(t, err)
	assert.Equal(t, "svc1", svc1.value)

	svc2, err := ResolveWithKey(c, Key2)
	require.NoError(t, err)
	assert.Equal(t, "svc2", svc2.value)

	svc3, err := ResolveWithKey(c, Key3)
	require.NoError(t, err)
	assert.Equal(t, "svc3", svc3.value)
}

func TestService_Constructor(t *testing.T) {
	factory := func(c Vessel) (any, error) {
		return &testService{value: "test"}, nil
	}

	reg := Service("test", factory, Singleton(), WithGroup("api"))

	assert.Equal(t, "test", reg.Name)
	assert.NotNil(t, reg.Factory)
	assert.Len(t, reg.Options, 2)
}

func TestTypedService_Constructor(t *testing.T) {
	factory := func(c Vessel) (*testService, error) {
		return &testService{value: "test"}, nil
	}

	reg := TypedService("test", factory, Singleton())

	assert.Equal(t, "test", reg.Name)
	assert.NotNil(t, reg.Factory)
	assert.Len(t, reg.Options, 1)
}

func TestKeyedService_Constructor(t *testing.T) {
	key := NewServiceKey[*testService]("test")
	factory := func(c Vessel) (*testService, error) {
		return &testService{value: "test"}, nil
	}

	reg := KeyedService(key, factory, Singleton())

	assert.Equal(t, "test", reg.Key.Name())
	assert.NotNil(t, reg.Factory)
	assert.Len(t, reg.Options, 1)
}

func TestRegisterServices_EmptyList(t *testing.T) {
	c := New()

	// Should not error with empty list
	err := RegisterServices(c)
	assert.NoError(t, err)
}

func TestRegisterServices_WithOptions(t *testing.T) {
	c := New()

	// Register services with various options
	err := RegisterServices(c,
		Service("svc1", func(c Vessel) (any, error) {
			return &testService{value: "svc1"}, nil
		}, Singleton(), WithGroup("api"), WithDIMetadata("version", "1.0")),
		Service("svc2", func(c Vessel) (any, error) {
			return &testService{value: "svc2"}, nil
		}, Scoped(), WithGroup("db")),
	)
	require.NoError(t, err)

	// Verify service info
	info1 := c.Inspect("svc1")
	assert.Equal(t, "singleton", info1.Lifecycle)
	assert.Equal(t, "1.0", info1.Metadata["version"])

	info2 := c.Inspect("svc2")
	assert.Equal(t, "scoped", info2.Lifecycle)
}
