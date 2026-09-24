package examples

import "context"

// Node is one point in a pre-declared classify/lookup/execute tree. See docs/architecture.md
// §5-6. The tree's SHAPE is static config authored ahead of time; a classifier only ever picks
// among children already declared for it — it never invents structure.
type Node struct {
	Key  string
	Type string // "classify" | "lookup" | "execute"

	// Options is the fixed vocabulary for a "classify" node: option key -> description fed
	// to Laya. Keep this small (~3-8) at every node — that's what avoids the head-budget
	// dilution problem described in docs/context-budgets.md.
	Options map[string]string

	// Index names the search index for a "lookup" node — used when the branching factor is
	// large or data-driven rather than conceptual (e.g. "which of 400 devices").
	Index string

	// Children maps a resolved option (classify) or entity id (lookup) to the next node.
	Children map[string]*Node
}

// Classifier answers a classify node's question given the current input and that node's options.
type Classifier func(ctx context.Context, input string, options map[string]string) (string, float64, error)

// Searcher resolves a lookup node's query against its named index.
type Searcher func(ctx context.Context, input string, index string) (string, error)

// Walk traverses a pre-declared tree, alternating classify and lookup steps as each node
// dictates, until it reaches an "execute" leaf.
func Walk(ctx context.Context, input string, n *Node, classify Classifier, search Searcher) (*Node, error) {
	switch n.Type {
	case "execute":
		return n, nil

	case "classify":
		choice, _, err := classify(ctx, input, n.Options)
		if err != nil {
			return nil, err
		}
		child, ok := n.Children[choice]
		if !ok {
			return nil, errUnknownBranch(n.Key, choice)
		}
		return Walk(ctx, input, child, classify, search)

	case "lookup":
		entity, err := search(ctx, input, n.Index)
		if err != nil {
			return nil, err
		}
		child, ok := n.Children[entity]
		if !ok {
			return nil, errUnknownBranch(n.Key, entity)
		}
		return Walk(ctx, input, child, classify, search)

	default:
		return nil, errUnknownNodeType(n.Type)
	}
}

func errUnknownBranch(node, key string) error {
	return &treeError{"intents: node " + node + " has no child for " + key}
}

func errUnknownNodeType(t string) error {
	return &treeError{"intents: unknown node type " + t}
}

type treeError struct{ msg string }

func (e *treeError) Error() string { return e.msg }
