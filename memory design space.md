# Memory design space

Designing a memory module `m` requires at a minimum categorizing it along the following dimensions:

- **ACCESS**: how do other modules communicate with it ?
- **COHERENCE**: is a coherence argument (i.e. memory consistency argument) required ?
- **MUTABILITY**: is that memory mutable or immutable ?
- **ADDRESS SPACE**: is data stored in a contiguous fashion or can it be sparse ?
- **SEGMENTATION**: is the memory accessible across several trace segments or not ?

We provide constraints for memories below:

## RAM: Random Access Memory

- **ACCESS**: logUpBus based
- **COHERENCE**: logUpBus-based `Σ ≡ 0` coherence argument involving timestamps
- **MUTABILITY**: unrestricted reads and writes
- **ADDRESS SPACE**: sparse
- **SEGMENTATION**: multi-segment

Use case: standard computer memory

## AOM: Access Once Memory

**AOM**'s provide an immutable memory that gets read start to finish in a single segment.

**Note.** I suggest using **AOM** as a replacement for **ROM** / **WOM**: these two notions are functionally identical and differ, AFAICT, only in their intended use cases. Both are meant to hold immutable data that's avaiable from the start of tracing, and either seen as _providing_ inputs or _receiving_ output. In any case, they are functionally identical.

- **ACCESS**: lookup based
- **COHERENCE**: none required
- **MUTABILITY**: immutable
- **ADDRESS SPACE**: contiguous
- **SEGMENTATION**: all accesses concentrated in one segment

Use case: provide inputs or extract outputs in one go,
e.g. read `m` start to finish in one trace segment.

## WOM: Write Once Memory

**WOM**'s provide an initially empty memory where every cell may be read from and written to, but the initial write is definitive.

- **ACCESS**: logUpBus based
- **COHERENCE**: logUpBus-based `Σ ≡ 0` coherence argument involving timestamps
- **MUTABILITY**:
  - arbitrary reads
  - first write sets value in stone (subsequent writes mustn't modify the current value)
  - first write marks an address as `WAS_ALREADY_WRITTEN_TO`
- **ADDRESS SPACE**: contiguous
- **SEGMENTATION**: multi-segment

Use case: extract definitive outputs that may be read by other processes

- is `m` single access ?
  - there are concrete cases (e.g. guest program ELF sections, guest program input)
  - can it be assumed that the accesses land in a single trace segment ? if so one doesn't have to care about inter segment coherence
- is `m`
  - immutable ?
  - set once ?
  - arbitrarily modifiable ?
  - the answer to that question dictates whether one requires whether access must be tracked or not
  - e.g. for WRITE ONCE MEMORY one should track
- is memory access confined to a single segment ?
  - the answer to that question dictates whether
    - memory consistency can be handled in module (and without permutation) or should be handled using a bus argument
    - whether timestamps are required to order accesses or not
- does the memory permit writes ?
  - if so, does it permit more than one write per address ?
- does the memory permit reads ?
- are accesses sequential and confined to a single segment or can they occur at any point in the execution ?
  - one can talk of the ONE TIME ACCESS property
- are accesses sequential and confined to a single segment or can they occur at any point in the execution ?
- does the touched address space have gaps ?
  - CONTIGUOUS memory allows potential initialization / finalization phases to increment addresses via `addr.next() = addr + 1`
