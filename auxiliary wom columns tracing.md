# Auxiliary WOM columns tracing

How to fill the auxiliary columns (`access` and `at_flag_*`) for
access-once memory (read-only + write-once) in zkc, and why this is done in the
trace **observer** rather than via a `schema.Assignment`.

Scope: `translateAccessOnceMemory` in `pkg/zkc/constraints/translator.go`
declares two kinds of computed columns:

- `access` — a 1-bit "this row is active (non-padding)" flag
- `at_flag_0 .. at_flag_{L-1}` — one-hot flags marking which address limb is the
  carry-stop when the address increments by one (only for multi-limb addresses,
  i.e. `L = len(addressRegisters) > 1`).

Throughout, address limb index `0` is the **most significant** limb
(`fm.Geometry().AddressRegisters()[0]` is the leading limb).

______________________________________________________________________

## Why an `Assignment` is NOT the right path (in zkc)

A `schema.Assignment[F]` (e.g. `pkg/ir/assignment/computed_register.go`) fills a
computed column during **trace expansion**, after the trace has been padded.
This is the canonical mechanism in the *general* go-corset pipeline, but it is
**not** how zkc fills its auxiliary columns. Evidence:

1. **zkc never registers assignments.** There are zero `AddAssignments` calls
   anywhere under `pkg/zkc/`.
1. **The precedent is the observer.** Multi-line functions declare `PC`, `RET`
   and the one-hot selector columns as `NewComputed` on the constraints side
   (`translateFunction`), yet they are filled in the trace observer —
   `assignControlRegisters` in `pkg/zkc/vm/internal/trace/full_observer.go`.
1. **ROM is filled in the observer too** — `initialiseROM` in the same file.

So in zkc, `NewComputed` means "not part of the user-facing input columns; the
observer materializes it", and there is no assignment for it. Following the same
convention for `access` / `at_flags` keeps one mechanism instead of two.

Two further, concrete problems with the assignment route specifically:

- **Padding boundary is invisible to `Compute`.** Spillage/defensive padding is
  prepended (front) *before* trace expansion and **skips computed columns**
  (`ArrayColumn.pad` is a no-op when `data == nil`; a computed column has nil
  data — `pkg/trace/array_trace.go`). By the time an assignment's `Compute`
  runs, `module.Height()` already includes the front padding rows, and `Compute`
  must return a full-height array — but it is never told how many front rows are
  padding. So it cannot reliably place the `access = 0 → 1` boundary.
- **The observer already knows everything.** The observer has the real per-row
  address/value state, so both `access` (always 1 on active rows) and the carry
  position for `at_flags` are trivial to compute there. The assignment would be
  re-deriving information that is free at trace-generation time.

Conclusion: fill `access` and `at_flags` in the observer. Delete
`pkg/zkc/constraints/at_flags_assignment.go`.

______________________________________________________________________

## Progress (status)

- [x] Step 1 — `initialiseWOM` materializes write-once rows (single- and
  multi-limb via `splitAddress`).
- [x] Column allocation — `traceModule` switch sizes `cols` to
  `m.Width()+extra` for access-once memory; `extraColumnsForAccessOnceMemory`
  returns `1` (single-limb) or `1 + nLines` (multi-limb).
- [~] Step 2 — `access` fill in `assignRomWomRegisters`: wired but **the `Set`
  result is still discarded** (Gotcha 1), so the column stays 0.
- [~] at_flags fill (multi-limb): scan + carry logic present, but three bugs:
  (a) index is `accessOffset + k` — must be `accessOffset + 1 + k` (access
  sits at `accessOffset`); (b) `Set` result discarded; (c) the `row != 0`
  guard wraps the whole set, so row 0 gets no flag and violates
  `Σ@k = access`. Fix: default `k = nLines-1`, guard only the scan loop, then
  always Set.
- [x] Step 3 — column naming: DONE. `traceColumns(regs, auxNames, cols)` now
  takes caller-supplied names for the trailing columns; `traceModule` builds
  `auxNames` per kind (`PC_NAME`/`RET_NAME`/`SelectorName` for functions,
  `io.ACCESS_BIT_NAME` + `io.AtFlagName(k)` for access-once memory). Names
  now match the schema registers, so alignment-by-name succeeds.

Carry rule (confirmed): the flag to set is `k` = highest-index (LSB-most)
non-zero address limb of the row. Compute it by scanning from `nLines-1` toward
0 and skipping rolled-over (zero) limbs: `for st.state[k].Cmp64(0) == 0 { k-- }`.
Special-case address 0 (row 0) → `k = nLines-1`. Unset flags stay 0 (zero-init).

### Gotchas found during implementation

1. **`MutArray.Set` is functional** — returns a new array; capture the result
   (`cols[i] = cols[i].Set(...)`). Discarding it leaves the column unchanged.
1. **`builder.NewArray(n, 1)` zero-initialises** (a `BitArray`), so remaining
   `at_flag` entries are `0` automatically; do not loop to zero them.
1. **Names must match the schema registers**: `access` = `io.ACCESS_BIT`,
   at_flags = `io.AtFlagName(k)` (alignment is by name —
   `trace_alignment.go:96-106`).
1. **`traceColumns` can't see the observer's `W`** (it operates on
   `util_word.BigEndian`), so it cannot type-assert the memory module. The
   trailing-column names must be decided in `traceModule` and threaded in.
1. **Two `word` packages.** `W` is constrained by
   `pkg/zkc/vm/internal/word.Word` (no `IsZero`), NOT `pkg/util/word.Word`. Test
   a limb for zero with `Cmp64(0) == 0`, not `IsZero()`.
1. **Layout indices:** `access` is at `accessOffset = len(mem.Registers())`;
   `at_flag_k` is at `accessOffset + 1 + k`.

______________________________________________________________________

## Padding: nothing to do

Emit only the **active rows** (one per memory cell), set `access = 1` on each,
and keep the declared register padding at `0` (already the case in
`translateAccessOnceMemory`).

All padding is added downstream and is **front-only**:

- `addSpillageAndDefensivePadding` → `tr.Pad(i, n, 0)` (front = n, back = 0)
- `padColumns` (the `--padding` flag) → `tr.Pad(i, n, 0)` (front only)

Padding rows are filled with each column's declared padding value (`0`). So
padding rows automatically get `access = 0` and `at_flag = 0`. This yields the
`0…0` (front padding) followed by `1…1` (active) shape that the constraints
expect:

- `ACCESS[0] = 0` (first row is padding) ✔
- `ACCESS[i] = 1 ⇒ ACCESS[i+1] = 1` (monotone; no back-padding to break it) ✔
- `Σ_k @k[i] = ACCESS[i]` (all flags 0 where access is 0) ✔

______________________________________________________________________

## Structs / interfaces

**None new.** Extend the existing `FullObserver[W, I, M]` in
`pkg/zkc/vm/internal/trace/full_observer.go` with a few functions. You are not
implementing `schema.Assignment`.

______________________________________________________________________

## Step-by-step plan

All file references are in
`pkg/zkc/vm/internal/trace/full_observer.go` unless noted.

### Step 1 — Materialize write-once (and multi-limb) memory rows

- **ELI5:** ROM rows already get built; write-once memory never does, so its
  trace is empty. Make the rows exist first.
- **Where:** `Initialise` (`:44`) only handles `*memory.ReadOnly`. Write-once
  data only exists *after* execution, so add a materialization step in `Trace`
  (`:90`) (or a helper it calls): for each write-once module, build `[]State[W]`
  from `WriteOnce.Contents()`, exactly like `initialiseROM` (`:330`).
  (`WriteOnce` embeds `StaticArray`, so `Contents()` works the same as ROM.)
- **Intent:** One `State` per active cell (`words = [address limbs…, value limbs…]`) so later steps have rows to annotate.
- **Caveat / prerequisite:** `initialiseROM` panics for `AddressLines() > 1`
  (`:338`). `at_flags` only exist for multi-limb addresses, so this step must
  also lay an address across several limbs — that panic must be removed.

### Step 2 — Emit the ACCESS (ASSIGN) column

- **ELI5:** Add one extra column that is `1` on every real row.
- **Where:** `traceModule` (`:107`). The `else` branch at `:122` allocates
  exactly `m.Width()` columns; for memory modules allocate `+1` (plus the
  at_flag columns from Step 3). After the register-transcription loop, set
  `access = 1` for every `row` in `states`. Name it `"access"` in
  `traceColumns` (`:307`).
- **Intent:** `access` becomes a real input column; `AlignTrace` matches it by
  name to the `NewComputed("access", …)` register; padding fills the rest with
  `0`.

### Step 3 — Emit the AT_FLAGS columns (multi-limb only)

- **ELI5:** For each row, mark which address limb "ticked over" when the address
  went up by one.
- **Where:** Same place in `traceModule`. With all `states` available, for row
  `i` compare the address limbs of `states[i-1]` and `states[i]`: the carry-stop
  limb `k` is where `curr[k] = prev[k] + 1` (index `0` = MSB, so the
  less-significant limbs `k < b < L` rolled over). Set `at_flag_k = 1`, the rest
  `0`. On the first active row (no real predecessor) set the flag consistent
  with `Σ @k = access = 1` (e.g. the least-significant limb). Name them
  `"at_flag_0" … "at_flag_{L-1}"` in `traceColumns`.
- **Intent:** The observer is the executable mirror of the carry constraints;
  computing it where the true addresses live guarantees the trace satisfies
  them. Emit these **only** when `len(addrRegs) > 1`, matching the constraints
  side.

### Step 4 — Padding: nothing to add

- See "Padding: nothing to do" above. Just confirm the `access` / `at_flag`
  registers keep padding `0` in `translateAccessOnceMemory` (they do).

### Step 5 — Remove the dead-end and reconcile names

- **ELI5:** Delete the wrong tool; make sure the labels match.
- **Where:** Delete `pkg/zkc/constraints/at_flags_assignment.go`. Verify the
  observer's column names (`"access"`, `"at_flag_%d"`) exactly equal the
  register names created in `translateAccessOnceMemory`, since alignment is by
  name.

______________________________________________________________________

## Suggested implementation / verification order

1. Step 1 (rows exist) — nothing else is observable without it.
1. Step 2 (access) — smallest; lets you check a single-limb write-once
   end-to-end.
1. Step 3 (at_flags) — needs multi-limb addresses (Step 1 panic removed).
1. Add a `Test_Zkc*` fixture: a write-once memory that forces a multi-limb
   carry, with `.accepts` / `.rejects` traces.

## Open design questions

- Confine `at_flags` to memory inside `traceModule`, or add a general
  "extra computed columns per module-kind" hook? (The latter is cleaner if
  read-write memory's `exec` / `finl` / timestamp columns need the same
  treatment next.)
- Remove the `AddressLines() > 1` panic / generalize memory materialization
  first, since it blocks Steps 3–4.

______________________________________________________________________

## Status — DONE (this branch)

The access-once (ROM + WOM) path is complete and green: observer fills
`access` + `at_flag` columns, multi-lane addressing works, and 5
`Test_ZkcUnit_AccessOnceMemory_*` fixtures (accepts + a genuine overflow
reject) pass. `make zkc-test` is green; `go build ./...` clean.

Key fixes that landed:

- `splitAddress` was dropping the `SetUint64` result (functional-setter trap) —
  all multi-lane addresses were 0. Fixed.
- at_flag scan guarded against `uint` underflow (`for k > 0 && …`).
- `translateAccessOnceMemory` now sets `AllowPadding = true` (the `Init` flag),
  so a leading padding row exists and `ACCESS[0] = 0` is satisfiable.
- `access_bit_monotony` reformulated backwards (`prevAccess ⇒ access`) to induce
  the front padding row.
- `initialiseROM` + WOM init merged into `initialiseAccessOnceMemory(memory.Memory[W])`.
- Test harness output comparison (`checkExpectedOutputs`) now compares canonical
  **bytes** (`EncodeBytes`) not cell arrays — length-tolerant for the odd-cell /
  trailing-padding-nibble case. Mirrors `compareGogenOutputs`.

______________________________________________________________________

## Remaining work / follow-ups (NOT done)

1. **Read-write (RAM) constraints.** `translateReadWriteMemory` is a stub
   (registers only, no constraints). Its `timestamp_read/write/delta` /
   `exec` / `finl` computed columns are declared in history but **not filled by
   the observer**, which panics ("column timestamp_write is unassigned"). This
   is the RAM analog of the WOM observer work done here.

1. **Actually *enforcing* write-once.** The current model is a **snapshot**: the
   observer emits the final memory `Contents()` (one row per address, in address
   order). This verifies the memory's *shape* but **discards write order and
   multiplicity** — so writing the same cell twice, or out of order, is NOT
   rejected (see `access_once_memory_02` / `_03`). To enforce write-once you need
   a **write-stream model**: one trace row per write op (with a timestamp), and a
   constraint that each address is written at most once. This is what `wom.md`
   sketches (timestamps + bilateral lookups), and mirrors the RAM approach.

1. **Minor robustness (from /code-review):**

   - `splitAddress` uses `MaxValue().Uint64()` — truncates address limbs wider
     than 64 bits. Assert `width <= 64` or use `big.Int`.
   - `isAccessOnceMemory` / `extraColumnsForAccessOnceMemory` `panic` on an
     unknown memory kind; consider returning false/0 instead.

1. **The `.lt` trace reader panics on `u4`/`u2` columns** (`DecodeU4Dense`
   `makeslice` underflow): `go-corset trace --print` / `inspect` can't read a
   zkc-written trace with sub-byte columns. Separate codec bug; blocks visual
   trace inspection of these modules.
