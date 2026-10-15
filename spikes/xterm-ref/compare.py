"""Compare vt (Go) and xterm.js snapshots line by line: compare.py vt.txt xterm.txt"""
import sys
def norm(path):
    lines = [l.rstrip() for l in open(path, encoding="utf-8").read().split("\n")]
    while lines and not lines[-1]:
        lines.pop()
    return lines
a, b = norm(sys.argv[1]), norm(sys.argv[2])
n = max(len(a), len(b))
diff = [(i, a[i] if i < len(a) else "", b[i] if i < len(b) else "") for i in range(n) if (a[i] if i < len(a) else "") != (b[i] if i < len(b) else "")]
print(f"lines vt={len(a)} xterm={len(b)} differing={len(diff)}")
for i, x, y in diff[:12]:
    print(f"  L{i+1}\n    vt   : {x[:110]!r}\n    xterm: {y[:110]!r}")
