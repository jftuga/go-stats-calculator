# docs

Screenshots of `stats --html` reports used in the [project README](../README.md#overview), plus the sample data and scripts that produce them.

## Contents

| Path | Purpose |
|------|---------|
| `fairbanks_daily_temp_f.txt` | 3,650 simulated daily mean temperatures (°F) over ten years in a subarctic city. Two-peaked histogram, negative values, strong yearly cycle. |
| `server_cpu_pct.txt` | 10,080 simulated per-minute CPU utilization readings (%) over one week. Right skew, a pile-up at exactly 100, a daily cycle with nightly batch spikes and a quieter weekend. |
| `img/fairbanks_daily_temp_f.png` | The top of the report: header and Descriptive Statistics. Shown in the README table. |
| `img/server_cpu_pct.png` | The histogram and trendline charts from the report, stacked. Shown in the README table. |
| `img/<name>_full.png` | The full report page. Opened by clicking the image in the README. |
| `make_screenshots.sh` | Regenerates every image in `img/`. |
| `shot.py` | Screenshots one report, either the full page or a single chart. Called by `make_screenshots.sh`. |

## Regenerating the screenshots

Requirements: Go, [uv](https://docs.astral.sh/uv/), [ImageMagick](https://imagemagick.org/) (`magick`) and Google Chrome. `uv` fetches Playwright on first run; Playwright drives the installed Chrome, so no browser is downloaded. [oxipng](https://github.com/shssoichiro/oxipng) is optional; when installed, it shrinks every PNG losslessly by about a quarter.

```bash
docs/make_screenshots.sh
```

The script can be run from any directory. It:

1. Builds `stats` from the repository and writes an `--html` report for a copy of each dataset in a temporary directory, so no HTML file is left in `docs/`.
2. Saves a full-page screenshot of each report to `img/<name>_full.png`.
3. Saves the top of the Fairbanks report, through the first section, to `img/fairbanks_daily_temp_f.png`.
4. Saves the CPU report's histogram and trendline charts separately and stacks them into `img/server_cpu_pct.png`.
5. Runs `oxipng` over the images, if it is installed.

Screenshots are taken at 2x scale, in the light theme with reduced motion so every chart is fully drawn. The viewport is 900px wide, except for the Fairbanks table image at 600px: at that width the header and first section have nearly the same aspect ratio as the stacked CPU charts, so the two README table cells line up, and the text stays legible at table-cell size.

Images that include the report header (both full-page images and the Fairbanks table image) show its "Generated" timestamp, so they change on every run even when nothing else does. The stacked CPU image changes only when the data or the report does.
