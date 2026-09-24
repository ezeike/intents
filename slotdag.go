package intents

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"golang.org/x/sync/errgroup"
)

// slotResult is one slot's outcome within a wave, owned by exactly one goroutine.
type slotResult struct {
	value   Value
	failure *Failure
}

// resolveSlotDAG resolves n's slots from the root risk tier: a convenience entry point equal to
// resolveSlotDAGFrom with RiskLow, paralleling walkTree; not currently called internally.
func (e *Engine) resolveSlotDAG(ctx context.Context, n *Node, state *State, path string) error {
	return e.resolveSlotDAGFrom(ctx, n, state, path, RiskLow)
}

// resolveSlotDAGFrom resolves n.Slots wave by wave (spec.md R7), each wave's slots concurrently,
// merging results into state in slot declaration order. It returns a non-nil error only for
// ctx.Err() (or a spec that bypassed Validate); every other miss is recorded in state.Failures.
func (e *Engine) resolveSlotDAGFrom(ctx context.Context, n *Node, state *State, path string, ancestorTier RiskTier) error {
	waves, err := slotWave(n.Slots)
	if err != nil {
		return fmt.Errorf("intents: slot_dag: %w", err)
	}
	tier := effectiveRiskTier(n.RiskTier, ancestorTier)

	for _, wave := range waves {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("intents: resolve: %w", err)
		}

		results := make([]slotResult, len(wave))
		g, gctx := errgroup.WithContext(ctx)
		for pos, idx := range wave {
			slot := &n.Slots[idx]
			if !dependenciesResolved(slot, state.Values) {
				results[pos] = slotResult{failure: &Failure{Reason: ReasonUnresolved}}
				continue
			}
			// Snapshot before launch: goroutines never touch the shared state, and the caller
			// only mutates it after g.Wait(). Failures is cloned too so a NodeFunc appending
			// to its State copy cannot write into the shared backing array.
			snapshot := *state
			snapshot.Values = cloneValues(state.Values)
			snapshot.Failures = slices.Clone(state.Failures)
			g.Go(func() error {
				v, f := e.resolveSlot(gctx, slot, snapshot)
				if err := gctx.Err(); err != nil {
					return err
				}
				results[pos] = slotResult{value: v, failure: f}
				return nil
			})
		}
		if err := g.Wait(); err != nil {
			return fmt.Errorf("intents: resolve: %w", err)
		}

		for pos, idx := range wave {
			slot := &n.Slots[idx]
			r := results[pos]
			if r.failure != nil {
				state.Failures = append(state.Failures, Failure{
					Path:      path + "." + slot.Name,
					Reason:    r.failure.Reason,
					Err:       r.failure.Err,
					Candidate: r.failure.Candidate,
				})
				continue
			}
			r.value.RiskTier = tier
			if state.Values == nil {
				state.Values = make(map[string]Value)
			}
			state.Values[slot.Name] = r.value
		}
	}

	state.Terminal = n.Key
	return nil
}

// resolveSlot computes a slot's grounding against snapshot and dispatches on its Kind. It
// returns exactly one of a Value or a *Failure; the caller sets Failure.Path.
func (e *Engine) resolveSlot(ctx context.Context, slot *SlotSpec, snapshot State) (Value, *Failure) {
	var buildContext func(State) (any, error)
	switch slot.Kind {
	case KindClassify:
		buildContext = slot.Classify.BuildContext
	case KindLookup:
		buildContext = slot.Lookup.BuildContext
	}
	grounding := snapshot.Context
	if buildContext != nil {
		g, err := buildContext(snapshot)
		if err != nil {
			return Value{}, &Failure{Reason: ReasonBackendError, Err: fmt.Errorf("intents: build context: %w", err)}
		}
		grounding = g
	}

	switch slot.Kind {
	case KindClassify:
		return e.resolveClassify(ctx, slot.Classify, snapshot.Input, grounding)
	case KindLookup:
		return e.resolveLookup(ctx, slot.Lookup, snapshot.Input, grounding)
	case KindCustom:
		return e.resolveCustom(ctx, slot.Custom, snapshot)
	}
	// Validate rejects any other slot Kind; reaching here means the spec bypassed it.
	return Value{}, &Failure{Reason: ReasonBackendError, Err: fmt.Errorf("intents: slot_dag: slot %q: %w", slot.Name, ErrInvalidSpec)}
}

// dependenciesResolved reports whether every name in slot.DependsOn has a resolved Value.
func dependenciesResolved(slot *SlotSpec, values map[string]Value) bool {
	for _, dep := range slot.DependsOn {
		if _, ok := values[dep]; !ok {
			return false
		}
	}
	return true
}

// cloneValues shallow-copies m; Value structs are copied by value, their pointees are shared
// (they are never mutated after construction). A nil m yields an empty, writable map.
func cloneValues(m map[string]Value) map[string]Value {
	out := make(map[string]Value, len(m))
	maps.Copy(out, m)
	return out
}
