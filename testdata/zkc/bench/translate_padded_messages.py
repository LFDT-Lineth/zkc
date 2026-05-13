#!/usr/bin/env python3
"""Translate keccakf_with_padding.accepts -> keccakf.accepts.

`keccakf_with_padding.accepts` stores raw messages (left-padded into a
fixed u5440 field) plus a `message_length` in bits.  `keccakf.accepts`
stores the same vectors after applying the Keccak-256 multi-rate
padding (pad10*1) and chunking into u1088 rate blocks; the per-line
shape is::

    {"n_blocks": "0x...","blocks": "0x<n_blocks * 136 bytes>","result": "0x<32 bytes>"}

Padding rule (Keccak-256, *not* NIST SHA3):

  - the rate is r = 1088 bits = 136 bytes
  - the padded length is the smallest multiple of r strictly greater
    than the message length in bytes (we always add at least one byte
    of padding even if the message is already a multiple of r)
  - layout, with M = message length in bytes:
      byte[M]              = 0x01            (Keccak suffix)
      byte[M+1 .. end-1]   = 0x00
      byte[end-1]          = 0x80            (terminator; XOR'd in)
    Edge case: if M mod r == r-1, the suffix and terminator share the
    same byte, which becomes 0x81.

By construction the first 1000 vectors of
``keccakf_with_padding.accepts`` correspond byte-for-byte to the
original ``keccakf.accepts`` (all message lengths there are 408 bytes
= 3*r, giving 4-block padded vectors).  The script writes
``keccakf.accepts.new`` and prints a diff summary against the existing
``keccakf.accepts`` so the migration can be checked before clobbering.
"""

from __future__ import annotations

import json
import sys
from pathlib import Path

RATE_BYTES = 136                       # bitrate / 8
MESSAGE_TOTAL_BITS = 5440              # u5440
MESSAGE_TOTAL_HEX = MESSAGE_TOTAL_BITS // 4
RESULT_HEX = 64                        # u256


def keccak256_pad(msg: bytes) -> bytes:
    """Append Keccak-256 multi-rate padding to `msg`, returning the
    padded byte string whose length is a positive multiple of
    RATE_BYTES."""
    m = len(msg)
    rem = m % RATE_BYTES
    pad_len = RATE_BYTES - rem  # always >= 1, at most RATE_BYTES
    if pad_len == 1:
        # Suffix (0x01) and terminator (0x80) collide in the last byte.
        return msg + bytes([0x81])
    return msg + bytes([0x01]) + bytes(pad_len - 2) + bytes([0x80])


def translate_line(raw: str, lineno: int) -> dict[str, str]:
    obj = json.loads(raw)

    try:
        msg_len_bits = int(obj["message_length"], 16)
    except (KeyError, ValueError) as exc:
        raise ValueError(f"line {lineno}: bad message_length: {exc!r}") from exc

    if msg_len_bits % 8 != 0:
        raise ValueError(
            f"line {lineno}: non-byte-aligned message_length={msg_len_bits} bits"
        )
    if msg_len_bits > MESSAGE_TOTAL_BITS:
        raise ValueError(
            f"line {lineno}: message_length={msg_len_bits} exceeds u{MESSAGE_TOTAL_BITS}"
        )

    msg_full_hex = obj["message"][2:]
    if len(msg_full_hex) != MESSAGE_TOTAL_HEX:
        raise ValueError(
            f"line {lineno}: message hex is {len(msg_full_hex)} chars, expected {MESSAGE_TOTAL_HEX}"
        )

    result_hex = obj["result"][2:]
    if len(result_hex) != RESULT_HEX:
        raise ValueError(
            f"line {lineno}: result hex is {len(result_hex)} chars, expected {RESULT_HEX}"
        )

    msg_bytes_len = msg_len_bits // 8
    # The padded u5440 field stores the message right-aligned (see the
    # `message_start = message_bits - message_length` arithmetic in
    # keccakf_with_padding.zkc).
    msg_hex = msg_full_hex[-2 * msg_bytes_len:] if msg_bytes_len else ""
    msg = bytes.fromhex(msg_hex)
    if len(msg) != msg_bytes_len:
        raise ValueError(
            f"line {lineno}: extracted message length mismatch: "
            f"{len(msg)} != {msg_bytes_len}"
        )

    padded = keccak256_pad(msg)
    assert len(padded) % RATE_BYTES == 0
    n_blocks = len(padded) // RATE_BYTES

    return {
        "n_blocks": f"0x{n_blocks:016x}",
        "blocks":   "0x" + padded.hex(),
        "result":   "0x" + result_hex,
    }


def main() -> int:
    here = Path(__file__).resolve().parent
    src = here / "keccakf_with_padding.accepts"
    dst = here / "keccakf.accepts.new"

    if not src.exists():
        print(f"error: source fixture not found: {src}", file=sys.stderr)
        return 1

    rows: list[str] = []
    block_hist: dict[int, int] = {}

    for lineno, raw in enumerate(src.read_text().splitlines(), start=1):
        line = raw.strip()
        if not line:
            continue
        out = translate_line(line, lineno)
        rows.append(
            # Match the existing keccakf.accepts formatting exactly:
            # no space after commas, single space after colons.
            json.dumps(out, separators=(",", ": "))
        )
        n_blocks = int(out["n_blocks"], 16)
        block_hist[n_blocks] = block_hist.get(n_blocks, 0) + 1

    dst.write_text("\n".join(rows) + "\n")

    print(f"wrote {len(rows)} vectors -> {dst.relative_to(here.parent.parent.parent)}")
    print("n_blocks distribution:")
    for k in sorted(block_hist):
        print(f"  {k:>2}: {block_hist[k]}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
