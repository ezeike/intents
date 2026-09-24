package intents

// RiskTier is advisory metadata only — the engine never blocks on it. See doc.go's
// "What this package deliberately does not know".
type RiskTier int

const (
	RiskLow RiskTier = iota
	RiskMedium
	RiskHigh
	RiskCritical
)

// ValueKind discriminates which field of Value is populated.
type ValueKind string

const (
	ValueKindChoice ValueKind = "choice"
	ValueKindEntity ValueKind = "entity"
	ValueKindRaw    ValueKind = "raw"
)

// Value is one resolved node or slot result.
type Value struct {
	Kind       ValueKind
	Choice     string  // set when Kind == ValueKindChoice
	Entity     *Entity // set when Kind == ValueKindEntity
	Raw        string  // set when Kind == ValueKindRaw
	Confidence float64
	RiskTier   RiskTier // effective, post-inheritance
	Source     string   // registered backend name (or "custom") that produced this
}

// FailureReason classifies why a node or slot did not produce a Value.
type FailureReason string

const (
	ReasonLowConfidence FailureReason = "low_confidence"
	ReasonNotFound      FailureReason = "not_found"
	ReasonBackendError  FailureReason = "backend_error"
	ReasonInvalidInput  FailureReason = "invalid_input"
	ReasonUnresolved    FailureReason = "unresolved"
)

// Failure records one node/slot miss. State.Failures is always populated, required-or-not,
// even on an otherwise fully successful resolution.
type Failure struct {
	Path      string // dot-joined Node.Key path; slot leaf appends SlotSpec.Name
	Reason    FailureReason
	Err       error
	Candidate *Value
}

// State is the resolution checkpoint threaded through ResolveState/ResumeState/Resolve[T] and
// returned to the caller. Every field is exported so a future session wrapper can serialize it
// without reaching into engine internals (see doc.go's serialization note).
type State struct {
	Input    string // set once at resolution start; unchanged by ResumeState
	Context  any    // caller-supplied situational grounding; set once; unchanged by ResumeState
	Values   map[string]Value
	Terminal string // Key of the leaf node reached; "" until the walk stops descending
	Failures []Failure
	RiskTier RiskTier // max across all resolved Values
}
