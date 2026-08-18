package session

import (
	"strings"

	"github.com/zenodea/zaino/internal/llm"
)

// A Node is a turn of yours, wherever it sits in the file — on the path that
// leads to the newest entry, or on a branch a rewind left behind — with the
// turns that were asked after it.
type Node struct {
	Entry    Entry
	Children []*Node
}

// Tree gathers every turn in the file into the tree the branches form. A
// turn's parent is the nearest turn above it, whatever settings entries sit
// between them; a file that never branched comes back as one chain.
func Tree(entries []Entry) []*Node {
	byID := make(map[string]Entry, len(entries))
	for _, e := range entries {
		byID[e.ID] = e
	}

	nodes := make(map[string]*Node)
	for _, e := range entries {
		if IsTurn(e) {
			nodes[e.ID] = &Node{Entry: e}
		}
	}

	// File order is the order things were said, so a node's children come out
	// oldest branch first.
	var roots []*Node
	for _, e := range entries {
		node, ok := nodes[e.ID]
		if !ok {
			continue
		}
		if parent := nearestTurn(e, byID, nodes); parent != nil {
			parent.Children = append(parent.Children, node)
			continue
		}
		roots = append(roots, node)
	}
	return roots
}

func nearestTurn(e Entry, byID map[string]Entry, nodes map[string]*Node) *Node {
	seen := map[string]bool{}
	for at := e; !seen[at.ID]; {
		seen[at.ID] = true
		parent, ok := byID[at.Parent]
		if !ok {
			return nil
		}
		if node, ok := nodes[parent.ID]; ok {
			return node
		}
		at = parent
	}
	return nil
}

// IsTurn says whether an entry is a prompt of your own: not a tool result,
// and not the summary a compaction folds the conversation into.
func IsTurn(e Entry) bool {
	if e.Type != KindMessage || e.Message == nil || e.Message.Role != llm.RoleUser {
		return false
	}
	for _, block := range e.Message.Content {
		if _, ok := block.(llm.ToolResultBlock); ok {
			return false
		}
	}
	text := strings.TrimSpace(e.Message.Text())
	return text != "" && !strings.HasPrefix(text, SummaryPrefix)
}
