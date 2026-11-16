package vessel

import (
	"context"
	"sync"
	"sync/atomic"
)

// resolveCtx carries per-call resolution state through nested resolutions.
//
// head is the chain of registrations currently being built on this call
// path. Re-entering one of them means the dependency graph has a cycle, which
// is reported as an error instead of blocking on the registration's init lock.
// Because the chain belongs to the call and not to the registration,
// concurrent first resolutions from different goroutines simply wait for each
// other instead of being mistaken for a cycle.
//
// The chain is a linked list so pushing a node never copies, and cached
// resolutions never touch it at all.
type resolveCtx struct {
	ctx   context.Context
	head  *pathNode
	scope *scope // set when resolving on behalf of a scope
}

type pathNode struct {
	id     any // registration pointer, unique per registration
	parent *pathNode
}

// pathLabel returns a display name for a chain entry. Names are derived only
// when a cycle is reported, so the hot path never formats them.
func pathLabel(id any) string {
	switch v := id.(type) {
	case *serviceRegistration:
		return v.name
	case *typeRegistration:
		return v.key.String()
	case *constructorMemo:
		return v.label
	default:
		return "?"
	}
}

// context returns the context for lifecycle hooks, defaulting to Background.
func (r resolveCtx) context() context.Context {
	if r.ctx == nil {
		return context.Background()
	}

	return r.ctx
}

// check returns a circular dependency error naming the full cycle if id is
// already being built on this chain. It does not allocate on success.
func (r resolveCtx) check(id any) error {
	for n := r.head; n != nil; n = n.parent {
		if n.id != id {
			continue
		}

		// Collect head..n, then reverse so the cycle reads in call order.
		var cycle []string
		for p := r.head; ; p = p.parent {
			cycle = append(cycle, pathLabel(p.id))
			if p == n {
				break
			}
		}

		for i, j := 0, len(cycle)-1; i < j; i, j = i+1, j-1 {
			cycle[i], cycle[j] = cycle[j], cycle[i]
		}

		return ErrCircularDependency(append(cycle, pathLabel(id)))
	}

	return nil
}

// push returns a copy of r with id on top of the chain.
func (r resolveCtx) push(id any) resolveCtx {
	r.head = &pathNode{id: id, parent: r.head}

	return r
}

// enter checks for a cycle and pushes id onto the chain.
func (r resolveCtx) enter(id any) (resolveCtx, error) {
	if err := r.check(id); err != nil {
		return r, err
	}

	return r.push(id), nil
}

// withoutScope returns a copy that resolves outside any scope. Singletons are
// built this way so they can never capture a scoped instance.
func (r resolveCtx) withoutScope() resolveCtx {
	r.scope = nil

	return r
}

// boundContainer is the Vessel handed to factories registered with Register.
// While the factory runs, nested Resolve calls carry the in-flight chain (for
// cycle detection) and the active scope (so scoped services can depend on
// other scoped services). Once the factory returns, the chain is dropped, so
// a factory that keeps its container for later use gets ordinary behaviour.
//
// The chain node for the registration being built lives inside this struct,
// so binding a factory costs a single allocation.
type boundContainer struct {
	*containerImpl
	node  pathNode
	ctx   context.Context
	scope *scope
	done  atomic.Bool
}

// bind returns a container for building the registration id, with id pushed
// onto rc's chain.
func (c *containerImpl) bind(rc resolveCtx, id any) *boundContainer {
	return &boundContainer{
		containerImpl: c,
		node:          pathNode{id: id, parent: rc.head},
		ctx:           rc.ctx,
		scope:         rc.scope,
	}
}

// current returns the resolution state to use for a call made through b.
func (b *boundContainer) current() resolveCtx {
	if b.done.Load() {
		return resolveCtx{scope: b.scope}
	}

	return resolveCtx{ctx: b.ctx, head: &b.node, scope: b.scope}
}

// Resolve resolves a named service, continuing the current resolution chain.
func (b *boundContainer) Resolve(name string) (any, error) {
	return b.resolveNamed(b.current(), name)
}

// ResolveReady resolves a named service after making sure it is started.
func (b *boundContainer) ResolveReady(ctx context.Context, name string) (any, error) {
	rc := b.current()
	rc.ctx = ctx

	return b.resolveReady(rc, name)
}

// unwrapContainer returns the concrete container and any resolution state
// carried by c.
func unwrapContainer(c Vessel) (*containerImpl, resolveCtx, bool) {
	switch v := c.(type) {
	case *containerImpl:
		return v, resolveCtx{}, true
	case *boundContainer:
		return v.containerImpl, v.current(), true
	default:
		return nil, resolveCtx{}, false
	}
}

// onceValue builds a value at most once, retrying after a failed build.
// Reads after a successful build are a single atomic load.
type onceValue struct {
	mu    sync.Mutex
	done  atomic.Bool
	value any
}

// load returns the value if it has been built.
func (o *onceValue) load() (any, bool) {
	if o.done.Load() {
		return o.value, true
	}

	return nil, false
}

// get returns the value, building it first if needed. created reports
// whether this call built it.
func (o *onceValue) get(build func() (any, error)) (value any, created bool, err error) {
	if v, ok := o.load(); ok {
		return v, false, nil
	}

	o.mu.Lock()
	defer o.mu.Unlock()

	if o.done.Load() {
		return o.value, false, nil
	}

	v, err := build()
	if err != nil {
		return nil, false, err
	}

	o.value = v
	o.done.Store(true)

	return v, true, nil
}
