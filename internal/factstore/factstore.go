// Package factstore is the per-run store of facts emitted by rules and
// queried by other rules via has_fact / not_has_fact predicates.
//
// A fact is keyed by (kind, byte-range): two facts of the same kind on
// the same node de-duplicate. When the anchor is an identifier-like
// node, the fact is ALSO indexed by (kind, identifier-text) so that
// rules can ask "is the function with this name tagged?" at every call
// site without having to walk back to the declaration. This is the
// shape needed by the errcheck pattern (go_errcheck).
//
// Cross-file / cross-package facts are out of scope here; this is a
// single-source-file store. See future-work.md for the extension
// sketch.
package factstore

import (
	"github.com/imjasonh/pasta/internal/tsutil"
)

// Key identifies a fact by its kind and the byte range of the node it's
// attached to.
type Key struct {
	StartByte uint32
	EndByte   uint32
	Kind      string
}

// NameKey identifies a fact by (identifier-text, kind). Populated as a
// secondary index whenever the emitted fact's anchor is an
// identifier-like node.
type NameKey struct {
	Name string
	Kind string
}

// Fact is a stored fact: its kind, the byte range of its anchor, and an
// opaque payload set by the emitting rule.
type Fact struct {
	Kind               string
	StartByte, EndByte uint32
	Payload            map[string]any
}

// Store is the per-run fact store.
type Store struct {
	byRange map[Key]Fact
	byName  map[NameKey]Fact
}

// New returns an empty store.
func New() *Store {
	return &Store{
		byRange: map[Key]Fact{},
		byName:  map[NameKey]Fact{},
	}
}

// identifierTypes are tree-sitter node types we treat as "identifier-like"
// across grammars. Used to decide whether to populate the by-name index.
var identifierTypes = map[string]bool{
	"identifier":                    true,
	"type_identifier":               true,
	"field_identifier":              true,
	"property_identifier":           true,
	"shorthand_property_identifier": true,
}

// Emit records a fact attached to node. Returns true if this is a new
// fact at this byte range; used by the scheduler to detect convergence
// in fixpoint groups.
func (s *Store) Emit(kind string, node tsutil.Node, payload map[string]any) bool {
	k := Key{StartByte: node.StartByte(), EndByte: node.EndByte(), Kind: kind}
	_, exists := s.byRange[k]
	fact := Fact{
		Kind:      kind,
		StartByte: node.StartByte(),
		EndByte:   node.EndByte(),
		Payload:   payload,
	}
	s.byRange[k] = fact
	if identifierTypes[node.Type()] {
		s.byName[NameKey{Name: node.Text(), Kind: kind}] = fact
	}
	return !exists
}

// Has reports whether a fact of the given kind is attached to node, or
// to any other identifier-like node with the same text.
func (s *Store) Has(node tsutil.Node, kind string) bool {
	if s == nil {
		return false
	}
	if _, ok := s.byRange[Key{StartByte: node.StartByte(), EndByte: node.EndByte(), Kind: kind}]; ok {
		return true
	}
	if identifierTypes[node.Type()] {
		if _, ok := s.byName[NameKey{Name: node.Text(), Kind: kind}]; ok {
			return true
		}
	}
	return false
}

// Get returns the fact attached to node, or zero-value+false if absent.
func (s *Store) Get(node tsutil.Node, kind string) (Fact, bool) {
	if s == nil {
		return Fact{}, false
	}
	if f, ok := s.byRange[Key{StartByte: node.StartByte(), EndByte: node.EndByte(), Kind: kind}]; ok {
		return f, true
	}
	if identifierTypes[node.Type()] {
		if f, ok := s.byName[NameKey{Name: node.Text(), Kind: kind}]; ok {
			return f, true
		}
	}
	return Fact{}, false
}

// Len returns the total number of facts (by-range only).
func (s *Store) Len() int {
	if s == nil {
		return 0
	}
	return len(s.byRange)
}
