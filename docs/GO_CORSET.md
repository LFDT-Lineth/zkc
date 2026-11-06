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
arguments_; and (4) _bus constraints_.

A bus constraint requires that the multiset of messages sent on a named
bus equals the multiset received on it. Modules participate through
`defsend` / `defrecv` declarations of the form:

```lisp
(defsend HANDLE BUS SELECTOR (COLUMNS...))
(defrecv HANDLE BUS SELECTOR (COLUMNS...))
```

On every row where `SELECTOR` (a binary column, mandatory) is one, the
values of `COLUMNS` form one message. Any number of senders and
receivers may share a bus name; unlike a lookup, duplicates matter (two
identical sends require two identical receives).

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
and traces. For example, we can check a trace is accepted by a set of
constraints; or, inspect the contents of a trace, etc. We now examine
a selection of the most useful top-level commands:

- [`go-corset check`](#check) allows one to check whether a given
  trace (or batch of traces) satisfies a given set of constraints. If
  a failure arises, a useful error report can be provided.
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
(lowest) or `--mir` (highest). Generally speaking, higher levels
require less work to check but, at the same time, are further removed
from the actual constraints used by the prover.

### Debug

The `go-corset debug` command provides insights into a given set of
constraints. For example, one can see the low level arithmetic
intermediate representation (AIR) generated using `--air`, or the
mid-level representation using `--mir`.

Useful options here include:

- `--source-map` will show the source map for the given constraints
  (columns, constants, etc).

- `--show-static` will include static tables when printing schemas.

### Inspect

The `go-corset inspect` command provides an interactive trace
visualisation tool, primarily intended to assist debugging. The tool
is not graphical and runs in a terminal. The tool supports general
queries over the trace (e.g. find a row where column `CT > 0`, etc).

### Trace

The `go-corset trace` command allows one to inspect and/or write out
a given trace file. Trace files are JSON (`.json`); no other format is
supported. Printing a trace requires a constraint file. With `--air`
or `--mir` the trace is first expanded (and padded) against the
constraints at that level; with `--raw` it is shown as is.

Useful options here include:

- `--out` writes the (possibly expanded) trace to the given JSON file.

- `--print` shows actual rows of the trace. Use `--start` and `--end`
  to determine the range of rows to show, along with `-f` to filter by
  module or column name, etc.
