# ROM (read ONCE memory) module

Here's one set of constraints for a read-once-memory (ROM) module.
We make the following assumptions:

- a ROM is an immutable table
- addresses are consecutive, start at 0; there are no gaps
- the full contents of that memory will be read start to finish at one specific point in time
- the ROM is never touched again

**Note.** The fact that this data is read **once** must be assured by the zkc program itself.
The ROM has no control over it. If the ROM were to enforce this we would have to add a source
time stamp column, that predicts the source module's time stamp.

The reason for these assumptions are

- they correspond to real use-cases
  - ROM for sections from the compiled guest program
  - ROM for inputs to the guest program
  - both get loaded in full at the start of execution and are never touched again after that
- there is no need to ensure coherence over time and over segments since there will be a single read of any cell

## ROM (once variant) vs WOM

There will be no difference between the two

## Constraints / columns side

The entire thing can be arranged as a single lookup

```rust
// columns of the source
ACCESS    // determines padding (ACCESS ≡ 0) and non padding rows (ACCESS ≡ 1)
[]ADDRESS // slice of columns that define the address space
[]VALUE   // slice of columns that define the value space

// binary constraint
ACCESS
```

and one will impose

```rust
ACCESS[0] = False
if ACCESS = False:
    []ADDRESS = 0

if ACCESS = True:
    next( ACCESS ) = True
    []ADDRESS = 1 + prev( []ADDRESS )
```

## Requirements for sources adding to the bus

Any zkc module `MOD` that may read a ROM requires the following columns

```rust
ROM_ACCESS
[]ROM_ADDRESS
[]ROM_VALUE
```

and we require bilateral conditional lookups

| MOD             | ROM         | Notes       |
| --------------- | ----------- | ----------- |
| ROM_ACCESS      | ACCESS      | condition   |
| --------------- | ----------- | ----------- |
| []ROM_ADDRESS   | []ADDRESS   |             |
| []ROM_VALUE     | []VALUE     |             |
