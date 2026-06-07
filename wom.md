## WOM module

Here's one set of constraints for a write-once-memory (WOM) module. We make the following assumptions:

- the WOM may be read from arbitrarily: before it's been written to reads should return 0
- multiple writes at a given address are allowed unless they overwrite a previously written value

By definition in a WOM, once a memory cell with address `a` has been written to, the value in memory may never change from here on out: every subsequent read, including the finalization read, returns that value. A WOM module will have the following columns.

### Triggering the finalization phase

To avoid WOM living parallel lives, there should be a single WOM finalization event. This will force a single initialization event and a single timeline for a given address in the WOM.

```rust
// columns of a WOM
EXEC
FINL
ADDRESS
TIMESTAMP_READ
TIMESTAMP_WRITTEN
VALUE
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

  // bus interactions
  // this WOM address may have already been touched, but it wasn't written to yet
  if WAS_ALREADY_WRITTEN_TO = false then
    rcv( ADDRESS, TIMESTAMP_READ,  false,    0     )
    snd( ADDRESS, TIMESTAMP_WRITTEN, IS_WRITE, VALUE )
    if IS_WRITE = false then VALUE = 0

  // this WOM address was previously written to
  if WAS_ALREADY_WRITTEN_TO = true then
    rcv( ADDRESS, TIMESTAMP_READ,  true, VALUE )
    snd( ADDRESS, TIMESTAMP_WRITTEN, true, VALUE )

// the finalization phase does both initializations and finalizations
if FINL = true then
  // address starts at 0 and increments by 1
  if prev FINL = false then
    ADDRESS = 0
  if prev FINL = true then
    ADDRESS = 1 + prev(ADDRESS)

  // no writes take place in the finalization phase
  IS_WRITE = false

  // bus interactions
  if WAS_ALREADY_WRITTEN_TO = false then
    // address of WOM was never written to
    snd( ADDRESS, 0,              false, 0 ) // initialization
    rcv( ADDRESS, TIMESTAMP_READ, false, 0 ) // finalization
  if WAS_ALREADY_WRITTEN_TO = true then
    // address of WOM was written to at some point
    snd( ADDRESS, 0,              false, 0     ) // initialization
    rcv( ADDRESS, TIMESTAMP_READ, true,  VALUE ) // finalization
```

where

```rust
rcv( address, timestamp, is_written, value )
snd( address, timestamp, is_written, value )
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

| MOD           | WOM       | Notes     |
| ------------- | --------- | --------- |
| WOM_TRIGGER   | EXEC      | condition |
| WOM_ADDRESS   | ADDRESS   |           |
| WOM_TIMESTAMP | TIMESTAMP |           |
| WOM_IS_WRITE  | IS_WRITE  |           |
| WOM_VALUE     | VALUE     |           |
