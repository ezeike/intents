// fakes_test.go
package intents_test

import (
	"context"
	"sync"

	"github.com/ezeike/intents"
)

// fakeClassifier returns a scripted ChoiceAnswer or error, and counts calls.
type fakeClassifier struct {
	mu         sync.Mutex
	choice     string
	confidence float64
	dist       map[string]float64
	err        error
	calls      int
	lastInput  string
	lastGround any
}

func (f *fakeClassifier) Classify(ctx context.Context, input string, grounding any) (intents.ChoiceAnswer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.lastInput = input
	f.lastGround = grounding
	if f.err != nil {
		return intents.ChoiceAnswer{}, f.err
	}
	return intents.ChoiceAnswer{Choice: f.choice, Confidence: f.confidence, Distribution: f.dist}, nil
}

func (f *fakeClassifier) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// fakeLookuper returns a scripted Entity or error, and counts calls.
type fakeLookuper struct {
	mu         sync.Mutex
	entity     intents.Entity
	err        error
	calls      int
	lastQuery  string
	lastIndex  string
	lastGround any
}

func (f *fakeLookuper) Lookup(ctx context.Context, query string, index string, grounding any) (intents.Entity, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.lastQuery = query
	f.lastIndex = index
	f.lastGround = grounding
	if f.err != nil {
		return intents.Entity{}, f.err
	}
	return f.entity, nil
}

func (f *fakeLookuper) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

var (
	_ intents.Classifier = (*fakeClassifier)(nil)
	_ intents.Lookuper   = (*fakeLookuper)(nil)
)
