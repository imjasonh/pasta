# pasta — Zed extension

Thin shim that tells Zed to spawn `pastals` as a language server.

## Install

1. Build and install the binary so it's on your `$PATH`:
   ```
   go install github.com/imjasonh/pasta/cmd/pastals@latest
   # or, from a checkout:
   cd /path/to/pasta && go install ./cmd/pastals
   ```
   `which pastals` should now resolve. If it doesn't, add
   `$(go env GOPATH)/bin` to your `PATH`.

2. Make sure you have a Rust toolchain with the `wasm32-wasip1`
   target (older Rust versions called this `wasm32-wasi`):
   ```
   rustup target add wasm32-wasip1
   ```

3. In Zed, open the command palette and run `zed: install dev
   extension`. Pick this directory (`editors/zed/`). Zed will compile
   the extension to WASM and load it.

## Configure

Drop a `pasta.cue`, `.pasta/`, or `analyzers/` directory in your
workspace and pasta will pick it up automatically.

Override the defaults via Zed settings (`~/.config/zed/settings.json`
or workspace `.zed/settings.json`):

```jsonc
{
  "lsp": {
    "pastals": {
      "binary": {
        "path": "/absolute/path/to/pastals"
      },
      "initialization_options": {
        "rules": [
          "/absolute/path/to/your/rules/**/*.cue"
        ],
        "debounceMs": 200,
        "runOnSave": false
      }
    }
  }
}
```

## Verify

1. Open a project containing pasta rules.
2. Open a source file that one of your rules targets.
3. Diagnostics should appear inline. The "code actions" menu should
   offer `pasta: fix <rule>` for any diagnostic with a rewrite.

For fix-on-save, configure Zed's `code_actions_on_format` (or your
editor's equivalent) to include the `source.fixAll.pasta` kind —
the server already advertises that action.

If nothing happens, tail Zed's log to see whether `pastals` was
spawned and what messages it exchanged:

```
tail -F ~/.local/share/zed/logs/Zed.log | grep -i pasta
```

## Bumping the API version

The `zed_extension_api` crate version in `Cargo.toml` is pinned. If
Zed refuses the dev-extension install with a version-mismatch error,
bump the dependency to whichever release Zed currently expects.
