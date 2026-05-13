#!/usr/bin/env python3
"""Build the batched keccakf .accepts fixture from the per-vector one.

The per-vector fixture (``keccakf.accepts``) has one JSON object per line:

    {"n_blocks": "0x...","blocks": "0x...","result": "0x..."}

Different lines may carry different numbers of pre-padded blocks (see
``translate_padded_messages.py`` for the source of truth on this).

The batched fixture (``keccakf_batched.accepts``) packs everything into a
single JSON object consumed by ``keccakf_batched.zkc`` in one VM boot:

    {"n_vectors":    "0x<u64>",
     "block_counts": "0x<n_vectors u64s concatenated>",
     "blocks":       "0x<concatenation of every line's blocks bytes>",
     "result":       "0x<concatenation of every line's result bytes>"}

Block counts are emitted as fixed-width 8-byte (16 hex char) big-endian
u64 values, matching the ``count:u64`` decoding in the schema.

Usage (from the repo root):
    python3 testdata/zkc/bench/batch_keccakf_accepts.py
"""

from __future__ import annotations

import json
import sys
from pathlib import Path

BYTES_PER_BLOCK = 1088 // 8  # 136 bytes
HEX_PER_BLOCK = BYTES_PER_BLOCK * 2  # 272
HEX_PER_RESULT = 32 * 2  # 64
HEX_PER_U64 = 16


def main() -> int:
    here = Path(__file__).resolve().parent
    src = here / "keccakf.accepts"
    dst = here / "keccakf_batched.accepts"

    if not src.exists():
        print(f"error: source fixture not found: {src}", file=sys.stderr)
        return 1

    counts: list[int] = []
    blocks_parts: list[str] = []
    result_parts: list[str] = []

    for lineno, raw in enumerate(src.read_text().splitlines(), start=1):
        line = raw.strip()
        if not line:
            continue

        row = json.loads(line)

        n_blocks = int(row["n_blocks"], 16)
        if n_blocks < 1:
            print(
                f"error: line {lineno}: n_blocks={n_blocks} is not positive",
                file=sys.stderr,
            )
            return 1

        blocks_hex = row["blocks"][2:]
        result_hex = row["result"][2:]

        if len(blocks_hex) != HEX_PER_BLOCK * n_blocks:
            print(
                f"error: line {lineno}: blocks hex is {len(blocks_hex)} chars, "
                f"expected {HEX_PER_BLOCK * n_blocks} for n_blocks={n_blocks}",
                file=sys.stderr,
            )
            return 1
        if len(result_hex) != HEX_PER_RESULT:
            print(
                f"error: line {lineno}: result hex is {len(result_hex)} chars, "
                f"expected {HEX_PER_RESULT}",
                file=sys.stderr,
            )
            return 1

        counts.append(n_blocks)
        blocks_parts.append(blocks_hex)
        result_parts.append(result_hex)

    n_vectors = len(counts)
    block_counts_hex = "".join(f"{c:0{HEX_PER_U64}x}" for c in counts)

    payload = {
        "n_vectors":    f"0x{n_vectors:016x}",
        "block_counts": "0x" + block_counts_hex,
        "blocks":       "0x" + "".join(blocks_parts),
        "result":       "0x" + "".join(result_parts),
    }
    dst.write_text(json.dumps(payload) + "\n")

    total_blocks = sum(counts)
    print(
        f"wrote {n_vectors} vectors "
        f"({total_blocks} total blocks, "
        f"{len(payload['blocks']) - 2} hex chars of blocks, "
        f"{len(payload['result']) - 2} hex chars of result) "
        f"-> {dst.relative_to(here.parent.parent.parent)}"
    )

    # Quick distribution dump so the operator can sanity-check the input.
    hist: dict[int, int] = {}
    for c in counts:
        hist[c] = hist.get(c, 0) + 1
    print("n_blocks distribution:")
    for k in sorted(hist):
        print(f"  {k:>2}: {hist[k]}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
