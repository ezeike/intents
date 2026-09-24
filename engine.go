// engine.go
package intents

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sync"
)

// Engine walks a validated spec against registered backends. There is no usable zero value —
// use NewEngine. Engine is safe for concurrent RegisterClassifier/RegisterLookuper/
// ResolveState/ResumeState calls.
type Engine struct {
	mu          sync.RWMutex
	classifiers map[string]Classifier
	lookupers   map[string]Lookuper

	validated sync.Map // *Node -> error (nil means valid); construction-time Validate cache
}

// NewEngine returns a ready-to-use *Engine with its backend maps allocated.
func NewEngine() *Engine {
	return &Engine{
		classifiers: make(map[string]Classifier),
		lookupers:   make(map[string]Lookuper),
	}
}

// RegisterClassifier registers c under name, replacing any previous registration under the
// same name. An empty name or a nil c is a programmer error and panics — mirrors
// net/http.Handle.
func (e *Engine) RegisterClassifier(name string, c Classifier) {
	if name == "" {
		panic("intents: RegisterClassifier: empty name")
	}
	if c == nil {
		panic("intents: RegisterClassifier: nil Classifier")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.classifiers[name] = c
}

// RegisterLookuper registers l under name, replacing any previous registration under the same
// name. An empty name or a nil l is a programmer error and panics.
func (e *Engine) RegisterLookuper(name string, l Lookuper) {
	if name == "" {
		panic("intents: RegisterLookuper: empty name")
	}
	if l == nil {
		panic("intents: RegisterLookuper: nil Lookuper")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.lookupers[name] = l
}

func (e *Engine) classifier(name string) (Classifier, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	c, ok := e.classifiers[name]
	return c, ok
}

func (e *Engine) lookuper(name string) (Lookuper, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	l, ok := e.lookupers[name]
	return l, ok
}

// ResolveState validates spec (cached per *Node pointer), checks every referenced backend is
// registered, then walks spec against input. A cancelled ctx returns a wrapped ctx.Err() with
// the partial State; any remaining required Failure returns *IncompleteError (unwrapped).
func (e *Engine) ResolveState(ctx context.Context, spec *Node, input string, callerContext any) (State, error) {
	if err := e.validateCached(spec); err != nil {
		return State{}, err
	}
	if err := checkBackendsRegistered(e, spec); err != nil {
		return State{}, err
	}

	state := State{Input: input, Context: callerContext, Values: make(map[string]Value)}
	if err := e.walkTree(ctx, spec, &state, spec.Key); err != nil {
		return state, fmt.Errorf("intents: resolve: %w", err)
	}
	state.RiskTier = maxValueRiskTier(state.Values)

	if len(requiredFailuresIn(spec, state.Failures)) > 0 {
		// IncompleteError carries every Failure, optional ones included (decision 5).
		return state, &IncompleteError{State: state}
	}
	return state, nil
}

// validation wraps a cached Validate result so a nil error is distinguishable from a cache miss.
type validation struct{ err error }

// validateCached runs Validate once per *Node pointer. A concurrent double-miss is benign:
// Validate is pure, so both computations agree.
func (e *Engine) validateCached(spec *Node) error {
	if spec == nil {
		return Validate(spec)
	}
	if v, ok := e.validated.Load(spec); ok {
		return v.(validation).err
	}
	err := Validate(spec)
	e.validated.Store(spec, validation{err: err})
	return err
}

// checkBackendsRegistered walks a validated spec depth-first (Children by sorted key, Slots in
// declaration order) and reports the first backend reference with no registration.
func checkBackendsRegistered(e *Engine, spec *Node) error {
	return checkNodeBackends(e, spec, spec.Key)
}

func checkNodeBackends(e *Engine, n *Node, path string) error {
	if err := checkSpecBackends(e, path, n.Classify, n.Lookup); err != nil {
		return err
	}
	for i := range n.Slots {
		slot := &n.Slots[i]
		if err := checkSpecBackends(e, path+"."+slot.Name, slot.Classify, slot.Lookup); err != nil {
			return err
		}
	}
	for _, k := range slices.Sorted(maps.Keys(n.Children)) {
		c := n.Children[k]
		if err := checkNodeBackends(e, c, path+"."+c.Key); err != nil {
			return err
		}
	}
	return nil
}

func checkSpecBackends(e *Engine, path string, cs *ClassifySpec, ls *LookupSpec) error {
	if cs != nil {
		if _, ok := e.classifier(cs.Backend); !ok {
			return fmt.Errorf("intents: node %q: backend %q: %w", path, cs.Backend, ErrUnregisteredBackend)
		}
		if cs.Fallback != nil {
			ls = cs.Fallback // a classify spec never also carries a Lookup (Validate)
		}
	}
	if ls != nil {
		if _, ok := e.lookuper(ls.Backend); !ok {
			return fmt.Errorf("intents: node %q: backend %q: %w", path, ls.Backend, ErrUnregisteredBackend)
		}
	}
	return nil
}

// collectRequiredPaths records every Failure path that counts as required: every tree node
// (spec.md R6) and each slot whose SlotSpec.Required is set.
func collectRequiredPaths(n *Node, path string, required map[string]bool) {
	required[path] = true
	for _, slot := range n.Slots {
		if slot.Required {
			required[path+"."+slot.Name] = true
		}
	}
	for _, c := range n.Children {
		collectRequiredPaths(c, path+"."+c.Key, required)
	}
}

// requiredFailuresIn returns the failures whose Path is required under spec. Shared by
// ResolveState and ResumeState so both decide IncompleteError identically.
func requiredFailuresIn(spec *Node, failures []Failure) []Failure {
	required := make(map[string]bool)
	collectRequiredPaths(spec, spec.Key, required)
	var out []Failure
	for _, f := range failures {
		if required[f.Path] {
			out = append(out, f)
		}
	}
	return out
}

// maxValueRiskTier returns the highest RiskTier across values (RiskLow when empty).
func maxValueRiskTier(values map[string]Value) RiskTier {
	var tier RiskTier
	for _, v := range values {
		tier = max(tier, v.RiskTier)
	}
	return tier
}

// Builder converts a resolved State into a caller-typed T. It is a plain function, never
// reflection — see doc.go's "It does not reflect into caller types".
type Builder[T any] func(State) (T, error)

// Resolve is the one-shot, caller-facing entry point: ResolveState followed by build. A nil
// build is a programmer error and panics — it is not special-cased.
func Resolve[T any](ctx context.Context, e *Engine, spec *Node, input string, callerContext any, build Builder[T]) (T, error) {
	if build == nil {
		panic("intents: Resolve: nil build")
	}
	var zero T
	state, err := e.ResolveState(ctx, spec, input, callerContext)
	if err != nil {
		return zero, err
	}
	t, err := build(state)
	if err != nil {
		return zero, fmt.Errorf("intents: build: %w", err)
	}
	return t, nil
}
