package intents

import (
	"context"
	"errors"
	"fmt"
)

// choiceOther is the literal reject choice (spec.md R4): a classifier answering "other" is a
// reject regardless of whether "other" is declared in ClassifySpec.Options.
const choiceOther = "other"

// resolveClassify resolves one classify spec against its registered Classifier. It returns
// exactly one of a non-zero Value or a non-nil *Failure. The caller (walkTree) has already
// verified cs.Backend is registered and computed grounding from cs.BuildContext.
func (e *Engine) resolveClassify(ctx context.Context, cs *ClassifySpec, input string, grounding any) (Value, *Failure) {
	c, _ := e.classifier(cs.Backend) // registration pre-checked by the caller (spec.md R9)
	ans, err := c.Classify(ctx, input, grounding)
	if err != nil {
		return Value{}, &Failure{Reason: ReasonBackendError, Err: fmt.Errorf("intents: classify: %w", err)}
	}

	choice := Value{Kind: ValueKindChoice, Choice: ans.Choice, Confidence: ans.Confidence, Source: cs.Backend}
	lowConfidence := cs.MinConfidence != 0 && ans.Confidence < cs.MinConfidence
	reject := ans.Choice == choiceOther

	if lowConfidence || reject {
		if cs.Fallback != nil {
			// The fallback receives the classify node's grounding: walkTree is the only place a
			// BuildContext runs, and this signature carries a single grounding value.
			v, f := e.resolveLookup(ctx, cs.Fallback, input, grounding)
			if f == nil {
				return v, nil
			}
			return Value{}, &Failure{Reason: f.Reason, Err: f.Err, Candidate: &choice}
		}
		if lowConfidence {
			return Value{}, &Failure{Reason: ReasonLowConfidence, Candidate: &choice}
		}
		return Value{}, &Failure{Reason: ReasonNotFound, Candidate: &choice}
	}

	if _, declared := cs.Options[ans.Choice]; !declared {
		return Value{}, &Failure{Reason: ReasonBackendError, Err: fmt.Errorf("intents: classify: %w", ErrUnknownChoice)}
	}
	return choice, nil
}

// branchKey returns the Children key a resolved Value descends into (spec.md R3).
func branchKey(v Value) string {
	switch v.Kind {
	case ValueKindChoice:
		return v.Choice
	case ValueKindEntity:
		if v.Entity == nil {
			return ""
		}
		return v.Entity.ID
	case ValueKindRaw:
		return v.Raw
	}
	return ""
}

// walkTree resolves n and descends through its Children from the root risk tier.
func (e *Engine) walkTree(ctx context.Context, n *Node, state *State, path string) error {
	return e.walkTreeFrom(ctx, n, state, path, RiskLow)
}

// walkTreeFrom is walkTree's recursive descent, threading the inherited ancestor RiskTier. It
// returns a non-nil error only for ctx.Err(); every other miss is recorded in state.Failures.
func (e *Engine) walkTreeFrom(ctx context.Context, n *Node, state *State, path string, ancestorTier RiskTier) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if n.Kind == KindSlotDAG {
		return e.resolveSlotDAG(ctx, n, state, path)
	}

	var buildContext func(State) (any, error)
	switch n.Kind {
	case KindClassify:
		buildContext = n.Classify.BuildContext
	case KindLookup:
		buildContext = n.Lookup.BuildContext
	}
	grounding := state.Context
	if buildContext != nil {
		g, err := buildContext(*state)
		if err != nil {
			state.Failures = append(state.Failures, Failure{
				Path:   path,
				Reason: ReasonBackendError,
				Err:    fmt.Errorf("intents: build context: %w", err),
			})
			return nil
		}
		grounding = g
	}

	var (
		v Value
		f *Failure
	)
	switch n.Kind {
	case KindClassify:
		v, f = e.resolveClassify(ctx, n.Classify, state.Input, grounding)
	case KindLookup:
		v, f = e.resolveLookup(ctx, n.Lookup, state.Input, grounding)
	case KindCustom:
		v, f = e.resolveCustom(ctx, n.Custom, *state)
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	if f != nil {
		f.Path = path
		state.Failures = append(state.Failures, *f)
		return nil
	}

	tier := effectiveRiskTier(n.RiskTier, ancestorTier)
	v.RiskTier = tier
	if state.Values == nil {
		state.Values = make(map[string]Value)
	}
	state.Values[n.Key] = v

	if child, ok := n.Children[branchKey(v)]; ok && child != nil {
		return e.walkTreeFrom(ctx, child, state, path+"."+child.Key, tier)
	}
	state.Terminal = n.Key
	return nil
}

// effectiveRiskTier returns the stricter of a node's own tier and its inherited ancestor tier.
func effectiveRiskTier(nodeTier, ancestorTier RiskTier) RiskTier {
	if nodeTier > ancestorTier {
		return nodeTier
	}
	return ancestorTier
}

// resolveLookup resolves one lookup spec against its registered Lookuper (spec.md R4). A lookup
// Failure never carries a Candidate; the caller has already verified ls.Backend is registered.
func (e *Engine) resolveLookup(ctx context.Context, ls *LookupSpec, query string, grounding any) (Value, *Failure) {
	l, _ := e.lookuper(ls.Backend) // registration pre-checked by the caller (spec.md R9)
	entity, err := l.Lookup(ctx, query, ls.Index, grounding)
	if err != nil {
		reason := ReasonBackendError
		if errors.Is(err, ErrNotFound) {
			reason = ReasonNotFound
		}
		return Value{}, &Failure{Reason: reason, Err: fmt.Errorf("intents: lookup: %w", err)}
	}
	if entity.ID == "" {
		return Value{}, &Failure{Reason: ReasonNotFound, Err: fmt.Errorf("intents: lookup: %w", ErrNotFound)}
	}
	return Value{Kind: ValueKindEntity, Entity: &entity, Confidence: 0, Source: ls.Backend}, nil
}
