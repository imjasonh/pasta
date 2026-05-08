-- pasta language server config for Neovim.
--
-- Drop into your config (e.g. `~/.config/nvim/lua/pasta.lua`) and
-- `require("pasta")` from `init.lua`. Requires `nvim-lspconfig`.

local M = {}

-- Filetypes pasta should attach to. Mirrors the extensions registered
-- by pkg/lang/*. Add or remove as you customize your build.
local default_filetypes = {
  "go", "python", "rust", "javascript", "javascriptreact",
  "typescript", "typescriptreact", "yaml", "sh", "bash", "json",
}

function M.setup(opts)
  opts = opts or {}
  local lspconfig = require("lspconfig")
  local configs = require("lspconfig.configs")

  if not configs.pastals then
    configs.pastals = {
      default_config = {
        cmd = { "pastals" },
        filetypes = opts.filetypes or default_filetypes,
        root_dir = lspconfig.util.root_pattern(
          "pasta.cue", ".pasta", "analyzers", ".git"),
        single_file_support = true,
        init_options = opts.init_options or {
          rules = {
            "./pasta.cue",
            "./.pasta/**/*.cue",
            "./analyzers/**/*.cue",
          },
        },
      },
    }
  end
  lspconfig.pastals.setup(opts.config or {})
end

return M
