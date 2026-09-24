// resume.go
package intents

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
)

// Answer resolves one prior Failure. Value, when non-nil, is recorded directly with no backend
// call; otherwise Raw is new free text the failed node/slot's backend is re-run against.
type Answer struct {
	Value *Value
	Raw   string
}

// Answers maps a prior Failure.Path to the Answer that resolves it.
type Answers map[string]Answer

// ResumeState continues prior (never mutated) by applying answers, each keyed by the Path of a
// Failure in prior.Failures (spec.md R11). Every required, non-unresolved Failure must be
// answered. An answered tree node continues descending through its Children; an answered slot
// re-runs only the ReasonUnresolved slots of its slot_dag that it unblocks. Input and Context
// are carried over unchanged. Remaining required Failures return *IncompleteError (unwrapped).
func (e *Engine) ResumeState(ctx context.Context, spec *Node, prior State, answers Answers) (State, error) {
	if err := e.validateCached(spec); err != nil {
		return State{}, err
	}
	if err := checkBackendsRegistered(e, spec); err != nil {
		return State{}, err
	}

	state := State{
		Input:    prior.Input,
		Context:  prior.Context,
		Values:   cloneValues(prior.Values),
		Terminal: prior.Terminal,
		Failures: slices.Clone(prior.Failures),
	}

	for path := range answers {
		if !slices.ContainsFunc(state.Failures, func(f Failure) bool { return f.Path == path }) {
			return State{}, fmt.Errorf("intents: resume: path %q: %w", path, ErrUnknownAnswerPath)
		}
	}

	required := make(map[string]bool)
	collectRequiredPaths(spec, spec.Key, required)
	for _, f := range state.Failures {
		if !required[f.Path] || f.Reason == ReasonUnresolved {
			continue
		}
		if ans, ok := answers[f.Path]; !ok || (ans.Value == nil && ans.Raw == "") {
			return State{}, fmt.Errorf("intents: resume: path %q: %w", f.Path, ErrUnansweredFailure)
		}
	}

	for _, path := range slices.Sorted(maps.Keys(answers)) {
		if ans := answers[path]; ans.Value == nil && ans.Raw == "" {
			continue // an empty Answer answers nothing; its Failure stays (only reachable for exempt ones)
		}
		if err := e.applyAnswer(ctx, spec, &state, path, answers[path]); err != nil {
			return state, fmt.Errorf("intents: resume: %w", err)
		}
	}

	state.RiskTier = maxValueRiskTier(state.Values)
	if len(requiredFailuresIn(spec, state.Failures)) > 0 {
		return state, &IncompleteError{State: state}
	}
	return state, nil
}

// applyAnswer records one answer at path and continues resolution from it. It returns a non-nil
// error only for ctx.Err() or a path that no longer maps onto spec (an internal invariant).
func (e *Engine) applyAnswer(ctx context.Context, spec *Node, state *State, path string, ans Answer) error {
	node, slot, slotParent, err := resolvePathToNode(spec, path)
	if err != nil {
		return err
	}
	state.Failures = slices.DeleteFunc(state.Failures, func(f Failure) bool { return f.Path == path })

	// target is the single-node view resolveSlot dispatches on, for a tree node or a slot alike.
	var (
		target    *SlotSpec
		targetKey string
		tier      RiskTier
	)
	if node != nil {
		target = &SlotSpec{Name: node.Key, Kind: node.Kind, Classify: node.Classify, Lookup: node.Lookup, Custom: node.Custom}
		targetKey = node.Key
		tier = effectiveRiskTier(node.RiskTier, ancestorTierAt(spec, path))
	} else {
		target = slot
		targetKey = slot.Name
		tier = slotDAGTier(spec, slotParent, parentPath(path))
	}

	var v Value
	if ans.Value != nil {
		v = *ans.Value
	} else {
		callState := *state
		callState.Input = ans.Raw
		callState.Values = cloneValues(state.Values)
		callState.Failures = slices.Clone(state.Failures)
		var f *Failure
		v, f = e.resolveSlot(ctx, target, callState)
		if err := ctx.Err(); err != nil {
			return err
		}
		if f != nil {
			f.Path = path
			state.Failures = append(state.Failures, *f)
			return nil // a re-run miss is recorded, not returned; nothing downstream is unblocked
		}
	}
	v.RiskTier = effectiveRiskTier(v.RiskTier, tier)
	state.Values[targetKey] = v

	if node != nil {
		if child, ok := node.Children[branchKey(v)]; ok && child != nil {
			return e.walkTreeFrom(ctx, child, state, path+"."+child.Key, tier)
		}
		state.Terminal = node.Key
		return nil
	}
	return e.resumeUnresolvedSlots(ctx, slotParent, state, parentPath(path), tier)
}

// resumeUnresolvedSlots re-runs, in topological wave order, each slot of n still carrying a
// ReasonUnresolved Failure whose dependencies now all have Values. Slots with a Value are never
// touched. One ordered pass reaches the fixpoint: a slot unblocked by an earlier-wave re-run is
// always visited after it. Slots run sequentially — a resume re-enters only the few slots an
// answer unblocked, so per-wave fan-out buys nothing here.
func (e *Engine) resumeUnresolvedSlots(ctx context.Context, n *Node, state *State, path string, tier RiskTier) error {
	waves, err := slotWave(n.Slots)
	if err != nil {
		return fmt.Errorf("intents: slot_dag: %w", err)
	}
	for _, wave := range waves {
		for _, idx := range wave {
			slot := &n.Slots[idx]
			slotPath := path + "." + slot.Name
			unresolved := func(f Failure) bool { return f.Path == slotPath && f.Reason == ReasonUnresolved }
			if !slices.ContainsFunc(state.Failures, unresolved) || !dependenciesResolved(slot, state.Values) {
				continue
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			snapshot := *state
			snapshot.Values = cloneValues(state.Values)
			snapshot.Failures = slices.Clone(state.Failures)
			v, f := e.resolveSlot(ctx, slot, snapshot)
			if err := ctx.Err(); err != nil {
				return err
			}
			state.Failures = slices.DeleteFunc(state.Failures, unresolved)
			if f != nil {
				f.Path = slotPath
				state.Failures = append(state.Failures, *f)
				continue
			}
			v.RiskTier = tier
			state.Values[slot.Name] = v
		}
	}
	return nil
}

// resolvePathToNode maps a Failure.Path back onto spec (spec.md R11 "Path mapping"). Each
// segment after the root matches a child's Key; a last segment matching no child of a slot_dag
// node matches a slot Name instead, returning (nil, slot, slotDAGNode, nil).
func resolvePathToNode(spec *Node, path string) (node *Node, slot *SlotSpec, slotParent *Node, err error) {
	segments := strings.Split(path, ".")
	if spec == nil || segments[0] != spec.Key {
		return nil, nil, nil, fmt.Errorf("intents: resume: path %q: %w", path, ErrUnknownAnswerPath)
	}
	cur := spec
	for i, seg := range segments[1:] {
		if child := childByKey(cur, seg); child != nil {
			cur = child
			continue
		}
		if i == len(segments)-2 && cur.Kind == KindSlotDAG {
			for j := range cur.Slots {
				if cur.Slots[j].Name == seg {
					return nil, &cur.Slots[j], cur, nil
				}
			}
		}
		return nil, nil, nil, fmt.Errorf("intents: resume: path %q: %w", path, ErrUnknownAnswerPath)
	}
	return cur, nil, nil, nil
}

// childByKey returns n's child whose Key is key, scanning Children in sorted map-key order so a
// (Validate-permitted) duplicate Key resolves deterministically.
func childByKey(n *Node, key string) *Node {
	for _, k := range slices.Sorted(maps.Keys(n.Children)) {
		if c := n.Children[k]; c != nil && c.Key == key {
			return c
		}
	}
	return nil
}

// ancestorTierAt returns the RiskTier inherited by the tree node at path: the max tier along
// the root-to-path walk, excluding the target node's own tier. An unmappable path yields RiskLow.
func ancestorTierAt(spec *Node, path string) RiskTier {
	segments := strings.Split(path, ".")
	if spec == nil || segments[0] != spec.Key {
		return RiskLow
	}
	var tier RiskTier
	cur := spec
	for _, seg := range segments[1:] {
		tier = effectiveRiskTier(cur.RiskTier, tier)
		if cur = childByKey(cur, seg); cur == nil {
			return tier
		}
	}
	return tier
}

// slotDAGTier is the tier resolveSlotDAGFrom stamps on every slot of n, which sits at path.
func slotDAGTier(spec, n *Node, path string) RiskTier {
	return effectiveRiskTier(n.RiskTier, ancestorTierAt(spec, path))
}

// parentPath drops the last dot-segment of path.
func parentPath(path string) string {
	if i := strings.LastIndexByte(path, '.'); i >= 0 {
		return path[:i]
	}
	return path
}
