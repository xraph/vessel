package vessel

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQuery_ByLifecycle(t *testing.T) {
	c := New()

	// Register services with different lifecycles
	err := RegisterServices(c,
		Service("svc1", func(c Vessel) (any, error) {
			return &testService{value: "svc1"}, nil
		}, Singleton()),
		Service("svc2", func(c Vessel) (any, error) {
			return &testService{value: "svc2"}, nil
		}, Transient()),
		Service("svc3", func(c Vessel) (any, error) {
			return &testService{value: "svc3"}, nil
		}, Scoped()),
	)
	require.NoError(t, err)

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
	err := RegisterServices(c,
		Service("svc1", func(c Vessel) (any, error) {
			return &testService{value: "svc1"}, nil
		}, WithGroup("api")),
		Service("svc2", func(c Vessel) (any, error) {
			return &testService{value: "svc2"}, nil
		}, WithGroup("db")),
		Service("svc3", func(c Vessel) (any, error) {
			return &testService{value: "svc3"}, nil
		}, WithGroup("api")),
	)
	require.NoError(t, err)

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
	err := RegisterServices(c,
		Service("svc1", func(c Vessel) (any, error) {
			return &testService{value: "svc1"}, nil
		}, WithDIMetadata("version", "1.0"), WithDIMetadata("env", "prod")),
		Service("svc2", func(c Vessel) (any, error) {
			return &testService{value: "svc2"}, nil
		}, WithDIMetadata("version", "2.0"), WithDIMetadata("env", "dev")),
		Service("svc3", func(c Vessel) (any, error) {
			return &testService{value: "svc3"}, nil
		}, WithDIMetadata("version", "1.0"), WithDIMetadata("env", "dev")),
	)
	require.NoError(t, err)

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
	err := RegisterServices(c,
		Service("svc1", func(c Vessel) (any, error) {
			return &testService{value: "svc1"}, nil
		}, Singleton()),
		Service("svc2", func(c Vessel) (any, error) {
			return &testService{value: "svc2"}, nil
		}, Singleton()),
		Service("svc3", func(c Vessel) (any, error) {
			return &testService{value: "svc3"}, nil
		}, Singleton()),
	)
	require.NoError(t, err)

	// Resolve svc1 and svc2 (starts singletons)
	_, err = Resolve[*testService](c, "svc1")
	require.NoError(t, err)
	_, err = Resolve[*testService](c, "svc2")
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
	err := RegisterServices(c,
		Service("svc1", func(c Vessel) (any, error) {
			return &testService{value: "svc1"}, nil
		}, Singleton(), WithGroup("api"), WithDIMetadata("version", "1.0")),
		Service("svc2", func(c Vessel) (any, error) {
			return &testService{value: "svc2"}, nil
		}, Singleton(), WithGroup("api"), WithDIMetadata("version", "2.0")),
		Service("svc3", func(c Vessel) (any, error) {
			return &testService{value: "svc3"}, nil
		}, Transient(), WithGroup("db"), WithDIMetadata("version", "1.0")),
	)
	require.NoError(t, err)

	// Resolve svc1
	_, err = Resolve[*testService](c, "svc1")
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
	err := RegisterServices(c,
		Service("svc1", func(c Vessel) (any, error) {
			return &testService{value: "svc1"}, nil
		}, WithGroup("api")),
		Service("svc2", func(c Vessel) (any, error) {
			return &testService{value: "svc2"}, nil
		}, WithGroup("api")),
		Service("svc3", func(c Vessel) (any, error) {
			return &testService{value: "svc3"}, nil
		}, WithGroup("db")),
	)
	require.NoError(t, err)

	// Query for api group names
	names := QueryNames(c, ServiceQuery{Group: "api"})
	assert.Len(t, names, 2)
	assert.Contains(t, names, "svc1")
	assert.Contains(t, names, "svc2")
}

func TestFindByGroup(t *testing.T) {
	c := New()

	// Register services
	err := RegisterServices(c,
		Service("svc1", func(c Vessel) (any, error) {
			return &testService{value: "svc1"}, nil
		}, WithGroup("api")),
		Service("svc2", func(c Vessel) (any, error) {
			return &testService{value: "svc2"}, nil
		}, WithGroup("api")),
	)
	require.NoError(t, err)

	results := FindByGroup(c, "api")
	assert.Len(t, results, 2)
}

func TestFindByLifecycle(t *testing.T) {
	c := New()

	// Register services
	err := RegisterServices(c,
		Service("svc1", func(c Vessel) (any, error) {
			return &testService{value: "svc1"}, nil
		}, Singleton()),
		Service("svc2", func(c Vessel) (any, error) {
			return &testService{value: "svc2"}, nil
		}, Singleton()),
	)
	require.NoError(t, err)

	results := FindByLifecycle(c, "singleton")
	assert.Len(t, results, 2)
}

func TestFindStarted(t *testing.T) {
	c := New()

	// Register and resolve services
	err := RegisterSingleton(c, "svc1", func(c Vessel) (*testService, error) {
		return &testService{value: "svc1"}, nil
	})
	require.NoError(t, err)

	_, err = Resolve[*testService](c, "svc1")
	require.NoError(t, err)

	results := FindStarted(c)
	assert.Len(t, results, 1)
	assert.Equal(t, "svc1", results[0].Name)
}

func TestFindNotStarted(t *testing.T) {
	c := New()

	// Register but don't resolve
	err := RegisterSingleton(c, "svc1", func(c Vessel) (*testService, error) {
		return &testService{value: "svc1"}, nil
	})
	require.NoError(t, err)

	results := FindNotStarted(c)
	assert.Len(t, results, 1)
	assert.Equal(t, "svc1", results[0].Name)
}

func TestQuery_NoMatches(t *testing.T) {
	c := New()

	// Register a service
	err := RegisterSingleton(c, "svc1", func(c Vessel) (*testService, error) {
		return &testService{value: "svc1"}, nil
	})
	require.NoError(t, err)

	// Query for nonexistent group
	results := Query(c, ServiceQuery{Group: "nonexistent"})
	assert.Empty(t, results)
}

func TestQuery_EmptyQuery(t *testing.T) {
	c := New()

	// Register services
	err := RegisterServices(c,
		Service("svc1", func(c Vessel) (any, error) {
			return &testService{value: "svc1"}, nil
		}),
		Service("svc2", func(c Vessel) (any, error) {
			return &testService{value: "svc2"}, nil
		}),
	)
	require.NoError(t, err)

	// Empty query should return all services
	results := Query(c, ServiceQuery{})
	assert.Len(t, results, 2)
}
