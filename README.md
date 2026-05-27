# Go Corset

The `go-corset` tool is a compiler for arithmetic constraints written
in a lisp-like [domain-specific
language](https://en.wikipedia.org/wiki/Domain-specific_language)
(called _Corset_). The tool is specifically designed for used with
the [Linea constraint
system](https://github.com/Consensys/linea-constraints/) but could, in
principle, be used elsewhere. The tool is based upon (but now
supercedes) the original Rust-based [`corset`
compiler](https://github.com/Consensys/corset) and
[language](https://github.com/Consensys/corset/wiki/The-Corset-Language).

### Table of Contents:

- [Overview](#overview)
- [Command-Line Interface](#command-line-interface)
- [Contributing](CONTRIBUTING.md)
- [Code of Conduct](CODE_OF_CONDUCT.md)
- [Developers](#developer-setup)
- [Installing LSP for ZkC](#installing-lsp-for-zkc)

## Overview

The purpose of Corset is to provide a human-friendly language for
writing arithmetic constraints. Corset constraints are compiled down
into an _Arithmetic Intermediate Representation (AIR)_ consisting of:
(1) _vanishing constraints_; (2) _range constraints_; (3) _lookup
arguments_; and (4) _permutation arguments_.

An example constraint set written in Corset is:

```lisp
(defcolumns
  (NIBBLE :i4)
  (BIT_0 :i1@prove)
  (BIT_1 :i1@prove)
  (BIT_2 :i1@prove)
  (BIT_3 :i1@prove))

;; NIBBLE = 8*BIT_3 + 4*BIT_2 + 2*BIT_1 + BIT_0
(defconstraint decomp () (eq! NIBBLE (+ BIT_0 (* 2 BIT_1) (* 4 BIT_2) (* 8 BIT_3))))
```

This constraint set ensures all rows of the column `NIBBLE` are in the
range `0..15` (inclusive) by decomposing its value into exactly four
bits. In turn, the row of each `BIT_X` column are enforced using a
range constraint (specified via the `@prove` modifier).

## Command-Line Interface

The `go-corset` tool provides a toolbox for working with constraints
and traces. For example, we can compile source constraint files
(`lisp`) into binary constraint files (`bin`); or, check a trace is
accepted by a set of constraints; or, inspect the contents of a trace
or binary constraint file, etc. We now examine a selection of the
most useful top-level commands:

- [`go-corset check`](#check) allows one to check whether a given
  trace (or batch of traces) satisfies a given set of constraints. If
  a failure arises, a useful error report can be provided.
- [`go-corset compile`](#compile). This command allows one to compile a given set
  of Corset source files into a single binary file. This is useful
  for packaging up constraints for use with other tools, etc.
- [`go-corset debug`](#debug). This command provides various ways of
  examining a given set of constraints. This is useful (amongst other
  things) for checking how certain constraints are compiled, or to
  look at summary statistics, etc.
- [`go-corset inspect`](#inspect). This command provides an interactive trace
  visualisation tool to assist debugging.
- [`go-corset trace`](#trace). This command allows ones to inspect and/or
  manipulate a given trace file in various ways.

### Check

The `go-corset check` command is used to check that one (or more)
traces satisfy (i.e. are accepted by) a set of constraints. The
_level_ at which checking is performed can be specified using `--air`
(lowest), `--mir` (middle) or `--hir` (highest). Generally speaking,
high-levels require less work to check but, at the same time, are
further removed from the actual constraints used by the prover.

### Compile

The `go-corset compile` command is used to build a binary constraint
(`bin`) file from a given set of source constraint (`lisp`) files.
During this process, metadata can be added to the `bin` file as
desired (e.g. `-Dcommit="0xabcdef01234"`). Likewise, the default
value of any externalised constants can be set
(e.g. `-Smyevm.GAS_LIMIT=0x1000`).

### Debug

The `go-corset debug` command provides insights into a given set of
constraints. For example, one can see the low level arithmetic
intermediate representation (AIR) generated; or, one can see summary
statistics (e.g. number of columns, number of constraints, etc); or,
look at any metadata embedded within a binary constraint file, etc.

Useful options here include:

- `--constants` will show the set of externalised constants defined
  in the given constraints, along with their default values.

- `--metadata` for a `bin` file, this will show any metadata that was
  embedded during compilation.

- `--stats` will show summary statistics for a given set of
  constraints, such as the number of columns, constraints, lookups,
  etc. **NOTE:** this requires one of `--air/--mir/--hir` to be
  specified (i.e. lower level representations have more
  columns/constraints which affects the stats, etc).

- `--spillage` will show the spillage determined for each module in
  the given constraints. This is the number of additional rows
  prepended to each module by `go-corset` during trace expansion.

### Inspect

The `go-corset inspect` command provides an interactive trace
visualisation tool, primarily intended to assist debugging. The tool
is not graphical and runs in a terminal. The tool supports general
queries over the trace (e.g. find a row where column `CT > 0`, etc).

### Trace

The `go-corset trace` command allows ones to inspect and/or manipulate
a given trace file in various ways. For example, one can obtain
statistical information such as the total number of cells; or, the
number of unique elements in a given column; or, print the values of
certain columns on specific rows; or, convert the trace file into a
different format (e.g. JSON); or, trim the trace file in some way
(e.g. keeping only the first `n` rows, etc); or, view any metadata
embedded within the trace file when it was generated.

Useful options here include:

- `--columns` shows column-level statistics (e.g. number of lines,
  unique elements, etc). Use `-f` to filter columns of interest.

- `--metadata` shows any embedded metadata within the trace.

- `--modules` shows module-level statistics (e.g. number of columns,
  lines, or cells, etc)

- `--out` allows one to write out the trace file in a given format
  (currently either `lt` or `json`). This can be used to convert one
  (or more) traces into a different format, and/or trim a given trace
  to some range of rows, or set of columns, etc.

- `--print` shows actual rows of the trace. Use `--start` and `--end`
  to determine the range of rows to show, along with `-f` to filter by
  module or column name, etc.

In addition, the `go-corset trace diff` subcommand allows one to
compare two traces, which is useful to identify any small differences
between traces.

## Installing LSP for ZkC

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

## Developer Setup

**Step 0.** Install [pre-commit](https://pre-commit.com/):

```shell
pip install pre-commit

# For macOS users.
brew install pre-commit
```

Then run `pre-commit install` to setup git hook scripts.
Used hooks can be found [here](.pre-commit-config.yaml).

______________________________________________________________________

NOTE

> `pre-commit` aids in running checks (end of file fixing,
> markdown linting, go linting, runs go tests, json validation, etc.)
> before you perform your git commits.

______________________________________________________________________

**Step 1.** Install external tooling (golangci-lint etc.):

```shell script
make install
```

**Step 2.** Setup project for local testing (code lint, runs tests, builds all needed binaries):

```shell script
make all
```

______________________________________________________________________

NOTE

> All binaries can be found in `<project_root>/bin` directory.
> Use `make clean` to delete old binaries.
>
> Check [Makefile](Makefile) for other useful commands.

______________________________________________________________________
