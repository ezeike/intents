package examples

import "context"

// SlotSpec is one field to resolve from an input, and how to resolve it. See
// docs/architecture.md §8. Slots with no DependsOn can resolve in the same wave; classify-type
// slots in a wave that share the same input state should be batched into a single call (see
// docs/architecture.md §1) rather than issued as separate requests.
type SlotSpec struct {
	Name      string
	Resolver  string // "classify" | "lookup"
	Options   map[string]string
	Index     string
	DependsOn []string
}

// SlotValue is a resolved slot.
type SlotValue struct {
	Name       string
	Source     string // "classify" | "lookup"
	Result     string
	Confidence float64
}

// ResolveDAG resolves a set of slots wave by wave, respecting DependsOn. Each wave's
// classify-type slots are batched into one classifyBatch call; lookup-type slots in a wave run
// concurrently via search. This is a synchronous sketch — a real implementation would run each
// wave's lookups with errgroup or similar.
func ResolveDAG(
	ctx context.Context,
	input string,
	slots []SlotSpec,
	classifyBatch func(ctx context.Context, input string, batch []SlotSpec) (map[string]ChoiceAnswer, error),
	search Searcher,
) (map[string]SlotValue, error) {
	resolved := map[string]SlotValue{}

	for len(resolved) < len(slots) {
		wave := readyWave(slots, resolved)
		if len(wave) == 0 {
			return resolved, errCycle()
		}

		var classifyWave []SlotSpec
		for _, s := range wave {
			if s.Resolver == "classify" {
				classifyWave = append(classifyWave, s)
			}
		}
		if len(classifyWave) > 0 {
			answers, err := classifyBatch(ctx, input, classifyWave)
			if err != nil {
				return resolved, err
			}
			for _, s := range classifyWave {
				ans := answers[s.Name]
				resolved[s.Name] = SlotValue{Name: s.Name, Source: "classify", Result: ans.Choice, Confidence: ans.Confidence}
			}
		}

		for _, s := range wave {
			if s.Resolver != "lookup" {
				continue
			}
			entity, err := search(ctx, input, s.Index)
			if err != nil {
				return resolved, err
			}
			resolved[s.Name] = SlotValue{Name: s.Name, Source: "lookup", Result: entity}
		}
	}

	return resolved, nil
}

// readyWave returns slots whose dependencies are all already resolved, and which are not yet
// resolved themselves.
func readyWave(slots []SlotSpec, resolved map[string]SlotValue) []SlotSpec {
	var wave []SlotSpec
	for _, s := range slots {
		if _, done := resolved[s.Name]; done {
			continue
		}
		ready := true
		for _, dep := range s.DependsOn {
			if _, ok := resolved[dep]; !ok {
				ready = false
				break
			}
		}
		if ready {
			wave = append(wave, s)
		}
	}
	return wave
}

func errCycle() error {
	return &treeError{"intents: slot dependency cycle or unresolved dependency"}
}
