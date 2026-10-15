#!/usr/bin/env python3
"""Decode Swift small-string immediates found in Hopper pseudocode.

Swift stores strings of up to 15 UTF-8 bytes inline in two 64-bit words:
the first word holds bytes 0..7 (little-endian), the second holds bytes
8..14 plus a discriminator in the top byte (0xE0 | count for ASCII).
Hopper prints these as hex immediates, so we pair them back up.

Usage: swift_small_strings.py <pseudocode-file> [--lines]
"""
import re
import sys

HEX = re.compile(r"0x([0-9a-fA-F]{1,16})")


def word_bytes(v):
    return v.to_bytes(8, "little")


def printable(bs):
    return all(32 <= b < 127 or b in (9, 10, 13) for b in bs)


def decode_pair(lo, hi):
    """lo = first word, hi = second word with discriminator in top byte."""
    disc = hi >> 56
    if disc & 0xF0 != 0xE0:
        return None
    n = disc & 0x0F
    raw = word_bytes(lo) + word_bytes(hi)[:7]
    s = raw[:n]
    if n and printable(s):
        return s.decode()
    return None


def decode_single(v):
    """A lone word whose bytes are all printable ASCII (>=3 chars)."""
    bs = word_bytes(v).rstrip(b"\x00")
    if len(bs) >= 3 and printable(bs):
        return bs.decode()
    return None


def main():
    path = sys.argv[1]
    show_lines = "--lines" in sys.argv
    text = open(path, encoding="utf-8", errors="replace").read()
    lines = text.splitlines()
    found = []
    for ln, line in enumerate(lines, 1):
        vals = [int(m.group(1), 16) for m in HEX.finditer(line)]
        # Pair with immediates on this line and the next two lines.
        window = vals[:]
        for extra in lines[ln:ln + 2]:
            window += [int(m.group(1), 16) for m in HEX.finditer(extra)]
        hits = set()
        for i, lo in enumerate(vals):
            for hi in window:
                if hi is lo:
                    continue
                s = decode_pair(lo, hi)
                if s:
                    hits.add(s)
            # discriminator-only small strings (<=0 bytes in lo) e.g. 0xe0...
            s1 = decode_single(lo)
            if s1:
                hits.add(s1)
        for v in vals:
            disc = v >> 56
            if disc & 0xF0 == 0xE0 and (disc & 0xF) <= 7:
                n = disc & 0xF
                bs = word_bytes(v)[:n]
                if n and printable(bs):
                    hits.add(bs.decode())
        for h in sorted(hits):
            found.append((ln, h))
    seen = set()
    for ln, h in found:
        key = h if not show_lines else (ln, h)
        if key in seen:
            continue
        seen.add(key)
        h = h.encode("unicode_escape").decode()
        print(f"{ln}\t{h}" if show_lines else h)


if __name__ == "__main__":
    main()
