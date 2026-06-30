## WOM module

We present constraints for a write-once-memory (WOM) module with the following features:

- the WOM may be read from and written to arbitrarily subject to the following rules:
  - a read (`IS_WRITE ≡ false`) to an address that hasn't been written to (`WAS_ALREADY_WRITTEN_TO ≡ false`) returns 0
  - a write (`IS_WRITE ≡ true`) to an address that hasn't been written to (`WAS_ALREADY_WRITTEN_TO ≡ false`) flips `WAS_ALREADY_WRITTEN_TO` to `true`
  - reads and writes to an address that has already been written to (`WAS_ALREADY_WRITTEN_TO ≡ true`) reproduce the same value in the snd / rcv requests
- note that the `WAS_ALREADY_WRITTEN_TO` bit is a _prediction_
- the WOM may be written to (at a given address) once; further writes are allowed unless they overwrite a previously written value
- reads / writes may occur in all segments of the trace
- regardless, the WOM must ensure its own consistency

By definition in a WOM, once a memory cell with address `a` has been written to,
the value in memory may never change from here on out: every subsequent read,
including the finalization read, returns that value.

### Triggering the finalization phase

To avoid WOM living parallel lives, there should be a single WOM finalization event. This will force a single initialization event and a single timeline for a given address in the WOM.

```rust
// columns of a WOM
EXEC
FINL

// address columns for the address lanes
[]ADDRESS

// value columns for the value lanes
[]VALUE

TIMESTAMP_READ
TIMESTAMP_WRITTEN
TIMESTAMP_DELTA

WAS_ALREADY_WRITTEN_TO
IS_WRITE
```

and one will impose

```rust
// binary columns
EXEC
FINL
WAS_ALREADY_WRITTEN_TO
IS_WRITE

// exclusivity
EXEC ∙ FINL ≡ 0

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
  // the constraint is enforced via  TIMESTAMP_WRITTEN = TIMESTAMP_READ + 1 + TIMESTAMP_DELTA

  // bus interactions
  // this WOM address may have already been touched, but it wasn't written to yet
  if WAS_ALREADY_WRITTEN_TO = false then
    rcv( []ADDRESS, []0,     TIMESTAMP_READ,    false,    ) // read
    snd( []ADDRESS, []VALUE, TIMESTAMP_WRITTEN, IS_WRITE, ) // write (maybe)
    if IS_WRITE = false then []VALUE = []0

  // this WOM address was previously written to
  if WAS_ALREADY_WRITTEN_TO = true then
    rcv( []ADDRESS, []VALUE, TIMESTAMP_READ,    true, ) // read
    snd( []ADDRESS, []VALUE, TIMESTAMP_WRITTEN, true, ) // write (the same value)

// the finalization phase does both initializations and finalizations
if FINL = true then
  // address starts at 0 and increments by 1
  if prev FINL = false then
    []ADDRESS = []0
  if prev FINL = true then
    []ADDRESS = 1 + prev( []ADDRESS )

  // no writes take place in the finalization phase
  IS_WRITE = false

  // bus interactions
  if WAS_ALREADY_WRITTEN_TO = false then
    // address of WOM was never written to
    snd( []ADDRESS, []0, 0,              false, ) // initialization, set initial value to 0
    rcv( []ADDRESS, []0, TIMESTAMP_READ, false, ) // finalization, read final value for what it is, 0
  if WAS_ALREADY_WRITTEN_TO = true then
    // address of WOM was written to at some point
    snd( []ADDRESS, []0,     0,              false, ) // initialization, set initial value to 0
    rcv( []ADDRESS, []VALUE, TIMESTAMP_READ, true,  ) // finalization, read final value for what it is
```

where

```rust
rcv( []address, []value, timestamp, has_been_written_to, )
snd( []address, []value, timestamp, has_been_written_to, )
```

Any zkc module `MOD` that allows one to touch the WOM requires the following columns

```rust
WOM_TRIGGER
[]WOM_ADDRESS
[]WOM_VALUE
WOM_TIMESTAMP_WRITTEN
WOM_IS_WRITE
```

and we require bilateral conditional lookups

| MOD           | WOM       | Notes     |
| ------------- | --------- | --------- |
| WOM_TRIGGER   | EXEC      | condition |
| []WOM_ADDRESS | []ADDRESS |           |
| []WOM_VALUE   | []VALUE   |           |
| WOM_TIMESTAMP | TIMESTAMP |           |
| WOM_IS_WRITE  | IS_WRITE  |           |
