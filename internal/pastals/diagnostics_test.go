package pastals

import (
	"testing"

	"github.com/imjasonh/pasta/internal/dsl"
	"github.com/imjasonh/pasta/internal/effect"
	"github.com/imjasonh/pasta/internal/pastals/lspconv"
	"github.com/imjasonh/pasta/internal/pastals/protocol"
)

func TestSeverityFromPasta(t *testing.T) {
	cases := []struct {
		in   dsl.Severity
		want int
	}{
		{dsl.SeverityError, protocol.SeverityError},
		{dsl.SeverityWarning, protocol.SeverityWarning},
		{dsl.SeverityInfo, protocol.SeverityInformation},
		{dsl.SeverityHint, protocol.SeverityHint},
		{"", protocol.SeverityWarning},        // empty defaults to warning
		{"unknown", protocol.SeverityWarning}, // unknown defaults to warning
	}
	for _, tc := range cases {
		if got := severityFromPasta(tc.in); got != tc.want {
			t.Errorf("severityFromPasta(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestBuildLSPDiagnostics_Basic(t *testing.T) {
	src := []byte("line one\nline two\nline three\n")
	conv := lspconv.New(src)
	diags := []effect.Diagnostic{{
		Rule:      "myrule",
		Message:   "found something",
		Severity:  dsl.SeverityWarning,
		StartByte: 9,  // start of "line two"
		EndByte:   17, // end of "line two"
		LineNum:   2,
	}}
	ops := []effect.Op{{
		Rule:  "myrule",
		Start: 9,
		End:   17,
		Text:  "REPLACED",
	}}
	got := buildLSPDiagnostics(conv, diags, ops)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	d := got[0]
	if d.Source != "pasta:myrule" {
		t.Errorf("Source = %q, want pasta:myrule", d.Source)
	}
	if d.Severity != protocol.SeverityWarning {
		t.Errorf("Severity = %d, want %d", d.Severity, protocol.SeverityWarning)
	}
	if d.Range.Start.Line != 1 || d.Range.Start.Character != 0 {
		t.Errorf("Range.Start = %+v, want line 1 col 0", d.Range.Start)
	}
	if d.Data == nil {
		t.Fatal("Data is nil; expected attached fix")
	}
	if d.Data.Rule != "myrule" {
		t.Errorf("Data.Rule = %q, want myrule", d.Data.Rule)
	}
	if len(d.Data.Ops) != 1 || d.Data.Ops[0].Text != "REPLACED" {
		t.Errorf("Data.Ops = %+v, want one op with text REPLACED", d.Data.Ops)
	}
}

func TestBuildLSPDiagnostics_PairsByRuleNotRange(t *testing.T) {
	// Diagnostic anchor is small; the rewrite spans a much larger
	// byte range. We must still pair op→diagnostic by rule name.
	src := []byte("aaaaaaaaaa\nbbbbbbbbbb\ncccccccccc\n")
	conv := lspconv.New(src)
	diags := []effect.Diagnostic{{
		Rule:      "iferr",
		Message:   "iferr finding",
		Severity:  dsl.SeverityWarning,
		StartByte: 12, // inside line 2
		EndByte:   14, // small anchor
		LineNum:   2,
	}}
	ops := []effect.Op{{
		Rule:  "iferr",
		Start: 0, // way before the anchor
		End:   uint32(len(src)),
		Text:  "rewrite-of-everything",
	}}
	got := buildLSPDiagnostics(conv, diags, ops)
	if len(got[0].Data.Ops) != 1 {
		t.Errorf("ops not paired with diagnostic; got %d", len(got[0].Data.Ops))
	}
}

func TestBuildLSPDiagnostics_OpForOtherRuleNotAttached(t *testing.T) {
	src := []byte("xxx\n")
	conv := lspconv.New(src)
	diags := []effect.Diagnostic{{
		Rule: "rule_a", Message: "a", Severity: dsl.SeverityWarning,
		StartByte: 0, EndByte: 3, LineNum: 1,
	}}
	ops := []effect.Op{{
		Rule: "rule_b", Start: 0, End: 3, Text: "z",
	}}
	got := buildLSPDiagnostics(conv, diags, ops)
	if len(got[0].Data.Ops) != 0 {
		t.Errorf("expected no ops attached (different rule), got %d", len(got[0].Data.Ops))
	}
}

func TestBuildLSPDiagnostics_MultipleOpsSameRule(t *testing.T) {
	src := []byte("aaa\nbbb\nccc\n")
	conv := lspconv.New(src)
	diags := []effect.Diagnostic{{
		Rule: "r", Message: "m", Severity: dsl.SeverityWarning,
		StartByte: 0, EndByte: 3, LineNum: 1,
	}}
	ops := []effect.Op{
		{Rule: "r", Start: 0, End: 3, Text: "A"},
		{Rule: "r", Start: 4, End: 7, Text: "B"},
	}
	got := buildLSPDiagnostics(conv, diags, ops)
	if len(got[0].Data.Ops) != 2 {
		t.Errorf("expected both ops, got %d", len(got[0].Data.Ops))
	}
}

func TestToLSPRange(t *testing.T) {
	r := lspconv.Range{
		Start: lspconv.Position{Line: 3, Character: 5},
		End:   lspconv.Position{Line: 3, Character: 9},
	}
	got := toLSPRange(r)
	if got.Start.Line != 3 || got.Start.Character != 5 {
		t.Errorf("Start = %+v", got.Start)
	}
	if got.End.Line != 3 || got.End.Character != 9 {
		t.Errorf("End = %+v", got.End)
	}
}
