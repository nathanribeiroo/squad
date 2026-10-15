#!/usr/bin/env python3
"""Annotate every <dir>/*.c pseudocode file with decoded Swift small strings
and resolved string-pool addresses, writing <dir>/<name>.ann.c."""
import glob
import os
import re
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from extract_decomp import annotate  # noqa: E402

KB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "kb")


def load_tsv(name, cols):
    out = {}
    with open(os.path.join(KB, name), encoding="utf-8", errors="replace") as fh:
        for line in fh:
            parts = line.rstrip("\n").split("\t")
            if len(parts) >= cols:
                out[int(parts[0], 16)] = parts[cols - 1]
    return out


procs = load_tsv("procs.tsv", 2)
CALL = re.compile(r"\bsub_([0-9a-f]{6,})\b")

for path in sys.argv[1:]:
    for f in glob.glob(os.path.join(path, "*.c")):
        if f.endswith(".ann.c"):
            continue
        code = open(f, encoding="utf-8", errors="replace").read()
        out = annotate(code)
        # Tag calls to procedures that have real (non sub_) names.
        def tag(m):
            name = procs.get(int(m.group(1), 16))
            return m.group(0) if not name or name.startswith("sub_") else "%s/*%s*/" % (m.group(0), name[:80])
        out = CALL.sub(tag, out)
        open(f[:-2] + ".ann.c", "w").write(out)
        print(f)
