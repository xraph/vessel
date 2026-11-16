package vessel

import (
	"maps"
	"sync"
	"sync/atomic"
)

// snapMap is a typed, read-mostly map for registries that are written at
// startup and read on every resolve.
//
// Reads go to an immutable snapshot with a single atomic load, using Go's
// built-in map hashing for K, which is much cheaper than sync.Map's generic
// interface hashing for struct keys. Writes go to an authoritative map under
// a mutex. A read that misses while writes are pending rebuilds the
// snapshot once, so registering everything and then resolving costs a
// single copy.
//
// Keys are never overwritten or deleted, which is what lets a stale snapshot
// answer hits correctly.
type snapMap[K comparable, V any] struct {
	read  atomic.Pointer[map[K]V]
	stale atomic.Bool // dirty has keys the snapshot doesn't

	mu    sync.Mutex
	dirty map[K]V
}

// Load returns the value for key.
func (m *snapMap[K, V]) Load(key K) (V, bool) {
	if p := m.read.Load(); p != nil {
		if v, ok := (*p)[key]; ok {
			return v, true
		}
	}

	if !m.stale.Load() {
		var zero V
		return zero, false
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	v, ok := m.dirty[key]
	if m.stale.Load() {
		snapshot := maps.Clone(m.dirty)
		m.read.Store(&snapshot)
		m.stale.Store(false)
	}

	return v, ok
}

// StoreNew adds key, returning false if it is already present.
func (m *snapMap[K, V]) StoreNew(key K, v V) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.dirty[key]; exists {
		return false
	}

	m.storeLocked(key, v)

	return true
}

// StoreAll adds every entry, or none if any key is present already or
// appears twice. It returns the first conflicting key.
func (m *snapMap[K, V]) StoreAll(keys []K, values []V) (K, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i, k := range keys {
		if _, exists := m.dirty[k]; exists {
			return k, false
		}

		for _, prev := range keys[:i] {
			if prev == k {
				return k, false
			}
		}
	}

	for i, k := range keys {
		m.storeLocked(k, values[i])
	}

	var zero K

	return zero, true
}

func (m *snapMap[K, V]) storeLocked(key K, v V) {
	if m.dirty == nil {
		m.dirty = make(map[K]V)
	}

	m.dirty[key] = v
	m.stale.Store(true)
}

// Range calls fn for every entry until it returns false.
func (m *snapMap[K, V]) Range(fn func(K, V) bool) {
	m.mu.Lock()
	snapshot := maps.Clone(m.dirty)
	m.mu.Unlock()

	for k, v := range snapshot {
		if !fn(k, v) {
			return
		}
	}
}
