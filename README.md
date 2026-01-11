# Vessel 🚢

[![Go Reference](https://pkg.go.dev/badge/github.com/xraph/vessel.svg)](https://pkg.go.dev/github.com/xraph/vessel)
[![Go Report Card](https://goreportcard.com/badge/github.com/xraph/vessel)](https://goreportcard.com/report/github.com/xraph/vessel)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

**Vessel** is a powerful, type-safe dependency injection container for Go, built as part of the Forge framework. It provides elegant service lifecycle management, flexible dependency resolution, and comprehensive testing support.

## 🆕 What's New

- **🔑 Typed Service Keys** - Strongly-typed service keys with `ServiceKey[T]` for compile-time safety and IDE autocomplete
- **🪝 Middleware System** - Hook into service resolution and lifecycle events for logging, metrics, and validation
- **📚 Batch Registration** - Register multiple services efficiently with `RegisterServices()` and typed variants
- **🔎 Service Discovery** - Query and filter services with `Query()`, `FindByGroup()`, and `FindByLifecycle()`
- **📦 Enhanced Scopes** - Scope context storage with `SetScoped()/GetScoped()` for request-specific data
- **🚨 Sentinel Errors** - Proper error handling with exported sentinel errors for `errors.Is()` checking

## ✨ Features

- 🎯 **Type-Safe Generics** - Compile-time type safety with Go generics
- � **Typed Service Keys** - Strongly-typed service keys for compile-time safety
- 🔄 **Multiple Lifecycles** - Singleton, Transient, and Scoped services
- ⚡ **Lazy Dependencies** - Defer expensive service initialization
- 🔗 **Typed Injection** - Automatic dependency resolution with type checking
- 🚀 **Service Lifecycle** - Built-in Start/Stop/Health management
- 🔍 **Circular Detection** - Automatic circular dependency detection
- 🧵 **Concurrency Safe** - Thread-safe container operations
- 📦 **Request Scoping** - Perfect for HTTP request-scoped services with context storage
- 🎭 **Interface Binding** - Register implementations as interfaces
- 🪝 **Middleware Hooks** - Intercept resolve, start, and lifecycle events
- 🔎 **Service Discovery** - Query and filter services by criteria
- 📚 **Batch Registration** - Register multiple services efficiently
- 🧪 **Test Friendly** - Easy mocking and testing utilities

## 📦 Installation

```bash
go get github.com/xraph/vessel
```

## 🚀 Quick Start

```go
package main

import (
    "context"
    "fmt"
    "github.com/xraph/vessel"
)

type Database struct {
    connectionString string
}

func (d *Database) Name() string { return "database" }
func (d *Database) Start(ctx context.Context) error {
    fmt.Println("Connecting to database...")
    return nil
}
func (d *Database) Stop(ctx context.Context) error {
    fmt.Println("Closing database connection...")
    return nil
}

type UserService struct {
    db *Database
}

func main() {
    // Create a new container
    c := vessel.New()
    
    // Register services
    vessel.RegisterSingleton(c, "database", func(c vessel.Vessel) (*Database, error) {
        return &Database{connectionString: "postgres://..."}, nil
    })
    
    vessel.RegisterSingleton(c, "userService", func(c vessel.Vessel) (*UserService, error) {
        db := vessel.Must[*Database](c, "database")
        return &UserService{db: db}, nil
    })
    
    // Start all services
    ctx := context.Background()
    if err := c.Start(ctx); err != nil {
        panic(err)
    }
    defer c.Stop(ctx)
    
    // Resolve and use services
    userService := vessel.Must[*UserService](c, "userService")
    fmt.Printf("User service ready: %v\n", userService)
}
```

## 📖 Core Concepts

### Service Lifecycles

Vessel supports three lifecycle types:

#### 🔹 Singleton (Default)
Created once and shared across the entire application.

```go
vessel.RegisterSingleton(c, "config", func(c vessel.Vessel) (*Config, error) {
    return LoadConfig(), nil
})
```

#### 🔹 Transient
Created new every time it's resolved.

```go
vessel.RegisterTransient(c, "request", func(c vessel.Vessel) (*Request, error) {
    return &Request{ID: uuid.New()}, nil
})
```

#### 🔹 Scoped
Created once per scope, perfect for HTTP requests.

```go
vessel.RegisterScoped(c, "session", func(c vessel.Vessel) (*Session, error) {
    return &Session{StartTime: time.Now()}, nil
})

// In HTTP handler
scope := c.BeginScope()
defer scope.End()

session, _ := vessel.ResolveScope[*Session](scope, "session")
```

### 🔹 Enhanced Scope Features

Scopes now support context storage for request-specific data:

```go
scope := c.BeginScope()
defer scope.End()

// Store request-specific context
vessel.SetScoped(scope, "requestID", "abc-123")
vessel.SetScoped(scope, "user", currentUser)

// Retrieve typed values
requestID, ok := vessel.GetScoped[string](scope, "requestID")
user, ok := vessel.GetScoped[*User](scope, "user")

// Check scope status
if !scope.(*vessel.Scope).IsEnded() {
    // Scope is still active
}

// List services resolved in this scope
services := scope.(*vessel.Scope).Services()
```

## 🔑 Typed Service Keys

Use strongly-typed service keys for compile-time safety and IDE autocomplete:

```go
// Define typed service keys
var (
    DatabaseKey    = vessel.NewServiceKey[*Database]("database")
    UserServiceKey = vessel.NewServiceKey[*UserService]("userService")
    LoggerKey      = vessel.NewServiceKey[Logger]("logger")
)

// Register with type safety
vessel.RegisterWithKey(c, DatabaseKey, func(c vessel.Vessel) (*Database, error) {
    return &Database{}, nil
}, vessel.Singleton())

vessel.RegisterWithKey(c, UserServiceKey, func(c vessel.Vessel) (*UserService, error) {
    db := vessel.MustWithKey(c, DatabaseKey) // Type-safe!
    return &UserService{db: db}, nil
})

// Resolve with full type safety and autocomplete
db, err := vessel.ResolveWithKey(c, DatabaseKey)
// db is *Database, no type assertion needed!

// Or use Must variant
userService := vessel.MustWithKey(c, UserServiceKey)

// Check if service exists
if vessel.HasKey(c, DatabaseKey) {
    // Service is registered
}
```

## 🪝 Middleware & Hooks

Intercept and observe service resolution and lifecycle events:

```go
// Create logging middleware
loggingMiddleware := &vessel.FuncMiddleware{
    BeforeResolveFunc: func(ctx context.Context, name string) error {
        log.Printf("Resolving service: %s", name)
        return nil
    },
    AfterResolveFunc: func(ctx context.Context, name string, service any, err error) error {
        if err != nil {
            log.Printf("Failed to resolve %s: %v", name, err)
        } else {
            log.Printf("Successfully resolved %s", name)
        }
        return nil
    },
    BeforeStartFunc: func(ctx context.Context, name string) error {
        log.Printf("Starting service: %s", name)
        return nil
    },
    AfterStartFunc: func(ctx context.Context, name string, err error) error {
        if err != nil {
            log.Printf("Failed to start %s: %v", name, err)
        }
        return nil
    },
}

// Register middleware
c.(*vessel.ContainerImpl).Use(loggingMiddleware)

// Create custom middleware
type MetricsMiddleware struct {
    metrics *Metrics
}

func (m *MetricsMiddleware) BeforeResolve(ctx context.Context, name string) error {
    m.metrics.IncrementResolveCount(name)
    return nil
}

func (m *MetricsMiddleware) AfterResolve(ctx context.Context, name string, service any, err error) error {
    if err != nil {
        m.metrics.IncrementResolveError(name)
    }
    return nil
}

// Implement other methods...
```

## 📚 Batch Registration

Register multiple services efficiently:

```go
// Batch register with untyped factories
err := vessel.RegisterServices(c,
    vessel.Service("database", NewDatabase, vessel.Singleton()),
    vessel.Service("cache", NewCache, vessel.Singleton()),
    vessel.Service("logger", NewLogger, vessel.Singleton(), vessel.WithGroup("core")),
)

// Batch register with type safety
err := vessel.RegisterTypedServices(c,
    vessel.TypedService("db", NewDatabase, vessel.Singleton()),
    vessel.TypedService("cache", NewCache, vessel.Singleton()),
)

// Batch register with service keys
err := vessel.RegisterKeyedServices(c,
    vessel.KeyedService(DatabaseKey, NewDatabase, vessel.Singleton()),
    vessel.KeyedService(CacheKey, NewCache, vessel.Singleton()),
    vessel.KeyedService(LoggerKey, NewLogger, vessel.Singleton()),
)
```

## 🔎 Service Discovery & Querying

Query and filter services by various criteria:

```go
// Find all singleton services
singletons := vessel.FindByLifecycle(c, "singleton")

// Find all services in a group
apiHandlers := vessel.FindByGroup(c, "api-handlers")

// Find started services
started := vessel.FindStarted(c)

// Find not started services
notStarted := vessel.FindNotStarted(c)

// Complex queries
started := true
results := vessel.Query(c, vessel.ServiceQuery{
    Lifecycle: "singleton",
    Group:     "api",
    Metadata: map[string]string{
        "version": "2.0",
        "env":     "production",
    },
    Started: &started,
})

// Get just the names
names := vessel.QueryNames(c, vessel.ServiceQuery{
    Group: "background-workers",
})

for _, info := range results {
    fmt.Printf("Found: %s (%s)\n", info.Name, info.Lifecycle)
}
```

## 🎯 Type-Safe Resolution

### Generic Resolve

```go
// Type-safe resolution with error handling
db, err := vessel.Resolve[*Database](c, "database")
if err != nil {
    log.Fatal(err)
}

// Panic on error (use during startup)
db := vessel.Must[*Database](c, "database")
```

### Resolve with Service Start

Ensure a service and its dependencies are started before use:

```go
// Resolves and starts the service if it implements di.Service
db, err := vessel.ResolveReady[*Database](ctx, c, "database")
```

## 💉 Typed Dependency Injection

Use `Provide` for automatic dependency injection with type safety:

```go
// Define dependencies with Inject
vessel.Provide[*UserService](c, "userService",
    vessel.Inject[*Database]("database"),
    vessel.Inject[*Logger]("logger"),
    func(db *Database, log *Logger) (*UserService, error) {
        return &UserService{
            db:     db,
            logger: log,
        }, nil
    },
)

// Or use lifecycle-specific helpers
vessel.RegisterSingletonWith[*UserService](c, "userService",
    vessel.Inject[*Database]("database"),
    func(db *Database) (*UserService, error) {
        return &UserService{db: db}, nil
    },
)
```

## ⚡ Lazy Dependencies

Break circular dependencies or defer expensive initialization:

```go
type EmailService struct {
    cache *vessel.Lazy[*Cache]
}

vessel.RegisterSingleton(c, "emailService", func(c vessel.Vessel) (*EmailService, error) {
    return &EmailService{
        cache: vessel.NewLazy[*Cache](c, "cache"),
    }, nil
})

// Later, when needed:
func (s *EmailService) SendEmail(to string, body string) error {
    cache, err := s.cache.Get() // Resolved on first access
    if err != nil {
        return err
    }
    // Use cache...
}
```

### Optional Lazy Dependencies

```go
vessel.Provide[*Service](c, "service",
    vessel.LazyInject[*Cache]("cache"),
    func(cache *vessel.Lazy[*Cache]) (*Service, error) {
        return &Service{cache: cache}, nil
    },
)

// With optional dependencies
vessel.Provide[*Service](c, "service",
    vessel.OptionalInject[*Cache]("cache"),
    func(cache *Cache) (*Service, error) {
        // cache will be nil if not registered
        return &Service{cache: cache}, nil
    },
)
```

## 🔧 Service Lifecycle Management

Implement the `di.Service` interface for automatic lifecycle management:

```go
type DatabaseService struct {
    conn *sql.DB
}

func (d *DatabaseService) Name() string {
    return "database"
}

func (d *DatabaseService) Start(ctx context.Context) error {
    conn, err := sql.Open("postgres", "...")
    if err != nil {
        return err
    }
    d.conn = conn
    return d.conn.PingContext(ctx)
}

func (d *DatabaseService) Stop(ctx context.Context) error {
    return d.conn.Close()
}

// Optional: Health checks
func (d *DatabaseService) Health(ctx context.Context) error {
    return d.conn.PingContext(ctx)
}

// Register and manage lifecycle
c := vessel.New()
vessel.RegisterSingleton(c, "database", func(c vessel.Vessel) (*DatabaseService, error) {
    return &DatabaseService{}, nil
})

// Start all services in dependency order
ctx := context.Background()
c.Start(ctx)

// Check health of all services
c.Health(ctx)

// Stop all services in reverse order
c.Stop(ctx)
```

## 🎭 Interface Registration

Register implementations as interfaces:

```go
type Logger interface {
    Log(msg string)
}

type ConsoleLogger struct{}

func (c *ConsoleLogger) Log(msg string) {
    fmt.Println(msg)
}

// Register implementation as interface
vessel.RegisterSingletonInterface[Logger, *ConsoleLogger](c, "logger",
    func(c vessel.Vessel) (*ConsoleLogger, error) {
        return &ConsoleLogger{}, nil
    },
)

// Resolve as interface
logger := vessel.Must[Logger](c, "logger")
logger.Log("Hello, World!")
```

## 📦 Scoped Services for HTTP Requests

Perfect for request-scoped resources with context storage:

```go
func httpHandler(c vessel.Vessel) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Create a new scope for this request
        scope := c.BeginScope()
        defer scope.End() // Cleanup when done
        
        // Store request-specific context
        vessel.SetScoped(scope, "requestID", r.Header.Get("X-Request-ID"))
        vessel.SetScoped(scope, "userID", getUserIDFromToken(r))
        
        // Resolve scoped services
        session, _ := vessel.ResolveScope[*Session](scope, "session")
        userCtx, _ := vessel.ResolveScope[*UserContext](scope, "userContext")
        
        // Retrieve context data in services
        requestID, _ := vessel.GetScoped[string](scope, "requestID")
        log.Printf("Handling request: %s", requestID)
        
        // Use services...
        
        // Services are automatically cleaned up when scope ends
    }
}
```

## 🔍 Dependency Inspection

```go
// Check if service is registered
if c.Has("database") {
    // Service exists
}

// Check if service has been started
if c.IsStarted("database") {
    // Service is running
}

// Get service information
info := c.Inspect("database")
fmt.Printf("Service: %s, Type: %s, Started: %v\n", 
    info.Name, info.Type, info.Started)

// List all registered services
services := c.Services()
for _, name := range services {
    fmt.Println(name)
}
```

## 🧪 Testing Support

Vessel makes testing easy with mock services:

```go
func TestUserService(t *testing.T) {
    c := vessel.New()
    
    // Register mock database
    vessel.RegisterSingleton(c, "database", func(c vessel.Vessel) (*MockDatabase, error) {
        return &MockDatabase{
            users: map[string]*User{
                "1": {ID: "1", Name: "Test User"},
            },
        }, nil
    })
    
    vessel.RegisterSingleton(c, "userService", func(c vessel.Vessel) (*UserService, error) {
        db := vessel.Must[*MockDatabase](c, "database")
        return &UserService{db: db}, nil
    })
    
    // Test with mocked dependencies
    service := vessel.Must[*UserService](c, "userService")
    user, err := service.GetUser("1")
    assert.NoError(t, err)
    assert.Equal(t, "Test User", user.Name)
}
```

## 🔗 Dependency Declaration

Declare dependencies explicitly for better documentation and validation:

```go
// With dependency tracking
c.Register("userService", func(c vessel.Vessel) (any, error) {
    return &UserService{}, nil
}, vessel.WithDependencies("database", "logger"))

// Dependencies are validated at registration time
// and used for proper startup order
```

## 🎨 Advanced Patterns

### Factory Pattern with Dependencies

```go
vessel.Provide[*HTTPClient](c, "httpClient",
    vessel.Inject[*Config]("config"),
    vessel.Inject[*Logger]("logger"),
    func(cfg *Config, log *Logger) (*HTTPClient, error) {
        return &HTTPClient{
            timeout: cfg.HTTPTimeout,
            logger:  log,
        }, nil
    },
)
```

### Value Registration

Register pre-built instances:

```go
config := &Config{Port: 8080}
vessel.RegisterValue(c, "config", config)
```

### Grouped Services

Register multiple services in the same group for discovery:

```go
// Register services with groups
vessel.RegisterSingleton(c, "handler1", ..., vessel.WithGroup("handlers"))
vessel.RegisterSingleton(c, "handler2", ..., vessel.WithGroup("handlers"))
vessel.RegisterSingleton(c, "handler3", ..., vessel.WithGroup("handlers"))

// Discover services by group
handlers := vessel.FindByGroup(c, "handlers")
for _, info := range handlers {
    fmt.Printf("Found handler: %s\n", info.Name)
}

// Query services with metadata
vessel.RegisterSingleton(c, "worker1", ..., 
    vessel.WithGroup("workers"),
    vessel.WithDIMetadata("priority", "high"),
)

highPriorityWorkers := vessel.Query(c, vessel.ServiceQuery{
    Group: "workers",
    Metadata: map[string]string{"priority": "high"},
})
```

## 🚨 Error Handling

Vessel provides structured errors with sentinel values for easy checking:

```go
service, err := vessel.Resolve[*Service](c, "unknown")
if err != nil {
    // Check with errors.Is for sentinel errors
    if errors.Is(err, vessel.ErrServiceNotFoundSentinel) {
        log.Println("Service not registered")
    }
    
    if errors.Is(err, vessel.ErrCircularDependencySentinel) {
        log.Println("Circular dependency detected")
    }
    
    if errors.Is(err, vessel.ErrScopeEnded) {
        log.Println("Scope has ended")
    }
    
    if errors.Is(err, vessel.ErrTypeMismatchSentinel) {
        log.Println("Type mismatch during resolution")
    }
    
    // Errors include contextual information
    fmt.Printf("Error: %v\n", err)
}

// Check for specific error conditions
scope := c.BeginScope()
scope.End()

_, err = scope.Resolve("service")
if errors.Is(err, vessel.ErrScopeEnded) {
    // Handle ended scope
}
```

## 📊 Performance

Vessel is optimized for production use:

```
BenchmarkResolve_Singleton_Cached-16     100M    12.00 ns/op     0 B/op    0 allocs/op
BenchmarkResolve_Transient-16             94M    12.78 ns/op     0 B/op    0 allocs/op
BenchmarkScope_Create-16                  21M    56.46 ns/op   160 B/op    3 allocs/op
BenchmarkScope_Resolve_Cached-16          77M    15.60 ns/op     0 B/op    0 allocs/op
BenchmarkStart_10Services-16             351K     3.34 μs/op  6960 B/op   86 allocs/op
BenchmarkStart_100Services-16             45K    26.34 μs/op 58709 B/op  857 allocs/op
BenchmarkConcurrentResolve-16              8M      152 ns/op     0 B/op    0 allocs/op
BenchmarkConcurrentScope-16                7M      181 ns/op   448 B/op    4 allocs/op
```

### Understanding the Benchmarks

**Resolve_Singleton_Cached** - Resolving an already-created singleton service. This is the most common operation in production. At ~12ns with zero allocations, it's essentially just a map lookup with a mutex read lock.

**Resolve_Transient** - Creating a new transient service instance each time. At ~13ns, the factory function is called but the service itself is simple (no dependencies), showing the framework's low overhead.

**Scope_Create** - Creating a new scope (e.g., for an HTTP request). At ~56ns with 160 bytes allocated, this is lightweight enough to create per-request without performance concerns.

**Scope_Resolve_Cached** - Resolving a scoped service that's already been created in the current scope. At ~16ns with zero allocations, subsequent resolutions within the same scope are very fast.

**Start_10Services** / **Start_100Services** - Starting services with lifecycle hooks. These scale linearly (~3.3μs for 10 services, ~26μs for 100 services), showing efficient startup even with many services. This happens once at application startup.

**ConcurrentResolve** - Multiple goroutines resolving the same singleton simultaneously. At ~152ns, the mutex contention is minimal, making Vessel safe for high-concurrency scenarios.

**ConcurrentScope** - Multiple goroutines creating and using separate scopes simultaneously. At ~181ns, isolated scopes have minimal contention, ideal for concurrent request handling.

### Key Performance Characteristics

- **Cached singleton resolve**: ~12ns (zero allocations) - The hot path for most applications
- **Transient service creation**: ~13ns (zero allocations) - Minimal framework overhead
- **Scope creation**: ~56ns (160 bytes, 3 allocations) - Efficient per-request scoping
- **Scoped service resolve**: ~16ns cached (zero allocations) - Fast repeated access within scope
- **Thread-safe**: Minimal contention under concurrent load (~10x slower than single-threaded)
- **Startup**: Linear scaling, ~260ns per service with lifecycle management

**What This Means for Your Application:**
- You can resolve services millions of times per second
- Creating scopes per HTTP request adds negligible overhead (~56ns)
- Concurrent access is safe and efficient for high-throughput services
- Startup time is predictable and scales with service count

## 🛠️ Best Practices

1. **Register services at startup** - Keep container immutable after initialization
2. **Use typed service keys** - Prefer `ServiceKey[T]` over strings for type safety
3. **Use generics for type safety** - Avoid `any` and type assertions
4. **Implement service lifecycle** - Use Start/Stop for resource management
5. **Leverage scopes for requests** - Create new scopes for HTTP handlers
6. **Use scope context storage** - Store request-specific data with `SetScoped/GetScoped`
7. **Use lazy dependencies sparingly** - Only for circular dependencies or expensive resources
8. **Declare dependencies explicitly** - Use `WithDependencies()` for documentation
9. **Use middleware for cross-cutting concerns** - Logging, metrics, security validation
10. **Query services for discovery** - Use `Query()` and `FindByGroup()` for dynamic service discovery
11. **Batch register related services** - Use `RegisterServices()` for cleaner code
12. **Test with mocks** - Create fresh containers per test with mock services

## 🔄 Migration from Other DI Containers

### From wire

```go
// Before (wire)
//go:build wireinject
func InitializeApp() (*App, error) {
    wire.Build(
        NewDatabase,
        NewUserService,
        NewApp,
    )
    return nil, nil
}

// After (vessel)
func InitializeApp() (*App, error) {
    c := vessel.New()
    vessel.RegisterSingleton(c, "database", NewDatabase)
    vessel.RegisterSingleton(c, "userService", NewUserService)
    vessel.RegisterSingleton(c, "app", NewApp)
    
    return vessel.Resolve[*App](c, "app")
}
```

### From dig

```go
// Before (dig)
c := dig.New()
c.Provide(NewDatabase)
c.Provide(NewUserService)

// After (vessel)
c := vessel.New()
vessel.RegisterSingleton(c, "database", NewDatabase)
vessel.RegisterSingleton(c, "userService", NewUserService)
```

## 📚 Complete Example

```go
package main

import (
    "context"
    "fmt"
    "log"
    "net/http"
    
    "github.com/xraph/vessel"
)

// Domain types
type Config struct {
    DatabaseURL string
    Port        int
}

type Database struct {
    url string
}

func (d *Database) Name() string { return "database" }
func (d *Database) Start(ctx context.Context) error {
    fmt.Printf("Connecting to %s\n", d.url)
    return nil
}
func (d *Database) Stop(ctx context.Context) error {
    fmt.Println("Closing database connection")
    return nil
}

type UserRepository struct {
    db *Database
}

type UserService struct {
    repo *UserRepository
}

type HTTPServer struct {
    container vessel.Vessel
    port      int
}

func (s *HTTPServer) Name() string { return "http-server" }
func (s *HTTPServer) Start(ctx context.Context) error {
    fmt.Printf("Starting HTTP server on port %d\n", s.port)
    return nil
}
func (s *HTTPServer) Stop(ctx context.Context) error {
    fmt.Println("Shutting down HTTP server")
    return nil
}

func main() {
    // Initialize container
    c := vessel.New()
    
    // Register configuration
    vessel.RegisterValue(c, "config", &Config{
        DatabaseURL: "postgres://localhost/myapp",
        Port:        8080,
    })
    
    // Register services with typed dependencies
    vessel.RegisterSingletonWith[*Database](c, "database",
        vessel.Inject[*Config]("config"),
        func(cfg *Config) (*Database, error) {
            return &Database{url: cfg.DatabaseURL}, nil
        },
    )
    
    vessel.RegisterSingletonWith[*UserRepository](c, "userRepo",
        vessel.Inject[*Database]("database"),
        func(db *Database) (*UserRepository, error) {
            return &UserRepository{db: db}, nil
        },
    )
    
    vessel.RegisterSingletonWith[*UserService](c, "userService",
        vessel.Inject[*UserRepository]("userRepo"),
        func(repo *UserRepository) (*UserService, error) {
            return &UserService{repo: repo}, nil
        },
    )
    
    vessel.RegisterSingletonWith[*HTTPServer](c, "httpServer",
        vessel.Inject[*Config]("config"),
        func(cfg *Config) (*HTTPServer, error) {
            return &HTTPServer{
                container: c,
                port:      cfg.Port,
            }, nil
        },
    )
    
    // Start all services
    ctx := context.Background()
    if err := c.Start(ctx); err != nil {
        log.Fatalf("Failed to start services: %v", err)
    }
    defer c.Stop(ctx)
    
    // Application is running...
    fmt.Println("Application started successfully!")
    
    // Check health
    if err := c.Health(ctx); err != nil {
        log.Printf("Health check failed: %v", err)
    }
}
```

## 🤝 Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## 📄 License

This project is licensed under the MIT License - see the LICENSE file for details.

## 🔗 Related Projects

- [Forge Framework](https://github.com/xraph/forge) - Complete application framework
- [go-utils](https://github.com/xraph/go-utils) - Shared utilities for Go applications

## 💬 Support

- 📫 Issues: [GitHub Issues](https://github.com/xraph/vessel/issues)
- 💬 Discussions: [GitHub Discussions](https://github.com/xraph/vessel/discussions)
- 📖 Documentation: [pkg.go.dev](https://pkg.go.dev/github.com/xraph/vessel)

---

Built with ❤️ as part of the Forge ecosystem
