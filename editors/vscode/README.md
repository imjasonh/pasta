# pasta — VS Code

A VS Code extension wrapping `pastals` is not yet shipped. The
language server itself is plain LSP-over-stdio and works with any
generic LSP client extension (e.g. "Generic LSP Client" on the
marketplace) configured to spawn `pastals`.

To stand up a real extension, the minimum surface is:

- `package.json` declaring `activationEvents` for each pasta-supported
  language (`onLanguage:go`, `onLanguage:python`, …)
- a `vscode-languageclient` `LanguageClient` pointing `serverOptions`
  at `pastals` and `documentSelector` at the same language list
- a configuration section exposing `pasta.rules`, `pasta.runOnSave`,
  `pasta.fixOnSave`, mapped to the server's `initializationOptions`

The diagnostic/code-action surface from `pastals` already covers
quickfix and `source.fixAll.pasta`, so editor-on-save fixing works
out of the box once the language IDs are registered:

```jsonc
"editor.codeActionsOnSave": {
  "source.fixAll.pasta": "explicit"
}
```
