package pastals

import (
	"context"

	"github.com/imjasonh/pasta/pkg/dsl"
	"github.com/imjasonh/pasta/pkg/effect"
	"github.com/imjasonh/pasta/pkg/pastals/lspconv"
	"github.com/imjasonh/pasta/pkg/pastals/protocol"
	"github.com/imjasonh/pasta/pkg/runner"
)

// runDiagnostics executes pasta over doc and publishes the result.
// Safe to call concurrently with other documents; the debouncer
// ensures only one run is in flight per document.
func (s *Server) runDiagnostics(ctx context.Context, doc *document) {
	if ctx.Err() != nil {
		return
	}
	analyzers := s.analyzers()
	if len(analyzers) == 0 {
		// No rules loaded — clear any stale diagnostics.
		s.publishDiagnostics(doc.uri, doc.version, nil)
		return
	}
	res, err := runner.RunFile(ctx, doc.path, doc.src, analyzers, false)
	if err != nil {
		// Most likely "no language registered for .ext" — silently
		// clear, since the editor sent us a file we can't analyze.
		s.publishDiagnostics(doc.uri, doc.version, nil)
		return
	}
	if ctx.Err() != nil {
		// A newer change canceled us before we could publish; drop.
		return
	}
	diags := buildLSPDiagnostics(doc.conv, res.Diagnostics, res.Ops)
	s.publishDiagnostics(doc.uri, doc.version, diags)
}

// buildLSPDiagnostics converts pasta diagnostics + ops into LSP
// diagnostics, attaching each op tagged with the diagnostic's rule to
// Diagnostic.Data so the code-action handler can build a quickfix
// without re-running pasta.
//
// Pairing is by rule name (effect.Op.Rule), not by range overlap —
// rules like iferr emit a diagnostic at the assign statement and a
// rewrite that spans the surrounding if-block, so range containment
// would miss the fix. Multiple matches of the same rule in one file
// bundle all of that rule's ops onto every diagnostic; quickfixing
// any one applies them all. Splitting per-match needs the engine to
// expose match-level grouping (future work).
func buildLSPDiagnostics(conv *lspconv.Doc, ds []effect.Diagnostic, ops []effect.Op) []protocol.Diagnostic {
	opsByRule := map[string][]protocol.EncodedOp{}
	for _, op := range ops {
		opsByRule[op.Rule] = append(opsByRule[op.Rule], protocol.EncodedOp{
			Start: op.Start, End: op.End, Text: op.Text,
		})
	}
	out := make([]protocol.Diagnostic, 0, len(ds))
	for _, d := range ds {
		r := conv.RangeFromBytes(d.StartByte, d.EndByte)
		out = append(out, protocol.Diagnostic{
			Range:    toLSPRange(r),
			Severity: severityFromPasta(d.Severity),
			Source:   "pasta:" + d.Rule,
			Message:  d.Message,
			Data: &protocol.DiagnosticData{
				Rule: d.Rule,
				Ops:  opsByRule[d.Rule],
			},
		})
	}
	return out
}

// severityFromPasta maps pasta's textual severity to LSP's numeric one.
func severityFromPasta(s dsl.Severity) int {
	switch s {
	case dsl.SeverityError:
		return protocol.SeverityError
	case dsl.SeverityInfo:
		return protocol.SeverityInformation
	case dsl.SeverityHint:
		return protocol.SeverityHint
	case dsl.SeverityWarning, "":
		return protocol.SeverityWarning
	}
	return protocol.SeverityWarning
}

// toLSPRange converts an lspconv.Range to a protocol.Range.
func toLSPRange(r lspconv.Range) protocol.Range {
	return protocol.Range{
		Start: protocol.Position{Line: r.Start.Line, Character: r.Start.Character},
		End:   protocol.Position{Line: r.End.Line, Character: r.End.Character},
	}
}

// publishDiagnostics ships the params via textDocument/publishDiagnostics.
func (s *Server) publishDiagnostics(uri string, version int, diags []protocol.Diagnostic) {
	if diags == nil {
		diags = []protocol.Diagnostic{}
	}
	if err := s.conn.Notify("textDocument/publishDiagnostics", protocol.PublishDiagnosticsParams{
		URI:         uri,
		Version:     &version,
		Diagnostics: diags,
	}); err != nil {
		s.logf("publishDiagnostics: %v", err)
	}
}
