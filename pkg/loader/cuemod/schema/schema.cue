// Package schema is the CUE definition of the pasta analyzer DSL.
//
// User analyzer files import this package and constrain their values
// against #Analyzer.
package schema

// ============================================================================
// Facts — typed data flowing between rules
// ============================================================================

#Fact: {
	kind:    string
	schema?: _
}

// ============================================================================
// Captures — bind AST nodes to names for use in predicates and rewrites
// ============================================================================

#Capture: {
	capture:     string
	pattern?:    #Pattern
	quantifier?: "*" | "+" | "?"
	// `preceding` is allowed on #Capture as well as #Pattern so that an
	// adjacent element can both bind a name and look back at the prior
	// sibling (e.g. iferr's `assign` capture absorbing a preceding
	// `var X T` declaration).
	preceding?: #Pattern | #Capture
}

// ============================================================================
// Predicates — constraints evaluated during pattern matching
// ============================================================================

#Predicate: {
	op: "eq" | "neq" | "matches" | "not_matches" |
		"has_fact" | "not_has_fact" |
		"ancestor_is" | "type_is" |
		"field_absent" |
		"last_non_blank" |
		"nil_comparison" |
		"same_ident" |
		"token_eq" |
		"all_match" |
		"stmt_index_delta" |
		"empty" |
		"named_child_count"
	args: [...string]
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

#Edit: {target: string, replacement: string} |
	{position: "before" | "after", anchor: string, text: string} |
	{
		delete_from:        string
		delete_to:          string
		delete_from_end?:   bool | *false
		delete_to_end?:     bool | *false
	} |
	{within: string, token: string, replace_with: string}

#Rewrite: {template: string} | {edits: [...#Edit]}

#RewriteOpts: {
	preserve_comments?: bool | *false
	preserve_indent?:   bool | *true
}

// ============================================================================
// Preconditions
// ============================================================================

#Precondition: {
	check: string
	args: {[string]: string}
	optional?: bool | *false
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
	file_match?: [...string]

	match: #Pattern

	pre_conditions?: [...#Precondition]

	rewrite_opts?: #RewriteOpts

	diagnose?: #Diagnostic
	rewrite?:  #Rewrite
	emit?: [...{
		fact:    string
		attach:  string
		payload: _
	}]
}

#Analyzer: {
	name:    string
	version: string
	doc?:    string
	facts: {[string]: #Fact}
	rules: {[string]: #Rule}
}

#Adapter: {
	language: string
	checks: [...string]
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
