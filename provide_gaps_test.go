package vessel

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xraph/go-utils/di"
)

type gapHandler interface{ ID() string }

type gapA struct{}

func (gapA) ID() string { return "a" }

type gapB struct{}

func (gapB) ID() string { return "b" }

func newGapA() gapHandler { return gapA{} }

func newGapB() *gapB { return &gapB{} }

func TestProvideValue_And_Inject(t *testing.T) {
	c := New()
	cfg := &testLogger{level: "debug"}

	require.NoError(t, ProvideValue(c, cfg))

	got, err := Inject[*testLogger](c)
	require.NoError(t, err)
	assert.Same(t, cfg, got)
	assert.True(t, HasType[*testLogger](c))
}

func TestProvideValue_Named(t *testing.T) {
	c := New()

	require.NoError(t, ProvideValue(c, &testLogger{level: "warn"}, WithName("audit")))

	got, err := InjectNamed[*testLogger](c, "audit")
	require.NoError(t, err)
	assert.Equal(t, "warn", got.level)
	assert.True(t, HasTypeNamed[*testLogger](c, "audit"))
	assert.False(t, HasTypeNamed[*testLogger](c, "other"))
}

func TestProvideNamed_ResolvesByName(t *testing.T) {
	c := New()

	require.NoError(t, ProvideNamed(c, "primary", newTestDatabase))

	got, err := InjectNamed[*testDatabase](c, "primary")
	require.NoError(t, err)
	assert.NotEmpty(t, got.connStr)
}

func TestDeprecatedProvideAndInjectWrappers(t *testing.T) {
	c := New()

	require.NoError(t, ProvideConstructor(c, newTestDatabase))

	got, err := InjectType[*testDatabase](c)
	require.NoError(t, err)
	assert.NotNil(t, got)
	assert.Same(t, got, MustInjectType[*testDatabase](c))
}

func TestProvide_LifecycleOptions(t *testing.T) {
	t.Run("singleton returns the same instance", func(t *testing.T) {
		c := New()
		require.NoError(t, Provide(c, newTestDatabase, AsSingleton()))

		first := MustInject[*testDatabase](c)
		second := MustInject[*testDatabase](c)
		assert.Same(t, first, second)
	})

	t.Run("scoped registers", func(t *testing.T) {
		c := New()
		require.NoError(t, Provide(c, newTestDatabase, AsScoped()))
		assert.True(t, HasType[*testDatabase](c))
	})
}

func TestInjectGroup(t *testing.T) {
	c := New()

	require.NoError(t, Provide(c, newGapA, AsGroup("handlers")))
	require.NoError(t, Provide(c, newGapB, AsGroup("handlers")))

	t.Run("collects every member", func(t *testing.T) {
		handlers, err := InjectGroup[gapHandler](c, "handlers")
		require.NoError(t, err)
		require.Len(t, handlers, 2)

		ids := []string{handlers[0].ID(), handlers[1].ID()}
		assert.ElementsMatch(t, []string{"a", "b"}, ids)
		assert.Len(t, MustInjectGroup[gapHandler](c, "handlers"), 2)
	})

	t.Run("empty group is not an error", func(t *testing.T) {
		handlers, err := InjectGroup[gapHandler](c, "missing")
		require.NoError(t, err)
		assert.Empty(t, handlers)
	})

	t.Run("wrong element type is reported", func(t *testing.T) {
		_, err := InjectGroup[*testDatabase](c, "handlers")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "type mismatch")
	})
}

func TestInjectGroup_NoRegistry(t *testing.T) {
	c := New().(*containerImpl)
	c.typeRegistry = nil

	_, err := InjectGroup[gapHandler](c, "handlers")
	require.Error(t, err)

	assert.Panics(t, func() { MustInjectGroup[gapHandler](c, "handlers") })
}

func TestResolveGroup(t *testing.T) {
	c := New().(*containerImpl)
	param := paramInfo{
		typ:      reflect.TypeFor[[]gapHandler](),
		group:    true,
		groupKey: "handlers",
	}

	t.Run("no registry", func(t *testing.T) {
		c.typeRegistry = nil

		_, err := resolveGroup(param, c)
		require.Error(t, err)

		param.optional = true
		got, err := resolveGroup(param, c)
		require.NoError(t, err)
		assert.Nil(t, got)

		param.optional = false
	})

	t.Run("empty group", func(t *testing.T) {
		c.typeRegistry = newTypeRegistry()

		_, err := resolveGroup(param, c)
		require.Error(t, err)

		param.optional = true
		got, err := resolveGroup(param, c)
		require.NoError(t, err)
		assert.Nil(t, got)

		param.optional = false
	})

	t.Run("populated group", func(t *testing.T) {
		c.typeRegistry = nil
		require.NoError(t, Provide(c, newGapA, AsGroup("handlers")))

		got, err := resolveGroup(param, c)
		require.NoError(t, err)
		assert.Len(t, got.([]gapHandler), 1)
	})
}

func TestHasType_UnregisteredAndNoRegistry(t *testing.T) {
	c := New()
	assert.False(t, HasType[*testDatabase](c))

	impl := c.(*containerImpl)
	impl.typeRegistry = nil
	assert.False(t, HasType[*testDatabase](c))
	assert.False(t, HasTypeNamed[*testDatabase](c, "x"))
}

func TestDeriveServiceName(t *testing.T) {
	assert.Equal(t, "custom", deriveServiceName(typeKey{typ: reflect.TypeFor[*testDatabase](), name: "custom"}))
	assert.Equal(t, "github.com/xraph/vessel.testDatabase", deriveServiceName(typeKey{typ: reflect.TypeFor[*testDatabase]()}))
	assert.Equal(t, "string", deriveServiceName(typeKey{typ: reflect.TypeFor[string]()}))
}

func TestErrTypeMismatch(t *testing.T) {
	err := ErrTypeMismatch("db", 42)

	assert.Equal(t, CodeTypeMismatch, err.Code)
	assert.Contains(t, err.Error(), "db")
	assert.Contains(t, err.Error(), "int")
}

func TestDependencyGraph_Lookups(t *testing.T) {
	g := NewDependencyGraph()
	deps := []di.Dep{{Name: "db"}, {Name: "cache", Mode: di.DepLazy}}
	g.AddNodeWithDeps("svc", deps)

	assert.Equal(t, []string{"db", "cache"}, g.GetDependencies("svc"))
	assert.Nil(t, g.GetDependencies("unknown"))

	assert.Equal(t, deps, g.GetDeps("svc"))
	assert.Nil(t, g.GetDeps("unknown"))
}

func TestWrapperNames(t *testing.T) {
	c := New()

	assert.Equal(t, "a", NewLazy[string](c, "a").Name())
	assert.Equal(t, "b", NewOptionalLazy[string](c, "b").Name())
	assert.Equal(t, "c", NewProvider[string](c, "c").Name())
}
