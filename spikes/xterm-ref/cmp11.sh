#!/bin/bash
# cmp.sh <rec_out dir> <session> <snap label> <cols> <rows>
R=$1; S=$2; L=$3; C=$4; W=$5
N=$(cd "$(dirname "$0")" && pwd)
off=$(awk -v l="$L" '$1==l{print $2}' "$R/$S.snaps")
(cd "$N" && node replay11.mjs "$R/$S.raw" "$C" "$W" "$off") > "$R/$S.$L.xterm11.txt"
echo "== $S/$L (offset $off)"
python3 -I "$N/compare.py" "$R/$S.$L.vt.txt" "$R/$S.$L.xterm11.txt"
