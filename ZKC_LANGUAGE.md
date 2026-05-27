# ZkC Language

ZkC is a simple imperative language designed primarily for writing
programs whose executions can be proved (e.g. using a [zero-knowledge
proof system](https://en.wikipedia.org/wiki/Zero-knowledge_proof) or
similar). ZkC provides a simple syntax with fixed-width unsigned
integer types and explicit memory declarations. ZkC programs are
compiled and executed by the `zkc` toolchain.

The overall model of a ZkC program is that it reads some number of
_inputs_, performs a _computation_ and writes some number of
_outputs_. The computation may additionally fail (e.g. if the inputs
were malformed). A ZkC program cannot perform other forms of I/O and
cannot, for example, interact with the operating system (e.g. to read
files, etc).

## QuickSort Example

To introduce the ZkC language, we consider a straightforward example
which showcases various features: namely, the classical
[quicksort](https://en.wikipedia.org/wiki/Quicksort) algorithm. This
program _reads in_ a set of zero or more bytes, _sorts_ them and then
_writes out_ the sorted bytes.

### Memory

The first aspect of our quicksort implementation is to define the
various forms of _memory_ required:

```zkc
// Input which identifies how many bytes to sort
input data_len(address:u1)->(len:u32)

// Input which holds bytes to be sorted
input data_in(address:u32)->(bytes:u8)

// Output where sorted bytes are written
output data_out(address:u32)->(bytes:u8)

// Scratch buffer where sorting occurrs
memory buffer(address:u32)->(bytes:u8)
```

Here, `input data_len` indicates `data_len` is an _input memory_.
This is a form of _read-only memory_ which cannot be written during
execution. Likewise, `output data_out` indicates that `data_out` is a
_write once_ output memory. This means two things: firstly, each
location of an output memory can only be written once; secondly,
locations of an output memory must be written _consecutively_
(i.e. location `0` is written first, then location `1`, etc). In
contrast, `memory buffer` indicates that `buffer` is a read-write
(i.e. random access) memory. A read-write memory can be read and
written in any order, with all locations initialised to `0` as
default. Furthermore, observe that data written into a read/write
memory is lost once execution completes. Hence, read/write memories
are typically used as a form of scratch space during the computation.

The _geometry_ of a memory determines its maximum size and
organisation. For `data_len` the _address space_ is a `u1` whilst the
_data space_ is a `u32`. This means we can have at most two entries
of `u32` (i.e. four byte) data values. In our example, we'll assume
there is always exactly one entry for `data_len` and that this
determines the length of `data_in` (i.e. the number of bytes to be
sorted).

### Functions

Functions are the fundamental building blocks of any ZkC program as
they define what computation is being performed. Every ZkC program
starts executing from the designated `main()` function which accepts
no inputs, and returns no outputs. Here is the `main` function for
our example:

```zkc
fn main<buffer>() {
  var len:u32 = data_len[0]
  // write input bytes into buffer
  read_input(len)
  // sort buffer
  sort_slice(0, len)
  // write buffer to output bytes
  write_output(len)
  return
}
```

This function begins by simply copying the bytes to be sorted into the
scratch `buffer`. This is because sorting requires an arbitrary mix of
reads / writes to the data. Once the sort is completed, the sorted
bytes are written from `buffer` into the output memory. **This is a
typical structure for ZkC programs**.

The declaration of `main<buffer>` includes `buffer` as a _memory
effect_. This is a declaration that the `main` function may access
the `buffer` memory. Whenever a function `f(...)` accesses read/write
memory `M` (inc. calling a function which accesses `M`), it must
declare this explicitly (e.g. `f<M>(...)`). Note that functions can
declare multiple memory effects as required.

The implementation of `read_input()` and `write_output()` is simple
enough:

```zkc
// Read n bytes of input data into buffer.
fn read_input<buffer>(n:u32) {
  for i:u32 = 0; i < n; i=i+1 {
    buffer[i] = data_in[i]
  }
}

// Write n bytes from buffer into output data.
fn write_output<buffer>(m:u32) {
  for i:u32 = 0; i < m; i=i+1 {
    data_out[i] = buffer[i]
  }
}
```

At this point, we can see that ZkC roughly resembles a simple
programming language like C. However, there are notable differences.
For example, parameters `n` and `m` are declared to be `u32` --- but,
unlike many languages, ZkC supports types of arbitrary bitwidth
(e.g. `u2`, `u3`, `u11`, `u15`, `u48`, `u160`, etc). Also, parameters
cannot be assigned and are immutable throughout a function. Finally,
unlike C, return values and local variables must be _defined_ before
being _used_ (i.e. ZkC enforces [definite
assignment](https://en.wikipedia.org/wiki/Definite_assignment_analysis)).

Functions can be _recursive_ and this provides an efficient way to
encode unbounded computation. For example, the `sort_slice()`
function is defined like so:

```zkc
// Sort buffer slice between offsets m (inclusive) and n (exclusive)
fn sort_slice<buffer>(m:u32, n:u32)
// PRE: m <= n
{
  var pivot:u32
  //
  if (m + 1) < n {
    // partition slice
    pivot = partition(m,n)
    // sort lower slice
    sort_slice(m,pivot)
    // sort upper slice
    sort_slice(pivot+1,n)
  }
}
```

In general, ZkC differs from a typical CPU-based programming language
in that loops are not always the most efficient way to implement
unbounded computation. Roughly speaking, the reason for this is that
function calls are translated into lookup constraints and this enables
reuse of identical calls to the same function (more on this later).

### Expressions

Finally, expressions in ZkC are fairly general as the following
illustrates:

```zkc
fn partition<buffer>(m:u32, n:u32) -> (p:u32) {
  // identify last element
  var last:u32 = n - 1
  // first element is pivot
  var pivot:u8 = buffer[last]
  // set pivot
  p = m
  //
  for i:u32=m; i < last; i=i+1 {
    var ith:u8 = buffer[i]
    //
    if ith <= pivot {
      swap(i,p)
      p = p + 1
    }
  }
  // Swap last
  swap(p,last)
}

fn swap<buffer>(i:u32,j:u32) {
  var tmp:u8 = buffer[j]
  buffer[j] = buffer[i]
  buffer[i] = tmp
}
```

One aspect of an expression, such as `a + b`, is that ZkC is strict
around argument types and does not support
[subtyping](https://en.wikipedia.org/wiki/Subtyping). For example, in
this case, `a` and `b` must have the same type (and, if not, a cast is
required to ensure the expression is well formed).

## Language Reference

We now provide a more detailed language reference which covers
subtleties not highlighted by the example above.

### Memory

Input / output memories can optionally be marked `pub` and, if not,
are considered private by default. The distinction is determined by
the proving system in use: `pub` inputs are committed to whilst
private inputs form part of [the
witness](https://en.wikipedia.org/wiki/Zero-knowledge_proof). For
example, consider a computation over some array of data. To avoid
committing to the entire set of data, we may prefer to commit only to
a _hash_ of the data which, consequently, would be a public input.
However, to open the hash (i.e. access its contents), we would need
one or more [Merkle proofs](https://en.wikipedia.org/wiki/Merkle_tree)
to establish that values of interest are indeed contained within.
Such proofs could be provided as private inputs and, hence, would be
part of.

_Static inputs_ are a form of memory whose contents is fixed in the
program source. For example, one can implement the input vectors for
the [BLAKE2 hashing
algorithm](<https://en.wikipedia.org/wiki/BLAKE_(hash_function)#BLAKE2>)
using a static input as follows:

```
static IV(address:u3) -> (iv:u64) {
  0x6a09e667f3bcc908, // IV0
  0xbb67ae8584caa73b, // IV1
  0x3c6ef372fe94f82b, // IV2
  0xa54ff53a5f1d36f1, // IV3
  0x510e527fade682d1, // IV4
  0x9b05688c2b3e6c1f, // IV5
  0x1f83d9abfb41bd6b, // IV6
  0x5be0cd19137e2179  // IV7
}
```

### Constants

Named constants can be declared at the top level and used in expressions:

```zkc
const MAX_ADDR:u8  = 255
const MASK:u8      = 0xFF
const FLAGS:u8     = 0b00001111
const SIZE:u16     = 2^10
```

Numeric literals may be written in decimal, hexadecimal (`0x…`), or binary
(`0b…`), and may use `_` as a visual separator (e.g. `0xFF_FF`).

### Variable Declarations

Local variables are declared inside a function body with an optional initialiser:

```zkc
var x:u8          // declared, initially undefined
var y:u8 = 42     // declared and initialised
```

Only a single variable may carry an initialiser per declaration statement.

### Assignments

```zkc
target(s) = expression
```

Expressions have a notion called their _arity_ which determine how
many values they produce. In most cases, the arity of an expression
is `1` --- meaning it produces exactly one value. For example, the
expression `a + b` has arity `1`. In contrast, a function call has an
arity which corresponds to the number of return values the called
function produces. The following illustrates:

```zkc
fn main() {
  ...
  val, err = compute()
  ...
}

fn compute() -> (val u32, err u1) {
  ...
}
```

Here we see that, since `compute()` has two returns the corresponding
function call requires two target variables.

A **destructuring assignment** splits a given value across a number of
smaller variables. The leftmost variable receives the **most
significant** bits; the rightmost receives the **least significant**
bits.

```zkc
var word:u32 = data[0]
var hi:u16
var lo:u16
hi::lo = word   // hi = upper 16 bits, lo = lower 16 bits
```

All target variables must already be declared. The sum of their bit widths
must equal the bit width of the right-hand side expression — mismatches are
caught at compile time.

### Loops

In addition to `if` conditions (as seen above), ZkC supports `while`
and `for` loops using a familiar syntax:

```zkc
var i:u8 = 0

while i < 10 {
  i = i + 1
  // body
}
```

The above can be written equivalently using the `for`-loop syntax:

```zkc
for i:u8 = 0; i < 10; i = i + 1 {
  // body
}
```

**NOTE**: loops in a function necessarily force it to be a so-called
_multi-line function_ (see below). As such, in many cases, it can be
more efficient (in terms of generated constraints) to use recursion.

### Exceptions

ZkC supports a `fail` instruction (similar to a `panic` in other
languages). If executed, this immediately terminates the program. More
importantly, however, is that the generated constraints cannot hold
for any execution which reaches a `fail`. The following illustrates:

```zkc
fn divide(x:u16, y:u16) -> (r:u16) {
  // sanity check that y != 0
  if y == 0 { fail }
  ...
}
```

In this case, the `fail` instruction is being used to enforce an
expected precondition to the function.

### Debugging

ZkC provides a `printf` statement for printing diagnostic output during
program execution. It is purely a debugging aid and **has no effect on the
generated constraints** — a `printf` is invisible to the ZK prover.

```zkc
printf "format string", expr1, expr2, ...
```

The format string is a double-quoted string literal containing literal text
and zero or more _format specifiers_. Each specifier consumes one argument
from the comma-separated list that follows:

| Specifier | Output format             |
| --------- | ------------------------- |
| `%d`      | decimal (e.g. `42`)       |
| `%x`      | hexadecimal (e.g. `0x2a`) |
| `%b`      | binary (e.g. `0b101010`)  |

The following escape sequences are recognised inside format strings:

| Sequence | Meaning           |
| -------- | ----------------- |
| `\n`     | newline           |
| `\t`     | horizontal tab    |
| `\r`     | carriage return   |
| `\\`     | literal backslash |

Arguments must be expressions that produce a concrete unsigned integer type.
Local variables, constants, and arithmetic/bitwise sub-expressions are all
accepted. The number of arguments must exactly match the number of format
specifiers in the string — providing too few or too many is a compile-time
error.

```zkc
fn main() {
  var x:u32 = data[0]
  var y:u32 = data[1]
  printf "x = %d, y = 0x%x\n", x, y
}
```

### Type Aliases

Type aliases introduce a new name for an existing type. They are useful
for improving readability and for defining domain-specific names (e.g.
`address` for `u160`, `bool` for `u1`):

```zkc
type address = u160
type bool    = u1
```

Circular alias definitions are rejected.

Aliases can be used anywhere a type is expected: in function parameters
and returns, variable declarations, constants, casts, and expressions.
An alias and its underlying type are interchangeable for type-checking
purposes (e.g. a `word` and a `u8` of the same bitwidth are compatible
in arithmetic, shifts, and comparisons).

```zkc
type word = u8
fn f(x:word) -> (r:u8) {
  var r = x << 1
  return
}
```

## Expressions

ZkC supports the following arithmetic operators:

| Operator | Meaning        |
| -------- | -------------- |
| `a + b`  | addition       |
| `a - b`  | subtraction    |
| `a * b`  | multiplication |

Bitwise operators:

| Operator | Meaning                               |
| -------- | ------------------------------------- |
| `a & b`  | bitwise AND                           |
| `a \| b` | bitwise OR                            |
| `a ^ b`  | bitwise XOR                           |
| `~a`     | bitwise NOT (complement within width) |
| `a << b` | left shift (result masked to width)   |
| `a >> b` | right shift                           |

All operands of a binary bitwise or shift expression must have the same
type. The result type equals the operand type. For shifts, the shift
amount must be the same type as the value being shifted; left-shift
results are masked to the declared bit width of the target.

**Parentheses are required** when mixing operators of different kinds.
Chains of the _same_ operator are permitted without extra parentheses:

```zkc
// OK — same operator chained
var r:u8 = x & y & z
var s:u8 = x << 1 << 2

// OK — different operators, disambiguated with braces
var t:u8 = (x & y) | z
var u:u8 = (x << 2) >> 1

// ERROR — mixing operators without braces
var bad:u8 = x & y | z
var bad2:u8 = x << y >> z
```

Comparison operators (used in conditions only):

| Operator | Meaning               |
| -------- | --------------------- |
| `a == b` | equal                 |
| `a != b` | not equal             |
| `a < b`  | less than             |
| `a <= b` | less than or equal    |
| `a > b`  | greater than          |
| `a >= b` | greater than or equal |

**Bitwise Concatenation.** Small values can be concatenated together
to form larger bitwise values. As for destructuring assignments, the
leftmost operand contributes the **most significant** bits.

```zkc
var hi:u16 = data[0] as u16
var lo:u16 = data[1] as u16
var word:u32
word = hi::lo   // word = (hi << 16) | lo
```

The operands of a concatenation expression can be arbitrary expressions,
not just variables:

```zkc
word = hi::(0xffee as u16)
```

The combined bit width of the operands must match the bit width of the
assignment target.

**Chaining** more than two parts works on both sides:

```zkc
var a:u8
var b:u8
var c:u16
a::b::c = some_u32   // a = bits[31:24], b = bits[23:16], c = bits[15:0]
```

All parts must be concrete unsigned integer types (`u1`, `u8`, `u16`,
etc.). Open or non-integer types are rejected by the type checker.

## File Inclusion

```zkc
include "path/to/other.zkc"
```

Included files are processed as if their declarations appeared at the include
site.

## Constraint Generation

The ultimate goal of ZkC is to compile programs down to the arithmetic
constraint systems used by ZK provers which consist of: **vanishing
constraints**, **lookup arguments**, and **permutation arguments**.

The underlying arithmetic constraint system is broken up into
_modules_, each of which is composed of one or more columns.
Polynomial constraints can be enforced on modules, whilst lookup
arguments can hold between columns either in the same module, or
between different modules. Finally, permutation arguments hold
between columns in the same module. A trace in this system is
likewise broken up into modules, where each module contains zero or
more rows of values (with one for each column).

In the ZkC system, each function body is implemented as a module
containing polynomial constraints generated from the instructions of
the function. Likewise, function calls are implemented as lookups
between modules and map parameters / returns at the call site to the
corresponding inputs / outputs of the function being called.

## CLI Usage

```shell
# Parse and type-check one or more source files; --ir prints the AST
zkc compile [--field BLS12_377] [--ir] file.zkc ...

# Execute a program; input.json provides the entry-point inputs
zkc execute [--field BLS12_377] input.json file.zkc ...
```

Available fields: `BLS12_377` (default), `KOALABEAR_16`, `GF_8209`, `GF_251`.

## Further Reading

- Source code lives under `pkg/zkc/` (compiler, AST, VM).
- Test fixtures are in `testdata/zkc/` (valid programs under `unit/`, expected
  errors under `invalid/`).
- The broader `go-corset` compilation pipeline is described in `CLAUDE.md`.
