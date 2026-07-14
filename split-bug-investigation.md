# Register-splitting bug: subtraction & division on KOALABEAR_16

## Symptom

Two reproductions (files in `~/Downloads/`):

```
zkc exec --field KOALABEAR_16 --split break.json break.zkc
ERRO bit overflow (0xf0001 not u0, pc=0xa4)          # division

zkc exec --field KOALABEAR_16 --split break_2.json break_2.zkc
ERRO bit overflow (0x7fffe not u16, pc=0xf)          # chained subtraction
```

### break.zkc (division)

```
pub input in(a:u16) -> (v:u32)
pub output out(a:u16) -> (q:u32)
fn main() { out[0] = in[0] / in[1] }
```

`break.json`: `{ "in": "0x12345678_0000ffff" }` → in[0]=0x12345678, in[1]=0x0000ffff
Expected q = 0x12345678 / 0xffff = 0x1234 (matches no-split run: `out = 0x00001234`).

### break_2.zkc (chained subtraction)

```
pub input in(a:u16) -> (v:u32)
pub output out(a:u16) -> (d:u32)
fn main() { out[0] = in[0] - in[1] - in[2] }
```

`break_2.json`: `{ "in": "0x00030000_00010001_00018001" }`
in[0]=0x00030000, in[1]=0x00010001, in[2]=0x00018001
Expected d = 0x7ffe (matches no-split run: `out = 0x00007ffe`).

## Isolation facts

- **Without `--split`**: subtraction works (`out = 0x00007ffe`). Division without split
  produces `out=0x00001234` but then reports range/out-of-bounds errors (separate concern —
  u32 value 305419896 out of bounds on a 16-bit field, likely just because KOALABEAR_16 can't
  hold u32 directly; div splitting is the intended path).
- **With `--split` on BLS12_377**: panics "insufficient bandwidth for prime field"
  (`pkg/zkc/vm/internal/interpreter/interpreter.go:137`). So `--split` is meant for small fields.
- So the bug is in the **register-splitting transformation** for a small (16-bit) field.

The error value for sub, `0x7fffe`, does NOT fit u16 — a u16-declared register received a
20-bit value. For div, `0xf0001` assigned to a `u0` register.

## Investigation log

(updates appended below, newest observations at bottom of each section)

### TODO / hypotheses

- [x] Find where `--split` is implemented and how sub/divrem are split into limbs.
- [ ] Understand borrow handling for chained subtraction.
- [ ] Understand quotient/remainder register widths for split division.

### Finding 1: split subtraction uses an addition-style "borrow" model (SUSPECT)

Code path for SUB split:
`transform/split_registers.go` dispatches `OP_SUB` → `split.Subtraction`
(`transform/split/subtraction.go`).

Model (from the doc comment): to split `x = y - 1` where x,y are u16 into u8 limbs:

```
b::x0 = y0 - 1     (borrow b is the HIGH limb of the low chunk's target)
   x1 = y1 - b     (borrow subtracted in the next chunk)
```

`insertSubBorrowLines` allocates borrow `b` of width `rhs - lhs` and appends it
to the current chunk's LHS (as a target limb) and the next chunk's RHS (as a
subtracted source). This mirrors the ADD carry mechanism.

The runtime executes each chunk via `executeSub_nm`
(`interpreter.go:761`). On underflow it does `val = val.Slice(bitwidth)` where
`bitwidth = CalculateSubBitwidth(...)` (sign bit + max operand width), then
`storeAcross(targets, val)` splits `val` low-limb-first into the target limbs;
`storeAcross` (interpreter.go:1650) errors "bit overflow (…not uN…)" when the
value has bits left over the total target width.

### The break_2 trace (chained sub, u32 → two u16 limbs)

in[0]=0x00030000 → lo=0x0000 hi=0x0003
in[1]=0x00010001 → lo=0x0001 hi=0x0001
in[2]=0x00018001 → lo=0x8001 hi=0x0001

Split chunks:

- chunk0 (low): `b::t0 = in0_0 - in1_0 - in2_0` = 0x0000 - 0x0001 - 0x8001
- chunk1 (high): `t1     = in0_1 - in1_1 - in2_1 - b`

**Correct borrow should be 1** (−0x8002 = 0x7ffe − 1·2^16). But the borrow `b`
allocated width = rhs−lhs = 18−16 = **2 bits**, and it captures the *high bits of
the wrapped value*, not the true borrow count. The error is reported at chunk1
(target = single u16 t1) with oval 0x7fffe → does not fit u16.

### Finding 1 CONFIRMED — subtraction borrow model only works for a 1-bit borrow

Hand-trace reproduces the exact error value `0x7fffe`:

Stack word = `word.Uint` (uint64-backed). `Sub` = `bits.Sub64` (wraps to 2^64 on
underflow, returns borrow flag). `Slice(w)` masks to low w bits.

**chunk0** `b::t0 = in0_0 - in1_0 - in2_0` (b width = rhs−lhs = 18−16 = **2**):

- val = 0x0000; acc = 0x0001 + 0x8001 = 0x8002
- 0x0000 − 0x8002 underflows → wraps; `Slice(18)` (CalculateSubBitwidth=18) → 0x37ffe
- storeAcross \[t0(16), b(2)\]: t0 = 0x7ffe (✓ correct low limb), **b = 0x37ffe>>16 = 3**
- but the TRUE borrow is **1** (−0x8002 = 0x7ffe − 1·2^16).

**chunk1** `t1 = in0_1 - in1_1 - in2_1 - b` (target = single u16):

- val = 0x0003; acc = 0x0001 + 0x0001 + b(3) = 0x0005
- 0x0003 − 0x0005 underflows; CalculateSubBitwidth = 19 → `Slice(19)` → **0x7fffe**
- storeAcross \[t1(16)\]: leftover 0x7 ≠ 0 → **"bit overflow (0x7fffe not u16, pc=0xf)"** ✓

**Root cause:** the borrow limb is filled from the *high bits of the two's-complement
wrapped value* (sign-extension bits), NOT the true borrow count. For a 1-bit borrow
(single subtrahend, e.g. `y - 1`) `2^1 − 1 = 1` happens to equal the true borrow, so the
trick works — which is why the doc example and simple subtractions pass. With a **chained
subtraction** the low chunk can borrow >1, needs a ≥2-bit borrow, and the sign-extension
bits (0x3) ≠ true borrow (1). Addition is immune: its carry genuinely *is* the high bits
of a non-negative sum. This matches the observed "breaks for sub, not add/mul".

Relationship found: stored_b = 2^borrowWidth − true_borrow (mod 2^borrowWidth), so it only
coincides with true_borrow when borrowWidth = 1.

### Finding 2 CONFIRMED — division failure is the SAME subtraction bug

`transform/lower_division.go` lowers `x / y` (before splitting) to:

```
Hint{DIV_HINT, q,r,w, x,y}
qy = q * y
z0 = x - qy - r      // SubConst(z0, [x,qy,r], 0),  z0 width = u0  -> asserts == 0
z1 = y - r  - w - 1  // SubConst(z1, [y,r,w], 1),   z1 width = u0  -> asserts == 0
```

`z0 = x - qy - r` is a **chained subtraction (2 subtrahends)** whose result is truly 0
(q·y + r = x), written into a **u0** register purely as a "== 0" assertion. When register
splitting rewrites this SUB, the broken borrow makes z0 non-zero → stored into the u0
target → `bit overflow (0xf0001 not u0)`. The `u0` in the message is just the assert
register's width; the real defect is the mis-split subtraction (Finding 1).

This explains the user's recollection exactly: add & mul split correctly; **sub** is broken,
and **div** inherits the break because it is lowered into chained subtractions.

### Empirical confirmation (KOALABEAR_16 --split)

| case                    | expr                        | low-limb borrow                 | result                 |
| ----------------------- | --------------------------- | ------------------------------- | ---------------------- |
| single sub              | `in0 - in1` (underflow low) | 1                               | OK (`0x0001ffff`)      |
| chained, no low borrow  | `in0-in1-in2`               | 0                               | OK                     |
| chained, low borrow = 2 | `in0-in1-in2`               | 2                               | FAIL `0x7fffe not u16` |
| break_2 (given)         | `in0-in1-in2`               | (borrow 1 but propagates wrong) | FAIL                   |

Single subtraction always has a 1-bit borrow (a−b ∈ (−2^w, 2^w)), so it works. Chained
subtraction can borrow ≥ 2, needs a ≥2-bit borrow limb, and the model then stores the wrong
value.

## Plain-language explanation

When you use `--split` on a small field, each `u32` is cut into two `u16` limbs, and
arithmetic is redone limb-by-limb with carry/borrow between them. Addition works because a
carry is just the genuine high bits of a non-negative sum. Subtraction copies that same
trick — but a subtraction limb can go *negative*, so the "borrow" it stores is actually the
two's-complement sign-extension bits, not the real borrow amount. That mistake only cancels
out when the borrow is a single bit (one subtrahend, e.g. `a - b`). With a chained
subtraction (`a - b - c`) the low limb can borrow 2, needs a 2-bit borrow, and the stored
value is wrong (`2^borrowWidth − true_borrow`). The bad borrow then pushes the next limb out
of range → `bit overflow`. Division is the same bug in disguise: `x / y` is lowered (before
splitting) into assert-zero checks `z0 = x - q*y - r` into a `u0` register — a chained
subtraction — so it mis-splits and z0 comes out non-zero → `0xf0001 not u0`.

## Exactly what value the borrow register receives (not "k constrained to 0/1")

Representation is as you'd expect: for `a0 - a1 - … - an` (each u16), the low chunk value
`v = a0 − a1 − … − an` lies in `(−n·2^16, 2^16)`, written as `v = low − k·2^16` with
`low ∈ [0,2^16)` and true borrow `k ∈ {0,1,…,n}`.

The borrow register **is allocated wide enough** for k: width = `rhs − lhs` (here 18−16 = 2
bits, so k up to 3). So k is *not* width-constrained to {0,1}. The bug is the **value stored**:
the interpreter computes `v`, wraps it to `bitwidth` bits (two's complement), then slices the
top bits as the borrow. For a negative `v` that top slice equals

```
stored_borrow = 2^borrowWidth − k        (when v < 0; and 0 when v ≥ 0)
```

not `k`. So `stored_borrow == k` iff `2^borrowWidth − k == k`, i.e. only reliably when
borrowWidth = 1 (then k∈{0,1}, both correct). With borrowWidth = 2: k=0→0 ✓, k=1→3 ✗,
k=2→2 ✓ (coincidence), k=3→1 ✗. The next chunk subtracts this wrong borrow and goes out of
range. So: your intuition about `−k·2^16 + nonneg` is exactly the intended representation;
the defect is that the code fills the borrow with the *sign-extension bits* rather than `k`,
and that only happens to equal `k` for a single-bit borrow.

## Summary of root cause

`split.Subtraction` (`transform/split/subtraction.go`) reuses the ADD carry mechanism:
the low chunk computes `borrow::low = minuend − subtrahends`, storing the result's **high
limbs** as the "borrow" that the next chunk subtracts. For ADD this is valid (carry = the
genuine high bits of a non-negative sum). For SUB the chunk value can be **negative**; the
interpreter wraps it (two's complement, `Slice(bitwidth)`) so the high "borrow" limb holds
**sign-extension bits**, not the borrow count. These coincide only when the borrow is a
single bit; with ≥2-bit borrows (chained subtraction) `stored_borrow = 2^bw − true_borrow`,
which is wrong. The wrong borrow then makes the next chunk underflow/overflow its target.

## Fix direction (NOT yet implemented — needs approval)

The borrow-as-high-bits trick cannot represent multi-unit borrows. Options:

1. **Bias method (recommended).** Rewrite each subtraction chunk with an additive bias so
   the intermediate is always non-negative and its high limbs become a genuine *carry*
   (like ADD). Fold the constant bias into the next chunk. The codebase already uses
   "biased subtraction" in `LowerComparisons`, so there's precedent. This is the standard
   bignum limb-subtraction approach and keeps the chunk/borrow framework.
1. **Lower SUB → ADD before splitting.** Rewrite `a - b - c` as `a + (~b+1) + (~c+1)` (two's
   complement) at bytecode level, then let the working ADD splitter handle carries; mask the
   final result to width. Larger change, reuses correct ADD path.
1. **Constrain to 1-bit borrows.** Force each subtraction to a single subtrahend before
   splitting (decompose chains into a sequence of 2-operand subtractions). Simplest, but adds
   temporaries/instructions and doesn't help div's `x - qy - r` unless that is also
   decomposed.

Open questions for the user:

- Is there an intended design for split subtraction (e.g. mirror `LowerComparisons`)?
- Should the fix live in `split.Subtraction`, or as a pre-split lowering pass?

## FIX IMPLEMENTED (option 3 — pre-split decomposition)

New pass `DecomposeSubtractions` (`transform/decompose_subtraction.go`): rewrites any SUB
with ≥2 negative terms (subtrahends + non-zero constant) into a chain of single-negative-term
subtractions, each of which splits with a correct 1-bit borrow. Wired into `compile.go`
before `SplitRegisters` (both fast and non-fast split guards). Wrapper in `vm/transform.go`.

Key correctness fact (confirmed empirically): the un-split VM REJECTS negative subtraction
results (`0x3ffffffff not u32`), so results are always ≥ 0. In a decomposed chain the
partials are non-increasing and ≥ the (non-negative) final result, so no intermediate ever
wraps → decomposition is exact.

### Results after fix

- `break_2` (chained sub) → **FIXED**: `out = 0x00007ffe` (correct).
- single subtraction → still correct (no regression).
- `break` (division) → subtraction part fixed, but now fails **elsewhere**:
  `bit overflow (0x1edcc not u16, pc=0xaa)` (was `0xf0001 not u0, pc=0xa4`).

### Finding 3 CONFIRMED — division has a SECOND, independent bug: hint limb order

Instrumented the DIV_HINT: for break.zkc it stores q=0x0, r=0x56781234, w=0xa986edcb.
`r = 0x56781234` is exactly the dividend `x = 0x12345678` with its two u16 limbs **swapped**.
Cause: DIV_HINT operands are encoded as `RegisterVector{Base, Len}` (contiguous limbs,
`encoding/div.go`). The splitter lays limbs out **big-endian** — MSB at the lower register
index (e.g. `r9 = $6'1` hi, `r10 = $6'0` lo), and `split.ApplyLimbsMap` builds the vector
`[hi, lo]` so `Base = hi`. But `loadHintOperand`/`storeHintResult` (interpreter.go) treat
`Base` as the *least* significant limb (`reg = base + i`, offset increasing). So every
multi-limb hint operand (dividend, divisor, q, r, w) is limb-swapped → wrong q/r/w.

- Independent of the subtraction fix (hint runs first; only reads x,y).
- Only manifests under `--split` (single-limb operands are order-agnostic → non-split
  division works: `out = 0x00001234`).
- Fix: make `loadHintOperand`/`storeHintResult` big-endian (Base = most significant),
  matching the split layout. Single-limb (non-split) behaviour is unchanged.
- NOTE / to flag: the gogen path (`emit_field.go` emitHint) and any constraint-side
  realization of the hint likely share the same LSB assumption and need the same audit.

### Finding 4 — REGRESSION caught by test suite, then fixed with a guard

Running `make zkc-test` after the fixes: `Test_ZkcUnit_Basic_64` FAILED (regression — it
passes on baseline). basic_64 tests `result[0] = (x - y - 2) as u16` with x,y:u8 and the
subtraction result typed u10 — deliberately exercising two's-complement **wrap on underflow**
(e.g. 0-0-2 → 0x3fe mod 2^10).

Why it broke: all types (u8/u10) fit the 16-bit register width, so **no splitting occurs** —
but `DecomposeSubtractions` ran anyway (it only checked `p.config.splitting`), rewriting
`x - y - 2` into `t = x - y ; result = t - 2`. That changes the wrap modulus (the second
step wraps mod 2^11 instead of the whole expression's 2^10) → overflow.

Correction to my earlier assumption: the VM does NOT universally reject negative subtraction
results; it supports two's-complement wrap when the **result type is sized to hold it**. The
"non-negative" property only holds for genuinely-split (u32+) subtractions, where an
underflowing result no longer fits the target and is correctly rejected.

Fix: decompose ONLY subtractions that splitting will actually touch — guard on
`decomposeWidth(insn) > regWidth` (max operand/target width exceeds the field register width).
basic_64 (all ≤ u16) is now left untouched; break_2 / division (u32) are still decomposed.
Threaded `field.Config` into the pass to get `RegisterWidth`. Both repros still pass.

### Finding 5 — constraint/gogen do NOT support split subtract-with-borrow (pre-existing)

Added regression tests. A *single* (non-decomposed) u32 subtraction with a low-limb borrow
(`sub_02`, `x - y`) is accepted by the interpreter but its **constraint** fails
(`main.pc0#3 does not hold`), and GoGen errors on multi-limb hints. This is NOT my
regression: `basic_45` is already annotated *"TODO: subtract with borrow; needs u128 word
(or fast mode splitting)"* and runs with `GoGen(false).Constraints(false)`. So split
subtract-with-borrow is a known, unfinished area of the constraint/gogen paths.

Scope decision: the reported failures are `zkc exec` (bytecode interpreter). Both are fixed
there. The new regression tests (`sub_01`, `div_06`) therefore run interpreter-only
(`GoGen(false).Constraints(false)`), matching the existing `basic_45` convention. Extending
the constraint/gogen paths to split subtract-with-borrow / multi-limb hints is separate
follow-up work.

## FINAL STATUS

Fixed (bytecode interpreter / `zkc exec`):

1. Split subtraction borrow for chained (≥2 negative terms) subtraction — via new pre-split
   `DecomposeSubtractions` pass (guarded to only split-affected subtractions).
1. DIV_HINT multi-limb operand ordering — `loadHintOperand`/`storeHintResult` made big-endian
   to match the splitter's limb layout.

Both original repros now produce correct output:

- `zkc exec --split break.json break.zkc` → `out = 0x00001234`
- `zkc exec --split break_2.json break_2.zkc` → `out = 0x00007ffe`

Files changed:

- `pkg/zkc/vm/internal/transform/decompose_subtraction.go` (new pass)
- `pkg/zkc/vm/transform.go` (wrapper), `pkg/zkc/compiler/codegen/compile.go` (wiring, both split guards)
- `pkg/zkc/vm/internal/interpreter/interpreter.go` (hint limb order)
- `pkg/test/zkc_unit_test.go` + `testdata/zkc/unit/{sub_01,div_06}.{zkc,accepts,rejects}` (tests)

Known remaining gaps (pre-existing, flagged): constraint + gogen paths don't support split
subtract-with-borrow or multi-limb DIV_HINT operands (see basic_45).

### Finding 6 — evaluated "sum-then-subtract" alternative to decomposition (REJECTED on measurement)

Question raised: the current fix decomposes `a - b - c - d` into a left-to-right chain
`t0 = a-b ; t1 = t0-c ; z = t1-d`, creating n−1 intermediate registers (each a trace
column). Proposed alternative: sum all subtrahends (+const) with ONE addition, then a single
subtraction — `s = b+c+d ; z = a-s`. Rationale: addition splits with just a carry, not a
chain, and `a - s` is single-subtrahend (1-bit borrow, already handled).

Measured register count of `main` (KOALABEAR_16, --split, after SplitRegisters, before range
constraints) for `out = in0 - in1 - ... ` with N subtrahends, via a temporary ZKC_DUMP hook:

| N subtrahends | decomposition (shipped) | sum-then-subtract |
| ------------- | ----------------------- | ----------------- |
| 2             | **16**                  | 20                |
| 3             | **22**                  | 23                |
| 4             | 28                      | **26**            |
| 5             | 34                      | **29**            |
| 6             | 40                      | **32**            |
| 7             | 46                      | **35**            |
| 8             | 52                      | **38**            |
| 16            | 100                     | **62**            |

(Bold = smaller.) Closed forms: decomposition ≈ 6N+4; sum-then-subtract ≈ 3N+14. Crossover
between N=3 and N=4 — decomposition wins for N≤3, sum-then-subtract for N≥4.

Reference points: a pure ADD of 5 addends adds only **1** computed register (one carry);
a single `a - b` (u32) is 10 registers total.

Interpretation:

- Decomposition grows ~linearly (~6N): each step adds a narrow (operand-width) 2-limb temp
  plus a borrow.
- Sum-then-subtract has a lower slope but higher constant: the sum `s = Σsub` is *wider* than
  the operands (e.g. b+c of two u32 needs 33 bits → 3 limbs) and needs its own carries, so for
  small N it costs MORE than decomposition's narrow temps. Crossover confirmed between N=3 and
  N=4: decomposition wins for N≤3, sum-then-subtract for N≥4.
- **Common case is N=2** (division lowers to `z0 = x - q*y - r` and `z1 = y - r - w - 1`;
  typical user subtractions are 2 terms). There decomposition is *smaller* (16 vs 20).

**Decision: keep the decomposition.** Per the "only switch if it saves registers" criterion,
sum-then-subtract does not save for the realistic workload (it regresses N=2). It would only
help for many-subtrahend subtractions, which are rare. Reverted the experimental
sum-then-subtract implementation; shipped code remains the left-to-right decomposition.

If a future workload is dominated by high-arity subtractions, revisit: a *hybrid* (sum-then-
subtract only when N is large) would capture the tail without regressing N=2, at the cost of
extra complexity.

### (old) DEAD END / open: second division failure (0x1edcc)

Division = DIV_HINT + `qy = q*y` + `z0 = x-qy-r` + `z1 = y-r-w-1`. Subtractions now decompose
fine. Multiplication in isolation (u32\*u32→u32, same values) splits correctly. So the
`0x1edcc` (= 0xedcc + 0x10000) failure is division-specific — investigating via a temporary
`ZKC_DUMP` disassembly (env-gated dump in compile.go, to be removed). Suspect: the `qy`
register is allocated `nX` bits but its own doc comment says it must be `2*nX` (product width);
`expandDivision`/`expandRemainder` in `lower_division.go:92,121`. Verifying.
