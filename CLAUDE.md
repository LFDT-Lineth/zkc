# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

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
your change to keep the iteration loop tight. The slow targets (`asm-unit`,
`asm-bench`, `corset-test`) will exceed the default 2-minute `go test`
timeout if invoked directly, which is why the Makefile passes `--timeout 0`.

```shell
# ZkC compiler/VM tests — Test_ZkcBench|Test_ZkcUnit|Test_ZkcInvalid (fast)
make zkc-test

# Cheap "everything else" — skips Asm/Bench/Corset/Zkc system tests (fast)
make unit-test

# Corset constraint tests (valid/invalid/agnostic) — slow (minutes)
make corset-test   # go test -run "Test_Agnostic|Test_Valid|Test_Invalid"

# Assembly tests — slow; Test_AsmUnit_ByteShift alone takes ~2+ min
make asm-unit      # go test -run "Test_AsmInvalid|Test_AsmUnit"
make asm-util      # go test -run "Test_AsmUtil"
make asm-bench     # go test -run "Test_AsmBench"  (slowest; -p 1)

# Run a single named test
go test --timeout 0 -run "Test_Valid_Basic_01" ./pkg/test/...

# Run tests with race detection
make asm-racer
```

When iterating on ZkC changes (e.g. `pkg/zkc/...`), `make zkc-test` is the
relevant target — `make asm-unit` will not exercise ZkC code and is too slow
for a quick check. Combining `Test_ZkcUnit|Test_AsmUnit` in a single
`go test -run` invocation will time out at the default 2-minute limit.

### CLI usage

```shell
# Check a trace against constraints
./bin/go-corset check trace.lt constraints.lisp

# Compile lisp sources to binary
./bin/go-corset compile -o out.bin constraints.lisp

# Debug / inspect constraints
./bin/go-corset debug --stats --air constraints.lisp
./bin/go-corset debug --mir constraints.lisp

# Trace inspection / conversion
./bin/go-corset trace --print trace.lt
./bin/go-corset trace --out json trace.lt > trace.json

# Interactive trace visualisation
./bin/go-corset inspect trace.lt constraints.lisp
```

Key CLI flags (available globally):

- `--field <name>`: prime field to use (default `BLS12_377`; others: `KOALABEAR_16`, `GF_8209`, `GF_251`)
- `--air / --mir / --asm / --uasm / --nasm`: select constraint representation level
- `--debug`: enable debug constraints
- `-S <module.CONST=val>`: set externalised constant values
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
    → MacroHirProgram  (asm macro instructions + HIR modules)
      → Macro ASM → Micro ASM → Nano ASM  (pkg/asm/)
        → MIR modules  (pkg/ir/mir/)
          → AIR schema  (pkg/ir/air/)
```

Alternatively, pre-compiled `.bin` binary files can feed in at the top (read via `pkg/binfile/`).

The `SchemaStacker` in `pkg/cmd/corset/util/schema_stacker.go` orchestrates which layers are built and held in memory, controlled by the `--asm/uasm/nasm/mir/air` CLI flags.

Layer constants (defined in `schema_stacker.go`):

- `MACRO_ASM_LAYER = 0` – highest-level; Corset output
- `MICRO_ASM_LAYER = 1` – vectorised, field-specific
- `NANO_ASM_LAYER  = 2` – after register splitting
- `MIR_LAYER       = 3` – true constraints, higher-level view
- `AIR_LAYER       = 4` – lowest level, passed to prover

### Key packages

| Package                  | Role                                                                                                                                           |
| ------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| `pkg/corset/`            | Corset DSL compiler: parses `.lisp`, resolves symbols, type-checks, and emits a `MacroHirProgram`. Standard library embedded as `stdlib.lisp`. |
| `pkg/corset/ast/`        | AST nodes for Corset: declarations, expressions, types, bindings                                                                               |
| `pkg/corset/compiler/`   | Compiler internals: parser, resolver, type-checker, preprocessor, translator, register allocator                                               |
| `pkg/asm/`               | Assembly layer: `MacroProgram` / `MicroProgram` types, lowering (macro→micro), vectorisation, concretisation to MIR                            |
| `pkg/asm/io/`            | Core abstractions: `Instruction`, `Function`, `Component`, `Program`, bus interface (Map)                                                      |
| `pkg/asm/io/macro/`      | Macro instruction set (high-level: assign, call, cast, divide, if/goto, …)                                                                     |
| `pkg/asm/io/micro/`      | Micro instruction set (low-level: polynomial, skip_if, jmp, …); includes DFA for analysis                                                      |
| `pkg/asm/assembler/`     | Parser/linker for `.zkasm` assembly text format                                                                                                |
| `pkg/ir/hir/`            | High-level IR: `LowerToMir()` — HIR modules → MIR modules                                                                                      |
| `pkg/ir/mir/`            | Mid-level IR: `LowerToAir()` — MIR modules → AIR schema, optimiser                                                                             |
| `pkg/ir/air/`            | AIR schema: final vanishing polynomials + gadgets                                                                                              |
| `pkg/schema/`            | Core schema interfaces (`Schema`, `Module`, `Assignment`, `Constraint`) parameterised over field element type `F`                              |
| `pkg/schema/constraint/` | Constraint types: vanishing, lookup, permutation, range                                                                                        |
| `pkg/trace/`             | Trace representation; `json/` and `lt/` (binary) format readers/writers                                                                        |
| `pkg/binfile/`           | Binary `.bin` file serialisation (gob-encoded)                                                                                                 |
| `pkg/zkc/`               | ZK compiler / VM: a separate compiler+virtual machine (`pkg/zkc/vm/`) with ROM, RAM, WOM memories and a call stack                             |
| `pkg/util/field/`        | Field element implementations: `bls12_377`, `koalabear`, `gf251`, `gf8209`, `mersenne31`                                                       |
| `pkg/util/`              | General utilities: collections, iterators, source maps, math, word types                                                                       |
| `cmd/go-corset/`         | Main entry point                                                                                                                               |
| `pkg/cmd/corset/`        | CLI commands: check, compile, debug, inspect, trace, generate                                                                                  |
| `pkg/cmd/zkc/`           | CLI commands for the ZK compiler toolchain                                                                                                     |

### Schema and field polymorphism

All schemas, constraints, assignments and modules are parameterised on a field element type `F` (implementing `field.Element[F]`). Most internal work uses `word.BigEndian` as the concrete field type during compilation; field-specific code lives under `pkg/util/field/<name>/`.

The `MixedProgram[F, T, M]` type in `pkg/asm/program.go` composes assembly components (parameterised on instruction type `T`) with legacy external HIR/MIR modules (type `M`), bridging the assembly and constraint worlds.

### Testing conventions

Tests live in `pkg/test/` and are named following the pattern:

- `Test_Valid_*` — traces that must be accepted by constraints
- `Test_Invalid_*` — traces that must be rejected
- `Test_Agnostic_*` — field-agnostic tests
- `Test_AsmUnit_*` — assembly unit tests
- `Test_AsmUtil_*` / `Test_AsmBench_*` — utility and benchmark tests

Test fixtures are in `testdata/`:

- `testdata/corset/valid/`, `testdata/corset/invalid/`, `testdata/corset/agnostic/`
- `testdata/asm/unit/`, `testdata/asm/invalid/`, `testdata/asm/bench/`

Each test case consists of a `.lisp` (or `.zkasm`) source file plus `.accepts` / `.rejects` JSON trace files. Tests run against multiple fields simultaneously (e.g. `BLS12_377`, `KOALABEAR_16`, `GF_8209`).

The `FIELD_REGEX` environment variable (in `pkg/test/util/check_valid.go`) can restrict which fields are tested — useful in CI pipelines.

When adding tests, always prefer end-to-end tests (a source fixture in
`testdata/` plus `.accepts` / `.rejects` trace files, registered in the
corresponding `pkg/test/` test file) over ad-hoc Go unit tests which construct
machines or schemas programmatically. End-to-end tests exercise the full
pipeline (parser, type checker, codegen, lowering, execution) and run across
all configured fields, words and interpreter variants, whereas ad-hoc unit
tests pin implementation details and miss integration regressions.
