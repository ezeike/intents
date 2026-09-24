// engine.go
package intents

import "sync"

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
