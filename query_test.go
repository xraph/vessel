package vessel

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xraph/go-utils/di"
)

// registerTestServices is a test helper to register multiple services sequentially.
func registerTestServices(t *testing.T, c Vessel, registrations ...func(Vessel) error) {
	t.Helper()
	for _, reg := range registrations {
		require.NoError(t, reg(c))
	}
}

func reg(name string, factory Factory, opts ...RegisterOption) func(Vessel) error {
	return func(c Vessel) error {
		return c.Register(name, factory, opts...)
	}
}

func TestQuery_ByLifecycle(t *testing.T) {
	c := New()

	// Register services with different lifecycles
	registerTestServices(t, c,
		reg("svc1", func(c Vessel) (any, error) {
			return &testService{value: "svc1"}, nil
		}, di.Singleton()),
		reg("svc2", func(c Vessel) (any, error) {
			return &testService{value: "svc2"}, nil
		}, di.Transient()),
		reg("svc3", func(c Vessel) (any, error) {
			return &testService{value: "svc3"}, nil
		}, di.Scoped()),
	)

	// Query for singletons
	results := Query(c, ServiceQuery{Lifecycle: "singleton"})
	assert.Len(t, results, 1)
	assert.Equal(t, "svc1", results[0].Name)

	// Query for transients
	results = Query(c, ServiceQuery{Lifecycle: "transient"})
	assert.Len(t, results, 1)
	assert.Equal(t, "svc2", results[0].Name)

	// Query for scoped
	results = Query(c, ServiceQuery{Lifecycle: "scoped"})
	assert.Len(t, results, 1)
	assert.Equal(t, "svc3", results[0].Name)
}

func TestQuery_ByGroup(t *testing.T) {
	c := New()

	// Register services in different groups
	registerTestServices(t, c,
		reg("svc1", func(c Vessel) (any, error) {
			return &testService{value: "svc1"}, nil
		}, di.WithGroup("api")),
		reg("svc2", func(c Vessel) (any, error) {
			return &testService{value: "svc2"}, nil
		}, di.WithGroup("db")),
		reg("svc3", func(c Vessel) (any, error) {
			return &testService{value: "svc3"}, nil
		}, di.WithGroup("api")),
	)

	// Query for api group
	results := Query(c, ServiceQuery{Group: "api"})
	assert.Len(t, results, 2)
	names := []string{results[0].Name, results[1].Name}
	assert.Contains(t, names, "svc1")
	assert.Contains(t, names, "svc3")

	// Query for db group
	results = Query(c, ServiceQuery{Group: "db"})
	assert.Len(t, results, 1)
	assert.Equal(t, "svc2", results[0].Name)
}

func TestQuery_ByMetadata(t *testing.T) {
	c := New()

	// Register services with metadata
	registerTestServices(t, c,
		reg("svc1", func(c Vessel) (any, error) {
			return &testService{value: "svc1"}, nil
		}, di.WithDIMetadata("version", "1.0"), di.WithDIMetadata("env", "prod")),
		reg("svc2", func(c Vessel) (any, error) {
			return &testService{value: "svc2"}, nil
		}, di.WithDIMetadata("version", "2.0"), di.WithDIMetadata("env", "dev")),
		reg("svc3", func(c Vessel) (any, error) {
			return &testService{value: "svc3"}, nil
		}, di.WithDIMetadata("version", "1.0"), di.WithDIMetadata("env", "dev")),
	)

	// Query for version 1.0
	results := Query(c, ServiceQuery{
		Metadata: map[string]string{"version": "1.0"},
	})
	assert.Len(t, results, 2)
	names := []string{results[0].Name, results[1].Name}
	assert.Contains(t, names, "svc1")
	assert.Contains(t, names, "svc3")

	// Query for version 1.0 AND env prod
	results = Query(c, ServiceQuery{
		Metadata: map[string]string{
			"version": "1.0",
			"env":     "prod",
		},
	})
	assert.Len(t, results, 1)
	assert.Equal(t, "svc1", results[0].Name)
}

func TestQuery_ByStarted(t *testing.T) {
	c := New()

	// Register services
	registerTestServices(t, c,
		reg("svc1", func(c Vessel) (any, error) {
			return &testService{value: "svc1"}, nil
		}, di.Singleton()),
		reg("svc2", func(c Vessel) (any, error) {
			return &testService{value: "svc2"}, nil
		}, di.Singleton()),
		reg("svc3", func(c Vessel) (any, error) {
			return &testService{value: "svc3"}, nil
		}, di.Singleton()),
	)

	// Resolve svc1 and svc2 (starts singletons)
	_, err := c.Resolve("svc1")
	require.NoError(t, err)
	_, err = c.Resolve("svc2")
	require.NoError(t, err)

	// Query for started services
	started := true
	results := Query(c, ServiceQuery{Started: &started})
	assert.Len(t, results, 2)
	names := []string{results[0].Name, results[1].Name}
	assert.Contains(t, names, "svc1")
	assert.Contains(t, names, "svc2")

	// Query for not started services
	notStarted := false
	results = Query(c, ServiceQuery{Started: &notStarted})
	assert.Len(t, results, 1)
	assert.Equal(t, "svc3", results[0].Name)
}

func TestQuery_Combined(t *testing.T) {
	c := New()

	// Register services
	registerTestServices(t, c,
		reg("svc1", func(c Vessel) (any, error) {
			return &testService{value: "svc1"}, nil
		}, di.Singleton(), di.WithGroup("api"), di.WithDIMetadata("version", "1.0")),
		reg("svc2", func(c Vessel) (any, error) {
			return &testService{value: "svc2"}, nil
		}, di.Singleton(), di.WithGroup("api"), di.WithDIMetadata("version", "2.0")),
		reg("svc3", func(c Vessel) (any, error) {
			return &testService{value: "svc3"}, nil
		}, di.Transient(), di.WithGroup("db"), di.WithDIMetadata("version", "1.0")),
	)

	// Resolve svc1
	_, err := c.Resolve("svc1")
	require.NoError(t, err)

	// Query for singleton + api group + version 1.0 + started
	started := true
	results := Query(c, ServiceQuery{
		Lifecycle: "singleton",
		Group:     "api",
		Metadata:  map[string]string{"version": "1.0"},
		Started:   &started,
	})
	assert.Len(t, results, 1)
	assert.Equal(t, "svc1", results[0].Name)
}

func TestQueryNames(t *testing.T) {
	c := New()

	// Register services
	registerTestServices(t, c,
		reg("svc1", func(c Vessel) (any, error) {
			return &testService{value: "svc1"}, nil
		}, di.WithGroup("api")),
		reg("svc2", func(c Vessel) (any, error) {
			return &testService{value: "svc2"}, nil
		}, di.WithGroup("api")),
		reg("svc3", func(c Vessel) (any, error) {
			return &testService{value: "svc3"}, nil
		}, di.WithGroup("db")),
	)

	// Query for api group names
	names := QueryNames(c, ServiceQuery{Group: "api"})
	assert.Len(t, names, 2)
	assert.Contains(t, names, "svc1")
	assert.Contains(t, names, "svc2")
}

func TestFindByGroup(t *testing.T) {
	c := New()

	// Register services
	registerTestServices(t, c,
		reg("svc1", func(c Vessel) (any, error) {
			return &testService{value: "svc1"}, nil
		}, di.WithGroup("api")),
		reg("svc2", func(c Vessel) (any, error) {
			return &testService{value: "svc2"}, nil
		}, di.WithGroup("api")),
	)

	results := FindByGroup(c, "api")
	assert.Len(t, results, 2)
}

func TestFindByLifecycle(t *testing.T) {
	c := New()

	// Register services
	registerTestServices(t, c,
		reg("svc1", func(c Vessel) (any, error) {
			return &testService{value: "svc1"}, nil
		}, di.Singleton()),
		reg("svc2", func(c Vessel) (any, error) {
			return &testService{value: "svc2"}, nil
		}, di.Singleton()),
	)

	results := FindByLifecycle(c, "singleton")
	assert.Len(t, results, 2)
}

func TestFindStarted(t *testing.T) {
	c := New()

	// Register and resolve services
	err := c.Register("svc1", func(c Vessel) (any, error) {
		return &testService{value: "svc1"}, nil
	}, di.Singleton())
	require.NoError(t, err)

	_, err = c.Resolve("svc1")
	require.NoError(t, err)

	results := FindStarted(c)
	assert.Len(t, results, 1)
	assert.Equal(t, "svc1", results[0].Name)
}

func TestFindNotStarted(t *testing.T) {
	c := New()

	// Register but don't resolve
	err := c.Register("svc1", func(c Vessel) (any, error) {
		return &testService{value: "svc1"}, nil
	}, di.Singleton())
	require.NoError(t, err)

	results := FindNotStarted(c)
	assert.Len(t, results, 1)
	assert.Equal(t, "svc1", results[0].Name)
}

func TestQuery_NoMatches(t *testing.T) {
	c := New()

	// Register a service
	err := c.Register("svc1", func(c Vessel) (any, error) {
		return &testService{value: "svc1"}, nil
	}, di.Singleton())
	require.NoError(t, err)

	// Query for nonexistent group
	results := Query(c, ServiceQuery{Group: "nonexistent"})
	assert.Empty(t, results)
}

func TestQuery_EmptyQuery(t *testing.T) {
	c := New()

	// Register services
	registerTestServices(t, c,
		reg("svc1", func(c Vessel) (any, error) {
			return &testService{value: "svc1"}, nil
		}),
		reg("svc2", func(c Vessel) (any, error) {
			return &testService{value: "svc2"}, nil
		}),
	)

	// Empty query should return all services
	results := Query(c, ServiceQuery{})
	assert.Len(t, results, 2)
}
