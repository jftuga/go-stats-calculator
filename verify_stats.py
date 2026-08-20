#!/usr/bin/env python3
"""Third verification layer for the go-stats-calculator program.

This script reimplements every statistic the Go program reports, using only the
Python standard library, and compares its own results against the program's JSON
output (``stats -j``). It complements verify_stats.sh, which uses bc for
arbitrary-precision arithmetic on a single dataset: this script trades bc's
precision for far broader coverage, reaching the histogram, trendline, symmetry,
modal bin, and flag interactions that are impractical to express in bc.
"""

from __future__ import annotations

import argparse
import json
import math
import os
import shutil
import subprocess
import sys
import tempfile
from typing import Any, Sequence

BLOCKS: str = "▁▂▃▄▅▆▇█"

# Fields rendered as block glyphs, compared with a one-level tolerance.
GLYPH_KEYS: frozenset[str] = frozenset({"histogram", "trendline"})
MAD_SCALE: float = 1.4826
MOD_Z_SCALE: float = 0.6745
CI95_Z: float = 1.96

# The same 31-value fixture used by stats_test.go and verify_stats.sh.
TEST_DATA: list[float] = [
    5, 10, 15.5, 20, 25.00, 30, 35.0, 40, 45, 50,
    55, 60, 65, 70, 75.25, 80, 85, 90, 95, 100,
    12.5, 37.5, 62.50, 87.5, 50, 50, 50, 3, 150, 7.75, 42.0,
]

# 20 disjoint pairs each summing to 1000, symmetric about 500.
SYMMETRIC_DATA: list[float] = [
    612, 137, 827, 495, 958, 264, 709, 19, 882, 455,
    349, 736, 88, 981, 545, 233, 61, 694, 42, 767,
    388, 118, 912, 651, 306, 994, 173, 588, 505, 377,
    863, 220, 939, 6, 623, 780, 151, 849, 291, 412,
]

# A tight cluster with two adjacent outliers that mask each other's z-scores.
MASKING_DATA: list[float] = [10, 10.2, 10.5, 10.8, 11, 11.2, 11.5, 11.8, 12, 12.3, 50, 52]


def round_half_up(value: float) -> int:
    """Round a non-negative value half away from zero, matching Go's math.Round.

    Args:
        value: The value to round. Must be non-negative.

    Returns:
        The rounded value as an integer.
    """
    return int(math.floor(value + 0.5))


def percentile(sorted_values: Sequence[float], fraction: float) -> float:
    """Compute a percentile using linear interpolation between order statistics.

    This is the R-7 method: rank = p * (n - 1), interpolating between the
    surrounding values.

    Args:
        sorted_values: Values in ascending order.
        fraction: The percentile expressed as a fraction between 0.0 and 1.0.

    Returns:
        The interpolated percentile value, or 0.0 for empty input.
    """
    n = len(sorted_values)
    if n == 0:
        return 0.0
    if n == 1:
        return float(sorted_values[0])
    rank = fraction * (n - 1)
    lower = math.floor(rank)
    upper = math.ceil(rank)
    if lower == upper:
        return float(sorted_values[int(rank)])
    weight = rank - lower
    return sorted_values[int(lower)] * (1 - weight) + sorted_values[int(upper)] * weight


def median_of(values: Sequence[float]) -> float:
    """Return the interpolated median of an unsorted sequence.

    Args:
        values: The values to summarize.

    Returns:
        The median value.
    """
    return percentile(sorted(values), 0.50)


def bin_counts(sorted_values: Sequence[float], num_bins: int) -> tuple[list[int], float, float]:
    """Bucket sorted values into equal-width bins spanning the data range.

    The bin count is capped at the number of values so bins never outnumber
    observations.

    Args:
        sorted_values: Values in ascending order.
        num_bins: The requested number of bins.

    Returns:
        A tuple of (bin counts, minimum value, bin width). The counts list is
        empty when the data has fewer than two values or no spread.
    """
    n = len(sorted_values)
    if n < 2:
        return [], 0.0, 0.0
    low = sorted_values[0]
    high = sorted_values[-1]
    if low == high:
        return [], 0.0, 0.0
    num_bins = min(num_bins, n)
    width = (high - low) / num_bins
    bins = [0] * num_bins
    for value in sorted_values:
        index = int((value - low) / width)
        if index >= num_bins:
            index = num_bins - 1
        bins[index] += 1
    return bins, low, width


def histogram(sorted_values: Sequence[float], num_bins: int) -> str:
    """Render a single-line Unicode histogram of the data distribution.

    Occupied bins are floored one level above empty so that a sparse bin is
    visually distinct from a bin holding nothing.

    Args:
        sorted_values: Values in ascending order.
        num_bins: The requested number of bins.

    Returns:
        The histogram string, or an empty string when the data cannot be binned.
    """
    bins, _, _ = bin_counts(sorted_values, num_bins)
    if not bins:
        return ""
    highest = max(bins)
    glyphs = []
    for count in bins:
        if count == 0:
            glyphs.append(BLOCKS[0])
            continue
        level = (count * 7) // highest
        glyphs.append(BLOCKS[max(level, 1)])
    return "".join(glyphs)


def trendline(values: Sequence[float], num_bins: int) -> str:
    """Render a single-line Unicode trendline of values in their input order.

    Args:
        values: The values in their original order.
        num_bins: The requested number of chunks.

    Returns:
        The trendline string, or an empty string when the data has no spread.
    """
    n = len(values)
    if n < 2:
        return ""
    low = min(values)
    high = max(values)
    if low == high:
        return ""
    num_bins = min(num_bins, n)
    step = n / num_bins
    glyphs = []
    for i in range(num_bins):
        start = round_half_up(i * step)
        end = min(round_half_up((i + 1) * step), n)
        if end <= start:
            end = start + 1
        average = sum(values[start:end]) / (end - start)
        normalized = (average - low) / (high - low)
        level = min(max(round_half_up(normalized * 7), 0), 7)
        glyphs.append(BLOCKS[level])
    return "".join(glyphs)


def detect_symmetry(sorted_values: Sequence[float]) -> tuple[bool, float, int]:
    """Report whether sorted data mirrors itself about a center value.

    Args:
        sorted_values: Values in ascending order.

    Returns:
        A tuple of (is symmetric, center, pair count). Center and pair count are
        meaningful only when the first element is True.
    """
    n = len(sorted_values)
    if n < 3:
        return False, 0.0, 0
    center = (sorted_values[0] + sorted_values[-1]) / 2
    scale = max(abs(sorted_values[0]), abs(sorted_values[-1]))
    tolerance = 1e-9 * max(1.0, scale)
    index = 0
    while index <= n - 1 - index:
        if abs(sorted_values[index] + sorted_values[n - 1 - index] - 2 * center) > tolerance:
            return False, 0.0, 0
        index += 1
    return True, center, n // 2


def detect_monotonicity(values: Sequence[float]) -> str:
    """Classify the input order of a sequence.

    Args:
        values: The values in their original order.

    Returns:
        One of "ascending", "descending", "constant", or "unordered". Ascending
        and descending are non-strict, so ties are allowed.
    """
    non_decreasing = all(values[i] >= values[i - 1] for i in range(1, len(values)))
    non_increasing = all(values[i] <= values[i - 1] for i in range(1, len(values)))
    if non_decreasing and non_increasing:
        return "constant"
    if non_decreasing:
        return "ascending"
    if non_increasing:
        return "descending"
    return "unordered"


def autocorrelation(values: Sequence[float], mean: float) -> float:
    """Compute the lag-1 autocorrelation of values in their input order.

    Args:
        values: The values in their original order.
        mean: The arithmetic mean of the values.

    Returns:
        The lag-1 autocorrelation, or 0.0 for fewer than three values or zero variance.
    """
    n = len(values)
    if n < 3:
        return 0.0
    denominator = sum((v - mean) ** 2 for v in values)
    if denominator == 0:
        return 0.0
    numerator = sum((values[i] - mean) * (values[i + 1] - mean) for i in range(n - 1))
    return numerator / denominator


def exponential_moving_average(values: Sequence[float], span: int) -> float:
    """Compute the final exponential moving average for a given span.

    Args:
        values: The values in their original order.
        span: The EMA span, which sets the smoothing factor to 2 / (span + 1).

    Returns:
        The final EMA value.
    """
    alpha = 2.0 / (span + 1.0)
    ema = values[0]
    for value in values[1:]:
        ema = alpha * value + (1 - alpha) * ema
    return ema


def trimmed_mean(sorted_values: Sequence[float], percent: float) -> float:
    """Compute the mean after removing a percentage of values from each tail.

    Args:
        sorted_values: Values in ascending order.
        percent: The percentage to remove from each tail, between 0 and 50.

    Returns:
        The trimmed mean.

    Raises:
        ValueError: If trimming would leave no values.
    """
    n = len(sorted_values)
    trim_count = math.floor(n * percent / 100.0)
    remaining = n - 2 * trim_count
    if remaining < 1:
        raise ValueError(f"dataset too small ({n} values) to trim {percent}% from each end")
    kept = sorted_values[trim_count:n - trim_count]
    return sum(kept) / len(kept)


class Reference:
    """An independent reimplementation of every statistic the program reports.

    The class computes each value from the raw input using only the standard
    library, so a shared bug in the Go implementation cannot hide behind a shared
    helper. Its as_dict output uses the same field names as the program's JSON.
    """

    def __init__(self, values: Sequence[float], num_bins: int = 16, iqr_multiplier: float = 1.5, z_threshold: float = 0.0, ema_span: int = 0, trim_mean_pct: float = 0.0) -> None:
        """Compute all statistics for the given values.

        Args:
            values: The input values in their original order.
            num_bins: Bin count for the histogram and trendline.
            iqr_multiplier: Multiplier for IQR-based outlier fences.
            z_threshold: Z-score threshold, or 0 to disable z-score detection.
            ema_span: EMA span, or 0 to disable.
            trim_mean_pct: Trimmed-mean percentage, or 0 to disable.
        """
        self.values = list(values)
        self.num_bins = num_bins
        self.iqr_multiplier = iqr_multiplier
        self.z_threshold = z_threshold
        self.ema_span = ema_span
        self.trim_mean_pct = trim_mean_pct
        self.sorted = sorted(self.values)
        self.count = len(self.values)
        self._compute()

    def _compute(self) -> None:
        """Populate every derived statistic. Called once from the constructor."""
        n = self.count
        self.total = sum(self.values)
        self.mean = self.total / n

        if n > 1:
            self.variance = sum((v - self.mean) ** 2 for v in self.values) / (n - 1)
            self.std_dev = math.sqrt(self.variance)
        else:
            self.variance = 0.0
            self.std_dev = 0.0

        self.std_err = self.std_dev / math.sqrt(n)
        self.ci_valid = n > 1 and self.std_err > 0
        self.ci_lower = self.mean - CI95_Z * self.std_err if self.ci_valid else 0.0
        self.ci_upper = self.mean + CI95_Z * self.std_err if self.ci_valid else 0.0

        self.minimum = self.sorted[0]
        self.maximum = self.sorted[-1]
        self.median = percentile(self.sorted, 0.50)
        self.q1 = percentile(self.sorted, 0.25)
        self.q3 = percentile(self.sorted, 0.75)
        self.p95 = percentile(self.sorted, 0.95)
        self.p99 = percentile(self.sorted, 0.99)
        self.iqr = self.q3 - self.q1

        deviations = sorted(abs(v - self.median) for v in self.values)
        self.mad = MAD_SCALE * percentile(deviations, 0.50)

        self.geo_mean_valid = self.minimum > 0
        self.geometric_mean = math.exp(sum(math.log(v) for v in self.values) / n) if self.geo_mean_valid else 0.0

        frequencies: dict[float, int] = {}
        for value in self.values:
            frequencies[value] = frequencies.get(value, 0) + 1
        self.distinct = len(frequencies)
        highest = max(frequencies.values())
        self.mode = sorted(v for v, f in frequencies.items() if f == highest) if highest > 1 else []

        self.modal_bin_valid = False
        self.modal_bin_low = 0.0
        self.modal_bin_high = 0.0
        self.modal_bin_count = 0
        if not self.mode:
            bins, low, width = bin_counts(self.sorted, self.num_bins)
            if bins:
                best = bins.index(max(bins))
                self.modal_bin_valid = True
                self.modal_bin_low = low + best * width
                self.modal_bin_high = self.modal_bin_low + width
                self.modal_bin_count = bins[best]

        lower_fence = self.q1 - self.iqr_multiplier * self.iqr
        upper_fence = self.q3 + self.iqr_multiplier * self.iqr
        self.outliers = sorted(v for v in self.values if v < lower_fence or v > upper_fence)

        self.z_valid = self.z_threshold > 0 and self.std_dev > 0
        self.mod_z_valid = self.z_threshold > 0 and self.mad > 0
        self.z_outliers: list[float] = []
        self.mod_z_outliers: list[float] = []
        if self.z_valid:
            self.z_outliers = sorted(v for v in self.values if abs((v - self.mean) / self.std_dev) > self.z_threshold)
        if self.mod_z_valid:
            raw_mad = self.mad / MAD_SCALE
            self.mod_z_outliers = sorted(v for v in self.values if abs(MOD_Z_SCALE * (v - self.median) / raw_mad) > self.z_threshold)

        self.skewness = self._skewness()
        self.kurtosis = self._kurtosis()
        self.is_symmetric, self.symmetry_center, self.symmetry_pairs = detect_symmetry(self.sorted)
        self.has_negative = any(v < 0 for v in self.values)
        self.cv_valid = abs(self.mean) >= 1e-10
        self.cv = (self.std_dev / abs(self.mean)) * 100 if self.cv_valid else 0.0

        self.histogram = histogram(self.sorted, self.num_bins)
        self.trendline = trendline(self.values, self.num_bins)
        self.autocorr_valid = n >= 3 and self.std_dev > 0
        self.autocorrelation = autocorrelation(self.values, self.mean) if self.autocorr_valid else 0.0
        self.input_order = detect_monotonicity(self.values) if n >= 2 else ""
        self.ema = exponential_moving_average(self.values, self.ema_span) if self.ema_span >= 2 else 0.0
        self.trimmed_mean = trimmed_mean(self.sorted, self.trim_mean_pct) if self.trim_mean_pct > 0 else 0.0

    def _skewness(self) -> float:
        """Compute the adjusted Fisher-Pearson standardized moment coefficient.

        Returns:
            The sample skewness, or 0.0 when it is undefined.
        """
        n = self.count
        if n < 3 or self.std_dev == 0:
            return 0.0
        cubed = sum((v - self.mean) ** 3 for v in self.values)
        return (n / ((n - 1) * (n - 2))) * (cubed / self.std_dev ** 3)

    def _kurtosis(self) -> float:
        """Compute the sample excess kurtosis.

        Returns:
            The excess kurtosis, or 0.0 when it is undefined.
        """
        n = self.count
        if n < 4 or self.std_dev == 0:
            return 0.0
        fourth = sum(((v - self.mean) / self.std_dev) ** 4 for v in self.values)
        return (n * (n + 1)) / ((n - 1) * (n - 2) * (n - 3)) * fourth - 3 * (n - 1) ** 2 / ((n - 2) * (n - 3))

    def as_dict(self) -> dict[str, Any]:
        """Return the statistics keyed by the program's JSON field names.

        Returns:
            A mapping from JSON field name to the independently computed value.
        """
        return {
            "count": self.count,
            "distinct": self.distinct,
            "duplicatePct": (self.count - self.distinct) / self.count * 100,
            "sum": self.total,
            "mean": self.mean,
            "stdErr": self.std_err,
            "ci95Lower": self.ci_lower,
            "ci95Upper": self.ci_upper,
            "ciValid": self.ci_valid,
            "geometricMean": self.geometric_mean,
            "geoMeanValid": self.geo_mean_valid,
            "median": self.median,
            "mode": self.mode,
            "modalBinLow": self.modal_bin_low,
            "modalBinHigh": self.modal_bin_high,
            "modalBinCount": self.modal_bin_count,
            "modalBinValid": self.modal_bin_valid,
            "min": self.minimum,
            "max": self.maximum,
            "stdDev": self.std_dev,
            "variance": self.variance,
            "mad": self.mad,
            "q1": self.q1,
            "q3": self.q3,
            "p95": self.p95,
            "p99": self.p99,
            "iqr": self.iqr,
            "iqrMultiplier": self.iqr_multiplier,
            "outliers": self.outliers,
            "zScoreThreshold": self.z_threshold,
            "zScoreValid": self.z_valid,
            "modZValid": self.mod_z_valid,
            "skewness": self.skewness,
            "kurtosis": self.kurtosis,
            "isSymmetric": self.is_symmetric,
            "symmetryCenter": self.symmetry_center,
            "symmetryPairs": self.symmetry_pairs,
            "cv": self.cv,
            "cvValid": self.cv_valid,
            "hasNegativeData": self.has_negative,
            "histogram": self.histogram,
            "trendline": self.trendline,
            "autocorrelation": self.autocorrelation,
            "autocorrValid": self.autocorr_valid,
            "inputOrder": self.input_order,
            "ema": self.ema,
            "emaSpan": self.ema_span,
            "trimmedMean": self.trimmed_mean,
            "trimmedMeanPct": self.trim_mean_pct,
        }


class Runner:
    """Invokes the stats program and returns parsed results."""

    def __init__(self, binary: str | None = None) -> None:
        """Locate the program to test.

        Args:
            binary: Explicit path to the stats binary, or None to auto-detect.
        """
        if binary:
            self.command = [binary]
        elif os.access("./stats", os.X_OK):
            self.command = ["./stats"]
        else:
            self.command = ["go", "run", "."]

    def run(self, values: Sequence[float] | None, *args: str, raw_input_text: str | None = None) -> subprocess.CompletedProcess[str]:
        """Run the program against a temporary data file.

        Args:
            values: Values to write one per line, or None when raw_input_text is used.
            *args: Command-line options to pass before the filename.
            raw_input_text: Exact file contents, bypassing float formatting.

        Returns:
            The completed process, including stdout, stderr, and return code.
        """
        text = raw_input_text if raw_input_text is not None else "\n".join(repr(v) for v in (values or [])) + "\n"
        handle = tempfile.NamedTemporaryFile("w", suffix=".txt", delete=False)
        try:
            handle.write(text)
            handle.close()
            return subprocess.run([*self.command, *args, handle.name], capture_output=True, text=True)
        finally:
            os.unlink(handle.name)

    def run_json(self, values: Sequence[float] | None, *args: str, raw_input_text: str | None = None) -> dict[str, Any]:
        """Run the program with -j and parse its JSON output.

        Args:
            values: Values to write one per line, or None when raw_input_text is used.
            *args: Command-line options to pass before the filename.
            raw_input_text: Exact file contents, bypassing float formatting.

        Returns:
            The parsed JSON object.

        Raises:
            RuntimeError: If the program exits non-zero or emits invalid JSON.
        """
        result = self.run(values, "-j", *args, raw_input_text=raw_input_text)
        if result.returncode != 0:
            raise RuntimeError(f"stats exited {result.returncode}: {result.stderr.strip()}")
        try:
            return json.loads(result.stdout)
        except json.JSONDecodeError as exc:
            raise RuntimeError(f"stats emitted invalid JSON: {exc}") from exc


class Verifier:
    """Accumulates pass and fail results and prints a report."""

    def __init__(self, tolerance: float = 1e-9) -> None:
        """Initialize an empty result set.

        Args:
            tolerance: Relative and absolute tolerance for float comparisons.
        """
        self.tolerance = tolerance
        self.passed = 0
        self.failures: list[str] = []

    def check(self, description: str, actual: Any, expected: Any) -> None:
        """Compare one value and record the outcome.

        Floats are compared with the configured tolerance; everything else must
        match exactly.

        Args:
            description: Human-readable name for the check.
            actual: The value reported by the program.
            expected: The independently computed value.
        """
        if isinstance(expected, float) or isinstance(actual, float):
            ok = isinstance(actual, (int, float)) and math.isclose(float(actual), float(expected), rel_tol=self.tolerance, abs_tol=self.tolerance)
        elif isinstance(expected, list):
            ok = isinstance(actual, list) and len(actual) == len(expected) and all(math.isclose(float(a), float(e), rel_tol=self.tolerance, abs_tol=self.tolerance) for a, e in zip(actual, expected))
        else:
            ok = actual == expected
        if ok:
            self.passed += 1
        else:
            self.failures.append(f"{description}: got {actual!r}, expected {expected!r}")

    def check_glyphs(self, description: str, actual: Any, expected: str) -> None:
        """Compare two block-glyph strings, tolerating a one-level difference.

        A chunk average can land exactly on a rounding boundary, where a one-ulp
        difference between Go's and Python's math libraries flips the chosen block.
        Differences of two or more levels still fail, so real scaling or ordering
        errors are caught.

        Args:
            description: Human-readable name for the check.
            actual: The glyph string reported by the program.
            expected: The independently rendered glyph string.
        """
        if not isinstance(actual, str) or len(actual) != len(expected):
            self.failures.append(f"{description}: got {actual!r}, expected {expected!r}")
            return
        for index, (got, want) in enumerate(zip(actual, expected)):
            if got not in BLOCKS or want not in BLOCKS:
                self.failures.append(f"{description}: non-block character at {index} in {actual!r}")
                return
            if abs(BLOCKS.index(got) - BLOCKS.index(want)) > 1:
                self.failures.append(f"{description}: position {index} differs by more than one level ({got} vs {want}) in {actual!r} vs {expected!r}")
                return
        self.passed += 1

    def check_true(self, description: str, condition: bool) -> None:
        """Record a boolean assertion.

        Args:
            description: Human-readable name for the check.
            condition: The assertion result.
        """
        if condition:
            self.passed += 1
        else:
            self.failures.append(description)

    def section(self, title: str) -> None:
        """Print a section heading.

        Args:
            title: The heading text.
        """
        print(f"\n--- {title} ---")

    def report(self) -> int:
        """Print the summary and return a process exit code.

        Returns:
            0 when every check passed, otherwise 1.
        """
        print()
        print("=" * 46)
        if not self.failures:
            print(f"Verification complete. All {self.passed} checks passed.")
            print("=" * 46)
            return 0
        print(f"Verification FAILED. {len(self.failures)} of {self.passed + len(self.failures)} checks did not match:")
        for failure in self.failures:
            print(f"  {failure}")
        print("=" * 46)
        return 1


def compare_dataset(verifier: Verifier, runner: Runner, name: str, values: Sequence[float], *args: str, **reference_kwargs: Any) -> None:
    """Compare every shared field between the reference and the program.

    Args:
        verifier: The result accumulator.
        runner: The program runner.
        name: Label used to prefix each check description.
        values: The dataset to analyze.
        *args: Command-line options passed to the program.
        **reference_kwargs: Keyword arguments forwarded to Reference.
    """
    verifier.section(f"{name} ({' '.join(args) if args else 'no options'})")
    payload = runner.run_json(values, *args)
    expected = Reference(values, **reference_kwargs).as_dict()
    for key, want in expected.items():
        if key in GLYPH_KEYS and want:
            verifier.check_glyphs(f"{name}.{key}", payload.get(key), want)
        else:
            verifier.check(f"{name}.{key}", payload.get(key), want)
    print(f"compared {len(expected)} fields against {len(values)} values")


def verify_flag_behavior(verifier: Verifier, runner: Runner) -> None:
    """Check flag validation, mutual exclusion, and degenerate-input reporting.

    Args:
        verifier: The result accumulator.
        runner: The program runner.
    """
    verifier.section("Flag validation and mutual exclusion")

    rejected = [
        ("-k 0 is rejected", ("-k", "0"), "greater than 0"),
        ("-k -1 is rejected", ("-k", "-1"), "greater than 0"),
        ("-b 3 is rejected", ("-b", "3"), "between 5 and 50"),
        ("-b 99 is rejected", ("-b", "99"), "between 5 and 50"),
        ("-z 0.5 is rejected", ("-z", "0.5"), ">= 1.0"),
        ("-e 1 is rejected", ("-e", "1"), ">= 2"),
        ("-t 60 is rejected", ("-t", "60"), "between 0 and 50"),
        ("-T 60 is rejected", ("-T", "60"), "between 0 and 50"),
        ("-t with -T is rejected", ("-t", "10", "-T", "10"), "mutually exclusive"),
        ("-e with -T is rejected", ("-e", "5", "-T", "10"), "mutually exclusive"),
        ("-p 101 is rejected", ("-p", "101"), "between 0 and 100"),
        ("-p abc is rejected", ("-p", "abc"), "invalid percentile"),
    ]
    for description, args, fragment in rejected:
        result = runner.run(TEST_DATA, *args)
        verifier.check_true(description, result.returncode != 0 and fragment in result.stderr)
        print(f"  {description}")

    # Options placed after the filename used to be parsed away and silently ignored
    result = runner.run(TEST_DATA, "-z", "2.0")
    verifier.check_true("options after the filename are rejected", result.returncode == 0)
    trailing = subprocess.run([*runner.command, "test_data.txt", "-z", "2.0"], capture_output=True, text=True)
    verifier.check_true("trailing options are rejected", trailing.returncode != 0 and "expected at most one input file" in trailing.stderr)
    two_files = subprocess.run([*runner.command, "test_data.txt", "symmetric_data.txt"], capture_output=True, text=True)
    verifier.check_true("two input files are rejected", two_files.returncode != 0)
    print("  argument handling")


def verify_input_sanitation(verifier: Verifier, runner: Runner) -> None:
    """Check that non-finite and unparseable input is rejected, not absorbed.

    Args:
        verifier: The result accumulator.
        runner: The program runner.
    """
    verifier.section("Input sanitation")

    payload = runner.run_json(None, raw_input_text="10\n\nabc\n20\n\nNaN\n30\nInf\n-Inf\n+Inf\n")
    verifier.check("valid values are kept", payload["count"], 3)
    verifier.check("blank lines are not counted as skipped", payload["skippedLines"], 5)
    verifier.check("sum is unpoisoned", payload["sum"], 60.0)
    verifier.check("min is unpoisoned", payload["min"], 10.0)
    verifier.check("max is unpoisoned", payload["max"], 30.0)

    for key, value in payload.items():
        if isinstance(value, float):
            verifier.check_true(f"{key} is finite", math.isfinite(value))
    print(f"  every numeric field finite across {len(payload)} keys")


def verify_formatting(verifier: Verifier, runner: Runner) -> None:
    """Check that small magnitudes survive text formatting without scientific notation.

    Args:
        verifier: The result accumulator.
        runner: The program runner.
    """
    verifier.section("Text formatting of small magnitudes")

    tiny = [0.00004, 0.00006]
    text = runner.run(tiny).stdout
    for fragment in ("0.00004", "0.00006", "0.00005"):
        verifier.check_true(f"{fragment} appears in the output", fragment in text)
    verifier.check_true("no scientific notation in output", "e-0" not in text and "E-0" not in text)

    # The bare value 0 must never stand in for a non-zero statistic
    for line in text.splitlines():
        label, separator, value = line.partition(":")
        if separator and value.strip() == "0" and label.strip() in ("Min", "Max", "Mean", "Median (p50)", "Sum"):
            verifier.check_true(f"{label.strip()} did not collapse to 0", False)
    print("  small-magnitude statistics preserved")


def verify_trim_dataset(verifier: Verifier, runner: Runner) -> None:
    """Check that -T recomputes every statistic on the trimmed data.

    Args:
        verifier: The result accumulator.
        runner: The program runner.
    """
    verifier.section("Trim dataset (-T 10)")

    payload = runner.run_json(TEST_DATA, "-T", "10")
    trim_count = math.floor(len(TEST_DATA) * 10 / 100)
    trimmed = sorted(TEST_DATA)[trim_count:len(TEST_DATA) - trim_count]
    expected = Reference(trimmed).as_dict()

    # Order-aware statistics are artifacts of the -T sort and must be suppressed
    verifier.check("trendline suppressed", payload["trendline"], "")
    verifier.check("inputOrder suppressed", payload["inputOrder"], "")
    verifier.check("autocorrValid false", payload["autocorrValid"], False)
    verifier.check("trimDatasetOrigN", payload["trimDatasetOrigN"], len(TEST_DATA))
    verifier.check("trimDatasetPct", payload["trimDatasetPct"], 10.0)

    skip = {"trendline", "inputOrder", "autocorrValid", "autocorrelation"}
    for key, want in expected.items():
        if key in skip:
            continue
        if key in GLYPH_KEYS and want:
            verifier.check_glyphs(f"trimmed.{key}", payload.get(key), want)
        else:
            verifier.check(f"trimmed.{key}", payload.get(key), want)

    text = runner.run(TEST_DATA, "-T", "10").stdout
    verifier.check_true("header shows before and after counts", "31 → 25 values" in text)
    verifier.check_true("footnote present", "all statistics above are computed on the trimmed dataset" in text)
    starred = [line for line in text.splitlines() if ":" in line and line.split(":", 1)[0].endswith("*")]
    verifier.check_true("no starred labels remain", not starred)
    print(f"  compared {len(expected) - len(skip)} fields on the trimmed dataset")


def verify_log_transform(verifier: Verifier, runner: Runner) -> None:
    """Check that -l computes every statistic in log space.

    Args:
        verifier: The result accumulator.
        runner: The program runner.
    """
    verifier.section("Log transform (-l)")

    payload = runner.run_json(TEST_DATA, "-l")
    expected = Reference([math.log(v) for v in TEST_DATA]).as_dict()
    for key, want in expected.items():
        # The geometric mean is deliberately suppressed under -l
        if key in ("geometricMean", "geoMeanValid"):
            continue
        if key in GLYPH_KEYS and want:
            verifier.check_glyphs(f"log.{key}", payload.get(key), want)
        else:
            verifier.check(f"log.{key}", payload.get(key), want)
    verifier.check("geoMeanValid suppressed under -l", payload["geoMeanValid"], False)
    verifier.check("logTransformed flag set", payload["logTransformed"], True)

    rejected = runner.run([1.0, 2.0, 0.0, 4.0], "-l")
    verifier.check_true("non-positive input is rejected", rejected.returncode != 0 and "requires all positive values" in rejected.stderr)
    print(f"  compared {len(expected) - 2} fields in log space")


def verify_custom_percentiles(verifier: Verifier, runner: Runner) -> None:
    """Check the -p flag against independently interpolated percentiles.

    Args:
        verifier: The result accumulator.
        runner: The program runner.
    """
    verifier.section("Custom percentiles (-p)")

    requested = [0.0, 10.0, 33.3, 90.0, 99.9, 100.0]
    payload = runner.run_json(TEST_DATA, "-p", ",".join(str(p) for p in requested))
    ordered = sorted(TEST_DATA)
    reported = payload.get("customPercentiles", {})
    verifier.check("all requested percentiles reported", len(reported), len(requested))
    for p in requested:
        key = str(int(p)) if p == int(p) else str(p)
        verifier.check(f"p{p}", reported.get(key), percentile(ordered, p / 100.0))
    verifier.check("p0 equals the minimum", reported.get("0"), min(TEST_DATA))
    verifier.check("p100 equals the maximum", reported.get("100"), max(TEST_DATA))
    print(f"  verified {len(requested)} custom percentiles")


def verify_masking(verifier: Verifier, runner: Runner) -> None:
    """Check that modified z-scores catch outliers that classic z-scores mask.

    Args:
        verifier: The result accumulator.
        runner: The program runner.
    """
    verifier.section("Z-score masking (-z 2.5)")

    payload = runner.run_json(MASKING_DATA, "-z", "2.5")
    verifier.check("classic z-scores are masked", payload["zScoreOutliers"], [])
    verifier.check("modified z-scores catch both", payload["modZOutliers"], [50.0, 52.0])
    verifier.check("IQR method also catches both", payload["outliers"], [50.0, 52.0])

    # Degenerate data must report the threshold and mark the result invalid,
    # rather than dropping the section entirely
    flat = runner.run_json([5.0, 5.0, 5.0, 5.0], "-z", "2.0")
    verifier.check("threshold recorded for flat data", flat["zScoreThreshold"], 2.0)
    verifier.check("zScoreValid false for flat data", flat["zScoreValid"], False)
    verifier.check("modZValid false for flat data", flat["modZValid"], False)
    flat_text = runner.run([5.0, 5.0, 5.0, 5.0], "-z", "2.0").stdout
    verifier.check_true("flat data reports N/A rather than nothing", "N/A - standard deviation is zero" in flat_text)
    print("  masking and degenerate-input behavior")


def verify_bin_counts(verifier: Verifier, runner: Runner) -> None:
    """Check histogram and trendline rendering across bin counts.

    Args:
        verifier: The result accumulator.
        runner: The program runner.
    """
    verifier.section("Histogram and trendline binning")

    for bins in (5, 8, 16, 32, 50):
        payload = runner.run_json(TEST_DATA, "-b", str(bins))
        expected_bins = min(bins, len(TEST_DATA))
        verifier.check(f"histogram length at -b {bins}", len(payload["histogram"]), expected_bins)
        verifier.check(f"trendline length at -b {bins}", len(payload["trendline"]), expected_bins)
        verifier.check_glyphs(f"histogram content at -b {bins}", payload["histogram"], histogram(sorted(TEST_DATA), bins))
        verifier.check_glyphs(f"trendline content at -b {bins}", payload["trendline"], trendline(TEST_DATA, bins))

    # Bins may never outnumber observations, or the two lines stop aligning
    short = [1.0, 5.0, 2.0, 8.0, 3.0, 9.0, 4.0]
    payload = runner.run_json(short, "-b", "20")
    verifier.check("histogram capped to data length", len(payload["histogram"]), len(short))
    verifier.check("trendline capped to data length", len(payload["trendline"]), len(short))

    # A bin holding one value must not render as the empty-bin glyph
    sparse = [0.0] * 100 + [10.0, 20.0, 30.0, 40.0, 50.0, 60.0, 70.0, 80.0, 90.0, 100.0]
    payload = runner.run_json(sparse, "-b", "11")
    verifier.check_true("sparse bins are visible", BLOCKS[0] not in payload["histogram"])
    print("  bin counts, capping, and sparse-bin visibility")


def main() -> int:
    """Run every verification scenario and report the result.

    Returns:
        A process exit code: 0 when all checks pass, otherwise 1.
    """
    parser = argparse.ArgumentParser(description="Independently verify the go-stats-calculator program using pure Python.")
    parser.add_argument("--binary", default=None, help="path to the stats binary (default: ./stats, else 'go run .')")
    parser.add_argument("--tolerance", type=float, default=1e-9, help="relative and absolute float comparison tolerance")
    arguments = parser.parse_args()

    if arguments.binary is None and not os.access("./stats", os.X_OK) and shutil.which("go") is None:
        print("Error: neither ./stats nor the go command is available", file=sys.stderr)
        return 1

    runner = Runner(arguments.binary)
    verifier = Verifier(arguments.tolerance)

    print("=" * 46)
    print("Independent Verification of Stats Calculator")
    print("Pure Python reimplementation vs. stats -j output")
    print("=" * 46)

    compare_dataset(verifier, runner, "testData", TEST_DATA, "-z", "2.0", z_threshold=2.0)
    compare_dataset(verifier, runner, "symmetric", SYMMETRIC_DATA)
    compare_dataset(verifier, runner, "masking", MASKING_DATA, "-z", "2.5", z_threshold=2.5)
    compare_dataset(verifier, runner, "sortedInput", sorted(TEST_DATA))
    compare_dataset(verifier, runner, "descending", sorted(TEST_DATA, reverse=True))
    compare_dataset(verifier, runner, "constant", [7.0] * 10)
    compare_dataset(verifier, runner, "negatives", [-10.0, -5.0, 0.0, 5.0, 10.0, 20.0, 30.0])
    compare_dataset(verifier, runner, "singleValue", [42.5])
    compare_dataset(verifier, runner, "ema", TEST_DATA, "-e", "5", ema_span=5)
    compare_dataset(verifier, runner, "trimmedMean", TEST_DATA, "-t", "10", trim_mean_pct=10.0)
    compare_dataset(verifier, runner, "customK", TEST_DATA, "-k", "3.0", iqr_multiplier=3.0)

    verify_custom_percentiles(verifier, runner)
    verify_masking(verifier, runner)
    verify_bin_counts(verifier, runner)
    verify_trim_dataset(verifier, runner)
    verify_log_transform(verifier, runner)
    verify_input_sanitation(verifier, runner)
    verify_formatting(verifier, runner)
    verify_flag_behavior(verifier, runner)

    return verifier.report()


if __name__ == "__main__":
    sys.exit(main())
