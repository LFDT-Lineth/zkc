# Abstract Syntax Tree (AST)

The front end parses `zkc` source files and produces an _Abstract Syntax
Tree_ (AST) — a structured, in-memory representation of the program
that subsequent compiler phases can analyse and transform. The AST is
a tree where each node represents a syntactic construct. The root of
the tree is a **program**, which is a list of top-level
**declarations** (functions, memories, constants, type_alias). A function
declaration carries a list of typed **variables** (parameters, return
values, and locals) together with a sequence **statements** which
constitute its body.

The concrete node types are:

- **Declarations** (`pkg/zkc/compiler/ast/decl/`) — `Function`, `Memory`
  (read-only input, write-once output, or read/write RAM), `Constant` and `TypeAlias`.
- **Statements** (`pkg/zkc/compiler/ast/stmt/`) — `Assign`, `IfGoto`
  (conditional branch), `Goto` (unconditional branch), `Return`, and `Fail`.
- **Expressions** (`pkg/zkc/compiler/ast/expr/`) — `Add`, `Sub`, `Mul`,
  `Const` (numeric literal), `LocalAccess` (read a local variable), and
  `NonLocalAccess` (reference to a constant or memory declared elsewhere).
- **Types** (`pkg/zkc/compiler/ast/data/`) — currently `UnsignedInt` (`uN`)
  and `Tuple` (a composite of multiple `uN` fields used for multi-word memory
  buses).

A subtle aspect here is the handling of structured control flow
(e.g. `if`/`else`, `while`, `for`) as found in `zkc` source files.
Specifically, it is lowered during parsing into an _unstructured_ form
comprising of conditional (`IfGoto`) and unconditional branches (`Goto`).

As an example, consider the following `zkc` function:

```
// compute n^m
fn pow(n:u4, m:u4) -> (r:u4) {
   var i:u8 = 0
   r = 1

   while i < m {
     r = r * n
     i = i + 1
   }

   return
}
```

This can be compiled (using e.g. `zkc compile --ir test.zkc`) into
the following AST form:

```
fn pow(n:u4, m:u4) -> (r:u4) {
        var i:u8
[0]     i = 0
[1]     r = 1
[2]     if i>=m goto 6
[3]     r = r * n
[4]     i = i + 1
[5]     goto 2
[6]     return
```

Here, the _Program Counter (PC)_ locations are given on the left of
each instruction. We can see how the original `while` loop was
transformed into a flat instruction sequence using a conditional
`if`/`goto` branch and an unconditional `goto`. Here, for example,
`goto 2` indicates that control flow branches to instruction `[2]` at
this point.

## Parsing

The parser (`pkg/zkc/compiler/parser/`) converts a `zkc` source file
into an `UnlinkedSourceFile` — a list of declarations whose cross-file
references are still _unresolved_ string names. It proceeds in the
usual fashion using two phases:

1. **Lexing** — the source text is tokenised into a flat token stream.
   Whitespace and comments are discarded. The lexer recognises keywords
   (`fn`, `const`, `pub`, `memory`, `input`, `output`, `if`, `while`,
   `for`, `return`, `fail`, `var`, `include`, `type` …), identifiers, numeric literals (decimal,
   hex, binary, and `2^N` exponent form), and operator symbols.

1. **Recursive-descent parsing** — the token stream is consumed
   top-down by a hand-written recursive-descent parser. At the top
   level it dispatches on the leading keyword to parse each
   declaration kind (`parseFunction`, `parseConstant`,
   `parseInputOutputMemory`, `parseReadWriteMemory`, `parseInclude`).
   Inside a function body, statements are parsed one at a time;
   structured control flow (`if`/`else`, `while`, `for`) is
   immediately lowered to sequences of `IfGoto` and `Goto`
   instructions so that all later phases work with a uniform, flat
   instruction list. A source map is built in parallel, recording the
   span of the original source file that corresponds to each
   instruction (for use in later error messages).

The parser halts and returns the accumulated errors on the first declaration
that cannot be parsed, rather than attempting error recovery.

## Linking

References between declarations in the AST (a function calling another
function, an expression reading a constant defined in a different
file, etc.) are initially represented as **unresolved symbols** —
plain string names. The _linker_ (`pkg/zkc/compiler/linker.go`)
replaces these with **resolved symbols** that point directly to the
target declaration, and reports errors when symbols cannot be found,
or are of wrong kind, etc.

## Typing

ZkC is somewhat unusual in that it uses an **integer-range type
system** (`pkg/zkc/compiler/validate/typing/`). Rather than tracking
only the declared type of a variable, the type checker propagates the
_maximum possible value_ of every sub-expression. This allows an
accurate calculation for the minimum bit-width required to store any
evaluation of that expression (which is needed later when lowering
into the IR layer).

## Control-Flow Analysis

Control-flow analysis (`pkg/zkc/compiler/validate/control_flow.go`)
checks certain properties of every function using a [dataflow
analysis](https://en.wikipedia.org/wiki/Data-flow_analysis):

1. **Definite assignment** — every variable must be [definitely
   assigned](https://en.wikipedia.org/wiki/Definite_assignment_analysis)
   before it is read.

1. **Termination** — every control-flow path must end at a `return` or
   `fail` instruction and, furthermore, all return values must be
   assigned whenever a `return` statement is reached.

1. **Reachability** - every instruction must be reachable via a path
   from the start of the function.

Errors are reported when these properties do not hold. For example,
the following program fails definite assignment:

```
fn f(n:u4) -> (r:u8) {
   var i:u4
   r = i + n
   return
}
```

Attempting to compile this produces an error `variable i possibly undefined` which is generated by the control-flow analysis.

## Lowering

## Code Generation

Code generation (`pkg/zkc/compiler/codegen/`) translates the validated
program into a _boot machine_ --- a representation of the program at
the IR layer below. This is intentionally simpler than the AST
representation and many features found at the AST layer do not exist
at the IR layer (e.g. compound types such as `struct`s or fixed-size
arrays).
