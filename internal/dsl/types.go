// Package dsl defines Go structs that mirror the CUE schema for analyzers,
// rules, patterns, predicates, and edits.
package dsl

import (
	"encoding/json"
	"fmt"
)

// Severity classifies a diagnostic.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
	SeverityHint    Severity = "hint"
)

// Analyzer is a top-level analysis bundle.
type Analyzer struct {
	Name    string          `json:"name"`
	Version string          `json:"version"`
	Doc     string          `json:"doc,omitempty"`
	Facts   map[string]Fact `json:"facts,omitempty"`
	Rules   map[string]Rule `json:"rules"`
}

// Fact is the type-level declaration of a fact kind.
type Fact struct {
	Kind   string         `json:"kind"`
	Schema map[string]any `json:"schema,omitempty"`
}

// Rule binds a pattern to effects.
type Rule struct {
	Name          string         `json:"name"`
	Doc           string         `json:"doc,omitempty"`
	Requires      []string       `json:"requires,omitempty"`
	Provides      []string       `json:"provides,omitempty"`
	Languages     []string       `json:"languages,omitempty"`
	FileMatch     []string       `json:"file_match,omitempty"`
	Match         Pattern        `json:"match"`
	PreConditions []Precondition `json:"pre_conditions,omitempty"`
	RewriteOpts   *RewriteOpts   `json:"rewrite_opts,omitempty"`
	Diagnose      *Diagnostic    `json:"diagnose,omitempty"`
	Rewrite       *Rewrite       `json:"rewrite,omitempty"`
	Emit          []EmitFact     `json:"emit,omitempty"`
}

// EmitFact attaches a fact to a captured node.
type EmitFact struct {
	Fact    string `json:"fact"`
	Attach  string `json:"attach"`
	Payload any    `json:"payload,omitempty"`
}

// Pattern matches a tree-sitter node.
//
// Node may be a single string or a list of strings; when decoded from CUE,
// a single string is normalized to a one-element slice.
type Pattern struct {
	Node         []string         `json:"node"`
	Fields       map[string]Child `json:"fields,omitempty"`
	Children     []Child          `json:"children,omitempty"`
	Where        []Predicate      `json:"where,omitempty"`
	Adjacent     []Child          `json:"adjacent,omitempty"`
	Preceding    *Child           `json:"preceding,omitempty"`
	AbsentFields []string         `json:"absent_fields,omitempty"`
}

// UnmarshalJSON normalizes string-or-list `node` to a slice.
func (p *Pattern) UnmarshalJSON(data []byte) error {
	type raw struct {
		Node         json.RawMessage  `json:"node"`
		Fields       map[string]Child `json:"fields,omitempty"`
		Children     []Child          `json:"children,omitempty"`
		Where        []Predicate      `json:"where,omitempty"`
		Adjacent     []Child          `json:"adjacent,omitempty"`
		Preceding    *Child           `json:"preceding,omitempty"`
		AbsentFields []string         `json:"absent_fields,omitempty"`
	}
	var r raw
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	p.Fields = r.Fields
	p.Children = r.Children
	p.Where = r.Where
	p.Adjacent = r.Adjacent
	p.Preceding = r.Preceding
	p.AbsentFields = r.AbsentFields

	if len(r.Node) == 0 {
		return nil
	}
	// Try single string first.
	var s string
	if err := json.Unmarshal(r.Node, &s); err == nil {
		p.Node = []string{s}
		return nil
	}
	// Then list.
	var ls []string
	if err := json.Unmarshal(r.Node, &ls); err != nil {
		return fmt.Errorf("pattern.node: must be string or [string]: %w", err)
	}
	p.Node = ls
	return nil
}

// Child is either a sub-#Pattern or a #Capture. CUE's union type means we
// can't statically know which; we represent both inline and dispatch at use.
type Child struct {
	// Capture (if non-empty, this is a capture-form).
	Capture string `json:"capture,omitempty"`

	// Quantifier on the capture: ?, *, +. Empty means required.
	Quantifier string `json:"quantifier,omitempty"`

	// Inline pattern (capture-form may also carry a constraining pattern).
	Pattern *Pattern `json:"pattern,omitempty"`

	// Pattern-form fields (apply when Capture == "" and Pattern == nil).
	Node         []string         `json:"node,omitempty"`
	Fields       map[string]Child `json:"fields,omitempty"`
	Children     []Child          `json:"children,omitempty"`
	Where        []Predicate      `json:"where,omitempty"`
	Adjacent     []Child          `json:"adjacent,omitempty"`
	Preceding    *Child           `json:"preceding,omitempty"`
	AbsentFields []string         `json:"absent_fields,omitempty"`
}

// AsPattern returns the inline pattern of c, whether it was written as a
// raw pattern or as a capture with an embedded `pattern: ...`. Returns nil
// if c has no constraining pattern.
func (c *Child) AsPattern() *Pattern {
	if c == nil {
		return nil
	}
	if c.Pattern != nil {
		return c.Pattern
	}
	if len(c.Node) == 0 && c.Fields == nil && c.Children == nil &&
		c.Where == nil && c.Adjacent == nil && c.Preceding == nil &&
		c.AbsentFields == nil {
		return nil
	}
	return &Pattern{
		Node:         c.Node,
		Fields:       c.Fields,
		Children:     c.Children,
		Where:        c.Where,
		Adjacent:     c.Adjacent,
		Preceding:    c.Preceding,
		AbsentFields: c.AbsentFields,
	}
}

// UnmarshalJSON normalizes string-or-list `node` and disambiguates the
// pattern-vs-capture union.
func (c *Child) UnmarshalJSON(data []byte) error {
	type raw struct {
		Capture      string           `json:"capture,omitempty"`
		Quantifier   string           `json:"quantifier,omitempty"`
		Pattern      *Pattern         `json:"pattern,omitempty"`
		Node         json.RawMessage  `json:"node,omitempty"`
		Fields       map[string]Child `json:"fields,omitempty"`
		Children     []Child          `json:"children,omitempty"`
		Where        []Predicate      `json:"where,omitempty"`
		Adjacent     []Child          `json:"adjacent,omitempty"`
		Preceding    *Child           `json:"preceding,omitempty"`
		AbsentFields []string         `json:"absent_fields,omitempty"`
	}
	var r raw
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	c.Capture = r.Capture
	c.Quantifier = r.Quantifier
	c.Pattern = r.Pattern
	c.Fields = r.Fields
	c.Children = r.Children
	c.Where = r.Where
	c.Adjacent = r.Adjacent
	c.Preceding = r.Preceding
	c.AbsentFields = r.AbsentFields

	if len(r.Node) == 0 {
		return nil
	}
	var s string
	if err := json.Unmarshal(r.Node, &s); err == nil {
		c.Node = []string{s}
		return nil
	}
	var ls []string
	if err := json.Unmarshal(r.Node, &ls); err != nil {
		return fmt.Errorf("child.node: must be string or [string]: %w", err)
	}
	c.Node = ls
	return nil
}

// Arg is one argument to a predicate or precondition: either a string
// (capture ref, regex, literal) or a list of strings (alternatives,
// e.g. node-type lists). Custom JSON unmarshaling routes to the right
// field based on whether the JSON value is a string or an array.
type Arg struct {
	Str  string
	List []string
}

// UnmarshalJSON parses either "foo" → Arg{Str: "foo"} or
// ["foo", "bar"] → Arg{List: ["foo", "bar"]}. Anything else is an
// error.
func (a *Arg) UnmarshalJSON(b []byte) error {
	if len(b) == 0 {
		return nil
	}
	if b[0] == '[' {
		return json.Unmarshal(b, &a.List)
	}
	return json.Unmarshal(b, &a.Str)
}

// MarshalJSON emits the string form when only Str is set, otherwise the
// list form. (Both empty serializes as the empty string.)
func (a Arg) MarshalJSON() ([]byte, error) {
	if a.List != nil {
		return json.Marshal(a.List)
	}
	return json.Marshal(a.Str)
}

// IsList reports whether this Arg holds a list value.
func (a Arg) IsList() bool { return a.List != nil }

// StringList returns the list form, or a single-element list wrapping
// the string form, or nil if both are empty. Use for predicates that
// accept either shape.
func (a Arg) StringList() []string {
	if a.List != nil {
		return a.List
	}
	if a.Str == "" {
		return nil
	}
	return []string{a.Str}
}

// Predicate is a constraint evaluated during matching. Args are
// positional.
type Predicate struct {
	Op   string `json:"op"`
	Args []Arg  `json:"args"`
}

// Precondition is a semantic check evaluated by the predicate registry.
type Precondition struct {
	Check    string         `json:"check"`
	Args     map[string]Arg `json:"args"`
	Optional bool           `json:"optional,omitempty"`
}

// Diagnostic is what the rule reports when it matches.
type Diagnostic struct {
	Message  string   `json:"message"`
	Severity Severity `json:"severity,omitempty"`
}

// Rewrite is one of:
//   - a template: a single string with @capture interpolations
//   - a list of edits
//
// We model both as fields and require exactly one to be set.
type Rewrite struct {
	Template string `json:"template,omitempty"`
	Edits    []Edit `json:"edits,omitempty"`
}

// Edit is the union of:
//   - replace: target + replacement
//   - insert : position + anchor + text
//   - delete : delete_from + delete_to
//   - within : within + token + replace_with
//
// `delete_from_end` and `delete_to_end` flip the boundary semantics for
// the corresponding capture: when true, the byte position is the
// capture's END rather than its START. Useful for removing trailing
// constructs like an empty else block: delete from consequence's End
// to alternative's End.
//
// `trim_start` and `trim_end` apply only on the replace form, AFTER
// `replacement` is interpolated. They drop N bytes from the start/end
// of the resulting text. Useful for "replace dbg!(EXPR) with EXPR" —
// interpolate @_root then trim the wrapping `dbg!(` and `)`.
type Edit struct {
	Target        string `json:"target,omitempty"`
	Replacement   string `json:"replacement,omitempty"`
	TrimStart     int    `json:"trim_start,omitempty"`
	TrimEnd       int    `json:"trim_end,omitempty"`
	Position      string `json:"position,omitempty"`
	Anchor        string `json:"anchor,omitempty"`
	Text          string `json:"text,omitempty"`
	DeleteFrom    string `json:"delete_from,omitempty"`
	DeleteFromEnd bool   `json:"delete_from_end,omitempty"`
	DeleteTo      string `json:"delete_to,omitempty"`
	DeleteToEnd   bool   `json:"delete_to_end,omitempty"`
	Within        string `json:"within,omitempty"`
	Token         string `json:"token,omitempty"`
	ReplaceWith   string `json:"replace_with,omitempty"`
}

// RewriteOpts controls rewrite behavior.
type RewriteOpts struct {
	PreserveComments bool `json:"preserve_comments,omitempty"`
	PreserveIndent   bool `json:"preserve_indent,omitempty"`
}

// LanguageDecl is a CUE-side language declaration. The runtime resolves
// `Grammar` against internal/lang/grammars.go. Lives in internal/dsl (not internal/lang)
// to avoid an import cycle: internal/loader populates these, internal/lang
// converts them to its registry type.
type LanguageDecl struct {
	Name         string   `json:"-"` // populated from the CUE binding key
	Grammar      string   `json:"grammar"`
	Extensions   []string `json:"extensions,omitempty"`
	CommentTypes []string `json:"comment_types,omitempty"`
}
