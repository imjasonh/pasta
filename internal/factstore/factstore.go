// Package factstore is the per-run store of facts emitted by rules and
// queried by other rules via has_fact / not_has_fact predicates.
//
// A fact is keyed by (kind, file-id, byte-range): two facts of the
// same kind on the same node de-duplicate. The file-id disambiguates
// nodes from different files in a multi-file analysis group — without
// it, two files with overlapping byte ranges would collide. When the
// anchor is an identifier-like node, the fact is ALSO indexed by
// (kind, identifier-text). The by-name index is intentionally
// file-agnostic so that a fact emitted on a function declaration in
// one file is visible at call sites in other files — that's how
// cross-file fact propagation works in pasta today.
package factstore

import (
	"sync"

	"github.com/imjasonh/pasta/internal/tsutil"
)

// Key identifies a fact by its kind, the file the anchor lives in,
// and the anchor's byte range within that file.
type Key struct {
	FileID    string
	StartByte uint32
	EndByte   uint32
	Kind      string
}

// NameKey identifies a fact by (identifier-text, kind). Populated as a
// secondary index whenever the emitted fact's anchor is an
// identifier-like node. Deliberately not file-scoped: looking up
// "fact F on identifier x" hits across every file in the group, which
// is what makes cross-file analysis work.
type NameKey struct {
	Name string
	Kind string
}

// Fact is a stored fact: its kind, the file + byte range of its
// anchor, and an opaque payload set by the emitting rule.
type Fact struct {
	Kind               string
	FileID             string
	StartByte, EndByte uint32
	Payload            map[string]any
}

// Store is the per-run fact store. Safe for concurrent use: the engine
// emits and queries facts from multiple worker goroutines when files
// are processed in parallel.
type Store struct {
	mu      sync.RWMutex
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
	k := Key{FileID: node.FileID, StartByte: node.StartByte(), EndByte: node.EndByte(), Kind: kind}
	fact := Fact{
		Kind:      kind,
		FileID:    node.FileID,
		StartByte: node.StartByte(),
		EndByte:   node.EndByte(),
		Payload:   payload,
	}
	isIdent := identifierTypes[node.Type()]
	var nameKey NameKey
	if isIdent {
		nameKey = NameKey{Name: node.Text(), Kind: kind}
	}
	s.mu.Lock()
	_, exists := s.byRange[k]
	s.byRange[k] = fact
	if isIdent {
		s.byName[nameKey] = fact
	}
	s.mu.Unlock()
	return !exists
}

// Has reports whether a fact of the given kind is attached to node, or
// to any other identifier-like node with the same text.
func (s *Store) Has(node tsutil.Node, kind string) bool {
	if s == nil {
		return false
	}
	rk := Key{FileID: node.FileID, StartByte: node.StartByte(), EndByte: node.EndByte(), Kind: kind}
	isIdent := identifierTypes[node.Type()]
	var nk NameKey
	if isIdent {
		nk = NameKey{Name: node.Text(), Kind: kind}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.byRange[rk]; ok {
		return true
	}
	if isIdent {
		if _, ok := s.byName[nk]; ok {
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
	rk := Key{FileID: node.FileID, StartByte: node.StartByte(), EndByte: node.EndByte(), Kind: kind}
	isIdent := identifierTypes[node.Type()]
	var nk NameKey
	if isIdent {
		nk = NameKey{Name: node.Text(), Kind: kind}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if f, ok := s.byRange[rk]; ok {
		return f, true
	}
	if isIdent {
		if f, ok := s.byName[nk]; ok {
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
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.byRange)
}
