package vessel

import (
	"github.com/xraph/go-utils/di"
)

// DependencyGraph manages service dependencies.
type DependencyGraph struct {
	nodes map[string]*node
	order []string // Preserve registration order
}

type node struct {
	name         string
	dependencies []string // Backward compatible: just names
	deps         []di.Dep // New: full dependency specs with modes
	eager        []string // names of non-lazy deps, computed once
}

func newNode(name string, dependencies []string, deps []di.Dep) *node {
	n := &node{name: name, dependencies: dependencies, deps: deps}
	for _, dep := range deps {
		if !dep.Mode.IsLazy() {
			n.eager = append(n.eager, dep.Name)
		}
	}

	return n
}

// NewDependencyGraph creates a new dependency graph.
func NewDependencyGraph() *DependencyGraph {
	return &DependencyGraph{
		nodes: make(map[string]*node),
	}
}

// AddNode adds a node with its dependencies (string-based, backward compatible).
// Nodes are processed in the order they are added (FIFO) when no dependencies exist.
func (g *DependencyGraph) AddNode(name string, dependencies []string) {
	g.nodes[name] = newNode(name, dependencies, di.DepsFromNames(dependencies))
	g.order = append(g.order, name)
}

// AddNodeWithDeps adds a node with full Dep specs.
// This is the new API that supports lazy/optional dependencies.
func (g *DependencyGraph) AddNodeWithDeps(name string, deps []di.Dep) {
	g.nodes[name] = newNode(name, di.DepNames(deps), deps)
	g.order = append(g.order, name)
}

// GetDependencies returns the dependency names for a node.
func (g *DependencyGraph) GetDependencies(name string) []string {
	if node, ok := g.nodes[name]; ok {
		return node.dependencies
	}

	return nil
}

// GetDeps returns the full Dep specs for a node.
func (g *DependencyGraph) GetDeps(name string) []di.Dep {
	if node, ok := g.nodes[name]; ok {
		return node.deps
	}

	return nil
}

// GetEagerDependencies returns only the eager (non-lazy) dependencies.
// These are the ones that must be resolved before the service can be created.
func (g *DependencyGraph) GetEagerDependencies(name string) []string {
	if node, ok := g.nodes[name]; ok {
		return node.eager
	}

	return nil
}

// HasNode checks if a node exists in the graph.
func (g *DependencyGraph) HasNode(name string) bool {
	_, ok := g.nodes[name]

	return ok
}

// TopologicalSort returns nodes in dependency order.
// Nodes without dependencies maintain their registration order (FIFO).
// Returns error if circular dependency detected.
func (g *DependencyGraph) TopologicalSort() ([]string, error) {
	return g.sort(func(n *node) []string { return n.dependencies })
}

// TopologicalSortEagerOnly returns nodes sorted considering only eager dependencies.
// Lazy dependencies are excluded from the ordering since they're resolved on-demand,
// which is what lets a lazy dependency break a cycle.
func (g *DependencyGraph) TopologicalSortEagerOnly() ([]string, error) {
	return g.sort(func(n *node) []string { return n.eager })
}

// sort runs a depth-first topological sort using edges to list each node's
// dependencies. A cycle is reported with its full path, e.g. a -> b -> a.
func (g *DependencyGraph) sort(edges func(*node) []string) ([]string, error) {
	visited := make(map[string]bool, len(g.nodes))
	result := make([]string, 0, len(g.nodes))

	var stack []string

	onStack := make(map[string]int) // name -> index in stack

	var visit func(name string) error
	visit = func(name string) error {
		if visited[name] {
			return nil
		}

		if i, ok := onStack[name]; ok {
			cycle := append(append([]string{}, stack[i:]...), name)

			return ErrCircularDependency(cycle)
		}

		n := g.nodes[name]
		if n == nil {
			// Not registered: optional, or reported when it's resolved.
			return nil
		}

		onStack[name] = len(stack)
		stack = append(stack, name)

		for _, dep := range edges(n) {
			if err := visit(dep); err != nil {
				return err
			}
		}

		stack = stack[:len(stack)-1]
		delete(onStack, name)

		visited[name] = true
		result = append(result, name)

		return nil
	}

	// Visit nodes in registration order to preserve FIFO for nodes without dependencies
	for _, name := range g.order {
		if err := visit(name); err != nil {
			return nil, err
		}
	}

	return result, nil
}
