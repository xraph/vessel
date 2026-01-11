# Vessel 🚢

[![Go Reference](https://pkg.go.dev/badge/github.com/xraph/vessel.svg)](https://pkg.go.dev/github.com/xraph/vessel)
[![Go Report Card](https://goreportcard.com/badge/github.com/xraph/vessel)](https://goreportcard.com/report/github.com/xraph/vessel)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

**Vessel** is a powerful, type-safe dependency injection container for Go, built as part of the Forge framework. It provides elegant service lifecycle management, flexible dependency resolution, and comprehensive testing support.

## ✨ Features

- 🎯 **Type-Safe Generics** - Compile-time type safety with Go generics
- 🔄 **Multiple Lifecycles** - Singleton, Transient, and Scoped services
- ⚡ **Lazy Dependencies** - Defer expensive service initialization
- 🔗 **Typed Injection** - Automatic dependency resolution with type checking
- 🚀 **Service Lifecycle** - Built-in Start/Stop/Health management
- 🔍 **Circular Detection** - Automatic circular dependency detection
- 🧵 **Concurrency Safe** - Thread-safe container operations
- 📦 **Request Scoping** - Perfect for HTTP request-scoped services
- 🎭 **Interface Binding** - Register implementations as interfaces
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

Perfect for request-scoped resources:

```go
func httpHandler(c vessel.Vessel) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Create a new scope for this request
        scope := c.BeginScope()
        defer scope.End() // Cleanup when done
        
        // Resolve scoped services
        session, _ := vessel.ResolveScope[*Session](scope, "session")
        userCtx, _ := vessel.ResolveScope[*UserContext](scope, "userContext")
        
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

Register multiple services in the same group:

```go
vessel.RegisterSingleton(c, "handler1", ..., vessel.WithGroup("handlers"))
vessel.RegisterSingleton(c, "handler2", ..., vessel.WithGroup("handlers"))
vessel.RegisterSingleton(c, "handler3", ..., vessel.WithGroup("handlers"))

// Services can be discovered by group metadata
```

## 🚨 Error Handling

Vessel provides structured errors with context:

```go
service, err := vessel.Resolve[*Service](c, "unknown")
if err != nil {
    // Check error type
    if vessel.IsServiceNotFound(err) {
        log.Println("Service not registered")
    }
    
    // Errors include contextual information
    fmt.Printf("Error: %v\n", err)
}
```

## 📊 Performance

Vessel is optimized for production use:

```
BenchmarkResolve_Singleton_Cached-16     148M    8.1 ns/op    0 B/op
BenchmarkResolve_Transient-16            144M    8.3 ns/op    0 B/op
BenchmarkScope_Create-16                  29M   40.8 ns/op   96 B/op
BenchmarkStart_10Services-16             352K    3.3 μs/op  6.9 KB/op
BenchmarkConcurrentResolve-16              8M    149 ns/op    0 B/op
```

## 🛠️ Best Practices

1. **Register services at startup** - Keep container immutable after initialization
2. **Use generics for type safety** - Avoid `any` and type assertions
3. **Implement service lifecycle** - Use Start/Stop for resource management
4. **Leverage scopes for requests** - Create new scopes for HTTP handlers
5. **Use lazy dependencies sparingly** - Only for circular dependencies or expensive resources
6. **Declare dependencies explicitly** - Use `WithDependencies()` for documentation
7. **Test with mocks** - Create fresh containers per test with mock services

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
