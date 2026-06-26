# Structs in ZkC — Design

## Context

ZkC already has primitives (`u8`…), field elements, type aliases, and
fixed-size arrays. We want user-defined **structs** on top, with methods,
mutation, chaining, compile-time constants, and overloading.

Guiding constraint: a struct is just a **bundle of registers** (≥1 per field).
No pointers/heap; methods are functions; a struct across a call boundary is all
its field-registers in, all out.

______________________________________________________________________

# Desirata

## Syntax

## Type declaration

```go
// type declaration
type X struct { a :A, b :B }

alias Y = X

// immutable const
const CONST_X : X = X { a: ..., b: ... }
const CONST_A : A = CONST_X.a

fn example() {
  // variable declaration
  var x  : X // zero-initialised fields
  var xx : X = X { a: ..., b: ... }

  var some_a : A

  y = xx.a              // read
  x.a = some_a          // write (mutable binding only)

  xx.b = CONST_X.b
  CONST_X.b = <expr>    // illegal
}

// add x to the input variabls of the decl.Function
fn (x:X) method_1(args...) -> (rets...) {
  x.update()
}

fn (x:X) update() {
  x.a = x.a + 1
}

// detection of attempted constant changes
CONST_U16 = 12 // illegal
CONST_X = X { a: 21,  b: 34 } // illegal
CONST_X.a = 32 // illegal
CONST_X.update() // illegal
aa = CONST_X.a // legal
cc = CONST_X.compute() // legal if compute doesn't change what it applies to

// determining if a method may mutate its receiver x
- is there any assignment of a field ?
- is there any method call on x which may mutate x ?

// the 2nd point is problematic given that on can have
fn (x:X) orange() { x.blue()   }
fn (x:X) blue()   { x.orange() }
```

```go
// maybe the criterion is "the method applies no obvious changes"
// and the default analysis state is
enum DirectMutation {
  UNKNOWN                       // default
  APPLIES_CHANGES_DIRECTLY      // by inspecting assignments, iff method body contains direct assignemnt to x or one of its fields
  DOESNT_APPLY_CHANGES_DIRECTLY // by inspecting assignments
}

enum Mutation {
  DOES_NOT_MUTATE_RECEIVER
  MAY_MUTATE_RECEIVER
}
```

Analyze every method according to whether or not it directly assigns to its receiver
ImmediateMutation goes from UNKNOWN to one of

- APPLIES_CHANGES_DIRECTLY
- DOESNT_APPLY_CHANGES_DIRECTLY

Define the recursive closure of all methods that may get invoked on the receiver: this is a set
that "stabilizes" at some point, even with self reference, cycles etc ... such as the orange / blue
case above if at any point you encounter a mehtod that has APPLIES_CHANGES_DIRECTLY Mutation
status and is applied to the method receiver, then define the method's Mutation status to be
MAY_MUTATE_RECEIVER. If none of the dependencies applied to the receiver are APPLIES_CHANGES_DIRECTLY
i.e. they're all DOES_NOT_APPLY_CHANGES_DIRECTLY, then the final Mutation status is DOES_NOT_MUTATE_RECEIVER.

my feeling is that functions are different, you pass values from what I can tell, not
pointers or references: f(x) won't change the underlying x

actually ... Is `x.update()` changing x in place even desirable ? Would it amount to in-lining
the update function ? or does it amount to
x = x.update()
?

How do other programming languages handle chaining ? E.g. Do they convert say
x.update1().update2();

into
x = x.update1()
x = x.update2()

This is honestly a complicated question. We could impose

1. a method call may only modify its receiver

- is that too restrictive ?

2. we could add a notation à la

- `(x:*X)` for receiver
- `(self)` for the returns

3. we could add 'implicit' return variables for chaining

```
type X struct { a :A, b :B }
```

Nominal types (identical fields ≠ same type). So if `X` and `Y` are two structs with the same field signature but different names (X vs Y) they are considered distinct.

Claude's concrete example is go's

```go
type Celsius float64
type Fahrenheit float64
```

**Note.** This has some consequences. I don't think that zkc will be able to support structures like this:

```go
type Node struct {
  value: uint64,
  next:  *Node,
}
```

We would need a zkc version of pointers. We can simulate pointers in memory. I.e. we could have

```
memory nodes(address:Address) -> (n:Node)

type Node struct {
  value :u64,
  next  :Address,
}
```

## Variable declaration

```
var x  : X   // mutable; fields zero-initialised
var xx : X = X { a: ..., b: ... }
```

Zero-init is free — registers already pad to 0.
Named-field literal. **Open:** partial literals (zero-fill) vs enforce-all;
positional form.

## Constant declarations of structs

There are valid reasons for wanting to be able to declare constants that are instances of a struct, for instance

```
// or some felt type
type bn254Point struct {
  x :u256,
  y :u256,
}

const BN254_POINT_AT_INFINITY :bn254Point = bn254Point {x: 0, y: 0}
const BN254_GENERATOR         :bn254Point = bn254Point {x: ?, y: ?}
```

Constant struct declarations should be immutable.

## Field access & assignment

```
y = x.a              // read
x.a = some_a         // write (mutable binding only)
```

Just selects the field's register(s).

## Homonymous field accesses

The question arises when you have two structs, say `X` and `Y`, with a field of the same name, say `a`. When you write

```
something.a
```

There will be several cases

- you are setting a constant, say

```
const a :A = something.a
// more generally an expression involving something.a
```

and `something` must itself be a constant (I imagine). Since constants must have different names, there will be no ambiguity as to what that `something` is: you will find it when parsing `Lexer.Declarations` and looking for constants.

- then there is the case of within a function body you use `something.a`; this time:
  - either the `env` is aware of a `PARAMETER`, `RETURN` or `LOCAL` variable called something, then there is no issue knowing the type;
  - or it isn't and you get an external dependency to `something` that must be resolved by the Linker; this must (?) resolve itself in a constant declaration `const something : X = ...` (or `Y`)

So in conclusion: we should be ok to resolve field accesses against `struct`'s

## Methods

Receiver method = a `fn` carrying a receiver; `x.m(a)` desugars to `m(x, a)`.
Static dispatch only (no interfaces).

```
fn (x:X) method_1(args) -> (rets) { ... }

m = x.method_1(args)
```

## Mutation

Value semantics, **explicit** return — no pointers, no implicit returns.
Persisting a change means threading the receiver out.

```
fn (x:X) modify_a(aa:A) -> X { x.a = aa; return x }
x = x.modify_a(aa)
```

Why: implicit returns would break ZkC's uniform arity/type checking. Explicit
returns map cleanly onto the call ABI (struct in *and* out).

Methods should be special functions. Maybe we should have something like

```go
// we will want Unresolved and Resolved variants
type Method[S Symbol[S]] struct {
  decl.Function          // embedded field, thanks Claude
  receiverType data.Type
  mutatesReceiver bool   // private, only accessible through method
}

func NewMethod(function decl.Function, receiverType data.Type, mutatesReceiver bool) {
  return Method{function: function, receiverType: receiverType, mutatesReceiver: mutatesReceiver, }
}

func (m Method[S]) ReceiverType()    data.Type { return m.receiverType    }
func (m Method[S]) MutatesReceiver() bool      { return m.mutatesReceiver }
```

where `mutatesReceiver` is derived recursively:

- if there is any instance of x.field being assigned to
- if there is any instance of x.method(...) where method

## Chaining

Falls out of value semantics — each call returns `X`, threaded into the next.

```
x = x.modify_a(aa).modify_a(aaa).modify_a(aaaa)
```

Composes only when every method returns the receiver type and the head is an
lvalue; from a temporary the result is discarded.

## Statement write-back (sugar)

```
x.update()           // ≡ x = x.update()
```

Rewrite `lhs.m(...)` → `lhs = lhs.m(...)` iff `lhs` is an lvalue **and** the
return type equals the receiver type. It's an implicit *assignment*, not return,
so the checker is unaffected. The type clause keeps `index = iter.index()`
(returns `u8 ≠ X`) from being rewritten. Trade-off: such a method called bare
*always* mutates `lhs` — no call-and-discard form.

## Register reuse

`x = x.update()` reuses `x`'s registers — no new columns; values just change on
the next PC step (cf. `ras.zkc`: `x=0; x=1; x=2` is one register, three rows).
**Verify:** if the frontend is SSA, the allocator must coalesce back onto the
same column.

## Mutability / `const`

Opt-in immutability — keep `var` mutable, don't flip the default.

```
const x : X = X{ a: 1, b: 2 }
```

`const` = **compile-time constant**: initializer must be fully evaluable at
compile time, folds to fixed values (no varying register). `X{ a: f(y) }` is
illegal. Checker rejects any assignment to a const (incl. write-back). Same type
machinery as `var`; only lowering differs.
**Verify:** does ZkC have a const evaluator to extend to struct literals?

## Making const structs immutable

```
const CONST_X : X = X { a: 12, b: 13 }

fn bla() {
  var x : X
  CONST_X.update()    // should blow up
  CONST_X = x.copy()  // should blow up
}
```

## Overloading

Functions identified by **full signature**, resolved at link time. Methods =
functions with a non-nil receiver.

```go
type functionDescriptor struct {
    name             string
    input_signature  []Type
    output_signature []Type
    receiver         *Type   // nil for non methods
}

func (fd functionDescriptor) String() {
  var s string;
  var res string;

  if receiver != nil {
    // more or less
    res = "(" + fd.receiver.(type).String() + ")"
  }

  res += fd.name + "("
  for i, t := range fd.input_signature {
    if i > 0 {
      res += ","
    }
    res += t.String()
  }
  res += ")("
  for i, t := range fd.output_signature {
    if i > 0 {
      res += ","
    }
    res += t.String()
  }
  res += ")"
}
```

- Derive a `Mangle()` string (e.g. `(X).update(A)->X`) as the symbol key.
- Dispatch on `(receiver, inputs)` **only** — never outputs (return type is
  unknown until the overload is picked; cf. Java/C++).
- `Type` identity: primitive | named struct (nominal) | alias (→ target).
- **Verify:** implicit conversions / untyped literals? Exact → simple
  exact-match; otherwise best-match ranking needed.

______________________________________________________________________

# Open facts to verify (read-only)

1. **Register coalescing** for `x = x.update()` (SSA?).
1. **Const evaluator** extendable to struct literals?
1. **Implicit conversions / untyped literals** → exact-match vs ranking.
1. **Where function symbols are keyed** today (name → signature change).
1. **Scope:** nested structs and arrays of structs in v1?
1. **Literals:** partial+zero-fill vs enforce-all; named vs positional.

# Status

Syntax/semantics largely agreed; the six items above are open and mostly hinge
on existing-compiler facts to verify before a concrete implementation plan.
