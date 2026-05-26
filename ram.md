## RAM module

Here's one set of constraints for a random access memory (RAM) module. We make the following assumptions:

- the RAM may be read from arbitrarily
- before an address has been written to for the first time it holds the value 0
- we must guarantee consistency across segments

### Triggering the finalization phase

To avoid ROM, WOM, and RAM all share the same issue wrt the logUpBus: if initialization/finalization isn't tightly constrained a memory cell can end up living many parallel lives. One constraint that removes this issue is to impose that these memory-types perform a single initialiazation/finalization event per address. Here's one way of doing this in our RISCV zkVM/zkc interpreter:

```rust
// We interpret pc == MAX_UINT_64 as the stop signal, which is set by the ecall instruction
while pc != MAX_UINT_64 {
    instruction = read_32(pc) as Instruction
    pc = interpreter(instruction, pc)
}

// executed at program end
if pc == MAX_UINT_64 {
  // finaliztion of ROM's
  finalize(rom_1)
  finalize(rom_2)
  ...
  finalize(rom_m)

  // finalization of WOM's
  finalize(wom_1)
  finalize(wom_2)
  ...
  finalize(wom_n)

  // finalization of RAM
  finalize(ram)
} else {
   // should be unreachable ...
   fail "Invalid final program counter %x", pc
}
```

```rust
// columns of RAM
EXEC
FINL
ADDRESS
TIMESTAMP_READ
TIMESTAMP_WRITTEN
VALUE_READ
VALUE_WRITTEN
IS_WRITE
```

and one will impose

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

Furthermore one wants

### The "constrain the full range output range" case

Note. The "constrain the nontrivial part of the output range" alternative has issues if the output is empty.

```rust
if EXEC = true then
  // timestamp comparisons are only meaningful if associated
  // to actual reads / writes in the execution phase
  TIMESTAMP_READ < TIMESTAMP_WRITTEN

  // value read and value written behave as expected
  TIMESTAMP_READ, VALUE_READ = hint(ram, ADDRESS)
  rcv( ADDRESS, TIMESTAMP_READ,    VALUE_READ    )
  snd( ADDRESS, TIMESTAMP_WRITTEN, VALUE_WRITTEN )

  if IS_WRITE = false then
    VALUE_READ = VALUE_WRITTEN

// the finalization phase does both initializations and finalizations
if FINL = true then
  // address starts at 0 and increments by 1
  if prev FINL = false then
    ADDRESS = 0
  if prev FINL = true then
    ADDRESS = 1 + prev(ADDRESS)

  // initialization and finalization
  TIMESTAMP_READ, VALUE_READ = hint(ram, ADDRESS)
  snd( ADDRESS, 0,              0          ) // init
  rcv( ADDRESS, TIMESTAMP_READ, VALUE_READ ) // finl
```

where

```rust
rcv( address, timestamp, value )
snd( address, timestamp, value )
```

Any zkc module `MOD` that allows one to touch the WOM requires the following columns

```rust
WOM_TRIGGER
WOM_ADDRESS
WOM_TIMESTAMP_WRITTEN
WOM_IS_WRITE
WOM_VALUE
```

and we require bilateral conditional lookups

| MOD             | WOM                 | Notes       |
| --------------- | ------------------- | ----------- |
| RAM_TRIGGER     | EXEC                | condition   |
| --------------- | ------------------- | ----------- |
| RAM_ADDRESS     | ADDRESS             |             |
| RAM_TIMESTAMP   | TIMESTAMP_WRITTEN   |             |
| RAM_IS_WRITE    | IS_WRITE            |             |
| RAM_VALUE       | VALUE_WRITTEN       |             |

### Lanes

There is an issue wrt _input lanes_: if the `ADDRESS` is a tuple then you need some canonical way to enumerate/list its items. Under the hood one can imagine that all components would still end up being `uX`'s for some `X`. The `VALUES_XXX` are tuples there shouldn't be much of an issue conceptually.

There is the question of emptyness: what to do if we have a "you only pay the initialization / finalization prize for those cells that you touched approach" and no operations were done in the various ROM/WOM/RAM's ? One simple approach to this would be to force a single interaction with every memory component.
