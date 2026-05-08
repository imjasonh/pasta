// Pasta language-server Zed extension.
//
// This is a thin shim that tells Zed how to spawn the `pastals`
// binary. The binary itself must be installed on PATH (or its
// absolute path passed via the user's settings).
//
// Build via Zed's "zed: install dev extension" command pointed at
// this directory. Requires a recent stable Rust toolchain with the
// wasm32-wasi target.

use zed_extension_api::{
    self as zed, settings::LspSettings, Command, LanguageServerId, Result, Worktree,
};

struct PastaExtension;

impl zed::Extension for PastaExtension {
    fn new() -> Self {
        PastaExtension
    }

    fn language_server_command(
        &mut self,
        id: &LanguageServerId,
        worktree: &Worktree,
    ) -> Result<Command> {
        // Allow the user to override the binary path via settings:
        //
        //   "lsp": { "pastals": { "binary": { "path": "..." } } }
        let path = LspSettings::for_worktree(id.as_ref(), worktree)
            .ok()
            .and_then(|s| s.binary)
            .and_then(|b| b.path)
            .unwrap_or_else(|| "pastals".into());

        Ok(Command {
            command: path,
            args: vec![],
            env: vec![],
        })
    }

    fn language_server_initialization_options(
        &mut self,
        id: &LanguageServerId,
        worktree: &Worktree,
    ) -> Result<Option<zed::serde_json::Value>> {
        // Pass through whatever the user configured under
        //
        //   "lsp": { "pastals": { "initialization_options": { ... } } }
        //
        // Lets users override `rules`, `debounceMs`, etc. without
        // editing the extension.
        Ok(LspSettings::for_worktree(id.as_ref(), worktree)
            .ok()
            .and_then(|s| s.initialization_options))
    }
}

zed::register_extension!(PastaExtension);
