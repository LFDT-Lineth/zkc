# RAM module

We document the constraints for random access memory (RAM) modules.
We make the following assumptions:

- the RAM may be read from and written to arbitrarily
- before an address has been written to for the first time it holds the value 0
- the RAM module must guarantee consistency across segments

## Triggering the initialization/finalization phase `FINL ≡ true`

If RAM initialization/finalization isn't tightly constrained a memory cell can
end up living parallel and wholly unrelated lives. One constraint that removes
this issue is to impose that these memory-types perform a single initialization
and finalization event per address. And here there are two options: go over a
contiguous chunk of addresses (with address increments of 1) or only initialize/
finalize active addresses. The first option is valid when address space is small
and can be expected to be filled contiguously. RV's RAM isn't in that category
given the way the linker script allocates swathes of memory in the order of mega
bytes to the program itself, inputs and the stack ... RV's registers, however,
fall in that category: address space is of size 32 and can be read beginning to
end without much trouble. The alternative and likely most useful option is to
only initialize/finalize active addresses, which requires proving address
monotony in the initialization/finalization phase.

One option (to ensure unique init/finl) is to have the RISCV zkVM/zkc interpreter
make these init/finl calls itself at specific points in time, e.g. after the
program is done executing. For instance one could have

```rust
// We interpret pc == MAX_UINT_64 as the stop signal
while pc != MAX_UINT_64 {
    instruction = read_32(pc) as Instruction
    pc = interpreter(instruction, pc)
}

// Program execution has ended, perform finalization
if pc == MAX_UINT_64 {
  // finalization of RAM
  // would require passing ram's as arguments in one way or another
  finalize(ram_1)
  finalize(ram_2)
  ...
  finalize(ram_m)
} else {
   // should be unreachable ...
   fail "Invalid final program counter %x", pc
}
```

## Ram module columns

```rust
// columns of RAM
EXEC
FINL

// slices of address columns
[]ADDRESS
[]ADDRESS_DELTA

// slices of value columns
[]VALUE_READ
[]VALUE_WRITTEN

// timestamp columns
TIMESTAMP_WRITTEN
TIMESTAMP_READ
TIMESTAMP_DELTA

IS_WRITE
```

## General constraints

```rust
// binary columns
EXEC
FINL
IS_WRITE
EXEC + FINL

// monotonous expressions (nondecreasing expressions)
FINL
EXEC + FINL
```

We differentiate between memory accesses stemming from the execution of the
guest program (`EXEC ≡ true`) and memory accesses stemming from the init/finl
phase (`FINL ≡ true`).

## Execution phase constraints

We impose the following 'execution-phase' constraints:

```rust
if EXEC = true then
  // timestamp comparisons are only meaningful if associated
  // to actual reads / writes in the execution phase
  TIMESTAMP_READ < TIMESTAMP_WRITTEN
  // Note: the constraint will be enforced with a TIMESTAMP_DELTA column via
  // TIMESTAMP_WRITTEN = TIMESTAMP_READ + 1 + TIMESTAMP_DELTA

  // value read and value written behave as expected
  TIMESTAMP_READ, []VALUE_READ = hint(ram, []ADDRESS)
  rcv( []ADDRESS, []VALUE_READ,    TIMESTAMP_READ,    )
  snd( []ADDRESS, []VALUE_WRITTEN, TIMESTAMP_WRITTEN, )

  if IS_WRITE = false then
    []VALUE_READ = []VALUE_WRITTEN

```

## Finalization phase constraints

We impose the following 'finalization-phase' constraints:

```rust
// the finalization phase does both initializations and finalizations
if FINL = true then
  // address starts at 0 and increments by 1
  if prev FINL = false then
    []ADDRESS = []0
  if prev FINL = true then
    // using []ADDRESS_DELTA
    []ADDRESS > prev( []ADDRESS )

    // initialization and finalization
  TIMESTAMP_READ, []VALUE_READ = hint(ram, []ADDRESS)
  snd( []ADDRESS, []0,          0,              ) // init
  rcv( []ADDRESS, []VALUE_READ, TIMESTAMP_READ, ) // finl
```

where

Any zkc module `MOD` that allows one to touch the RAM requires the following columns

```rust
RAM_TRIGGER
[]RAM_ADDRESS
[]RAM_VALUE            // the value retrieved by
RAM_TIMESTAMP_WRITTEN
RAM_IS_WRITE
```

and we require bilateral conditional lookups

| MOD                     | RAM                 | Notes     |
| ----------------------- | ------------------- | --------- |
| `RAM_TRIGGER`           | `EXEC`              | condition |
| `[]RAM_ADDRESS`         | `[]ADDRESS`         |           |
| `[]RAM_VALUE`           | `[]VALUE_WRITTEN`   |           |
| `RAM_TIMESTAMP_WRITTEN` | `TIMESTAMP_WRITTEN` |           |
| `RAM_IS_WRITE`          | `IS_WRITE`          |           |

### Lanes

There is an issue wrt _input lanes_: if the `[]ADDRESS` is a tuple then you need some canonical way to enumerate/list its items. Under the hood one can imagine that all components would still end up being `uX`'s for some `X`. The `VALUES_XXX` are tuples there shouldn't be much of an issue conceptually.

What to do in case of empty RAM ? It shouldn't be an issue in our RV-interpreter.
In the "you only initialize / finalize those cells that you touched" approach
you can force an interaction with address `[]0`, for instance
what

# Claude additions

## Go backing store (VM execution side)

How a `memory ram(address:...) -> (...)` is backed at execution time (distinct
from the constraints above):

- A READWRITE memory is a `*RandomAccess[W]` embedding `StaticArray[W]`, whose
  storage is a flat, dynamically-grown slice `data []W` — an **array/slice, not
  a map** (a paged variant exists for large/sparse RAM, still slices of words).
- `W` is the concrete word type, chosen per execution path:
  - reference interpreter → `word.Uint` (`big.Int`, arbitrary precision);
  - fixed-width / bytecode / gogen → `word.Uint64` (**native `uint64`**), via
    `WordToWordMachine[Uint, Uint64]`;
  - wide-word programs → `word.Uint128`.
- **Storage is one `W` cell per data word — no bit-packing.** `Decode` reserves
  `numOutputs` whole cells per row (`start = index * numOutputs`) and the VM
  advances the address by 1 per data word. So `-> (byte:u8)` stores each byte in
  its own cell; in the fast path that is a full native `uint64` (8 bytes) per
  byte. The declared width (`u8`) only bounds the value (checked at write via
  `val.FitsWithin(width)`); it never changes how many values share a cell.
- The address tuple is flattened by `Geometry.Decode` into a single `uint64`
  index (big-endian shift-or of the limbs), so the **sum of address-limb widths
  must fit in 64 bits** for slice-backed RAM.

### Layout: array-of-structs vs struct-of-arrays

If a cell needs to carry more than the value (e.g. a timestamp), there is a
choice between struct-of-arrays — `values []uint64` + `timestamps []uint64` —
and array-of-structs — `accesses []Access` with `Access{ timestamp, value uint64 }`. Both store the same 16 bytes per cell; the difference is locality.

- **Paired per-cell access (our case): prefer array-of-structs.** We almost
  always touch a cell's timestamp and value together, so keeping them adjacent
  means one cache-line fetch gets both, expansion grows one slice (one alloc +
  copy, no risk of the two arrays drifting apart), there is one backing array
  for the GC, and `accesses[i]` is a single bounds check rather than two.
- **Field-at-a-time bulk scans: prefer struct-of-arrays.** If instead you often
  scan a single field across many cells (e.g. compare all timestamps without
  reading values), the separate arrays pack the accessed field densely and
  vectorize better, whereas the struct wastes half of each cache line.
- `Access{uint64, uint64}` is 16 bytes with no padding, so array-of-structs has
  no layout waste; adding a smaller field (`bool`/`u8`) would pad to 8 bytes and
  tilt the balance toward struct-of-arrays.

The effect is real but modest — only worth optimizing if the array is hot.
