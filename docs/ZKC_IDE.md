## IDE Support for ZkC

The `zkc` tool includes a built-in Language Server Protocol (LSP) server for
the ZkC language (`.zkc` files). The server communicates over stdio using
JSON-RPC 2.0, which makes it compatible with any LSP-capable editor.

### Building the `zkc` binary

First, build the `zkc` binary:

```shell
go build -o bin/zkc cmd/zkc/main.go
```

Then copy (or symlink) it somewhere on your `$PATH`, for example:

```shell
cp bin/zkc /usr/local/bin/zkc
```

The LSP server is invoked by the editor automatically using:

```shell
zkc lsp
```

### Neovim

Requires Neovim 0.9 or later (semantic token support was added in 0.9).

#### Neovim 0.11+ (native LSP, no plugins required)

Add the following to your Neovim configuration:

**`~/.config/nvim/lua/autocmd.lua`** (or equivalent — anywhere that runs at startup):

```lua
vim.filetype.add({ extension = { zkc = 'zkc' } })
```

**`~/.config/nvim/lsp/zkc.lua`**:

```lua
return {
  cmd = { 'zkc', 'lsp' },
  filetypes = { 'zkc' },
  root_markers = { '.git' },
}
```

**`~/.config/nvim/lua/lsp.lua`** (or wherever you call `vim.lsp.enable`):

```lua
vim.lsp.enable({ 'zkc' })
```

To enable `gc`/`gcc` commenting, add **`~/.config/nvim/ftplugin/zkc.lua`**:

```lua
vim.bo.commentstring = '// %s'
```

#### Neovim 0.9+ (via nvim-lspconfig)

Install [nvim-lspconfig](https://github.com/neovim/nvim-lspconfig) and add the
following to your Neovim configuration (e.g. `~/.config/nvim/init.lua`):

```lua
-- Associate the .zkc extension with the zkc filetype
vim.filetype.add({ extension = { zkc = 'zkc' } })

local lspconfig = require('lspconfig')
local configs = require('lspconfig.configs')

-- Register the zkc language server
if not configs.zkc then
  configs.zkc = {
    default_config = {
      cmd = { 'zkc', 'lsp' },
      filetypes = { 'zkc' },
      -- Fall back to cwd if there is no .git root
      root_dir = function(fname)
        return lspconfig.util.root_pattern('.git')(fname) or vim.fn.getcwd()
      end,
      -- Pass the full client capabilities so the server receives the
      -- semantic token support flags
      capabilities = vim.lsp.protocol.make_client_capabilities(),
    },
  }
end

lspconfig.zkc.setup {}
```

### Emacs

Install [lsp-mode](https://emacs-lsp.github.io/lsp-mode/) and add the
following to your configuration:

```elisp
;; Define a simple major mode for .zkc files
(define-derived-mode zkc-mode prog-mode "ZkC"
  "Major mode for ZkC source files.")
(add-to-list 'auto-mode-alist '("\\.zkc\\'" . zkc-mode))

;; Enable semantic token highlighting (off by default in lsp-mode).
(setq lsp-semantic-tokens-enable t)

;; Configure Language ID
(with-eval-after-load 'lsp-mode
  (add-to-list 'lsp-language-id-configuration
               '(zkc-mode . "zkc")))

;; Register the zkc language server with lsp-mode
(with-eval-after-load 'lsp-mode
  (lsp-register-client
   (make-lsp-client
    :new-connection (lsp-stdio-connection '("zkc" "lsp"))
    :activation-fn (lsp-activate-on "zkc")
    :server-id 'zkc)))

;; Automatically start lsp-mode when opening a .zkc file
(add-hook 'zkc-mode-hook 'lsp)
```
