#!/usr/bin/env bash
#
# Regenerates the README screenshots in docs/img from the sample datasets in
# docs/. Builds stats, writes an --html report for each dataset in a temporary
# directory, then uses shot.py to save a full-page screenshot of each report
# and one smaller image per dataset for the README table.
#
# Requires: go, uv, ImageMagick (magick) and Google Chrome. Uses oxipng, when
# installed, to shrink the PNGs losslessly.
# Run from anywhere: docs/make_screenshots.sh

set -euo pipefail

datasets=(fairbanks_daily_temp_f server_cpu_pct)

docs=$(cd "$(dirname "$0")" && pwd)
repo=$(dirname "$docs")
img="$docs/img"

for cmd in go uv magick; do
    if ! command -v "$cmd" >/dev/null; then
        echo "error: $cmd not found in PATH" >&2
        exit 1
    fi
done

work=$(mktemp -d)
trap 'rm -rf -- "$work"' EXIT

shot() {
    uv run -q --with playwright python -I "$docs/shot.py" "$@"
}

(cd "$repo" && go build -o "$work/stats" .)
mkdir -p "$img"

for name in "${datasets[@]}"; do
    # --html writes the report beside its input, so run on a copy to keep
    # the report out of docs/.
    cp -f "$docs/$name.txt" "$work/"
    if ! "$work/stats" --html "$work/$name.txt" >/dev/null 2>"$work/stderr"; then
        cat "$work/stderr" >&2
        exit 1
    fi
    shot "$work/$name.html" "$img/${name}_full.png"
done

# README table images. The two differ on purpose: the top of one report and
# the charts of the other. At 600px wide the header plus the first section
# has nearly the same aspect ratio as the stacked charts, so the table cells
# line up.
shot "$work/fairbanks_daily_temp_f.html" "$img/fairbanks_daily_temp_f.png" --width 600 --sections 1

shot "$work/server_cpu_pct.html" "$work/histogram.png" --chart histogram
shot "$work/server_cpu_pct.html" "$work/trendline.png" --chart trendline
magick "$work/histogram.png" "$work/trendline.png" +repage -background white -append +repage "$img/server_cpu_pct.png"

if command -v oxipng >/dev/null; then
    oxipng -q -o 4 --strip safe "$img"/*.png
else
    echo "note: oxipng not found, PNGs left unoptimized" >&2
fi

ls -l "$img"
