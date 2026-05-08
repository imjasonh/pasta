package protocol

import (
	"encoding/json"
	"testing"
)

// TestDiagnosticJSON pins the wire shape of Diagnostic — fields,
// names, and ordering of optional fields. The format is what editors
// will see; regressions here are user-visible.
func TestDiagnosticJSON(t *testing.T) {
	d := Diagnostic{
		Range: Range{
			Start: Position{Line: 2, Character: 4},
			End:   Position{Line: 2, Character: 9},
		},
		Severity: SeverityWarning,
		Source:   "pasta:iferr",
		Message:  "if-err pattern can be inlined",
		Data: &DiagnosticData{
			Rule: "iferr_inline",
			Ops: []EncodedOp{
				{Start: 10, End: 20, Text: "x := y()"},
			},
		},
	}
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"severity": "2",
		"source":   `"pasta:iferr"`,
		"message":  `"if-err pattern can be inlined"`,
	}
	for k, want := range want {
		bb, _ := json.Marshal(got[k])
		if string(bb) != want {
			t.Errorf("%s = %s, want %s", k, string(bb), want)
		}
	}
	// Round-trip preserves value.
	var back Diagnostic
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back.Source != d.Source || back.Severity != d.Severity {
		t.Errorf("round-trip mismatch: got %+v", back)
	}
	if back.Data == nil || back.Data.Rule != "iferr_inline" {
		t.Errorf("data lost in round-trip: %+v", back.Data)
	}
	if len(back.Data.Ops) != 1 || back.Data.Ops[0].Text != "x := y()" {
		t.Errorf("ops lost: %+v", back.Data.Ops)
	}
}

// TestSeverityConstants pins the LSP severity codes.
func TestSeverityConstants(t *testing.T) {
	cases := map[int]int{
		SeverityError:       1,
		SeverityWarning:     2,
		SeverityInformation: 3,
		SeverityHint:        4,
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("severity = %d, want %d", got, want)
		}
	}
}

// TestRangeOmitEmpty - omitempty on optional fields shouldn't make
// required Range fields disappear.
func TestRangeMarshalsEvenAtZero(t *testing.T) {
	r := Range{}
	b, _ := json.Marshal(r)
	want := `{"start":{"line":0,"character":0},"end":{"line":0,"character":0}}`
	if string(b) != want {
		t.Errorf("Range{} = %s, want %s", string(b), want)
	}
}

// TestDiagnosticDataOmittedWhenNil - we shouldn't emit a stray "data":null.
func TestDiagnosticDataOmittedWhenNil(t *testing.T) {
	d := Diagnostic{Severity: SeverityHint, Message: "no fix"}
	b, _ := json.Marshal(d)
	var raw map[string]json.RawMessage
	_ = json.Unmarshal(b, &raw)
	if _, present := raw["data"]; present {
		t.Errorf("data field present when nil: %s", string(b))
	}
}
