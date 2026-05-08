// Package schema is the CUE definition of the pasta analyzer DSL.
//
// User analyzer files import this package and constrain their values
// against #Analyzer.
package schema

// ============================================================================
// Facts — typed data flowing between rules
// ============================================================================

#Fact: {
	kind: string
}

// ============================================================================
// Captures — bind AST nodes to names for use in predicates and rewrites
// ============================================================================

#Capture: {
	capture:  string
	pattern?: #Pattern
	// `preceding` is allowed on #Capture as well as #Pattern so that an
	// adjacent element can both bind a name and look back at the prior
	// sibling.
	preceding?: #Pattern | #Capture
}

// ============================================================================
// Predicates — constraints evaluated during pattern matching
// ============================================================================

#Predicate: {
	// Closed set of predicate ops implemented by the runtime. Keep in
	// sync with internal/match/predicate.go DefaultRegistry().Predicates.
	op: "eq" | "neq" | "matches" | "not_matches" |
		"has_fact" | "not_has_fact" |
		"ancestor_is" |
		"last_non_blank" |
		"nil_comparison" |
		"same_ident" |
		"token_eq" |
		"stmt_index_delta" |
		"empty" |
		"named_child_count" |
		"prev_sibling_matches"
	// Positional args. Each is either a string (capture ref, regex,
	// literal) or a list of strings (e.g. node-type alternatives).
	args: [...string | [...string]]
	optional?: bool | *false
}

// ============================================================================
// Pattern — the structural shape to match
// ============================================================================

#Pattern: {
	node:           string | [...string]
	fields?:        {[string]: #Pattern | #Capture}
	children?:      [...#Pattern | #Capture]
	where?:         [...#Predicate]
	adjacent?:      [...#Pattern | #Capture]
	preceding?:     #Pattern | #Capture
	absent_fields?: [...string]
}

// ============================================================================
// Diagnostics and Rewrites
// ============================================================================

#Severity: "error" | "warning" | "info" | "hint"

#Diagnostic: {
	message:   string
	severity?: #Severity | *"warning"
}

#Edit: {
		target:       string
		replacement:  string
		trim_start?:  int & >=0
		trim_end?:    int & >=0
	} |
	{position: "before" | "after", anchor: string, text: string} |
	{
		delete_from:        string
		delete_to:          string
		delete_from_end?:   bool | *false
		delete_to_end?:     bool | *false
	} |
	{within: string, token: string, replace_with: string}

#Rewrite: {edits: [...#Edit]}

#RewriteOpts: {
	preserve_comments?: bool | *false
}

// ============================================================================
// Preconditions
// ============================================================================

#Precondition: {
	// Closed set of check names registered by pasta's default
	// PredicateRegistry. Adapters that add language-specific checks
	// will need to extend this. Keep in sync with
	// internal/match/predicate.go DefaultRegistry().Checks.
	check: "all_children_type" |
		"siblings_after_no_ident" |
		"ancestor_field_subtree_no_ident" |
		"decl_matches_idents"
	// Each arg is either a string (a capture ref like "@foo", a regex,
	// or a literal) or a list of strings (e.g. `ancestor_types: [...]`).
	args: {[string]: string | [...string]}
}

// ============================================================================
// Rule and Analyzer
// ============================================================================

#Rule: {
	name: string & =~"^[a-z][a-z0-9_]*$"
	doc:  string

	requires: [...string]
	provides: [...string]

	languages: [...string] | *["*"]

	match: #Pattern

	pre_conditions?: [...#Precondition]

	rewrite_opts?: #RewriteOpts

	diagnose?: #Diagnostic
	rewrite?:  #Rewrite
	emit?: [...{
		fact:     string
		attach:   string
		payload?: _
	}]
}

#Analyzer: {
	name:    string
	version: string
	doc?:    string
	facts: {[string]: #Fact}
	rules: {[string]: #Rule}

	// Compute every fact kind any rule provides. Used below to
	// constrain `requires` to be a subset.
	_provided_facts: [
		for _, r in rules
		for f in r.provides {
			f
		},
	]

	// Every rule's `requires` entry must be in `_provided_facts`. CUE's
	// `or(_provided_facts)` builds a disjunction of the provided fact
	// names; `[...or(_provided_facts)]` then constrains every list
	// element to match one of them. Unsatisfied requires fail at load
	// time with a CUE error pointing at the offending rule.
	//
	// If `_provided_facts` is empty, `or([])` is bottom and any
	// non-empty `requires` is rejected — which is the right answer:
	// nothing's provided, so nothing can be required.
	rules: [string]: {
		requires: [...or(_provided_facts)]
	}
}

// ============================================================================
// Language config — declared by language packages, loaded at startup.
// ============================================================================

#Language: {
	// `grammar` names a tree-sitter grammar registered in the runtime's
	// grammar map (pkg/lang/grammars.go). Only grammars present in the
	// pasta binary can be referenced. Adding a new alias for an
	// already-registered grammar is a CUE-only change.
	grammar: string

	// File extensions that map to this language. Each must include the
	// leading ".".
	extensions: [...string]

	// Tree-sitter node types to skip during adjacency matching (for
	// comment filtering primarily).
	comment_types: [...string]
}
