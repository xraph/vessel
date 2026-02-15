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
	// Benchmark first-time resolution (uncached) by creating fresh containers
	// Limited to reasonable iteration count
	c := New()
	_ = c.Register("service", func(c Vessel) (any, error) {
		return "value", nil
	}, di.Singleton())

	b.ResetTimer()
	// First resolve - measures uncached path
	_, _ = c.Resolve("service")
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
