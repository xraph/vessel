package vessel

import (
	"context"
	"testing"

	"github.com/xraph/go-utils/di"
)

// Benchmark service registration.
func BenchmarkRegister_Singleton(b *testing.B) {
	for i := 0; i < b.N; i++ {
		c := New()
		name := "service"
		_ = c.Register(name, func(c Vessel) (any, error) {
			return "value", nil
		}, di.Singleton())
	}
}

func BenchmarkRegister_Transient(b *testing.B) {
	for i := 0; i < b.N; i++ {
		c := New()
		name := "service"
		_ = c.Register(name, func(c Vessel) (any, error) {
			return "value", nil
		}, di.Transient())
	}
}

func BenchmarkRegister_Scoped(b *testing.B) {
	for i := 0; i < b.N; i++ {
		c := New()
		name := "service"
		_ = c.Register(name, func(c Vessel) (any, error) {
			return "value", nil
		}, di.Scoped())
	}
}

// Benchmark service resolution.
func BenchmarkResolve_Singleton_Cached(b *testing.B) {
	c := New()
	_ = c.Register("service", func(c Vessel) (any, error) {
		return "value", nil
	}, di.Singleton())

	// Warm up cache
	_, _ = c.Resolve("service")

	for i := 0; i < b.N; i++ {
		_, _ = c.Resolve("service")
	}
}

func BenchmarkResolve_Singleton_Uncached(b *testing.B) {
	// First-time resolution: each iteration builds a fresh container, so this
	// also includes New and Register.
	for i := 0; i < b.N; i++ {
		c := New()
		_ = c.Register("service", func(c Vessel) (any, error) {
			return "value", nil
		}, di.Singleton())

		_, _ = c.Resolve("service")
	}
}

func BenchmarkResolve_Transient(b *testing.B) {
	c := New()
	_ = c.Register("service", func(c Vessel) (any, error) {
		return "value", nil
	}, di.Transient())

	for i := 0; i < b.N; i++ {
		_, _ = c.Resolve("service")
	}
}

// Benchmark scope operations.
func BenchmarkScope_Create(b *testing.B) {
	c := New()

	for i := 0; i < b.N; i++ {
		scope := c.BeginScope()
		_ = scope.End()
	}
}

func BenchmarkScope_Resolve_Cached(b *testing.B) {
	c := New()
	_ = c.Register("service", func(c Vessel) (any, error) {
		return "value", nil
	}, di.Scoped())

	scope := c.BeginScope()
	defer func() { _ = scope.End() }()

	// Warm up cache
	_, _ = scope.Resolve("service")

	for i := 0; i < b.N; i++ {
		_, _ = scope.Resolve("service")
	}
}

func BenchmarkScope_Resolve_Uncached(b *testing.B) {
	c := New()
	_ = c.Register("service", func(c Vessel) (any, error) {
		return "value", nil
	}, di.Scoped())

	for i := 0; i < b.N; i++ {
		scope := c.BeginScope()
		_, _ = scope.Resolve("service")
		_ = scope.End()
	}
}

// Benchmark lifecycle operations.
func BenchmarkStart_10Services(b *testing.B) {
	for i := 0; i < b.N; i++ {
		c := New()

		for j := range 10 {
			name := string(rune('a' + j))
			_ = c.Register(name, func(c Vessel) (any, error) {
				return &mockService{name: name, healthy: true}, nil
			})
		}

		ctx := context.Background()
		_ = c.Start(ctx)
	}
}

func BenchmarkStart_100Services(b *testing.B) {
	for i := 0; i < b.N; i++ {
		c := New()

		for j := range 100 {
			name := string(rune('a' + (j % 26)))
			_ = c.Register(name, func(c Vessel) (any, error) {
				return &mockService{name: name, healthy: true}, nil
			})
		}

		ctx := context.Background()
		_ = c.Start(ctx)
	}
}

func BenchmarkHealth_10Services(b *testing.B) {
	c := New()

	for i := range 10 {
		name := string(rune('a' + i))
		_ = c.Register(name, func(c Vessel) (any, error) {
			return &mockService{name: name, healthy: true}, nil
		})
	}

	ctx := context.Background()
	_ = c.Start(ctx)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.Health(ctx)
	}
}

func BenchmarkHealth_100Services(b *testing.B) {
	c := New()

	for i := range 100 {
		name := string(rune('a' + (i % 26)))
		_ = c.Register(name, func(c Vessel) (any, error) {
			return &mockService{name: name, healthy: true}, nil
		})
	}

	ctx := context.Background()
	_ = c.Start(ctx)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.Health(ctx)
	}
}

// Benchmark generic helpers.
func BenchmarkResolveGeneric(b *testing.B) {
	c := New()
	_ = c.Register("service", func(c Vessel) (any, error) {
		return &mockService{name: "test"}, nil
	}, di.Singleton())

	// Warm up cache
	_, _ = c.Resolve("service")

	for i := 0; i < b.N; i++ {
		_, _ = c.Resolve("service")
	}
}

func BenchmarkMust(b *testing.B) {
	c := New()
	_ = c.Register("service", func(c Vessel) (any, error) {
		return &mockService{name: "test"}, nil
	}, di.Singleton())

	// Warm up cache
	val, _ := c.Resolve("service")
	_ = val.(*mockService)

	for i := 0; i < b.N; i++ {
		val, _ := c.Resolve("service")
		_ = val.(*mockService)
	}
}

// Benchmark concurrent access.
func BenchmarkConcurrentResolve(b *testing.B) {
	c := New()
	_ = c.Register("service", func(c Vessel) (any, error) {
		return "value", nil
	}, di.Singleton())

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = c.Resolve("service")
		}
	})
}

func BenchmarkConcurrentScope(b *testing.B) {
	c := New()
	_ = c.Register("service", func(c Vessel) (any, error) {
		return "value", nil
	}, di.Scoped())

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			scope := c.BeginScope()
			_, _ = scope.Resolve("service")
			_ = scope.End()
		}
	})
}

// Benchmarks for constructor injection (type registry).

type benchConfig struct{ dsn string }
type benchDB struct{ cfg *benchConfig }
type benchRepo struct{ db *benchDB }
type benchService struct{ repo *benchRepo }

type benchHandler interface{ Handle() }
type benchHandlerImpl struct{}

func (benchHandlerImpl) Handle() {}

func newBenchContainer(b *testing.B, opts ...ConstructorOption) Vessel {
	b.Helper()

	c := New()
	if err := ProvideValue(c, &benchConfig{dsn: "x"}); err != nil {
		b.Fatal(err)
	}
	if err := Provide(c, func(cfg *benchConfig) *benchDB { return &benchDB{cfg} }); err != nil {
		b.Fatal(err)
	}
	if err := Provide(c, func(db *benchDB) *benchRepo { return &benchRepo{db} }); err != nil {
		b.Fatal(err)
	}
	if err := Provide(c, func(r *benchRepo) *benchService { return &benchService{r} }, opts...); err != nil {
		b.Fatal(err)
	}

	return c
}

func BenchmarkInject_Singleton_Cached(b *testing.B) {
	c := newBenchContainer(b)
	_, _ = Inject[*benchService](c)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Inject[*benchService](c)
	}
}

func BenchmarkInject_Singleton_Parallel(b *testing.B) {
	c := newBenchContainer(b)
	_, _ = Inject[*benchService](c)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = Inject[*benchService](c)
		}
	})
}

func BenchmarkInject_Transient(b *testing.B) {
	// A transient service on top of three cached singletons.
	c := newBenchContainer(b, AsTransient())
	_, _ = Inject[*benchService](c)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Inject[*benchService](c)
	}
}

func BenchmarkInject_ChainUncached(b *testing.B) {
	// Builds a three-level constructor chain from scratch each iteration,
	// including New and four Provide calls.
	for i := 0; i < b.N; i++ {
		c := newBenchContainer(b)
		_, _ = Inject[*benchService](c)
	}
}

func BenchmarkInjectScoped_Cached(b *testing.B) {
	c := newBenchContainer(b, AsScoped())
	scope := c.BeginScope()
	_, _ = InjectScoped[*benchService](scope)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = InjectScoped[*benchService](scope)
	}
}

func BenchmarkInjectGroup_10(b *testing.B) {
	c := New()
	for i := 0; i < 10; i++ {
		_ = Provide(c, func() benchHandler { return benchHandlerImpl{} },
			WithName(string(rune('a'+i))), AsGroup("handlers"))
	}
	_, _ = InjectGroup[benchHandler](c, "handlers")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = InjectGroup[benchHandler](c, "handlers")
	}
}

func BenchmarkResolve_WithMiddleware(b *testing.B) {
	c := New()
	c.(interface{ Use(Middleware) }).Use(&FuncMiddleware{})
	_ = c.Register("service", func(c Vessel) (any, error) { return "value", nil })
	_, _ = c.Resolve("service")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = c.Resolve("service")
	}
}
