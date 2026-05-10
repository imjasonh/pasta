// Package tsutil wraps the gotreesitter primitives (pure-Go tree-sitter
// runtime) into a stable Node abstraction with byte ranges, named
// children, field lookups, and per-language statement-list traversal.
package tsutil

import (
	"context"

	gts "github.com/odvcencio/gotreesitter"
)

// Node is a thin wrapper around a gotreesitter node that carries the
// source bytes and language so the rest of pasta can call methods like
// Type() / ChildByFieldName() without threading the language through
// every call site.
//
// FileID identifies the source file the node belongs to. It's the
// disambiguator used by the fact store when several files share a
// single store (multi-file analysis groups). Single-file callers can
// leave it empty.
type Node struct {
	N      *gts.Node
	Src    []byte
	Lang   *gts.Language
	FileID string
}

// IsValid reports whether the node is non-nil.
func (n Node) IsValid() bool { return n.N != nil }

// Type returns the tree-sitter node type.
func (n Node) Type() string {
	if n.N == nil {
		return ""
	}
	return n.N.Type(n.Lang)
}

// IsNamed reports whether the node is a named (non-anonymous) child.
func (n Node) IsNamed() bool {
	if n.N == nil {
		return false
	}
	return n.N.IsNamed()
}

// StartByte and EndByte return the node's byte range in the source.
func (n Node) StartByte() uint32 { return n.N.StartByte() }
func (n Node) EndByte() uint32   { return n.N.EndByte() }

// Range returns (start, end).
func (n Node) Range() (uint32, uint32) {
	return n.N.StartByte(), n.N.EndByte()
}

// Text returns the source text covered by the node.
func (n Node) Text() string {
	return string(n.Src[n.StartByte():n.EndByte()])
}

// NamedChildren returns the named children in order.
func (n Node) NamedChildren() []Node {
	c := n.N.NamedChildCount()
	out := make([]Node, c)
	for i := 0; i < c; i++ {
		out[i] = Node{N: n.N.NamedChild(i), Src: n.Src, Lang: n.Lang, FileID: n.FileID}
	}
	return out
}

// AllChildren returns every child including anonymous ones.
func (n Node) AllChildren() []Node {
	kids := n.N.Children()
	out := make([]Node, len(kids))
	for i, c := range kids {
		out[i] = Node{N: c, Src: n.Src, Lang: n.Lang, FileID: n.FileID}
	}
	return out
}

// ChildByFieldName returns the named child bound to fieldName, or an
// invalid Node if no such field exists.
func (n Node) ChildByFieldName(fieldName string) Node {
	c := n.N.ChildByFieldName(fieldName, n.Lang)
	if c == nil {
		return Node{Src: n.Src, Lang: n.Lang, FileID: n.FileID}
	}
	return Node{N: c, Src: n.Src, Lang: n.Lang, FileID: n.FileID}
}

// HasFieldName reports whether the node has any child with the given field
// name. Used to drive `absent_fields`.
func (n Node) HasFieldName(fieldName string) bool {
	return n.N.ChildByFieldName(fieldName, n.Lang) != nil
}

// FieldNameForChildIdx returns the field name for the i-th child (including
// anonymous), or "" if none.
func (n Node) FieldNameForChildIdx(i int) string {
	return n.N.FieldNameForChild(i, n.Lang)
}

// Parent returns the parent node, or an invalid Node if at the root.
func (n Node) Parent() Node {
	p := n.N.Parent()
	if p == nil {
		return Node{Src: n.Src, Lang: n.Lang, FileID: n.FileID}
	}
	return Node{N: p, Src: n.Src, Lang: n.Lang, FileID: n.FileID}
}

// HasError reports whether the subtree contains a parse error.
func (n Node) HasError() bool { return n.N.HasError() }

// String returns the s-expression representation of the subtree (for tests).
func (n Node) String() string { return n.N.SExpr(n.Lang) }

// Parse parses src with the given gotreesitter language and returns the
// tree and root Node. The caller must call tree.Release() when done.
//
// fileID disambiguates nodes from this parse from those of other files
// in the same multi-file analysis group; pass "" for single-file usage.
func Parse(ctx context.Context, lang *gts.Language, src []byte, fileID string) (*gts.Tree, Node, error) {
	parser := gts.NewParser(lang)
	tree, err := parser.Parse(src)
	if err != nil {
		return nil, Node{}, err
	}
	root := Node{N: tree.RootNode(), Src: src, Lang: lang, FileID: fileID}
	return tree, root, nil
}

// Walk invokes fn pre-order on every named descendant of n, including n.
// fn returning false skips traversal of n's children.
func Walk(n Node, fn func(Node) bool) {
	if !fn(n) {
		return
	}
	for _, c := range n.NamedChildren() {
		Walk(c, fn)
	}
}

// StmtListProvider returns the named children of a container that should
// be considered ordered statements for `adjacent` matching. Languages
// supply this so comments and other non-statement nodes can be skipped.
type StmtListProvider func(container Node) []Node
