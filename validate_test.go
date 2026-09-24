// validate_test.go
package intents_test

import (
	"errors"
	"math"
	"testing"

	"github.com/ezeike/intents"
)

func validClassifyLeaf(key string) *intents.Node {
	return &intents.Node{
		Key:  key,
		Kind: intents.KindClassify,
		Classify: &intents.ClassifySpec{
			Backend: "laya",
			Options: map[string]string{"a": "option a", "b": "option b"},
		},
	}
}

func TestValidate_NilSpec_ReturnsErrInvalidSpec(t *testing.T) {
	err := intents.Validate(nil)
	if !errors.Is(err, intents.ErrInvalidSpec) {
		t.Fatalf("Validate(nil) = %v, want errors.Is ErrInvalidSpec", err)
	}
}

func TestValidate_EmptyKey_ReturnsErrInvalidSpec(t *testing.T) {
	n := validClassifyLeaf("")
	if err := intents.Validate(n); !errors.Is(err, intents.ErrInvalidSpec) {
		t.Fatalf("Validate() = %v, want errors.Is ErrInvalidSpec", err)
	}
}

func TestValidate_KeyContainsDot_ReturnsErrInvalidSpec(t *testing.T) {
	n := validClassifyLeaf("bad.key")
	if err := intents.Validate(n); !errors.Is(err, intents.ErrInvalidSpec) {
		t.Fatalf("Validate() = %v, want errors.Is ErrInvalidSpec", err)
	}
}

func TestValidate_DuplicateKeyAlongPath_ReturnsErrInvalidSpec(t *testing.T) {
	leaf := validClassifyLeaf("root") // same Key as its ancestor
	root := &intents.Node{
		Key:      "root",
		Kind:     intents.KindClassify,
		Classify: &intents.ClassifySpec{Backend: "laya", Options: map[string]string{"a": "opt"}},
		Children: map[string]*intents.Node{"a": leaf},
	}
	if err := intents.Validate(root); !errors.Is(err, intents.ErrInvalidSpec) {
		t.Fatalf("Validate() = %v, want errors.Is ErrInvalidSpec", err)
	}
}

func TestValidate_NodeCycle_ReturnsErrInvalidSpec(t *testing.T) {
	root := &intents.Node{Key: "root", Kind: intents.KindClassify}
	root.Classify = &intents.ClassifySpec{Backend: "laya", Options: map[string]string{"a": "opt"}}
	root.Children = map[string]*intents.Node{"a": root} // self-reference: tree cycle
	if err := intents.Validate(root); !errors.Is(err, intents.ErrInvalidSpec) {
		t.Fatalf("Validate() = %v, want errors.Is ErrInvalidSpec", err)
	}
}

func TestValidate_DuplicateChildKeyAcrossSiblings_ReturnsErrInvalidSpec(t *testing.T) {
	root := &intents.Node{
		Key:      "root",
		Kind:     intents.KindClassify,
		Classify: &intents.ClassifySpec{Backend: "laya", Options: map[string]string{"a": "opt a", "b": "opt b"}},
		Children: map[string]*intents.Node{
			"a": validClassifyLeaf("leaf"),
			"b": validClassifyLeaf("leaf"), // distinct pointer, same Key as its sibling
		},
	}
	if err := intents.Validate(root); !errors.Is(err, intents.ErrInvalidSpec) {
		t.Fatalf("Validate() = %v, want errors.Is ErrInvalidSpec", err)
	}
}

func TestValidate_SameNodeAliasedUnderTwoBranches_ReturnsNil(t *testing.T) {
	shared := validClassifyLeaf("leaf")
	root := &intents.Node{
		Key:      "root",
		Kind:     intents.KindClassify,
		Classify: &intents.ClassifySpec{Backend: "laya", Options: map[string]string{"a": "opt a", "b": "opt b"}},
		Children: map[string]*intents.Node{"a": shared, "b": shared},
	}
	if err := intents.Validate(root); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
}

func TestValidate_UnknownKind_ReturnsErrInvalidSpec(t *testing.T) {
	n := &intents.Node{Key: "root", Kind: intents.NodeKind("bogus")}
	if err := intents.Validate(n); !errors.Is(err, intents.ErrInvalidSpec) {
		t.Fatalf("Validate() = %v, want errors.Is ErrInvalidSpec", err)
	}
}

func TestValidate_ClassifyMissingClassifyField_ReturnsErrInvalidSpec(t *testing.T) {
	n := &intents.Node{Key: "root", Kind: intents.KindClassify}
	if err := intents.Validate(n); !errors.Is(err, intents.ErrInvalidSpec) {
		t.Fatalf("Validate() = %v, want errors.Is ErrInvalidSpec", err)
	}
}

func TestValidate_ClassifyWithLookupFieldSet_ReturnsErrInvalidSpec(t *testing.T) {
	n := validClassifyLeaf("root")
	n.Lookup = &intents.LookupSpec{Backend: "sqllookup"}
	if err := intents.Validate(n); !errors.Is(err, intents.ErrInvalidSpec) {
		t.Fatalf("Validate() = %v, want errors.Is ErrInvalidSpec", err)
	}
}

func TestValidate_NilChildrenEntry_ReturnsErrInvalidSpec(t *testing.T) {
	root := validClassifyLeaf("root")
	root.Children = map[string]*intents.Node{"a": nil}
	if err := intents.Validate(root); !errors.Is(err, intents.ErrInvalidSpec) {
		t.Fatalf("Validate() = %v, want errors.Is ErrInvalidSpec", err)
	}
}

func TestValidate_ClassifyEmptyBackend_ReturnsErrInvalidSpec(t *testing.T) {
	n := validClassifyLeaf("root")
	n.Classify.Backend = ""
	if err := intents.Validate(n); !errors.Is(err, intents.ErrInvalidSpec) {
		t.Fatalf("Validate() = %v, want errors.Is ErrInvalidSpec", err)
	}
}

func TestValidate_ClassifyEmptyOptions_ReturnsErrInvalidSpec(t *testing.T) {
	n := validClassifyLeaf("root")
	n.Classify.Options = nil
	if err := intents.Validate(n); !errors.Is(err, intents.ErrInvalidSpec) {
		t.Fatalf("Validate() = %v, want errors.Is ErrInvalidSpec", err)
	}
}

func TestValidate_MinConfidenceBelowZero_ReturnsErrInvalidSpec(t *testing.T) {
	n := validClassifyLeaf("root")
	n.Classify.MinConfidence = -0.1
	if err := intents.Validate(n); !errors.Is(err, intents.ErrInvalidSpec) {
		t.Fatalf("Validate() = %v, want errors.Is ErrInvalidSpec", err)
	}
}

func TestValidate_MinConfidenceAboveOne_ReturnsErrInvalidSpec(t *testing.T) {
	n := validClassifyLeaf("root")
	n.Classify.MinConfidence = 1.1
	if err := intents.Validate(n); !errors.Is(err, intents.ErrInvalidSpec) {
		t.Fatalf("Validate() = %v, want errors.Is ErrInvalidSpec", err)
	}
}

func TestValidate_MinConfidenceNaN_ReturnsErrInvalidSpec(t *testing.T) {
	n := validClassifyLeaf("root")
	n.Classify.MinConfidence = math.NaN()
	if err := intents.Validate(n); !errors.Is(err, intents.ErrInvalidSpec) {
		t.Fatalf("Validate() = %v, want errors.Is ErrInvalidSpec", err)
	}
}

func TestValidate_ClassifyChildKeyNotInOptionsWithNoFallback_ReturnsErrInvalidSpec(t *testing.T) {
	root := validClassifyLeaf("root")
	root.Children = map[string]*intents.Node{"not_an_option": validClassifyLeaf("child")}
	if err := intents.Validate(root); !errors.Is(err, intents.ErrInvalidSpec) {
		t.Fatalf("Validate() = %v, want errors.Is ErrInvalidSpec", err)
	}
}

func TestValidate_ClassifyChildKeyNotInOptionsWithFallbackSet_ReturnsNil(t *testing.T) {
	root := validClassifyLeaf("root")
	root.Classify.Fallback = &intents.LookupSpec{Backend: "sqllookup"}
	root.Children = map[string]*intents.Node{"not_an_option": validClassifyLeaf("child")}
	if err := intents.Validate(root); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
}

func TestValidate_LookupEmptyBackend_ReturnsErrInvalidSpec(t *testing.T) {
	n := &intents.Node{Key: "root", Kind: intents.KindLookup, Lookup: &intents.LookupSpec{}}
	if err := intents.Validate(n); !errors.Is(err, intents.ErrInvalidSpec) {
		t.Fatalf("Validate() = %v, want errors.Is ErrInvalidSpec", err)
	}
}

func TestValidate_LookupEmptyIndex_ReturnsNil(t *testing.T) {
	n := &intents.Node{Key: "root", Kind: intents.KindLookup, Lookup: &intents.LookupSpec{Backend: "sqllookup"}}
	if err := intents.Validate(n); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
}

func TestValidate_SlotDAGEmptySlots_ReturnsErrInvalidSpec(t *testing.T) {
	n := &intents.Node{Key: "root", Kind: intents.KindSlotDAG}
	if err := intents.Validate(n); !errors.Is(err, intents.ErrInvalidSpec) {
		t.Fatalf("Validate() = %v, want errors.Is ErrInvalidSpec", err)
	}
}

func TestValidate_SlotDAGWithChildren_ReturnsErrInvalidSpec(t *testing.T) {
	n := &intents.Node{
		Key:  "root",
		Kind: intents.KindSlotDAG,
		Slots: []intents.SlotSpec{
			{Name: "amount", Kind: intents.KindClassify, Classify: &intents.ClassifySpec{Backend: "laya", Options: map[string]string{"a": "opt"}}},
		},
		Children: map[string]*intents.Node{"x": validClassifyLeaf("x")},
	}
	if err := intents.Validate(n); !errors.Is(err, intents.ErrInvalidSpec) {
		t.Fatalf("Validate() = %v, want errors.Is ErrInvalidSpec", err)
	}
}

func TestValidate_SlotEmptyName_ReturnsErrInvalidSpec(t *testing.T) {
	n := &intents.Node{
		Key:  "root",
		Kind: intents.KindSlotDAG,
		Slots: []intents.SlotSpec{
			{Name: "", Kind: intents.KindClassify, Classify: &intents.ClassifySpec{Backend: "laya", Options: map[string]string{"a": "opt"}}},
		},
	}
	if err := intents.Validate(n); !errors.Is(err, intents.ErrInvalidSpec) {
		t.Fatalf("Validate() = %v, want errors.Is ErrInvalidSpec", err)
	}
}

func TestValidate_DuplicateSlotName_ReturnsErrInvalidSpec(t *testing.T) {
	classify := &intents.ClassifySpec{Backend: "laya", Options: map[string]string{"a": "opt"}}
	n := &intents.Node{
		Key:  "root",
		Kind: intents.KindSlotDAG,
		Slots: []intents.SlotSpec{
			{Name: "amount", Kind: intents.KindClassify, Classify: classify},
			{Name: "amount", Kind: intents.KindClassify, Classify: classify},
		},
	}
	if err := intents.Validate(n); !errors.Is(err, intents.ErrInvalidSpec) {
		t.Fatalf("Validate() = %v, want errors.Is ErrInvalidSpec", err)
	}
}

// A slot name may equal its own immediate slot_dag node's Key: that node never writes
// State.Values under its own Key, so the two cannot collide.
func TestValidate_SlotNameEqualsNodeKey_ReturnsNil(t *testing.T) {
	n := &intents.Node{
		Key:  "amount",
		Kind: intents.KindSlotDAG,
		Slots: []intents.SlotSpec{
			{Name: "amount", Kind: intents.KindClassify, Classify: &intents.ClassifySpec{Backend: "laya", Options: map[string]string{"a": "opt"}}},
		},
	}
	if err := intents.Validate(n); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
}

// A slot name equal to a higher ancestor's Key would share that ancestor's State.Values entry.
func TestValidate_SlotNameEqualsAncestorNodeKey_ReturnsErrInvalidSpec(t *testing.T) {
	dag := &intents.Node{
		Key:  "transfer",
		Kind: intents.KindSlotDAG,
		Slots: []intents.SlotSpec{
			{Name: "payments", Kind: intents.KindClassify, Classify: &intents.ClassifySpec{Backend: "laya", Options: map[string]string{"a": "opt"}}},
		},
	}
	root := &intents.Node{
		Key:      "payments",
		Kind:     intents.KindClassify,
		Classify: &intents.ClassifySpec{Backend: "laya", Options: map[string]string{"transfer": "transfer"}},
		Children: map[string]*intents.Node{"transfer": dag},
	}
	if err := intents.Validate(root); !errors.Is(err, intents.ErrInvalidSpec) {
		t.Fatalf("Validate() = %v, want errors.Is ErrInvalidSpec", err)
	}
}

func TestValidate_SlotDependsOnUnknownSlot_ReturnsErrInvalidSpec(t *testing.T) {
	classify := &intents.ClassifySpec{Backend: "laya", Options: map[string]string{"a": "opt"}}
	n := &intents.Node{
		Key:  "root",
		Kind: intents.KindSlotDAG,
		Slots: []intents.SlotSpec{
			{Name: "amount", Kind: intents.KindClassify, Classify: classify, DependsOn: []string{"nonexistent"}},
		},
	}
	if err := intents.Validate(n); !errors.Is(err, intents.ErrInvalidSpec) {
		t.Fatalf("Validate() = %v, want errors.Is ErrInvalidSpec", err)
	}
}

func TestValidate_SlotDependsOnUnknownSlot_ReportsProblemOnlyOnce(t *testing.T) {
	classify := &intents.ClassifySpec{Backend: "laya", Options: map[string]string{"a": "opt"}}
	n := &intents.Node{
		Key:  "root",
		Kind: intents.KindSlotDAG,
		Slots: []intents.SlotSpec{
			{Name: "amount", Kind: intents.KindClassify, Classify: classify, DependsOn: []string{"nonexistent"}},
		},
	}
	err := intents.Validate(n)
	joined, ok := err.(interface{ Unwrap() []error })
	if !ok {
		t.Fatalf("Validate() = %v (%T), want an errors.Join result", err, err)
	}
	if got := len(joined.Unwrap()); got != 1 {
		t.Fatalf("Validate() joined %d problems, want exactly 1 (the per-slot loop's own report, not also slotWave's)", got)
	}
}

func TestValidate_SlotDependsOnSelf_ReturnsErrInvalidSpec(t *testing.T) {
	classify := &intents.ClassifySpec{Backend: "laya", Options: map[string]string{"a": "opt"}}
	n := &intents.Node{
		Key:  "root",
		Kind: intents.KindSlotDAG,
		Slots: []intents.SlotSpec{
			{Name: "amount", Kind: intents.KindClassify, Classify: classify, DependsOn: []string{"amount"}},
		},
	}
	if err := intents.Validate(n); !errors.Is(err, intents.ErrInvalidSpec) {
		t.Fatalf("Validate() = %v, want errors.Is ErrInvalidSpec", err)
	}
}

func TestValidate_SlotDAGDependencyCycle_ReturnsErrCyclicSlotDAGAndErrInvalidSpec(t *testing.T) {
	classify := &intents.ClassifySpec{Backend: "laya", Options: map[string]string{"a": "opt"}}
	n := &intents.Node{
		Key:  "root",
		Kind: intents.KindSlotDAG,
		Slots: []intents.SlotSpec{
			{Name: "a", Kind: intents.KindClassify, Classify: classify, DependsOn: []string{"b"}},
			{Name: "b", Kind: intents.KindClassify, Classify: classify, DependsOn: []string{"a"}},
		},
	}
	err := intents.Validate(n)
	if !errors.Is(err, intents.ErrCyclicSlotDAG) {
		t.Fatalf("Validate() = %v, want errors.Is ErrCyclicSlotDAG", err)
	}
	if !errors.Is(err, intents.ErrInvalidSpec) {
		t.Fatalf("Validate() = %v, want errors.Is ErrInvalidSpec", err)
	}
}

func TestValidate_SlotKindMismatchedField_ReturnsErrInvalidSpec(t *testing.T) {
	n := &intents.Node{
		Key:  "root",
		Kind: intents.KindSlotDAG,
		Slots: []intents.SlotSpec{
			{Name: "amount", Kind: intents.KindClassify, Lookup: &intents.LookupSpec{Backend: "sqllookup"}},
		},
	}
	if err := intents.Validate(n); !errors.Is(err, intents.ErrInvalidSpec) {
		t.Fatalf("Validate() = %v, want errors.Is ErrInvalidSpec", err)
	}
}

func TestValidate_MultipleProblems_ReportsAll(t *testing.T) {
	n := &intents.Node{
		Key:  "", // problem 1: empty key
		Kind: intents.KindClassify,
		Classify: &intents.ClassifySpec{
			Backend:       "",  // problem 2: empty backend
			Options:       nil, // problem 3: empty options
			MinConfidence: 2.0, // problem 4: out of range
		},
	}
	err := intents.Validate(n)
	if err == nil {
		t.Fatalf("Validate() = nil, want a joined error reporting multiple problems")
	}
	joined, ok := err.(interface{ Unwrap() []error })
	if !ok {
		t.Fatalf("Validate() error does not support Unwrap() []error (errors.Join shape)")
	}
	if got := len(joined.Unwrap()); got < 4 {
		t.Fatalf("Validate() reported %d problems, want at least 4", got)
	}
}

func TestValidate_ValidMixedKindTree_ReturnsNil(t *testing.T) {
	invite := &intents.Node{
		Key:  "invite_collaborator",
		Kind: intents.KindSlotDAG,
		Slots: []intents.SlotSpec{
			{Name: "invitee", Kind: intents.KindLookup, Lookup: &intents.LookupSpec{Backend: "sqllookup"}, Required: true},
			{Name: "permission", Kind: intents.KindClassify, Classify: &intents.ClassifySpec{Backend: "laya", Options: map[string]string{"editor": "editor", "viewer": "viewer"}}, Required: true},
		},
	}
	collaboration := &intents.Node{
		Key:      "collaboration",
		Kind:     intents.KindClassify,
		Classify: &intents.ClassifySpec{Backend: "laya", Options: map[string]string{"invite": "invite"}},
		Children: map[string]*intents.Node{"invite": invite},
	}
	root := &intents.Node{
		Key:      "app_action",
		Kind:     intents.KindClassify,
		Classify: &intents.ClassifySpec{Backend: "laya", Options: map[string]string{"collaboration": "collaboration"}},
		Children: map[string]*intents.Node{"collaboration": collaboration},
	}
	if err := intents.Validate(root); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
}
