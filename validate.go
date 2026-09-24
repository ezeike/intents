package intents

import (
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"
)

// Validate checks a spec tree for construction-time errors: bad keys, duplicate keys along a
// root-to-node path, tree cycles, kind/field mismatches, out-of-range confidences, unreachable
// children, and slot_dag dependency problems. It reports every problem it finds, joined with
// errors.Join; each wraps ErrInvalidSpec, and a slot_dag dependency cycle also wraps
// ErrCyclicSlotDAG. Validate never caches — every call re-walks the tree.
func Validate(spec *Node) error {
	if spec == nil {
		return fmt.Errorf("intents: nil spec: %w", ErrInvalidSpec)
	}
	v := &validator{}
	v.walk(spec)
	return errors.Join(v.problems...)
}

// validator carries the current root-to-node path during Validate's depth-first walk. keys and
// nodes are parallel stacks; slices (not sets) so a duplicated key pops cleanly.
type validator struct {
	keys     []string
	nodes    []*Node
	problems []error
}

func (v *validator) addf(format string, args ...any) {
	v.problems = append(v.problems, fmt.Errorf("intents: "+format+": %w", append(args, ErrInvalidSpec)...))
}

func (v *validator) walk(n *Node) {
	at := strings.Join(append(slices.Clone(v.keys), n.Key), ".")

	if n.Key == "" || strings.Contains(n.Key, ".") {
		v.addf("node %q: key must be non-empty and contain no '.'", at)
	}
	if slices.Contains(v.keys, n.Key) {
		v.addf("node %q: key %q duplicated along its path", at, n.Key)
	}
	if slices.Contains(v.nodes, n) {
		// This node's fields were already checked at its first occurrence on the path; stop here
		// so the cycle is reported once and the walk terminates.
		v.addf("node %q: node appears in its own subtree (tree cycle)", at)
		return
	}

	subject := fmt.Sprintf("node %q", at)
	knownKind := isNodeKind(n.Kind)
	if !knownKind {
		v.addf("node %q: unknown kind %q", at, n.Kind)
	} else {
		v.checkKindFields(subject, n.Kind, n.Classify, n.Lookup, len(n.Slots) != 0, n.Custom != nil)
	}

	for _, k := range slices.Sorted(maps.Keys(n.Children)) {
		if n.Children[k] == nil {
			v.addf("node %q: child %q is nil", at, k)
		}
	}

	if knownKind {
		switch n.Kind {
		case KindClassify:
			if n.Classify != nil {
				v.checkClassify(subject, n.Classify)
				v.checkChildrenInOptions(at, n)
			}
		case KindLookup:
			if n.Lookup != nil {
				v.checkLookup(subject, n.Lookup)
			}
		case KindSlotDAG:
			if len(n.Children) != 0 {
				v.addf("node %q: slot_dag with Children is unreachable", at)
			}
			v.checkSlots(at, n)
		}
	}

	v.keys = append(v.keys, n.Key)
	v.nodes = append(v.nodes, n)
	for _, k := range slices.Sorted(maps.Keys(n.Children)) {
		if c := n.Children[k]; c != nil {
			v.walk(c)
		}
	}
	v.keys = v.keys[:len(v.keys)-1]
	v.nodes = v.nodes[:len(v.nodes)-1]
}

func isNodeKind(k NodeKind) bool {
	switch k {
	case KindClassify, KindLookup, KindSlotDAG, KindCustom:
		return true
	}
	return false
}

// checkKindFields applies rules 5 and 6 (the kind's own field present, no other kind's field
// set) to either a Node or a SlotSpec; subject names it in the problem message.
func (v *validator) checkKindFields(subject string, kind NodeKind, classify *ClassifySpec, lookup *LookupSpec, hasSlots, hasCustom bool) {
	present := map[NodeKind]bool{
		KindClassify: classify != nil,
		KindLookup:   lookup != nil,
		KindSlotDAG:  hasSlots,
		KindCustom:   hasCustom,
	}
	if !present[kind] {
		v.addf("%s: kind %q requires its own spec field to be set", subject, kind)
	}
	var foreign []string
	for _, k := range []NodeKind{KindClassify, KindLookup, KindSlotDAG, KindCustom} {
		if k != kind && present[k] {
			foreign = append(foreign, string(k))
		}
	}
	if len(foreign) != 0 {
		v.addf("%s: kind %q has fields of other kinds set: %s", subject, kind, strings.Join(foreign, ", "))
	}
}

func (v *validator) checkClassify(subject string, c *ClassifySpec) {
	if c.Backend == "" {
		v.addf("%s: classify backend is empty", subject)
	}
	if len(c.Options) == 0 {
		v.addf("%s: classify options are empty", subject)
	}
	if mc := c.MinConfidence; math.IsNaN(mc) || mc < 0 || mc > 1 {
		v.addf("%s: classify min confidence %v outside [0,1]", subject, mc)
	}
	if c.Fallback != nil {
		v.checkLookup(subject+" fallback", c.Fallback)
	}
}

func (v *validator) checkLookup(subject string, l *LookupSpec) {
	if l.Backend == "" {
		v.addf("%s: lookup backend is empty", subject)
	}
}

func (v *validator) checkChildrenInOptions(at string, n *Node) {
	if len(n.Children) == 0 || n.Classify.Fallback != nil {
		return
	}
	var missing []string
	for _, k := range slices.Sorted(maps.Keys(n.Children)) {
		if _, ok := n.Classify.Options[k]; !ok {
			missing = append(missing, k)
		}
	}
	if len(missing) != 0 {
		v.addf("node %q: children keys not in options and no fallback set: %s", at, strings.Join(missing, ", "))
	}
}

func (v *validator) checkSlots(at string, n *Node) {
	names := make(map[string]bool, len(n.Slots))
	for _, s := range n.Slots {
		names[s.Name] = true
	}
	seen := make(map[string]bool, len(n.Slots))
	for i, s := range n.Slots {
		subject := fmt.Sprintf("node %q slot %d (%q)", at, i, s.Name)
		if s.Name == "" || strings.Contains(s.Name, ".") {
			v.addf("%s: slot name must be non-empty and contain no '.'", subject)
		}
		if seen[s.Name] {
			v.addf("%s: slot name duplicated within slot_dag", subject)
		}
		seen[s.Name] = true

		switch s.Kind {
		case KindClassify, KindLookup, KindCustom:
			v.checkKindFields(subject, s.Kind, s.Classify, s.Lookup, false, s.Custom != nil)
			if s.Kind == KindClassify && s.Classify != nil {
				v.checkClassify(subject, s.Classify)
			}
			if s.Kind == KindLookup && s.Lookup != nil {
				v.checkLookup(subject, s.Lookup)
			}
		default:
			v.addf("%s: slot kind %q must be classify, lookup, or custom", subject, s.Kind)
		}

		for _, dep := range s.DependsOn {
			if dep == s.Name {
				v.addf("%s: slot depends on itself", subject)
			} else if !names[dep] {
				v.addf("%s: slot depends on unknown slot %q", subject, dep)
			}
		}
	}
	if _, err := slotWave(n.Slots); err != nil {
		v.problems = append(v.problems, err)
	}
}

// slotWave groups slot indices into execution waves by topological level over DependsOn: wave
// k holds every slot whose longest dependency chain has length k+1, in declaration order. It
// is the package's single topological sort, shared by Validate and the slot_dag executor.
func slotWave(slots []SlotSpec) (waves [][]int, err error) {
	index := make(map[string]int, len(slots))
	for i, s := range slots {
		index[s.Name] = i
	}
	deps := make([][]int, len(slots))
	for i, s := range slots {
		for _, dep := range s.DependsOn {
			j, ok := index[dep]
			if !ok {
				return nil, fmt.Errorf("intents: slot_dag: slot %q depends on unknown slot %q: %w", s.Name, dep, ErrInvalidSpec)
			}
			deps[i] = append(deps[i], j)
		}
	}

	const (
		white = iota
		gray
		black
	)
	color := make([]int, len(slots))
	level := make([]int, len(slots))
	var visit func(i int) error
	visit = func(i int) error {
		switch color[i] {
		case gray:
			return fmt.Errorf("intents: %w: %w", ErrCyclicSlotDAG, ErrInvalidSpec)
		case black:
			return nil
		}
		color[i] = gray
		lvl := 1
		for _, j := range deps[i] {
			if err := visit(j); err != nil {
				return err
			}
			lvl = max(lvl, level[j]+1)
		}
		level[i] = lvl
		color[i] = black
		return nil
	}

	maxLevel := 0
	for i := range slots {
		if err := visit(i); err != nil {
			return nil, err
		}
		maxLevel = max(maxLevel, level[i])
	}
	waves = make([][]int, maxLevel)
	for i := range slots {
		waves[level[i]-1] = append(waves[level[i]-1], i)
	}
	return waves, nil
}
