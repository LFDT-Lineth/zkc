# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commits

Always commit with `git commit -s`: the project enforces the DCO (Developer
Certificate of Origin), and the PR check fails on any commit missing a
`Signed-off-by` trailer matching the author email.

## Commands

```shell
# Build the binary (outputs to bin/go-corset)
make build

# Run all tests
make test

# Run linter
make lint

# Full pipeline: clean + lint + test + build
make all

# Install tooling (golangci-lint, cobra-cli)
make install
```

### Running specific test subsets

The test targets vary widely in runtime — pick the smallest set that covers
your change to keep the iteration loop tight. The slow targets (`corset-test`,
`zkc-unit-test`, the benchmarks) will exceed the default 2-minute `go test`
timeout if invoked directly, which is why the Makefile passes `--timeout 0`.

```shell
# ZkC compiler/VM tests — Test_ZkcUnit|Test_ZkcMixed|Test_ZkcInvalid (slow)
make zkc-unit-test
make zkc-util-test   # go test -run "Test_ZkcUtil"
make zkc-bench-test  # go test -run "Test_ZkcBench"

# Cheap "everything else" — skips Bench/Corset/Zkc system tests (fast)
make unit-test

# Corset constraint tests (valid/invalid/agnostic) — slow (minutes)
make corset-test   # go test -run "Test_Agnostic|Test_Valid|Test_Invalid"
make corset-bench  # go test -run "Test_Bench" (slowest; -p 1)

# Run a single named test
go test --timeout 0 -run "Test_Valid_Basic_01" ./pkg/test/...

# Run tests with race detection
make corset-racer
make zkc-racer-test
```

When iterating on ZkC changes (e.g. `pkg/zkc/...`), `make zkc-unit-test` is the
relevant target — `make corset-test` will not exercise ZkC code. Note the
`zkc-*` targets depend on `zkc-lint`, which builds `bin/zkc` and checks the
formatting of the `.zkc` sources.

### CLI usage

```shell
# Check a trace against constraints
./bin/go-corset check trace.lt constraints.lisp

# Debug / inspect constraints
./bin/go-corset debug --air constraints.lisp
./bin/go-corset debug --mir constraints.lisp

# Trace inspection / conversion (output format follows the file extension)
./bin/go-corset trace --print trace.lt
./bin/go-corset trace --stats trace.lt
./bin/go-corset trace -o trace.json trace.lt

# Interactive trace visualisation
./bin/go-corset inspect trace.lt constraints.lisp
```

Key CLI flags (available globally):

- `--field <name>`: prime field to use (default `BLS12_377`; others: `KOALABEAR_16`, `GF_8209`, `GF_251`)
- `--air / --mir`: select constraint representation level
- `-O <n>`: optimisation level for MIR→AIR lowering

## Code navigation

Prefer the LSP tool (gopls) over grep for Go symbol work: `goToDefinition` /
`hover` for declarations, `findReferences` for call sites, `goToImplementation`
for interface implementations, and `incomingCalls` / `outgoingCalls` for
call-graph tracing. The tool schema is deferred, so load it once per session
via `ToolSearch("select:LSP")` before first use. Grep remains appropriate for
non-symbol searches (strings, comments, file patterns, test fixtures).

## Architecture

### Compilation pipeline

The central pipeline transforms `.lisp` (Corset source) into an Arithmetic Intermediate Representation (AIR) suitable for a ZK prover. The stages are:

```
.lisp source
  → Corset compiler (pkg/corset/)
    → MIR schema, field-agnostic  (pkg/ir/mir/)
      → MIR schema, concretized   (pkg/ir/mir/, via Concretize)
        → AIR schema              (pkg/ir/air/)
```

The Corset translator emits MIR directly. Since MIR arithmetic terms cannot
contain conditionals, `(if c a b)` appearing in an arithmetic position is lifted
to the logical level by case splitting — see `pkg/corset/compiler/conditional.go`.

Constraints are always compiled from source; there is no serialised form of a
compiled schema.

The `SchemaStacker` in `pkg/cmd/corset/util/schema_stacker.go` orchestrates which layers are built and held in memory, controlled by the `--mir/air` CLI flags.

Layer constants (defined in `schema_stacker.go`):

- `MIR_LAYER = 3` – true constraints, higher-level view
- `AIR_LAYER = 4` – lowest level, passed to prover

### Key packages

| Package                  | Role                                                                                                                                                     |
| ------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `pkg/corset/`            | Corset DSL compiler: parses `.lisp`, resolves symbols, type-checks, and emits a field-agnostic `mir.Schema`. Standard library embedded as `stdlib.lisp`. |
| `pkg/corset/ast/`        | AST nodes for Corset: declarations, expressions, types, bindings                                                                                         |
| `pkg/corset/compiler/`   | Compiler internals: parser, resolver, type-checker, preprocessor, translator, register allocator                                                         |
| `pkg/ir/mir/`            | Mid-level IR: `Concretize()` — split registers for target field; `LowerToAir()` — MIR modules → AIR schema, optimiser                                    |
| `pkg/ir/air/`            | AIR schema: final vanishing polynomials + gadgets                                                                                                        |
| `pkg/schema/`            | Core schema interfaces (`Schema`, `Module`, `Assignment`, `Constraint`) parameterised over field element type `F`                                        |
| `pkg/schema/constraint/` | Constraint types: vanishing, lookup, range                                                                                                               |
| `pkg/trace/`             | Trace representation; `json/` and `lt/` (binary) format readers/writers                                                                                  |
| `pkg/zkc/`               | ZK compiler / VM: a separate compiler+virtual machine (`pkg/zkc/vm/`) with ROM, RAM, WOM memories and a call stack                                       |
| `pkg/util/field/`        | Field element implementations: `bls12_377`, `koalabear`, `gf251`, `gf8209`, `mersenne31`                                                                 |
| `pkg/util/`              | General utilities: collections, iterators, source maps, math, word types                                                                                 |
| `cmd/go-corset/`         | Main entry point                                                                                                                                         |
| `pkg/cmd/corset/`        | CLI commands: check, debug, inspect, trace, verify                                                                                                       |
| `pkg/cmd/zkc/`           | CLI commands for the ZK compiler toolchain                                                                                                               |

### Schema and field polymorphism

All schemas, constraints, assignments and modules are parameterised on a field element type `F` (implementing `field.Element[F]`). Most internal work uses `word.BigEndian` as the concrete field type during compilation; field-specific code lives under `pkg/util/field/<name>/`.

### Testing conventions

Tests live in `pkg/test/` and are named following the pattern:

- `Test_Valid_*` — traces that must be accepted by constraints
- `Test_Invalid_*` — traces that must be rejected
- `Test_Agnostic_*` — field-agnostic tests
- `Test_Bench_*` — corset benchmark tests
- `Test_ZkcUnit_*` / `Test_ZkcMixed_*` / `Test_ZkcInvalid_*` — ZkC compiler/VM tests
- `Test_ZkcUtil_*` / `Test_ZkcBench_*` — ZkC utility and benchmark tests

Test fixtures are in `testdata/`:

- `testdata/corset/valid/`, `testdata/corset/invalid/`, `testdata/corset/agnostic/`, `testdata/corset/bench/`
- `testdata/zkc/unit/`, `testdata/zkc/invalid/`, `testdata/zkc/mixed/`, `testdata/zkc/util/`, `testdata/zkc/bench/`

Each test case consists of a `.lisp` (or `.zkc`) source file plus `.accepts` / `.rejects` JSON trace files. Tests run against multiple fields simultaneously (e.g. `BLS12_377`, `KOALABEAR_16`, `GF_8209`).

The `FIELD_REGEX` environment variable (in `pkg/test/util/check_legacy.go`) can restrict which fields are tested — useful in CI pipelines.

When adding tests, always prefer end-to-end tests (a source fixture in
`testdata/` plus `.accepts` / `.rejects` trace files, registered in the
corresponding `pkg/test/` test file) over ad-hoc Go unit tests which construct
machines or schemas programmatically. End-to-end tests exercise the full
pipeline (parser, type checker, codegen, lowering, execution) and run across
all configured fields, words and interpreter variants, whereas ad-hoc unit
tests pin implementation details and miss integration regressions.
