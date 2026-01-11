package vessel

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xraph/go-utils/di"
)

func TestDepMode_String(t *testing.T) {
	tests := []struct {
		mode     di.DepMode
		expected string
	}{
		{di.DepEager, "eager"},
		{di.DepLazy, "lazy"},
		{di.DepOptional, "optional"},
		{di.DepLazyOptional, "lazy_optional"},
		{di.DepMode(99), "unknown"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.mode.String())
	}
}

func TestDepMode_IsLazy(t *testing.T) {
	assert.False(t, di.DepEager.IsLazy())
	assert.True(t, di.DepLazy.IsLazy())
	assert.False(t, di.DepOptional.IsLazy())
	assert.True(t, di.DepLazyOptional.IsLazy())
}

func TestDepMode_IsOptional(t *testing.T) {
	assert.False(t, di.DepEager.IsOptional())
	assert.False(t, di.DepLazy.IsOptional())
	assert.True(t, di.DepOptional.IsOptional())
	assert.True(t, di.DepLazyOptional.IsOptional())
}

func TestDep_Helpers(t *testing.T) {
	eager := di.Eager("db")
	assert.Equal(t, "db", eager.Name)
	assert.Equal(t, di.DepEager, eager.Mode)

	lazy := di.Lazy("cache")
	assert.Equal(t, "cache", lazy.Name)
	assert.Equal(t, di.DepLazy, lazy.Mode)

	optional := di.Optional("tracer")
	assert.Equal(t, "tracer", optional.Name)
	assert.Equal(t, di.DepOptional, optional.Mode)

	lazyOptional := di.LazyOptional("analytics")
	assert.Equal(t, "analytics", lazyOptional.Name)
	assert.Equal(t, di.DepLazyOptional, lazyOptional.Mode)
}

func TestDepNames(t *testing.T) {
	deps := []di.Dep{
		di.Eager("db"),
		di.Lazy("cache"),
		di.Optional("tracer"),
	}

	names := di.DepNames(deps)
	assert.Equal(t, []string{"db", "cache", "tracer"}, names)
}

func TestDepsFromNames(t *testing.T) {
	names := []string{"db", "cache", "logger"}
	deps := di.DepsFromNames(names)

	require.Len(t, deps, 3)

	for i, dep := range deps {
		assert.Equal(t, names[i], dep.Name)
		assert.Equal(t, di.DepEager, dep.Mode) // All should be eager
	}
}

func TestDependencyGraph_AddNodeWithDeps(t *testing.T) {
	graph := NewDependencyGraph()

	deps := []di.Dep{
		di.Eager("db"),
		di.Lazy("cache"),
		di.Optional("tracer"),
	}

	graph.AddNodeWithDeps("service", deps)

	// Should have stored the deps
	retrievedDeps := graph.GetDeps("service")
	require.Len(t, retrievedDeps, 3)
	assert.Equal(t, "db", retrievedDeps[0].Name)
	assert.Equal(t, di.DepEager, retrievedDeps[0].Mode)
	assert.Equal(t, "cache", retrievedDeps[1].Name)
	assert.Equal(t, di.DepLazy, retrievedDeps[1].Mode)
}

func TestDependencyGraph_GetEagerDependencies(t *testing.T) {
	graph := NewDependencyGraph()

	deps := []di.Dep{
		di.Eager("db"),
		di.Lazy("cache"),
		di.Optional("tracer"),
		di.LazyOptional("analytics"),
	}

	graph.AddNodeWithDeps("service", deps)

	eagerDeps := graph.GetEagerDependencies("service")
	// Only eager and optional (non-lazy) should be returned
	require.Len(t, eagerDeps, 2)
	assert.Contains(t, eagerDeps, "db")
	assert.Contains(t, eagerDeps, "tracer")
}

func TestDependencyGraph_TopologicalSortEagerOnly(t *testing.T) {
	graph := NewDependencyGraph()

	// Service A depends on B (eager) and C (lazy)
	graph.AddNodeWithDeps("A", []di.Dep{
		di.Eager("B"),
		di.Lazy("C"),
	})

	// B has no deps
	graph.AddNode("B", nil)

	// C depends on D (eager)
	graph.AddNodeWithDeps("C", []di.Dep{
		di.Eager("D"),
	})

	// D has no deps
	graph.AddNode("D", nil)

	// Eager-only sort should not include C->D dependency
	order, err := graph.TopologicalSortEagerOnly()
	require.NoError(t, err)

	// B should come before A (eager dependency)
	bIdx := sliceIndexOf(order, "B")
	aIdx := sliceIndexOf(order, "A")
	assert.True(t, bIdx < aIdx, "B should come before A")

	// D should come before C (C depends on D eagerly)
	dIdx := sliceIndexOf(order, "D")
	cIdx := sliceIndexOf(order, "C")
	assert.True(t, dIdx < cIdx, "D should come before C")
}

func TestDependencyGraph_HasNode(t *testing.T) {
	graph := NewDependencyGraph()

	graph.AddNode("service", nil)

	assert.True(t, graph.HasNode("service"))
	assert.False(t, graph.HasNode("non-existent"))
}

func TestRegisterOption_GetAllDeps(t *testing.T) {
	// Test with both old and new style dependencies
	opt := di.RegisterOption{
		Dependencies: []string{"legacy1", "legacy2"},
		Deps: []di.Dep{
			di.Eager("new1"),
			di.Lazy("new2"),
		},
	}

	allDeps := opt.GetAllDeps()
	require.Len(t, allDeps, 4)

	// New deps should come first
	assert.Equal(t, "new1", allDeps[0].Name)
	assert.Equal(t, "new2", allDeps[1].Name)

	// Legacy deps should be converted to eager
	assert.Equal(t, "legacy1", allDeps[2].Name)
	assert.Equal(t, di.DepEager, allDeps[2].Mode)
	assert.Equal(t, "legacy2", allDeps[3].Name)
	assert.Equal(t, di.DepEager, allDeps[3].Mode)
}

func TestRegisterOption_GetAllDepNames(t *testing.T) {
	opt := di.RegisterOption{
		Dependencies: []string{"legacy1"},
		Deps: []di.Dep{
			di.Eager("new1"),
		},
	}

	names := opt.GetAllDepNames()
	require.Len(t, names, 2)
	assert.Contains(t, names, "new1")
	assert.Contains(t, names, "legacy1")
}

func TestContainer_RegisterWithDeps(t *testing.T) {
	c := newContainerImpl()

	// Register using the new WithDeps option
	err := c.Register("service", func(_ Vessel) (any, error) {
		return "test", nil
	}, di.WithDeps(
		di.Eager("dep1"),
		di.Lazy("dep2"),
	))
	require.NoError(t, err)

	// Check the service info
	info := c.Inspect("service")
	require.Len(t, info.Deps, 2)
	assert.Equal(t, "dep1", info.Deps[0].Name)
	assert.Equal(t, di.DepEager, info.Deps[0].Mode)
	assert.Equal(t, "dep2", info.Deps[1].Name)
	assert.Equal(t, di.DepLazy, info.Deps[1].Mode)
}

func sliceIndexOf(slice []string, item string) int {
	for i, v := range slice {
		if v == item {
			return i
		}
	}

	return -1
}
