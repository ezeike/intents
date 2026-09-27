//go:build integration

// New decision point, distinct from addresses_category (chain_guideline_test.go): given that some
// categories are already settled, which of the still-OPEN categories should the assistant ask
// about next. The option set here is dynamic and app-controlled -- categories already settled are
// never offered as options at all, since that's app-known state (from the *_settled nouls'
// eventual threshold decision), not something to ask Jev about. Only one condition per question
// (per the jev-classifier skill): "which one of these open options" is a single Choice judgment,
// kept entirely separate from "does the current message address a category" and from "is category
// X settled."
//
// Exploratory: there's no known-correct priority ordering yet, so most of these are measurement
// grids/log-only, with one trivial hard-asserted sanity check.
package jev_test

import (
	"context"
	"testing"

	"github.com/ezeike/intents/jev"
)

// priorityState carries only conversation grounding -- unlike guidelineState, there's no
// "latest_message" here: "what should we ask about next" isn't a reaction to the current message,
// it's a judgment over the conversation so far plus which categories remain open.
type priorityState struct {
	ConversationSoFar string `json:"conversation_so_far,omitempty"`
}

// categoryDescription looks up one category's description from guidelineCriteria() (its criteria
// text is the same regardless of which question -- addresses_category or which_next -- is asking
// about it), so the two questions never drift out of sync on how a category is described.
func categoryDescription(key string) string {
	for _, c := range guidelineCriteria() {
		if c.Key == key {
			return c.Description
		}
	}
	return ""
}

// priorityCriteria builds a which_next Choice's option set from exactly the open categories, in
// the given order -- no "other" catch-all, since by construction every option offered here IS a
// real open category (the app already knows the full category set; there's nothing else it could
// mean). Caller-declared order matters for the position-bias test below.
func priorityCriteria(open []string) jev.OrderedCriteria {
	c := make(jev.OrderedCriteria, 0, len(open))
	for _, key := range open {
		c = append(c, jev.Criterion{Key: key, Description: categoryDescription(key)})
	}
	return c
}

// askGuidelinePriority sends the which_next question alone -- open categories only, per
// priorityCriteria.
func askGuidelinePriority(t *testing.T, c *jev.Client, conversationSoFar string, open []string) jev.ChoiceAnswer {
	t.Helper()
	resp, err := c.SystemOne(context.Background(), jev.Request{
		State: priorityState{ConversationSoFar: conversationSoFar},
		Questions: map[string]jev.Question{
			"which_next": {
				Type:         "choice",
				Instructions: "Of the categories that are still open (not yet answered), which one should be asked about next?",
				Criteria:     priorityCriteria(open),
			},
		},
		Model: jevModel(),
	})
	if err != nil {
		t.Fatalf("SystemOne(open=%v): %v", open, err)
	}
	a, err := resp.Choice("which_next")
	if err != nil {
		t.Fatalf("Choice(which_next, open=%v): %v", open, err)
	}
	return a
}

// askGuidelineWithPriority batches addresses_category (reacting to latestMessage) and which_next
// (over the open set) into ONE request, to test that the two judgments don't leak into each other
// -- see TestGuidelinePriority_IndependentFromCurrentMessageCategory.
func askGuidelineWithPriority(t *testing.T, c *jev.Client, conversationSoFar, latestMessage string, open []string) (category, next jev.ChoiceAnswer) {
	t.Helper()
	resp, err := c.SystemOne(context.Background(), jev.Request{
		State: guidelineState{ConversationSoFar: conversationSoFar, LatestMessage: latestMessage},
		Questions: map[string]jev.Question{
			"addresses_category": {
				Type:         "choice",
				Instructions: "Which guideline category does the user's latest message address?",
				Criteria:     guidelineCriteria(),
			},
			"which_next": {
				Type:         "choice",
				Instructions: "Of the categories that are still open (not yet answered), which one should be asked about next?",
				Criteria:     priorityCriteria(open),
			},
		},
		Model: jevModel(),
	})
	if err != nil {
		t.Fatalf("SystemOne(latest=%q, open=%v): %v", latestMessage, open, err)
	}
	category, err = resp.Choice("addresses_category")
	if err != nil {
		t.Fatalf("Choice(addresses_category): %v", err)
	}
	next, err = resp.Choice("which_next")
	if err != nil {
		t.Fatalf("Choice(which_next): %v", err)
	}
	return category, next
}

var allCategories = []string{"core_functionality", "success_metrics", "scope_boundary", "general_entities"}

// TestGuidelinePriority_SingleOpenCategory_PicksIt is the trivial forced-choice sanity check: with
// exactly one option, which_next must return it. Not testing judgment, just that a 1-option Choice
// doesn't misbehave (e.g. by returning something outside the option set).
func TestGuidelinePriority_SingleOpenCategory_PicksIt(t *testing.T) {
	c := requireJev(t)
	for _, category := range allCategories {
		a := askGuidelinePriority(t, c, "", []string{category})
		if a.Choice != category {
			t.Errorf("open=[%s]: Choice = %q, want %q", category, a.Choice, category)
		}
	}
}

// TestGuidelinePriority_AllFourOpen_BaselineDistribution asks the same neutral question 5 times
// with all four categories open and an empty conversation, to see whether there's a stable default
// ordering (my prior: core_functionality first, since the other three logically depend on knowing
// what the thing does) or whether the pick looks arbitrary/unstable across repeats. No hard
// assertion -- this is what establishes whether that prior is even true.
func TestGuidelinePriority_AllFourOpen_BaselineDistribution(t *testing.T) {
	c := requireJev(t)

	counts := map[string]int{}
	const trials = 5
	for i := 0; i < trials; i++ {
		a := askGuidelinePriority(t, c, "", allCategories)
		counts[a.Choice]++
		t.Logf("[trial=%d] which_next=%-20s distribution=%+v", i, a.Choice, a.Probabilities)
	}
	t.Logf("== all-four-open baseline over %d trials: %+v ==", trials, counts)
}

// TestGuidelinePriority_PairwiseGrid_NeutralContext runs every 2-open-category pair with an empty
// conversation, logging which one wins each pairing -- maps out whether the model has any
// consistent priority ordering at all when nothing in the conversation favors either side.
func TestGuidelinePriority_PairwiseGrid_NeutralContext(t *testing.T) {
	c := requireJev(t)

	for i := 0; i < len(allCategories); i++ {
		for j := i + 1; j < len(allCategories); j++ {
			pair := []string{allCategories[i], allCategories[j]}
			a := askGuidelinePriority(t, c, "", pair)
			t.Logf("[pair=%v] which_next=%-20s distribution=%+v", pair, a.Choice, a.Probabilities)
		}
	}
}

// TestGuidelinePriority_ContentDrivenOverride takes the core_functionality vs scope_boundary pair
// and adds conversation context giving a concrete, on-topic reason to prioritize scope_boundary
// (a compliance deadline) despite core_functionality remaining open too -- confirms the pick
// tracks actual content rather than a fixed positional/label default, regardless of what the
// neutral pairwise grid above found.
func TestGuidelinePriority_ContentDrivenOverride(t *testing.T) {
	c := requireJev(t)

	open := []string{"core_functionality", "scope_boundary"}

	neutral := askGuidelinePriority(t, c, "", open)
	t.Logf("[neutral context] which_next=%-20s distribution=%+v", neutral.Choice, neutral.Probabilities)

	const complianceContext = "The team is under a hard compliance deadline and needs the transaction " +
		"and staking constraints nailed down before anything else."
	overridden := askGuidelinePriority(t, c, complianceContext, open)
	t.Logf("[compliance context] which_next=%-20s distribution=%+v", overridden.Choice, overridden.Probabilities)

	if overridden.Choice != "scope_boundary" {
		t.Logf("content did not shift the pick toward scope_boundary (got %q) -- logged, not failed: "+
			"first measurement of whether which_next is content-sensitive", overridden.Choice)
	}
}

// TestGuidelinePriority_PositionBias asks about the same two open categories, same empty context,
// once in each declared order -- per jev/doc.go, Jev's claimed absence of Laya's documented
// first-slot bias is unverified against live traffic. If the pick flips purely because of
// declaration order, that's a real finding, not noise.
func TestGuidelinePriority_PositionBias(t *testing.T) {
	c := requireJev(t)

	forward := askGuidelinePriority(t, c, "", []string{"success_metrics", "general_entities"})
	reversed := askGuidelinePriority(t, c, "", []string{"general_entities", "success_metrics"})

	t.Logf("[order=success_metrics,general_entities] which_next=%-20s distribution=%+v", forward.Choice, forward.Probabilities)
	t.Logf("[order=general_entities,success_metrics] which_next=%-20s distribution=%+v", reversed.Choice, reversed.Probabilities)

	if forward.Choice != reversed.Choice {
		t.Logf("position bias detected: declared order alone flipped the pick (%q vs %q) -- logged, not failed: "+
			"first measurement of this specific claim against live Jev", forward.Choice, reversed.Choice)
	}
}

// TestGuidelinePriority_IndependentFromCurrentMessageCategory batches addresses_category and
// which_next in one request: the current message clearly addresses general_entities, while the
// open set for which_next is a completely different pair (success_metrics, scope_boundary) that
// doesn't even include general_entities as an option. Confirms which_next answers its own
// question (which of ITS options to prioritize) rather than drifting toward whatever
// addresses_category concluded about the unrelated current message.
func TestGuidelinePriority_IndependentFromCurrentMessageCategory(t *testing.T) {
	c := requireJev(t)

	open := []string{"success_metrics", "scope_boundary"}
	category, next := askGuidelineWithPriority(t, c, "",
		"the main objects are accounts, transactions, and validator sets", open)

	if category.Choice != "general_entities" {
		t.Errorf("addresses_category = %q, want %q", category.Choice, "general_entities")
	}
	if next.Choice != "success_metrics" && next.Choice != "scope_boundary" {
		t.Errorf("which_next = %q, want one of %v (its declared option set)", next.Choice, open)
	}
	t.Logf("addresses_category=%s which_next=%s (open=%v)", category.Choice, next.Choice, open)
}
