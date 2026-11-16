# Vessel

[![Go Reference](https://pkg.go.dev/badge/github.com/xraph/vessel.svg)](https://pkg.go.dev/github.com/xraph/vessel)
[![Go Report Card](https://goreportcard.com/badge/github.com/xraph/vessel)](https://goreportcard.com/report/github.com/xraph/vessel)
[![codecov](https://codecov.io/gh/xraph/vessel/branch/main/graph/badge.svg)](https://app.codecov.io/gh/xraph/vessel)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Vessel is a dependency injection container for Go. You hand it constructors, it works out what each one needs from the parameter types, builds things in the right order and starts and stops them with your app. It's the container behind the [Forge](https://github.com/xraph/forge) framework, but it has no dependency on Forge and works fine on its own.

There are two ways to register services, and you can use both in the same container:

- **By type**, dig style. You call `Provide` with a constructor and resolve with `Inject[T]`. This is what you'll want most of the time.
- **By name**. You call `Register` with a string key and a factory, and resolve with `Resolve`. This is the older API, and it's the one that supports request scopes, lazy dependencies and middleware.

## Installation

```bash
go get github.com/xraph/vessel
```

You'll need Go 1.25 or later.

Registration options for the named API live in `github.com/xraph/go-utils/di`, so most code imports both packages.

## Quick start

```go
package main

import (
	"context"
	"log"

	"github.com/xraph/vessel"
)

type Config struct{ DSN string }

type Database struct{ dsn string }

func NewDatabase(cfg *Config) (*Database, error) {
	return &Database{dsn: cfg.DSN}, nil
}

func (d *Database) Start(ctx context.Context) error { return nil } // open the pool
func (d *Database) Stop(ctx context.Context) error  { return nil } // close it

type UserService struct{ db *Database }

func NewUserService(db *Database) *UserService {
	return &UserService{db: db}
}

func main() {
	c := vessel.New()
	ctx := context.Background()

	vessel.ProvideValue(c, &Config{DSN: "postgres://localhost/app"})
	vessel.Provide(c, NewDatabase)
	vessel.Provide(c, NewUserService)

	if err := c.Start(ctx); err != nil {
		log.Fatal(err)
	}
	defer c.Stop(ctx)

	users := vessel.MustInject[*UserService](c)
	_ = users
}
```

`Inject[*UserService]` sees that `NewUserService` needs a `*Database`, builds that first from `NewDatabase`, which in turn gets the `*Config` you provided as a value. Because `*Database` has a `Start` method, Vessel calls it the first time the database is built. `c.Stop` calls `Stop` on everything that was started.

Return values from `Provide` are errors, and you should check them in real code. They're dropped above to keep the example short.

## Constructor injection

A constructor is any function. Its parameters are its dependencies and its return values are what it provides. A trailing `error` return is treated as a construction failure.

```go
func NewDatabase(cfg *Config) (*Database, error)
func NewUserService(db *Database, log *Logger) *UserService
```

Services built this way are singletons by default and are only built when something first asks for them.

### Resolving

```go
db, err := vessel.Inject[*Database](c)
db := vessel.MustInject[*Database](c) // panics on error

ok := vessel.HasType[*Database](c)
```

### Names and aliases

When you have more than one of the same type, give each a name.

```go
vessel.ProvideNamed(c, "primary", NewPrimaryDB)
vessel.Provide(c, NewReplicaDB, vessel.WithName("replica")) // same thing

primary, err := vessel.InjectNamed[*Database](c, "primary")
ok := vessel.HasTypeNamed[*Database](c, "replica")
```

`WithAliases` keeps the type registration unnamed and adds extra names that point to the same instance:

```go
vessel.Provide(c, NewLogger, vessel.WithAliases("logger", "log"))

a := vessel.MustInject[*Logger](c)
b := vessel.MustInjectNamed[*Logger](c, "log") // a == b
```

### Values

If you've already built something, `ProvideValue` registers it as is.

```go
vessel.ProvideValue(c, &Config{Port: 8080})
vessel.ProvideValue(c, clock, vessel.WithName("utc"))
```

This is also the easiest way to swap in fakes in tests.

### Interfaces

Return an interface from the constructor and that's the type it's registered under:

```go
func NewHandler() Handler { return &usersHandler{} }

vessel.Provide(c, NewHandler)
h := vessel.MustInject[Handler](c)
```

You can also register a concrete constructor under extra interface types with `As`. Every type it's registered under resolves to the same instance, and `Provide` returns an error if the type doesn't implement one of the interfaces.

```go
vessel.Provide(c, NewFileStore, vessel.As(new(Reader), new(Writer)))

store := vessel.MustInject[*FileStore](c)
r := vessel.MustInject[Reader](c) // same *FileStore
```

### Lifecycle and eager construction

```go
vessel.Provide(c, NewRequestID, vessel.AsTransient()) // new instance on every Inject
vessel.Provide(c, NewSession, vessel.AsScoped())      // one per scope, see Scopes
vessel.Provide(c, NewDatabase, vessel.WithEager())    // build now, fail fast
```

With `WithEager`, `Provide` calls the constructor straight away and returns its error, so a bad DSN shows up at registration time and not on the first request.

### Groups

Put several providers in a group and pull them out together as a slice.

```go
vessel.Provide(c, NewUsersHandler, vessel.WithName("users"), vessel.AsGroup("routes"))
vessel.Provide(c, NewOrdersHandler, vessel.WithName("orders"), vessel.AsGroup("routes"))

handlers, err := vessel.InjectGroup[Handler](c, "routes")
```

Each member of a group still needs a unique type and name pair, which is why the handlers above are named.

### Parameter structs

Once a constructor has more than a handful of dependencies, embed `vessel.In` in a struct and take that struct as the only parameter. Each exported field is resolved, and struct tags control how.

```go
type ServerParams struct {
	vessel.In

	DB       *Database
	Cache    *Cache    `name:"redis"`     // InjectNamed[*Cache](c, "redis")
	Logger   *Logger   `optional:"true"`  // left nil if nothing provides it
	Handlers []Handler `group:"routes"`   // every member of the "routes" group
}

func NewServer(p ServerParams) *Server {
	return &Server{db: p.DB, handlers: p.Handlers}
}
```

A field with a `group` tag must be a slice.

### Result structs

`vessel.Out` does the reverse. Each exported field of the returned struct is registered as its own service, and the same `name` and `group` tags apply. The constructor runs once and every field comes from that one call. The same goes for a constructor with several plain return values, like `func() (*A, *B, error)`.

```go
type Repos struct {
	vessel.Out

	Users  *UserRepo
	Orders *OrderRepo `name:"orders"`
}

func NewRepos(db *Database) Repos {
	return Repos{Users: &UserRepo{db}, Orders: &OrderRepo{db}}
}

vessel.Provide(c, NewRepos)

users := vessel.MustInject[*UserRepo](c)
orders := vessel.MustInjectNamed[*OrderRepo](c, "orders")
```

## Named services

The named API takes a string key and a factory that receives the container.

```go
import "github.com/xraph/go-utils/di"

c.Register("config", func(c vessel.Vessel) (any, error) {
	return LoadConfig()
})

c.Register("database", func(c vessel.Vessel) (any, error) {
	cfg, err := c.Resolve("config")
	if err != nil {
		return nil, err
	}
	return NewDatabase(cfg.(*Config))
}, di.WithDependencies("config"))

svc, err := c.Resolve("database")
db := svc.(*Database)
```

`Resolve` returns `any`, so you type-assert the result yourself.

### Registration options

All of these come from the `di` package and can be combined.

| Option | What it does |
|---|---|
| `di.Singleton()` | One shared instance. This is the default. |
| `di.Transient()` | A new instance on every `Resolve`. |
| `di.Scoped()` | One instance per scope. See [Scopes](#scopes). |
| `di.WithDependencies("a", "b")` | Declares dependencies by name, used for start and stop ordering. |
| `di.WithDeps(...)` | Same, with a mode per dependency (see below). |
| `di.WithGroup("storage")` | Adds the service to a group for querying. |
| `di.WithDIMetadata("tier", "primary")` | Attaches a key/value pair you can query on. |

`WithDeps` takes `di.Eager(name)`, `di.Lazy(name)`, `di.Optional(name)` and `di.LazyOptional(name)`. There are also typed variants such as `di.EagerTyped[*Database]("database")`.

Declaring dependencies doesn't inject anything. Your factory still resolves what it needs. What the declarations give you is ordering: `c.Start` starts singletons in dependency order, and a cycle in the declared dependencies makes `Start` fail with a circular dependency error. Lazy dependencies don't count towards that ordering, so `di.Lazy` is how you declare a cycle on purpose.

Cycles in what your factories actually resolve are caught too. If `a` resolves `b` and `b` resolves `a`, the `Resolve` call returns an error naming the path, `a -> b -> a`. For this to work, resolve through the container your factory is given, not one captured from outside.

### Other container methods

```go
c.Has("database")                        // is it registered?
c.IsStarted("database")                  // has it been started?
c.Services()                             // every registered name
svc, err := c.ResolveReady(ctx, "database") // resolve, starting it first if needed
```

## Scopes

A scope gives you one instance of each scoped service for as long as the scope lives. The usual case is one scope per HTTP request.

```go
c.Register("request", func(c vessel.Vessel) (any, error) {
	return &RequestContext{}, nil
}, di.Scoped())

func handle(w http.ResponseWriter, r *http.Request) {
	scope := c.BeginScope()
	defer scope.End()

	rc, err := scope.Resolve("request") // same instance for the rest of this request
	// ...
}
```

Singletons resolved through a scope come from the parent container. Transients are built fresh each time. Resolving a scoped service straight from the container returns an error.

The container passed to a scoped or transient factory is bound to the scope, so a scoped service can depend on another scoped service in the same request:

```go
c.Register("tx", func(c vessel.Vessel) (any, error) {
	req, err := c.Resolve("request") // the same request instance as the handler gets
	if err != nil {
		return nil, err
	}
	return BeginTx(req.(*RequestContext))
}, di.Scoped())
```

When you call `End`, every scoped instance that implements `di.Disposable` (a `Dispose() error` method) gets disposed, newest first. If any of them fail, `End` returns all the errors joined, and `errors.Is` works on each. Anything you resolve from the scope after that returns `vessel.ErrScopeEnded`.

Scopes can also hold plain values for the request. Those methods aren't on the `di.Scope` interface, so you reach them with a type assertion:

```go
store := scope.(interface {
	Set(key string, value any)
	Get(key string) (any, bool)
})

store.Set("requestID", "abc-123")
id, ok := store.Get("requestID")
```

### Scoped constructors

Constructors registered with `vessel.AsScoped()` work the same way. Resolve them with `InjectScoped`:

```go
vessel.Provide(c, NewSession, vessel.AsScoped())
vessel.Provide(c, NewCart, vessel.AsScoped()) // func NewCart(s *Session) *Cart

scope := c.BeginScope()
defer scope.End()

cart := vessel.MustInjectScoped[*Cart](scope) // and its *Session, built for this scope
```

A singleton can't depend on a scoped service, since it would hang on to one request's instance forever. Trying it returns an error when the singleton is built.

## Lifecycle

Vessel looks for these interfaces from `go-utils/di` on every service it builds:

| Interface | Method | When it's called |
|---|---|---|
| `di.Starter` | `Start(ctx) error` | The first time the service is built, or by `c.Start` |
| `di.Stopper` | `Stop(ctx) error` | By `c.Stop`, in reverse dependency order |
| `di.HealthChecker` | `Health(ctx) error` | By `c.Health` and `c.Inspect` |
| `di.Disposable` | `Dispose() error` | By `scope.End` for scoped services |

You don't have to call `c.Start` for things to start. A singleton that implements `Starter` is started the first time something resolves it, and each transient instance is started when it's built. What `c.Start` adds is that it builds and starts every named singleton up front, in dependency order, along with any constructor-registered singleton that has already been built. If one fails, it stops everything it already started before returning the error. Calling `Start` twice is safe.

`c.Stop` stops every started singleton in the reverse of the order it actually started, whether that was through `Start` or a first `Resolve` or `Inject`. A service always finishes starting after the things it resolved, so this is always a safe order. If a service fails to stop, the rest still get stopped and you get all the errors back joined together. You can call `Start` again afterwards.

Transient instances are started but never stopped. You own those once you have them.

`c.Health(ctx)` runs `Health` on every singleton that has been built and returns the first error. Services that were never built are skipped.

## Lazy dependencies

These wrap a named service so it's resolved on first use and not when the wrapper is created.

```go
mailer := vessel.NewLazy[*Mailer](c, "mailer")
m, err := mailer.Get()     // resolves once, then caches
m = mailer.MustGet()       // panics on error
mailer.IsResolved()

metrics := vessel.NewOptionalLazy[*Metrics](c, "metrics")
mt, err := metrics.Get()   // zero value and nil error if "metrics" isn't registered
metrics.IsFound()

tokens := vessel.NewProvider[*Token](c, "token")
t1 := tokens.MustProvide() // calls Resolve every time, so a transient gives you a new one each call
```

`Lazy` caches the error as well as the value. If the first `Get` fails, every later `Get` returns the same error.

## Middleware

Middleware sees every `Resolve` on the container and every automatic start. Any hook can return an error to abort the operation.

```go
mw := &vessel.FuncMiddleware{
	BeforeResolveFunc: func(ctx context.Context, name string) error {
		return nil
	},
	AfterResolveFunc: func(ctx context.Context, name string, svc any, err error) error {
		log.Printf("resolved %s (err=%v)", name, err)
		return nil
	},
	AfterStartFunc: func(ctx context.Context, name string, err error) error {
		metrics.Inc("service_started", name)
		return nil
	},
}

c.(interface{ Use(vessel.Middleware) }).Use(mw)
```

`Use` isn't part of the `Vessel` interface, hence the type assertion. You can also implement the four-method `vessel.Middleware` interface on your own type. Middleware runs in the order you add it. It covers every resolve of a named service, including through a scope, but not `Inject`.

## Inspecting and querying

```go
info := c.Inspect("database")
// info.Lifecycle, info.Type, info.Dependencies, info.Started, info.Healthy, info.Metadata

vessel.Query(c, vessel.ServiceQuery{
	Lifecycle: "singleton",
	Group:     "storage",
	Metadata:  map[string]string{"tier": "primary"},
})

vessel.QueryNames(c, vessel.ServiceQuery{Group: "storage"})
vessel.FindByGroup(c, "storage")
vessel.FindByLifecycle(c, "transient")
vessel.FindStarted(c)
vessel.FindNotStarted(c)
```

Mind you, `Inspect` calls `Health` on the service to fill in `Healthy`, and the query helpers call `Inspect` on every service they look at. Keep that in mind if your health checks talk to the network.

Constructor-registered services show up in `Services()` and in queries only if they implement `di.HealthChecker`.

## Errors

Errors from the named API carry a code, and you can match them with `errors.Is` against these sentinels:

```go
_, err := c.Resolve("missing")
errors.Is(err, vessel.ErrServiceNotFoundSentinel) // true
```

| Sentinel | Code |
|---|---|
| `ErrServiceNotFoundSentinel` | `SERVICE_NOT_FOUND` |
| `ErrCircularDependencySentinel` | `CIRCULAR_DEPENDENCY` |
| `ErrInvalidFactory` | `INVALID_FACTORY` |
| `ErrScopeEnded` | `SCOPE_ENDED` |
| `ErrTypeMismatchSentinel` | `TYPE_MISMATCH` |

Registering the same name twice returns an error with the code `SERVICE_ALREADY_EXISTS`. Failures inside a factory or a `Start` call come back wrapped with the code `SERVICE_ERROR`, and the original error is still reachable with `errors.Is` and `errors.As`.

`Inject` and constructor resolution use the same codes for a missing provider and for a cycle, so `errors.Is(err, vessel.ErrServiceNotFoundSentinel)` works there too.

## Testing

Build a fresh container per test and provide fakes as values:

```go
func TestCheckout(t *testing.T) {
	c := vessel.New()
	vessel.ProvideValue[PaymentGateway](c, &fakeGateway{})
	vessel.Provide(c, NewCheckout)

	checkout := vessel.MustInject[*Checkout](c)
	// ...
}
```

Note the explicit type parameter on `ProvideValue`. Without it, the fake is registered under `*fakeGateway` and `NewCheckout` won't find a `PaymentGateway`.

## Development

```bash
make test        # run the tests
make test-race   # with the race detector
make bench       # benchmarks
make lint        # golangci-lint
make verify      # fmt check, vet, tidy check and lint
```

`make help` lists everything else.

## License

MIT. See [LICENSE](LICENSE).
