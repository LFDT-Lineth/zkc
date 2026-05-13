#!/usr/bin/env python3
"""Regenerate `keccakf_with_padding.accepts` with N_VECTORS uniform-length
random messages.

Each row is::

    {"message_length": "0x<u64 bits>", "message": "0x<u5440, left-padded>", "result": "0x<u256 Keccak-256>"}

* `message_length` is in **bits** (matches the ZkC schema).
* `message` is the message right-aligned into a u5440 field (i.e. the
  message field that ``keccakf_with_padding.zkc`` expects, with
  ``message_start = 5440 - message_length`` leading zero bytes).
* `result` is the Ethereum-style Keccak-256 of the actual message bytes
  (not NIST SHA3-256).

The RNG is seeded so the output is bit-for-bit reproducible across
machines.

Requires PyCryptodome::

    python3 -m pip install --user pycryptodome

Usage (from the repo root)::

    python3 testdata/zkc/bench/generate_padded_messages.py
"""

from __future__ import annotations

import json
import random
import sys
from pathlib import Path

try:
    from Crypto.Hash import keccak
except ImportError as exc:
    print(
        "error: PyCryptodome is required (provides Ethereum-style "
        "Keccak-256). Install with: python3 -m pip install --user pycryptodome",
        file=sys.stderr,
    )
    raise SystemExit(1) from exc


MESSAGE_TOTAL_BITS = 5440                       # u5440 message field
MESSAGE_TOTAL_BYTES = MESSAGE_TOTAL_BITS // 8   # 680
MESSAGE_LENGTH_BITS = 4096                      # the requested per-vector message length
MESSAGE_LENGTH_BYTES = MESSAGE_LENGTH_BITS // 8 # 512
N_VECTORS = 10_000
SEED = 0xC0FFEE_DEC0DE                          # deterministic, change if you want fresh data


def keccak256(data: bytes) -> bytes:
    h = keccak.new(digest_bits=256)
    h.update(data)
    return h.digest()


def main() -> int:
    if MESSAGE_LENGTH_BITS > MESSAGE_TOTAL_BITS:
        print(
            f"error: requested message length ({MESSAGE_LENGTH_BITS} bits) "
            f"exceeds the u{MESSAGE_TOTAL_BITS} schema field",
            file=sys.stderr,
        )
        return 1
    if MESSAGE_LENGTH_BITS % 8 != 0:
        print("error: only byte-aligned message lengths are supported", file=sys.stderr)
        return 1

    here = Path(__file__).resolve().parent
    dst = here / "keccakf_with_padding.accepts"

    # Self-check the Keccak-256 implementation against a known vector
    # before producing the bulk output.  This is cheap and catches a
    # NIST-SHA3-vs-Keccak mix-up the moment someone swaps the library.
    expected = "4e03657aea45a94fc7d47ba826c8d667c0d1e6e33a64a036ec44f58fa12d6c45"
    if keccak256(b"abc").hex() != expected:
        print("error: Keccak-256 self-check failed", file=sys.stderr)
        return 1

    rng = random.Random(SEED)
    leading_zeros = bytes(MESSAGE_TOTAL_BYTES - MESSAGE_LENGTH_BYTES)
    msg_len_hex = f"0x{MESSAGE_LENGTH_BITS:016x}"

    rows: list[str] = []
    for _ in range(N_VECTORS):
        msg = rng.randbytes(MESSAGE_LENGTH_BYTES)
        padded = leading_zeros + msg
        result = keccak256(msg)

        rows.append(
            json.dumps(
                {
                    "message_length": msg_len_hex,
                    "message":        "0x" + padded.hex(),
                    "result":         "0x" + result.hex(),
                }
                # default separators = (", ", ": "), matching the existing format.
            )
        )

    dst.write_text("\n".join(rows) + "\n")

    rel = dst.relative_to(here.parent.parent.parent)
    print(
        f"wrote {N_VECTORS} vectors of {MESSAGE_LENGTH_BITS}-bit "
        f"({MESSAGE_LENGTH_BYTES}-byte) messages -> {rel}"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
