package vessel

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xraph/go-utils/di"
)

// Regression tests for lifecycle, concurrency and scoping bugs.

type regIface interface{ ID() int }

type regImpl struct{ id int }

func (r *regImpl) ID() int { return r.id }

type regStarter struct {
	name    string
	log     *[]string
	mu      *sync.Mutex
	started int
	stopErr error
}

func (s *regStarter) Start(context.Context) error {
	s.started++
	s.record("start " + s.name)
	return nil
}

func (s *regStarter) Stop(context.Context) error {
	s.record("stop " + s.name)
	return s.stopErr
}

func (s *regStarter) record(msg string) {
	if s.log == nil {
		return
	}
	s.mu.Lock()
	*s.log = append(*s.log, msg)
	s.mu.Unlock()
}

type regDisposable struct {
	name string
	log  *[]string
}

func (d *regDisposable) Dispose() error {
	*d.log = append(*d.log, d.name)
	return nil
}

// withTimeout fails the test instead of hanging if fn deadlocks.
func withTimeout(t *testing.T, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { defer close(done); fn() }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("deadlock: call did not return")
	}
}

func TestStart_WithScopedAndTransientServices(t *testing.T) {
	c := New()
	var transientBuilds atomic.Int32

	require.NoError(t, c.Register("request", func(Vessel) (any, error) { return &regImpl{}, nil }, di.Scoped()))
	require.NoError(t, c.Register("token", func(Vessel) (any, error) {
		transientBuilds.Add(1)
		return &regImpl{}, nil
	}, di.Transient()))

	require.NoError(t, c.Start(context.Background()))
	assert.Zero(t, transientBuilds.Load(), "Start must not build transient services")
}

func TestInject_ConcurrentFirstResolution(t *testing.T) {
	c := New()
	var builds atomic.Int32

	require.NoError(t, Provide(c, func() *regImpl {
		builds.Add(1)
		time.Sleep(20 * time.Millisecond)
		return &regImpl{}
	}))

	var wg sync.WaitGroup
	results := make([]*regImpl, 16)
	errs := make([]error, 16)

	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], errs[i] = Inject[*regImpl](c)
		}()
	}
	wg.Wait()

	for i := range results {
		require.NoError(t, errs[i])
		assert.Same(t, results[0], results[i])
	}
	assert.Equal(t, int32(1), builds.Load())
}

func TestResolve_ConcurrentFirstResolution(t *testing.T) {
	c := New()
	var builds atomic.Int32

	require.NoError(t, c.Register("svc", func(Vessel) (any, error) {
		builds.Add(1)
		time.Sleep(20 * time.Millisecond)
		return &regImpl{}, nil
	}))

	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := c.Resolve("svc")
			assert.NoError(t, err)
		}()
	}
	wg.Wait()

	assert.Equal(t, int32(1), builds.Load())
}

func TestResolve_NamedCycleReturnsError(t *testing.T) {
	c := New()
	require.NoError(t, c.Register("a", func(c Vessel) (any, error) { return c.Resolve("b") }))
	require.NoError(t, c.Register("b", func(c Vessel) (any, error) { return c.Resolve("c") }))
	require.NoError(t, c.Register("c", func(c Vessel) (any, error) { return c.Resolve("a") }))

	withTimeout(t, func() {
		_, err := c.Resolve("a")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrCircularDependencySentinel)
		assert.Contains(t, err.Error(), "a -> b -> c -> a")
	})
}

func TestResolve_SelfCycleReturnsError(t *testing.T) {
	c := New()
	require.NoError(t, c.Register("a", func(c Vessel) (any, error) { return c.Resolve("a") }))

	withTimeout(t, func() {
		_, err := c.Resolve("a")
		assert.ErrorIs(t, err, ErrCircularDependencySentinel)
	})
}

func TestResolve_RetainedContainerHasNoStalePath(t *testing.T) {
	c := New()

	// A factory that keeps its container and uses it later must not see a
	// false cycle from the path that was active while it was being built.
	var kept Vessel
	require.NoError(t, c.Register("server", func(c Vessel) (any, error) {
		kept = c
		return &regImpl{id: 1}, nil
	}))

	_, err := c.Resolve("server")
	require.NoError(t, err)

	svc, err := kept.Resolve("server")
	require.NoError(t, err)
	assert.Equal(t, 1, svc.(*regImpl).ID())
}

func TestLazy_SelfReferenceInFactory(t *testing.T) {
	c := New()

	type node struct{ self *Lazy[*node] }

	require.NoError(t, c.Register("node", func(c Vessel) (any, error) {
		return &node{self: NewLazy[*node](c, "node")}, nil
	}))

	n, err := c.Resolve("node")
	require.NoError(t, err)

	self, err := n.(*node).self.Get()
	require.NoError(t, err)
	assert.Same(t, n, self)
}

func TestProvide_AsSharesInstance(t *testing.T) {
	c := New()
	var builds int

	require.NoError(t, Provide(c, func() *regImpl { builds++; return &regImpl{id: 7} }, As(new(regIface))))

	concrete := MustInject[*regImpl](c)
	iface := MustInject[regIface](c)

	assert.Same(t, concrete, iface.(*regImpl))
	assert.Equal(t, 1, builds)
}

func TestProvide_AsRejectsUnimplementedInterface(t *testing.T) {
	c := New()
	err := Provide(c, func() *regImpl { return &regImpl{} }, As(new(di.Starter)))
	require.Error(t, err)
	assert.False(t, HasType[*regImpl](c), "nothing should be registered on failure")
}

func TestProvideValue_AsStartsAndStopsOnce(t *testing.T) {
	c := New()
	svc := &regStarter{name: "v"}

	require.NoError(t, ProvideValue(c, svc, As(new(di.Starter)), WithAliases("alias")))

	_ = MustInject[*regStarter](c)
	_ = MustInject[di.Starter](c)

	ctx := context.Background()
	require.NoError(t, c.Start(ctx))
	require.NoError(t, c.Stop(ctx))

	assert.Equal(t, 1, svc.started)
}

type regOut struct {
	Out
	A *regImpl
	B *regStarter
}

func TestProvide_OutConstructorCalledOnce(t *testing.T) {
	c := New()
	var calls int

	require.NoError(t, Provide(c, func() regOut {
		calls++
		return regOut{A: &regImpl{id: calls}, B: &regStarter{name: "b"}}
	}))

	_ = MustInject[*regImpl](c)
	_ = MustInject[*regStarter](c)
	_ = MustInject[*regImpl](c)

	assert.Equal(t, 1, calls)
}

type regSingleOut struct {
	Out
	Only *regImpl
}

func TestProvide_SingleFieldOutStruct(t *testing.T) {
	c := New()
	require.NoError(t, Provide(c, func() regSingleOut { return regSingleOut{Only: &regImpl{id: 3}} }))

	got, err := Inject[*regImpl](c)
	require.NoError(t, err)
	assert.Equal(t, 3, got.ID())
}

func TestProvide_MultipleReturnValues(t *testing.T) {
	c := New()
	var calls int

	require.NoError(t, Provide(c, func() (*regImpl, *regStarter, error) {
		calls++
		return &regImpl{id: 9}, &regStarter{name: "second"}, nil
	}))

	assert.Equal(t, 9, MustInject[*regImpl](c).ID())
	assert.Equal(t, "second", MustInject[*regStarter](c).name)
	assert.Equal(t, 1, calls)
}

func TestProvide_OutCycleThroughSibling(t *testing.T) {
	c := New()

	// The Out constructor needs *regStarter, whose constructor needs a
	// field of the same Out struct.
	type out struct {
		Out
		A *regImpl
	}
	require.NoError(t, Provide(c, func(_ *regStarter) out { return out{A: &regImpl{}} }))
	require.NoError(t, Provide(c, func(_ *regImpl) *regStarter { return &regStarter{} }))

	withTimeout(t, func() {
		_, err := Inject[*regImpl](c)
		assert.ErrorIs(t, err, ErrCircularDependencySentinel)
	})
}

func TestProvide_RegistrationIsAtomic(t *testing.T) {
	c := New()
	require.NoError(t, ProvideValue(c, &regStarter{}))

	err := Provide(c, func() (*regImpl, *regStarter) { return &regImpl{}, &regStarter{} })
	require.Error(t, err)
	assert.False(t, HasType[*regImpl](c), "first result must not stay registered after a conflict")
}

func TestInjectGroup_NoDuplicatesFromAliasesOrAs(t *testing.T) {
	c := New()
	require.NoError(t, Provide(c, func() *regImpl { return &regImpl{} },
		AsGroup("g"), WithAliases("x", "y"), As(new(regIface))))

	group, err := InjectGroup[regIface](c, "g")
	require.NoError(t, err)
	assert.Len(t, group, 1)
}

func TestInject_TransientStartsEveryInstance(t *testing.T) {
	c := New()
	require.NoError(t, Provide(c, func() *regStarter { return &regStarter{} }, AsTransient()))

	a := MustInject[*regStarter](c)
	b := MustInject[*regStarter](c)

	assert.NotSame(t, a, b)
	assert.Equal(t, 1, a.started)
	assert.Equal(t, 1, b.started)
}

func TestProvide_AsScoped(t *testing.T) {
	c := New()
	var log []string

	require.NoError(t, Provide(c, func() *regDisposable { return &regDisposable{name: "session", log: &log} }, AsScoped()))

	_, err := Inject[*regDisposable](c)
	require.Error(t, err, "scoped services can't be resolved from the container")

	s1 := c.BeginScope()
	a1 := MustInjectScoped[*regDisposable](s1)
	a2 := MustInjectScoped[*regDisposable](s1)
	assert.Same(t, a1, a2)

	s2 := c.BeginScope()
	b := MustInjectScoped[*regDisposable](s2)
	assert.NotSame(t, a1, b)

	require.NoError(t, s1.End())
	assert.Equal(t, []string{"session"}, log)

	_, err = InjectScoped[*regDisposable](s1)
	assert.ErrorIs(t, err, ErrScopeEnded)
}

func TestProvide_ScopedDependsOnScoped(t *testing.T) {
	c := New()

	type session struct{ id *regImpl }
	require.NoError(t, Provide(c, func() *regImpl { return &regImpl{} }, AsScoped()))
	require.NoError(t, Provide(c, func(r *regImpl) *session { return &session{id: r} }, AsScoped()))

	s := c.BeginScope()
	sess := MustInjectScoped[*session](s)
	assert.Same(t, MustInjectScoped[*regImpl](s), sess.id)
}

func TestProvide_SingletonCannotCaptureScoped(t *testing.T) {
	c := New()

	type app struct{}
	require.NoError(t, Provide(c, func() *regImpl { return &regImpl{} }, AsScoped()))
	require.NoError(t, Provide(c, func(*regImpl) *app { return &app{} }))

	_, err := InjectScoped[*app](c.BeginScope())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be resolved from a scope")
}

func TestScope_NamedScopedDependsOnScoped(t *testing.T) {
	c := New()
	require.NoError(t, c.Register("s1", func(Vessel) (any, error) { return &regImpl{id: 1}, nil }, di.Scoped()))
	require.NoError(t, c.Register("s2", func(c Vessel) (any, error) { return c.Resolve("s1") }, di.Scoped()))
	require.NoError(t, c.Register("t", func(c Vessel) (any, error) { return c.Resolve("s1") }, di.Transient()))

	s := c.BeginScope()
	s1, err := s.Resolve("s1")
	require.NoError(t, err)

	withTimeout(t, func() {
		s2, err := s.Resolve("s2")
		require.NoError(t, err)
		assert.Same(t, s1, s2)

		tr, err := s.Resolve("t")
		require.NoError(t, err)
		assert.Same(t, s1, tr)
	})
}

func TestScope_ScopedCycleReturnsError(t *testing.T) {
	c := New()
	require.NoError(t, c.Register("a", func(c Vessel) (any, error) { return c.Resolve("b") }, di.Scoped()))
	require.NoError(t, c.Register("b", func(c Vessel) (any, error) { return c.Resolve("a") }, di.Scoped()))

	withTimeout(t, func() {
		_, err := c.BeginScope().Resolve("a")
		assert.ErrorIs(t, err, ErrCircularDependencySentinel)
	})
}

func TestScope_EndDisposesInReverseCreationOrder(t *testing.T) {
	c := New()
	var log []string

	for _, name := range []string{"first", "second", "third"} {
		require.NoError(t, c.Register(name, func(Vessel) (any, error) {
			return &regDisposable{name: name, log: &log}, nil
		}, di.Scoped()))
	}

	s := c.BeginScope()
	for _, name := range []string{"first", "second", "third"} {
		_, err := s.Resolve(name)
		require.NoError(t, err)
	}

	require.NoError(t, s.End())
	assert.Equal(t, []string{"third", "second", "first"}, log)
}

func TestScope_EndErrorsMatchWithErrorsIs(t *testing.T) {
	c := New()
	boom := errors.New("boom")

	require.NoError(t, c.Register("x", func(Vessel) (any, error) {
		return &errorDisposable{name: "x", err: boom}, nil
	}, di.Scoped()))

	s := c.BeginScope()
	_, err := s.Resolve("x")
	require.NoError(t, err)

	assert.ErrorIs(t, s.End(), boom)
}

func TestStart_LazyDependencyBreaksCycle(t *testing.T) {
	c := New()
	require.NoError(t, c.Register("a", func(Vessel) (any, error) { return 1, nil }, di.WithDeps(di.Lazy("b"))))
	require.NoError(t, c.Register("b", func(Vessel) (any, error) { return 2, nil }, di.WithDeps(di.Eager("a"))))

	assert.NoError(t, c.Start(context.Background()))
}

func TestHealth_ConcurrentWithResolve(t *testing.T) {
	c := New()
	for _, name := range []string{"a", "b", "c"} {
		require.NoError(t, c.Register(name, func(Vessel) (any, error) {
			return &mockService{name: name, healthy: true}, nil
		}))
	}

	var wg sync.WaitGroup
	for _, name := range []string{"a", "b", "c"} {
		wg.Add(2)
		go func() { defer wg.Done(); _, _ = c.Resolve(name) }()
		go func() { defer wg.Done(); _ = c.Health(context.Background()); _ = c.Inspect(name) }()
	}
	wg.Wait()
}

func TestHealth_CheckMayUseContainer(t *testing.T) {
	c := New()
	require.NoError(t, c.Register("dep", func(Vessel) (any, error) { return 1, nil }))
	require.NoError(t, c.Register("svc", func(c Vessel) (any, error) {
		return &containerHealth{c: c}, nil
	}))

	_, err := c.Resolve("svc")
	require.NoError(t, err)

	withTimeout(t, func() {
		assert.NoError(t, c.Health(context.Background()))
		assert.True(t, c.Inspect("svc").Healthy)
	})
}

type containerHealth struct{ c Vessel }

func (h *containerHealth) Health(context.Context) error {
	_, err := h.c.Resolve("dep")
	return err
}

func TestProvide_NilConstructor(t *testing.T) {
	c := New()

	assert.Error(t, Provide(c, nil))

	var fn func() *regImpl
	assert.Error(t, Provide(c, fn))
}

type regErr struct{}

func (*regErr) Error() string { return "custom" }

func TestProvide_ConcreteErrorTypeIsAResult(t *testing.T) {
	c := New()
	require.NoError(t, Provide(c, func() *regErr { return &regErr{} }))
	assert.True(t, HasType[*regErr](c))
}

func TestStop_ReverseStartOrderAcrossRegistries(t *testing.T) {
	c := New()
	var (
		log []string
		mu  sync.Mutex
	)

	type db struct{ *regStarter }
	type server struct{ *regStarter }

	require.NoError(t, Provide(c, func() *db { return &db{&regStarter{name: "db", log: &log, mu: &mu}} }))
	require.NoError(t, Provide(c, func(*db) *server { return &server{&regStarter{name: "server", log: &log, mu: &mu}} }))
	require.NoError(t, c.Register("worker", func(Vessel) (any, error) {
		return &regStarter{name: "worker", log: &log, mu: &mu}, nil
	}))

	ctx := context.Background()
	require.NoError(t, c.Start(ctx))
	_ = MustInject[*server](c)

	require.NoError(t, c.Stop(ctx))
	assert.Equal(t, []string{
		"start worker", "start db", "start server",
		"stop server", "stop db", "stop worker",
	}, log)
}

func TestStop_ContinuesAfterErrorAndJoins(t *testing.T) {
	c := New()
	errA := errors.New("a failed")
	errB := errors.New("b failed")
	var (
		log []string
		mu  sync.Mutex
	)

	for name, err := range map[string]error{"a": errA, "b": errB, "ok": nil} {
		require.NoError(t, c.Register(name, func(Vessel) (any, error) {
			return &regStarter{name: name, log: &log, mu: &mu, stopErr: err}, nil
		}))
	}

	ctx := context.Background()
	require.NoError(t, c.Start(ctx))

	err := c.Stop(ctx)
	assert.ErrorIs(t, err, errA)
	assert.ErrorIs(t, err, errB)

	mu.Lock()
	defer mu.Unlock()
	assert.Contains(t, log, "stop a")
	assert.Contains(t, log, "stop b")
	assert.Contains(t, log, "stop ok")
}

func TestStop_AutoStartedWithoutStart(t *testing.T) {
	c := New()
	svc := &mockService{name: "svc"}
	require.NoError(t, c.Register("svc", func(Vessel) (any, error) { return svc, nil }))

	_, err := c.Resolve("svc")
	require.NoError(t, err)

	require.NoError(t, c.Stop(context.Background()))
	assert.True(t, svc.stopped)
}

func TestStop_ThenStartAgain(t *testing.T) {
	c := New()
	svc := &regStarter{name: "svc"}
	require.NoError(t, c.Register("svc", func(Vessel) (any, error) { return svc, nil }))
	require.NoError(t, c.Register("plain", func(Vessel) (any, error) { return 1, nil }))

	ctx := context.Background()
	require.NoError(t, c.Start(ctx))
	require.NoError(t, c.Stop(ctx))

	assert.False(t, c.IsStarted("svc"))
	assert.False(t, c.IsStarted("plain"), "services without Stop are marked stopped too")

	require.NoError(t, c.Start(ctx))
	assert.Equal(t, 2, svc.started)
	assert.True(t, c.IsStarted("svc"))
}

func TestStart_FailureRollsBackTypedServices(t *testing.T) {
	c := New()
	var (
		log []string
		mu  sync.Mutex
	)
	typed := &regStarter{name: "typed", log: &log, mu: &mu}
	// A constructor, not ProvideValue: a resolve starts what the container
	// built, and that start is what the failed Start has to roll back.
	require.NoError(t, Provide(c, func() (*regStarter, error) { return typed, nil }))
	_ = MustInject[*regStarter](c)

	require.NoError(t, c.Register("broken", func(Vessel) (any, error) {
		return &mockService{name: "broken", startErr: errors.New("nope")}, nil
	}))

	require.Error(t, c.Start(context.Background()))
	assert.Equal(t, []string{"start typed", "stop typed"}, log)
	assert.Empty(t, c.(*containerImpl).startOrder, "everything started must be rolled back")
}

type ctxKey struct{}

type ctxStarter struct{ got any }

func (s *ctxStarter) Start(ctx context.Context) error {
	s.got = ctx.Value(ctxKey{})
	return nil
}

func TestResolveReady_PassesContext(t *testing.T) {
	c := New()
	svc := &ctxStarter{}
	require.NoError(t, c.Register("svc", func(Vessel) (any, error) { return svc, nil }))

	ctx := context.WithValue(context.Background(), ctxKey{}, "value")
	_, err := c.ResolveReady(ctx, "svc")
	require.NoError(t, err)
	assert.Equal(t, "value", svc.got)
}

func TestStart_PassesContext(t *testing.T) {
	c := New()
	svc := &ctxStarter{}
	require.NoError(t, c.Register("svc", func(Vessel) (any, error) { return svc, nil }))

	ctx := context.WithValue(context.Background(), ctxKey{}, "start")
	require.NoError(t, c.Start(ctx))
	assert.Equal(t, "start", svc.got)
}

func TestServices_Sorted(t *testing.T) {
	c := New()
	for _, name := range []string{"zeta", "alpha", "mid"} {
		require.NoError(t, c.Register(name, func(Vessel) (any, error) { return 1, nil }))
	}

	assert.Equal(t, []string{"alpha", "mid", "zeta"}, c.Services())
}

func TestInject_InsideNamedFactory(t *testing.T) {
	c := New()
	require.NoError(t, ProvideValue(c, &regImpl{id: 5}))
	require.NoError(t, c.Register("svc", func(c Vessel) (any, error) {
		return Inject[*regImpl](c)
	}))

	svc, err := c.Resolve("svc")
	require.NoError(t, err)
	assert.Equal(t, 5, svc.(*regImpl).ID())
}

func TestMiddleware_UseConcurrentWithResolve(t *testing.T) {
	c := New()
	require.NoError(t, c.Register("svc", func(Vessel) (any, error) { return 1, nil }))
	user := c.(interface{ Use(Middleware) })

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(2)
		go func() { defer wg.Done(); user.Use(&FuncMiddleware{}) }()
		go func() { defer wg.Done(); _, _ = c.Resolve("svc") }()
	}
	wg.Wait()
}

func TestTypedErrors_CarryNotFoundCode(t *testing.T) {
	c := New()
	_, err := Inject[*regImpl](c)
	assert.ErrorIs(t, err, ErrServiceNotFoundSentinel)
}

// countingStarter counts its Start and Stop calls.
type countingStarter struct{ starts, stops atomic.Int32 }

func (s *countingStarter) Start(context.Context) error { s.starts.Add(1); return nil }
func (s *countingStarter) Stop(context.Context) error  { s.stops.Add(1); return nil }

// A value handed to ProvideValue was built by its caller, who decides when it
// starts. Resolving it must not start it: an app that resolves it while
// extensions register would run its Start before the database is migrated.
// Container.Start still starts it once, and Stop stops it.
func TestProvideValue_ResolveDoesNotStart(t *testing.T) {
	c := New()
	svc := &countingStarter{}
	require.NoError(t, ProvideValue(c, svc))

	got, err := Inject[*countingStarter](c)
	require.NoError(t, err)
	assert.Same(t, svc, got)
	_, err = Inject[*countingStarter](c)
	require.NoError(t, err)
	assert.Zero(t, svc.starts.Load(), "resolving a provided value must not start it")

	require.NoError(t, c.Start(context.Background()))
	assert.EqualValues(t, 1, svc.starts.Load(), "Container.Start starts the value once")

	_, err = Inject[*countingStarter](c)
	require.NoError(t, err)
	assert.EqualValues(t, 1, svc.starts.Load())

	require.NoError(t, c.Stop(context.Background()))
	assert.EqualValues(t, 1, svc.stops.Load())
}

// A singleton the container builds is still started on its first resolve.
func TestProvide_ResolveStillStartsBuiltSingleton(t *testing.T) {
	c := New()
	svc := &countingStarter{}
	require.NoError(t, Provide(c, func() (*countingStarter, error) { return svc, nil }))

	_, err := Inject[*countingStarter](c)
	require.NoError(t, err)
	assert.EqualValues(t, 1, svc.starts.Load())

	require.NoError(t, c.Start(context.Background()))
	assert.EqualValues(t, 1, svc.starts.Load(), "Start must not start it again")
}
