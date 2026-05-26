## ROM module

Here's one set of constraints for a write-once-memory (ROM) module. We make the following assumptions:

- a ROM is an immutable table: reads always return the same value across the full lifetime of the execution

### Triggering the finalization phase

To avoid ROM living parallel lives, there should be a single ROM finalization event.
This will force a single initialization event and a single timeline for a given address in the ROM.

```rust
// columns of a ROM
EXEC
FINL
ADDRESS // a multitude of columns, potentially
VALUE   // a multitude of columns, potentially
TIMESTAMP_READ
TIMESTAMP_WRITTEN
```

and one will impose

```rust
// binary columns
EXEC
FINL
EXEC + FINL // EXEC ∙ FINL ≡ 0

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

  // bus interactions
  // this ROM address may have already been touched, but it wasn't written to yet
  // this ROM address was previously written to
  rcv( ADDRESS, TIMESTAMP_READ,    VALUE )
  snd( ADDRESS, TIMESTAMP_WRITTEN, VALUE )

// the finalization phase does both initializations and finalizations
if FINL = true then
  // below we assume for simplicity that addresses are described by a single int
  // address starts at 0 and increments by 1
  if prev FINL = false then
    ADDRESS = 0
  if prev FINL = true then
    ADDRESS = 1 + prev(ADDRESS)

  // bus interactions
  if WAS_ALREADY_WRITTEN_TO = true then
    // address of ROM was written to at some point
    snd( ADDRESS, 0,              VALUE ) // initialization
    rcv( ADDRESS, TIMESTAMP_READ, VALUE ) // finalization
```

where

```rust
rcv( address, timestamp, is_written, value )
snd( address, timestamp, is_written, value )
```

Any zkc module `MOD` that allows one to touch the ROM requires the following columns

```rust
ROM_TRIGGER
ROM_ADDRESS
ROM_TIMESTAMP_WRITTEN
ROM_IS_WRITE
ROM_VALUE
```

and we require bilateral conditional lookups

| MOD           | ROM       | Notes     |
| ------------- | --------- | --------- |
| ROM_TRIGGER   | EXEC      | condition |
| ROM_ADDRESS   | ADDRESS   |           |
| ROM_TIMESTAMP | TIMESTAMP |           |
| ROM_IS_WRITE  | IS_WRITE  |           |
| ROM_VALUE     | VALUE     |           |
