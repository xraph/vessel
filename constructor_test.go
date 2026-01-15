package vessel

import (
	"errors"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test types for constructor injection
type testDatabase struct {
	connStr string
}

type testLogger struct {
	level string
}

type testCache struct {
	host string
}

type testUserService struct {
	db     *testDatabase
	logger *testLogger
}

type testProductService struct {
	db    *testDatabase
	cache *testCache
}

// Simple constructors
func newTestDatabase() *testDatabase {
	return &testDatabase{connStr: "postgres://localhost/test"}
}

func newTestLogger() *testLogger {
	return &testLogger{level: "info"}
}

func newTestCache() *testCache {
	return &testCache{host: "localhost:6379"}
}

func newTestUserService(db *testDatabase, logger *testLogger) *testUserService {
	return &testUserService{db: db, logger: logger}
}

func newTestUserServiceWithError(db *testDatabase) (*testUserService, error) {
	if db == nil {
		return nil, errors.New("database is required")
	}
	return &testUserService{db: db}, nil
}

// === Basic Constructor Tests ===

func TestProvideConstructor_Simple(t *testing.T) {
	c := New()

	err := ProvideConstructor(c, newTestDatabase)
	require.NoError(t, err)

	db, err := InjectType[*testDatabase](c)
	require.NoError(t, err)
	assert.Equal(t, "postgres://localhost/test", db.connStr)
}

func TestProvideConstructor_WithDependencies(t *testing.T) {
	c := New()

	// Register dependencies first
	err := ProvideConstructor(c, newTestDatabase)
	require.NoError(t, err)

	err = ProvideConstructor(c, newTestLogger)
	require.NoError(t, err)

	// Register service that depends on them
	err = ProvideConstructor(c, newTestUserService)
	require.NoError(t, err)

	// Resolve
	svc, err := InjectType[*testUserService](c)
	require.NoError(t, err)
	assert.NotNil(t, svc.db)
	assert.NotNil(t, svc.logger)
	assert.Equal(t, "postgres://localhost/test", svc.db.connStr)
	assert.Equal(t, "info", svc.logger.level)
}

func TestProvideConstructor_WithError(t *testing.T) {
	c := New()

	// Provide database
	err := ProvideConstructor(c, newTestDatabase)
	require.NoError(t, err)

	// Provide service that can error
	err = ProvideConstructor(c, newTestUserServiceWithError)
	require.NoError(t, err)

	// Resolve
	svc, err := InjectType[*testUserService](c)
	require.NoError(t, err)
	assert.NotNil(t, svc.db)
}

func TestProvideConstructor_ErrorReturned(t *testing.T) {
	c := New()

	// Constructor that always errors
	err := ProvideConstructor(c, func() (*testDatabase, error) {
		return nil, errors.New("connection failed")
	})
	require.NoError(t, err)

	// Resolution should fail
	_, err = InjectType[*testDatabase](c)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "connection failed")
}

func TestProvideConstructor_MissingDependency(t *testing.T) {
	c := New()

	// Register service without its dependencies
	err := ProvideConstructor(c, newTestUserService)
	require.NoError(t, err)

	// Resolution should fail
	_, err = InjectType[*testUserService](c)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no provider")
}

func TestProvideConstructor_Singleton(t *testing.T) {
	c := New()
	callCount := 0

	err := ProvideConstructor(c, func() *testDatabase {
		callCount++
		return &testDatabase{connStr: "test"}
	})
	require.NoError(t, err)

	// Resolve multiple times
	db1, err := InjectType[*testDatabase](c)
	require.NoError(t, err)

	db2, err := InjectType[*testDatabase](c)
	require.NoError(t, err)

	// Should be same instance
	assert.Same(t, db1, db2)
	assert.Equal(t, 1, callCount)
}

func TestProvideConstructor_Transient(t *testing.T) {
	c := New()
	callCount := 0

	err := ProvideConstructor(c, func() *testDatabase {
		callCount++
		return &testDatabase{connStr: "test"}
	}, AsTransient())
	require.NoError(t, err)

	// Resolve multiple times
	db1, err := InjectType[*testDatabase](c)
	require.NoError(t, err)

	db2, err := InjectType[*testDatabase](c)
	require.NoError(t, err)

	// Should be different instances
	assert.NotSame(t, db1, db2)
	assert.Equal(t, 2, callCount)
}

// === Named Services Tests ===

func TestProvideConstructor_Named(t *testing.T) {
	c := New()

	// Primary database
	err := ProvideConstructor(c, func() *testDatabase {
		return &testDatabase{connStr: "primary"}
	}, WithName("primary"))
	require.NoError(t, err)

	// Replica database
	err = ProvideConstructor(c, func() *testDatabase {
		return &testDatabase{connStr: "replica"}
	}, WithName("replica"))
	require.NoError(t, err)

	// Resolve by name
	primary, err := InjectNamed[*testDatabase](c, "primary")
	require.NoError(t, err)
	assert.Equal(t, "primary", primary.connStr)

	replica, err := InjectNamed[*testDatabase](c, "replica")
	require.NoError(t, err)
	assert.Equal(t, "replica", replica.connStr)
}

func TestProvideConstructor_DuplicateType(t *testing.T) {
	c := New()

	err := ProvideConstructor(c, newTestDatabase)
	require.NoError(t, err)

	// Should error on duplicate
	err = ProvideConstructor(c, newTestDatabase)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")
}

// === In Struct Tests ===

type testServiceParamsIn struct {
	In

	DB     *testDatabase
	Logger *testLogger
}

func newServiceWithIn(p testServiceParamsIn) *testUserService {
	return &testUserService{db: p.DB, logger: p.Logger}
}

func TestProvideConstructor_InStruct(t *testing.T) {
	c := New()

	// Register dependencies
	err := ProvideConstructor(c, newTestDatabase)
	require.NoError(t, err)

	err = ProvideConstructor(c, newTestLogger)
	require.NoError(t, err)

	// Register service with In struct
	err = ProvideConstructor(c, newServiceWithIn)
	require.NoError(t, err)

	// Resolve
	svc, err := InjectType[*testUserService](c)
	require.NoError(t, err)
	assert.NotNil(t, svc.db)
	assert.NotNil(t, svc.logger)
}

type testOptionalParamsIn struct {
	In

	DB    *testDatabase
	Cache *testCache `optional:"true"`
}

func newServiceWithOptional(p testOptionalParamsIn) *testUserService {
	return &testUserService{db: p.DB}
}

func TestProvideConstructor_InStruct_Optional(t *testing.T) {
	c := New()

	// Only register database, not cache
	err := ProvideConstructor(c, newTestDatabase)
	require.NoError(t, err)

	// Register service with optional dependency
	err = ProvideConstructor(c, newServiceWithOptional)
	require.NoError(t, err)

	// Resolve - should succeed even without cache
	svc, err := InjectType[*testUserService](c)
	require.NoError(t, err)
	assert.NotNil(t, svc.db)
}

type testNamedParamsIn struct {
	In

	Primary *testDatabase `name:"primary"`
	Replica *testDatabase `name:"replica"`
}

type testMultiDBService struct {
	primary *testDatabase
	replica *testDatabase
}

func newMultiDBService(p testNamedParamsIn) *testMultiDBService {
	return &testMultiDBService{primary: p.Primary, replica: p.Replica}
}

func TestProvideConstructor_InStruct_Named(t *testing.T) {
	c := New()

	// Register named databases
	err := ProvideConstructor(c, func() *testDatabase {
		return &testDatabase{connStr: "primary"}
	}, WithName("primary"))
	require.NoError(t, err)

	err = ProvideConstructor(c, func() *testDatabase {
		return &testDatabase{connStr: "replica"}
	}, WithName("replica"))
	require.NoError(t, err)

	// Register service that depends on named services
	err = ProvideConstructor(c, newMultiDBService)
	require.NoError(t, err)

	// Resolve
	svc, err := InjectType[*testMultiDBService](c)
	require.NoError(t, err)
	assert.Equal(t, "primary", svc.primary.connStr)
	assert.Equal(t, "replica", svc.replica.connStr)
}

// === Out Struct Tests ===

type testServicesOut struct {
	Out

	UserService    *testUserService
	ProductService *testProductService
}

func newTestServices(db *testDatabase, logger *testLogger, cache *testCache) testServicesOut {
	return testServicesOut{
		UserService:    &testUserService{db: db, logger: logger},
		ProductService: &testProductService{db: db, cache: cache},
	}
}

func TestProvideConstructor_OutStruct(t *testing.T) {
	c := New()

	// Register dependencies
	err := ProvideConstructor(c, newTestDatabase)
	require.NoError(t, err)

	err = ProvideConstructor(c, newTestLogger)
	require.NoError(t, err)

	err = ProvideConstructor(c, newTestCache)
	require.NoError(t, err)

	// Register constructor that returns Out struct
	err = ProvideConstructor(c, newTestServices)
	require.NoError(t, err)

	// Resolve both services
	userSvc, err := InjectType[*testUserService](c)
	require.NoError(t, err)
	assert.NotNil(t, userSvc.db)

	productSvc, err := InjectType[*testProductService](c)
	require.NoError(t, err)
	assert.NotNil(t, productSvc.db)
}

// === Value Groups Tests ===

type testUserHandler struct{}

func (h *testUserHandler) Handle() string { return "user" }

type testProductHandler struct{}

func (h *testProductHandler) Handle() string { return "product" }

func TestProvideConstructor_Group(t *testing.T) {
	c := New()

	// Register handlers in a group
	err := ProvideConstructor(c, func() *testUserHandler {
		return &testUserHandler{}
	}, AsGroup("handlers"))
	require.NoError(t, err)

	err = ProvideConstructor(c, func() *testProductHandler {
		return &testProductHandler{}
	}, AsGroup("handlers"))
	require.NoError(t, err)

	// Resolve group - get concrete types
	impl, ok := c.(*containerImpl)
	require.True(t, ok)

	regs := impl.typeRegistry.getGroup("handlers")
	assert.Len(t, regs, 2)
}

// === Has/HasNamed Tests ===

func TestHasType(t *testing.T) {
	c := New()

	assert.False(t, HasType[*testDatabase](c))

	err := ProvideConstructor(c, newTestDatabase)
	require.NoError(t, err)

	assert.True(t, HasType[*testDatabase](c))
}

func TestHasTypeNamed(t *testing.T) {
	c := New()

	assert.False(t, HasTypeNamed[*testDatabase](c, "primary"))

	err := ProvideConstructor(c, newTestDatabase, WithName("primary"))
	require.NoError(t, err)

	assert.True(t, HasTypeNamed[*testDatabase](c, "primary"))
	assert.False(t, HasTypeNamed[*testDatabase](c, "replica"))
}

// === Must* Helpers Tests ===

func TestMustInjectType_Success(t *testing.T) {
	c := New()

	err := ProvideConstructor(c, newTestDatabase)
	require.NoError(t, err)

	db := MustInjectType[*testDatabase](c)
	assert.NotNil(t, db)
}

func TestMustInjectType_Panic(t *testing.T) {
	c := New()

	assert.Panics(t, func() {
		MustInjectType[*testDatabase](c)
	})
}

func TestMustInjectNamed_Success(t *testing.T) {
	c := New()

	err := ProvideConstructor(c, newTestDatabase, WithName("primary"))
	require.NoError(t, err)

	db := MustInjectNamed[*testDatabase](c, "primary")
	assert.NotNil(t, db)
}

func TestMustInjectNamed_Panic(t *testing.T) {
	c := New()

	assert.Panics(t, func() {
		MustInjectNamed[*testDatabase](c, "nonexistent")
	})
}

// === Circular Dependency Tests ===

type testCircularA struct {
	B *testCircularB
}

type testCircularB struct {
	A *testCircularA
}

func TestProvideConstructor_CircularDependency(t *testing.T) {
	c := New()

	// This creates a circular dependency: A -> B -> A
	err := ProvideConstructor(c, func(b *testCircularB) *testCircularA {
		return &testCircularA{B: b}
	})
	require.NoError(t, err)

	err = ProvideConstructor(c, func(a *testCircularA) *testCircularB {
		return &testCircularB{A: a}
	})
	require.NoError(t, err)

	// Resolution should detect cycle
	_, err = InjectType[*testCircularA](c)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "circular")
}

// === Constructor Analysis Tests ===

func TestAnalyzeConstructor_NotAFunction(t *testing.T) {
	_, err := analyzeConstructor("not a function")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must be a function")
}

func TestAnalyzeConstructor_NoReturns(t *testing.T) {
	_, err := analyzeConstructor(func() {})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must return at least one")
}

func TestAnalyzeConstructor_ErrorNotLast(t *testing.T) {
	//nolint:staticcheck // Testing that error-not-last is detected
	_, err := analyzeConstructor(func() (error, *testDatabase) {
		return nil, nil
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error must be the last")
}

func TestIsInStruct(t *testing.T) {
	assert.True(t, isInStruct(reflect.TypeOf(testServiceParamsIn{})))
	assert.True(t, isInStruct(reflect.TypeOf(&testServiceParamsIn{})))
	assert.False(t, isInStruct(reflect.TypeOf(testDatabase{})))
	assert.False(t, isInStruct(reflect.TypeOf("string")))
}

func TestIsOutStruct(t *testing.T) {
	assert.True(t, isOutStruct(reflect.TypeOf(testServicesOut{})))
	assert.True(t, isOutStruct(reflect.TypeOf(&testServicesOut{})))
	assert.False(t, isOutStruct(reflect.TypeOf(testDatabase{})))
	assert.False(t, isOutStruct(reflect.TypeOf("string")))
}

// === As Option Tests ===

type testReader interface {
	Read() string
}

type testWriter interface {
	Write(s string)
}

type testReadWriter struct{}

func (rw *testReadWriter) Read() string   { return "data" }
func (rw *testReadWriter) Write(s string) {}

func TestProvideConstructor_As(t *testing.T) {
	c := New()

	err := ProvideConstructor(c, func() *testReadWriter {
		return &testReadWriter{}
	}, As(new(testReader)))
	require.NoError(t, err)

	// Should be resolvable as interface
	reader, err := InjectType[testReader](c)
	require.NoError(t, err)
	assert.Equal(t, "data", reader.Read())
}

func TestWithAliases_MultipleNames(t *testing.T) {
	c := New()

	// Register with primary name and aliases
	err := ProvideConstructor(c, newTestDatabase, WithName("primary"), WithAliases("default", "main"))
	require.NoError(t, err)

	// Should be resolvable by primary name
	db1, err := InjectNamed[*testDatabase](c, "primary")
	require.NoError(t, err)
	assert.Equal(t, "postgres://localhost/test", db1.connStr)

	// Should be resolvable by first alias
	db2, err := InjectNamed[*testDatabase](c, "default")
	require.NoError(t, err)
	assert.Equal(t, "postgres://localhost/test", db2.connStr)

	// Should be resolvable by second alias
	db3, err := InjectNamed[*testDatabase](c, "main")
	require.NoError(t, err)
	assert.Equal(t, "postgres://localhost/test", db3.connStr)

	// All should be the same instance (singleton)
	assert.Same(t, db1, db2)
	assert.Same(t, db2, db3)
}

func TestWithAliases_EmptyStringForUnnamed(t *testing.T) {
	c := New()

	// Register with a name but also as unnamed (empty string alias)
	err := ProvideConstructor(c, newTestDatabase, WithName("named"), WithAliases(""))
	require.NoError(t, err)

	// Should be resolvable by name
	db1, err := InjectNamed[*testDatabase](c, "named")
	require.NoError(t, err)
	assert.Equal(t, "postgres://localhost/test", db1.connStr)

	// Should also be resolvable without name
	db2, err := InjectType[*testDatabase](c)
	require.NoError(t, err)
	assert.Equal(t, "postgres://localhost/test", db2.connStr)

	// Should be the same instance
	assert.Same(t, db1, db2)
}

func TestWithAliases_WithAsTypes(t *testing.T) {
	c := New()

	// Register with name, aliases, and additional interface types
	err := ProvideConstructor(c, func() *testReadWriter {
		return &testReadWriter{}
	}, WithName("rw"), WithAliases("default", ""), As(new(testReader), new(testWriter)))
	require.NoError(t, err)

	// Should be resolvable as concrete type by name
	rw1, err := InjectNamed[*testReadWriter](c, "rw")
	require.NoError(t, err)

	// Should be resolvable as concrete type by alias
	rw2, err := InjectNamed[*testReadWriter](c, "default")
	require.NoError(t, err)

	// Should be resolvable as concrete type without name
	rw3, err := InjectType[*testReadWriter](c)
	require.NoError(t, err)

	// Should be resolvable as interface by name
	reader1, err := InjectNamed[testReader](c, "rw")
	require.NoError(t, err)

	// Should be resolvable as interface by alias
	reader2, err := InjectNamed[testReader](c, "default")
	require.NoError(t, err)

	// Should be resolvable as interface without name
	reader3, err := InjectType[testReader](c)
	require.NoError(t, err)

	// All should be the same instance
	assert.Same(t, rw1, rw2)
	assert.Same(t, rw2, rw3)
	assert.Same(t, rw1, reader1)
	assert.Same(t, reader1, reader2)
	assert.Same(t, reader2, reader3)
}

func TestWithAliases_ConflictDetection(t *testing.T) {
	c := New()

	// Register first database
	err := ProvideConstructor(c, newTestDatabase, WithName("primary"))
	require.NoError(t, err)

	// Try to register second database with same name - should fail
	err = ProvideConstructor(c, newTestDatabase, WithName("primary"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")

	// Try to register with alias that conflicts with existing named service
	err = ProvideConstructor(c, func() *testDatabase {
		return &testDatabase{connStr: "different"}
	}, WithName("secondary"), WithAliases("primary"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "alias")
}
