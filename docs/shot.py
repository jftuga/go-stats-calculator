"""Screenshot a stats --html report with the locally installed Google Chrome.

Renders the report through Playwright at a fixed viewport width, in the light
theme with reduced motion so charts are drawn without scroll-in animation, at
2x scale. Saves the full page, a single chart figure, or the top of the page
through a given number of section cards. It is called by
make_screenshots.sh, which builds the README images in docs/img.
"""

import argparse
from pathlib import Path

from playwright.sync_api import Page, sync_playwright

PAD = 16  # CSS pixels of margin kept around a single chart figure


def figure_clip(page: Page, chart: str) -> dict[str, float]:
    """Return the clip rectangle around the figure holding a chart.

    Args:
        page: The loaded report page.
        chart: The data-chart attribute value, such as "histogram".

    Returns:
        A dict with x, y, width and height in CSS pixels, padded by PAD.

    Raises:
        ValueError: If the chart is not present on the page.
    """
    box = page.locator(f'figure:has(> .chart[data-chart="{chart}"])').bounding_box()
    if box is None:
        raise ValueError(f"chart not found: {chart}")
    return {"x": box["x"] - PAD, "y": box["y"] - PAD, "width": box["width"] + 2 * PAD, "height": box["height"] + 2 * PAD}


def top_clip(page: Page, sections: int) -> dict[str, float]:
    """Return the clip rectangle from the page top through the nth section card.

    Args:
        page: The loaded report page.
        sections: How many section cards to include below the header.

    Returns:
        A dict with x, y, width and height in CSS pixels, padded by PAD at the
        sides and half of PAD at the bottom, since cards are PAD apart and a
        full PAD would catch the next card's top border.

    Raises:
        ValueError: If the report has fewer than the requested sections.
    """
    report = page.locator("main.report").bounding_box()
    last = page.locator("section.card").nth(sections - 1).bounding_box()
    if report is None or last is None:
        raise ValueError(f"report has fewer than {sections} sections")
    return {"x": report["x"] - PAD, "y": 0, "width": report["width"] + 2 * PAD, "height": last["y"] + last["height"] + PAD / 2}


def parse_args() -> argparse.Namespace:
    """Parse and validate command-line arguments.

    Returns:
        The parsed arguments.
    """
    parser = argparse.ArgumentParser(description="Screenshot a stats --html report with Google Chrome.")
    parser.add_argument("html", type=Path, help="report HTML file")
    parser.add_argument("png", type=Path, help="output PNG file")
    parser.add_argument("--width", type=int, default=900, help="viewport width in CSS pixels (default 900)")
    region = parser.add_mutually_exclusive_group()
    region.add_argument("--chart", help="capture only this chart, e.g. histogram or trendline")
    region.add_argument("--sections", type=int, help="capture the header and the first N section cards")
    args = parser.parse_args()
    if not args.html.is_file():
        parser.error(f"no such file: {args.html}")
    if args.sections is not None and args.sections < 1:
        parser.error("--sections must be at least 1")
    return args


def main() -> None:
    """Render the report and write the screenshot."""
    args = parse_args()
    with sync_playwright() as p:
        browser = p.chromium.launch(channel="chrome")
        page = browser.new_page(viewport={"width": args.width, "height": 1000}, device_scale_factor=2, color_scheme="light", reduced_motion="reduce")
        page.goto(args.html.resolve().as_uri())
        page.wait_for_load_state("networkidle")
        if args.chart:
            page.screenshot(path=args.png, clip=figure_clip(page, args.chart), full_page=True)
        elif args.sections:
            page.screenshot(path=args.png, clip=top_clip(page, args.sections), full_page=True)
        else:
            page.screenshot(path=args.png, full_page=True)
        browser.close()


if __name__ == "__main__":
    main()
