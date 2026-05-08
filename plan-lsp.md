# pasta LSP integration plan

Pasta's existing output shape — `effect.Diagnostic` (rule, message,
severity, byte range, line) and `effect.Op` (start, end, replacement
text) — maps almost 1:1 onto LSP's `Diagnostic` and `TextEdit`. The
work is mostly plumbing: a server binary, a byte→UTF-16 range
converter, and a configuration story for finding rule files.

## Architecture

New binary `cmd/pastals/`. Stdio JSON-RPC. Links `internal/runner`
directly — no subprocess invocation, no CLI-output scraping.

LSP types and JSON-RPC framing are hand-rolled (one ~100-line
`jsonrpc.go`, one `protocol.go` declaring only the types we touch).
Skipping `go.lsp.dev/*` keeps the dependency surface small and
removes a source of API churn — the LSP messages we use are
small, stable, and well-documented.

```
cmd/pastals/
  main.go            entry, sets up stdio rpc loop
  jsonrpc.go         Content-Length framing + rpcMessage dispatch
  protocol.go        LSP types we use (Position, Range, Diagnostic, ...)
  server.go          server struct: documents map, loaded analyzers, opts
  document.go        in-memory text buffers, byte<->UTF-16 line/col
  diagnostics.go     run internal/runner -> publishDiagnostics
  codeaction.go      effect.Op -> WorkspaceEdit, quick-fix actions
  config.go          discover/load rule files at init
```

## Lifecycle

1. **`initialize`**: read `initializationOptions.rules` (a list of
   globs, default `./pasta.cue`, `./.pasta/**/*.cue`,
   `./analyzers/**/*.cue`). Load via `internal/runner.LoadRules`. Cache
   the analyzer set on the server.

2. **`textDocument/didOpen` / `didChange`**: store full file text in
   the document map (we don't bother with incremental tree-sitter
   reparse for v1 — full reparse on each change is fast enough for
   single-file analyses). Debounce 200ms before running diagnostics.

3. **Diagnostic run**: extension → language. Call
   `runner.RunFile(ctx, path, src, analyzers, false /*applyFixes*/)`.
   Convert each `effect.Diagnostic` to LSP and publish via
   `textDocument/publishDiagnostics`. Embed the original
   `effect.Op` for the diagnostic's matching rewrite (if any) in
   `Diagnostic.Data` so the code-action handler can recover it
   without re-running the analyzer.

4. **`textDocument/codeAction`**: for each diagnostic in the
   request range, build a `CodeAction` with kind
   `quickfix` whose `edit` is the `WorkspaceEdit` derived from the
   stashed op. Also offer a `source.fixAll.pasta` action that
   applies every safe op in the file.

5. **`workspace/didChangeWatchedFiles`**: when any rule file under
   the configured globs changes, reload analyzers and re-run on
   every open document.

6. **`shutdown` / `exit`**: standard LSP teardown.

## Type mapping

| pasta | LSP |
|-------|-----|
| `effect.Diagnostic.Message` | `Diagnostic.Message` |
| `effect.Diagnostic.Rule` | `Diagnostic.Source = "pasta:" + Rule` |
| `effect.Diagnostic.Severity` (`error`/`warning`/`info`/`hint`) | `DiagnosticSeverity` (`Error`/`Warning`/`Information`/`Hint`) — direct map |
| `effect.Diagnostic.{StartByte,EndByte}` | `Diagnostic.Range` — see range conversion |
| `effect.Op.{Start,End,Text}` | `TextEdit.{Range,NewText}` |
| ordered list of `Op` | `WorkspaceEdit.Changes[uri] = []TextEdit` |

## Range conversion (the only subtle bit)

LSP positions are `(line, character)` where `character` counts
UTF-16 code units. Pasta produces byte offsets. The converter:

- Per document, build a line-start byte-offset table on update
  (one pass, `\n`-scan).
- Byte → `(line, byteCol)` is a binary search on the table.
- `byteCol` → UTF-16 col: walk the line's bytes, decode each rune,
  add `1` for BMP runes and `2` for supplementary-plane runes.

Cache the line table per document; invalidate on `didChange`. ASCII
fast path (no multi-byte runes) avoids the rune walk.

Put this in `internal/lspconv/` (separate package) so it can be unit-
tested independently of the LSP server.

## Configuration

Loaded once at `initialize` from `initializationOptions`:

```jsonc
{
  "rules": ["./pasta.cue", "./.pasta/**/*.cue", "./analyzers/**/*.cue"],
  "runOnSave": false,        // if true, only run on didSave
  "debounceMs": 200
}
```

Workspace-relative glob expansion at startup. Re-resolved when
`didChangeWatchedFiles` fires for a `*.cue` under one of the globs.

If no rules are found, the server logs a warning and serves with an
empty analyzer set (so opening pasta itself in an editor doesn't
spam errors when the workspace happens to lack a config).

## Editor integration

- **VS Code**: thin extension under `editors/vscode/`. Spawns
  `pastals` via `vscode-languageclient`. `documentSelector` registers
  every extension declared by loaded `lang/*` packages — query
  `internal/lang.AllExtensions()` and emit it during the extension's
  build. Settings panel exposes `pasta.rules` and `pasta.runOnSave`.
  On-save fixing is handled editor-side via
  `editor.codeActionsOnSave` with kind `source.fixAll.pasta`.

- **Neovim**: a `nvim-lspconfig` recipe in `editors/nvim/pasta.lua`.
  Six lines.

- **Helix**: a `editors/helix/languages.toml` snippet showing
  `[[language]] language-servers = ["pastals"]` per supported
  language.

These are documentation, not requirements — once the binary exists
any LSP client can wire it up.

## Performance

- One-shot analyzer load amortizes CUE compilation across the
  session.
- Re-parse on every change is single-file tree-sitter, microseconds
  for typical sizes. Worth measuring before adding incremental
  parsing.
- Debounce 200ms (configurable). Cancel in-flight runs when a newer
  `didChange` arrives by stashing a `context.CancelFunc` per
  document.
- Diagnostic publication is per-file; a workspace-wide refresh
  (e.g. after rule reload) fans out across open documents
  sequentially with debounce.

## Code actions in detail

Each diagnostic carries its associated `effect.Op` (or list of ops
for multi-op rewrites) in the LSP `Diagnostic.Data` field. The
client passes the diagnostic back when invoking
`textDocument/codeAction`, so the server doesn't need to re-run the
analyzer.

Three action shapes:

1. **`quickfix`** — fixes one diagnostic. Title: `"pasta: " +
   rule_name`. Built from that diagnostic's stashed ops.
2. **`source.fixAll.pasta`** — fixes every diagnostic in the
   document with a non-empty rewrite. Conflict detection mirrors
   `internal/apply.Apply` — overlapping ops drop later ops with a log
   note rather than failing the whole action.
3. **`source.organizeImports`-style placeholder** — none for v1.

The `source.fixAll.pasta` kind is what enables editor-on-save
fixing (VS Code: `editor.codeActionsOnSave`).

## Testing

- `internal/lspconv/`: table-driven byte↔UTF-16 conversion tests; ASCII,
  multi-byte, surrogate-pair, multi-line, edge cases (empty, trailing
  newline, no-final-newline).
- `cmd/pastals/`: integration tests using `testscript` (per Jason's
  preference). Spawn the binary, drive it with scripted JSON-RPC,
  assert published diagnostics. Reuse `analyzers/go_iferr/testdata/`
  as input.
- One end-to-end VS Code test would be nice but not blocking — the
  server contract is fully testable from Go.

## Limitations and alignment with future work

- **Single-file analysis**: matches pasta's current scope. When
  cross-file facts land (see future-work.md), the LSP server gains
  a workspace-wide pre-pass that runs fact-emitting rules over all
  open + watched files before per-file diagnostic runs.
- **Capture validation errors at rule-load** become `window/showMessage`
  warnings on `initialize` and on `didChangeWatchedFiles`. The
  `cue/errors.Details` formatting we just added gives the user a
  clean "rule X: ..." message.
- **No semantic tokens, hover, or completion**. Out of scope —
  pasta is a linter, not a language server in the full sense.
- **No range/format requests**. Same reason.

## Effort

- `internal/lspconv/` byte↔UTF-16 + tests: ~half day.
- `cmd/pastals/` server skeleton (init, didOpen/didChange, publish
  diagnostics): ~1 day.
- Code actions + Diagnostic.Data round-trip: ~half day.
- Watched-file rule reload + debounce: ~half day.
- VS Code thin extension: ~half day.
- Neovim/Helix recipes: ~hour.
- Tests: ~1 day.

Total: ~4-5 days for a usable v1.

## Open questions

1. Do we want diagnostics on `didChange` (responsive but noisy
   while typing) or only on `didSave` (calm but stale)? Default
   `didChange` with 200ms debounce; expose as a config knob.
   - A: Yes, `didChange` with debounce.
   
2. Should rule files outside the workspace root be loadable?
   Probably yes via absolute paths in `initializationOptions.rules`,
   but not via globs.
   - A: Agreed.

3. How to surface fix-conflict cases to the user? For `fixAll`,
   logging the dropped ops to the LSP `window/logMessage` channel
   is enough — they can see what was skipped and re-trigger a
   single-fix code action.
   - A: Sounds good.
