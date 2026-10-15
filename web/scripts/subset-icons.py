#!/usr/bin/env python3
"""Cuts Material Symbols Outlined down to the icons in src/lib/icons.ts.

Run `make icons` after changing that list (`npm run build` refuses a stale
font). Needs fontTools and brotli: pip install fonttools brotli.

- Keeps only the listed icons' ligatures, so <Icon name="terminal"> works.
- Pins weight 400, grade 0 and optical size 20 (the .msi CSS in
  src/theme/base.css) and keeps the FILL axis (<Icon fill>).
"""
import json
import re
import sys
from pathlib import Path

WEB = Path(__file__).resolve().parent.parent
PKG = WEB / "node_modules" / "material-symbols"
SRC_FONT = PKG / "material-symbols-outlined.woff2"
ICONS_TS = WEB / "src" / "lib" / "icons.ts"
OUT_FONT = WEB / "src" / "assets" / "material-symbols-subset.woff2"
OUT_LIST = WEB / "src" / "assets" / "material-symbols-subset.json"
PINNED = {"wght": 400, "GRAD": 0, "opsz": 20}
# Characters icon names are typed with (the ligature components).
LETTERS = "abcdefghijklmnopqrstuvwxyz0123456789_"


def icon_names():
    m = re.search(r"ICONS = \[(.*?)\] as const", ICONS_TS.read_text(), re.S)
    if not m:
        sys.exit(f"no ICONS list in {ICONS_TS}")
    return re.findall(r"'([a-z0-9_]+)'", m.group(1))


def ligatures(font):
    """Maps each icon name the font's ligatures spell to its glyph."""
    cmap = font.getBestCmap()
    by_glyphs = {}
    for lookup in font["GSUB"].table.LookupList.Lookup:
        for st in lookup.SubTable:
            st = getattr(st, "ExtSubTable", st)
            for first, ligs in getattr(st, "ligatures", {}).items():
                for lig in ligs:
                    by_glyphs[tuple([first] + lig.Component)] = lig.LigGlyph
    return lambda name: by_glyphs.get(tuple(cmap.get(ord(c)) for c in name))


def main():
    try:
        from fontTools import subset
        from fontTools.ttLib import TTFont
        from fontTools.varLib import instancer
    except ImportError:
        sys.exit("subset-icons needs fontTools and brotli: pip install fonttools brotli")
    if not SRC_FONT.exists():
        sys.exit(f"{SRC_FONT} not found: run npm ci in web/ first")

    names = icon_names()
    font = TTFont(SRC_FONT)
    glyphs = set(font.getGlyphOrder())
    missing = [n for n in names if n not in glyphs]
    if missing:
        sys.exit("not in Material Symbols: " + ", ".join(missing))

    opts = subset.Options()
    opts.layout_closure = False  # only the listed icons, not every ligature
    opts.layout_features = ["*"]
    opts.notdef_outline = True
    sub = subset.Subsetter(opts)
    sub.populate(glyphs=[g for n in names for g in (n, n + ".fill") if g in glyphs], text=LETTERS)
    sub.subset(font)
    font = instancer.instantiateVariableFont(font, PINNED)

    lost = [n for n in names if ligatures(font)(n) is None]
    if lost:
        sys.exit("ligatures lost in the subset: " + ", ".join(lost))

    font.flavor = "woff2"
    OUT_FONT.parent.mkdir(parents=True, exist_ok=True)
    font.save(OUT_FONT)
    version = json.loads((PKG / "package.json").read_text())["version"]
    OUT_LIST.write_text(json.dumps({"source": f"material-symbols@{version}", "icons": names}, indent=2) + "\n")
    print(f"{OUT_FONT.relative_to(WEB)}: {len(names)} icons, {OUT_FONT.stat().st_size} bytes")


if __name__ == "__main__":
    main()
