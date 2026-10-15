#!/usr/bin/env python3
"""Split a REA batch_decompile / procedure_pseudo_code result into one file
per procedure and annotate Swift small-string immediates inline.

Usage: extract_decomp.py <rea-result.json> <out-dir>
Writes <out-dir>/<address>.c and prints a size summary.
"""
import json
import os
import re
import sys

PAIR = re.compile(r"\{?(0x[0-9a-fA-F]{1,16})\s*[:,]\s*(0x[0-9a-fA-F]{1,16})\}?")
HEX = re.compile(r"0x([0-9a-fA-F]{1,16})")


def b8(v):
    return v.to_bytes(8, "little")


def ok(bs):
    return len(bs) > 0 and all(32 <= c < 127 or c in (9, 10, 13) for c in bs)


def small(lo, hi):
    disc = hi >> 56
    if disc & 0xF0 != 0xE0:
        return None
    n = disc & 0xF
    raw = (b8(lo) + b8(hi)[:7])[:n]
    return raw.decode() if ok(raw) else None


def single(v):
    disc = v >> 56
    if disc & 0xF0 == 0xE0:
        n = disc & 0xF
        raw = b8(v)[:n]
        if n <= 7 and ok(raw):
            return raw.decode()
    raw = b8(v).rstrip(b"\x00")
    if len(raw) >= 2 and ok(raw) and v > 0xFFFF:
        return raw.decode()
    return None


def annotate(code):
    lines = code.splitlines()
    out = []
    for i, line in enumerate(lines):
        notes = []
        vals = [int(m.group(1), 16) for m in HEX.finditer(line)]
        nxt = []
        for extra in lines[i + 1:i + 3]:
            nxt += [int(m.group(1), 16) for m in HEX.finditer(extra)]
        used = set()
        for a in vals:
            for b in vals[vals.index(a) + 1:] + nxt:
                s = small(a, b)
                if s and len(s) > 1:
                    notes.append(s)
                    used.add(a)
                    break
        for a in vals:
            if a in used:
                continue
            s = single(a)
            if s:
                notes.append(s)
        if notes:
            esc = [n.encode("unicode_escape").decode() for n in notes]
            line = f"{line}    /* str: {' | '.join(repr(e) for e in esc)} */"
        out.append(line)
    return "\n".join(out) + "\n"


def main():
    src, outdir = sys.argv[1], sys.argv[2]
    os.makedirs(outdir, exist_ok=True)
    data = json.load(open(src))
    res = data.get("result", data)
    items = res.get("items")
    if items is None:  # single procedure_pseudo_code result
        items = [{"address": res.get("procedure", "proc"),
                  "pseudocode": res.get("pseudo_code") or res.get("pseudocode") or ""}]
    for it in items:
        addr = it.get("address", "unknown")
        code = it.get("pseudocode") or ""
        path = os.path.join(outdir, f"{addr}.c")
        with open(path, "w") as fh:
            fh.write(annotate(code))
        print(f"{addr}\t{it.get('status', '?')}\t{code.count(chr(10))} lines\t{path}")


if __name__ == "__main__":
    main()
