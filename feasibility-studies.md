# Feasibility Studies for Potential New Features

## Summary Ranking

Of the six features studied, ranked by fit and value for this tool:

| Rank | Feature | Recommendation |
|:-----|:--------|:---------------|
| 1 | **Kurtosis** | Easiest win. Mirrors existing skewness pattern exactly. Pairs naturally with skewness to describe distribution shape. |
| 2 | **Histogram** | Highest visual impact, simple implementation. Surfaces distribution shape (bimodality, skew) that numbers alone hide. |
| 3 | **Z-Score Outlier Detection** | Best algorithmic fit, lowest cost, clearest value. Complements existing IQR detection. |
| 4 | **Log Transformation** | Good fit, minimal code, real use case for heavy-tailed data. |
| 5 | **Moving Averages / Rolling Windows** | Useful but introduces philosophical tension -- transforms data rather than describing it. |
| 6 | **Sample vs. Population Variance** | Skip. The current `N-1` default is already correct for the target audience. |

A potential follow-up to the histogram: an **ASCII Box Plot** could be added later, especially if the tool ever supports comparing multiple datasets.

---

## Moving Averages / Rolling Windows

### Question

I am thinking about a *possible* new feature.  Let's discuss and plan first.  This is a feasability study.

### Moving Averages / Rolling Windows

If the input data is a stream or a time-series, "Global" stats can be misleading.

    The Feature: A --window flag to calculate the average of every N samples.
                 If N% is used instead of N, then average every N% samples.

    Benefit: This smooths out noise and helps identify trends over time rather than just looking at the dataset as a static pile of numbers.

## Analysis

I've reviewed both files. Here's my feasibility analysis of the `--window` feature.

### Current Architecture

The program reads all numbers into a `[]float64` slice, then passes the entire slice to `computeStats()` which computes everything at once. Output is a single block of stats. This is a batch-oriented, whole-dataset design.

### Feasibility: Straightforward, but with key design decisions

The core computation is easy -- slice the data into windows and call `computeStats()` on each. The interesting questions are about **what to output and how**.

#### Design Questions to Resolve

**1. What stats per window?**

Running full `computeStats()` on each window (outliers, skewness, mode, percentiles, etc.) would produce a wall of output. Options:

- **Full stats per window** -- verbose, probably overwhelming for large datasets
- **Summary stats only** (mean, min, max, stddev per window) -- more practical for trend analysis
- **Just the moving average** (mean per window) -- simplest, matches the stated goal of "smoothing noise"

**2. Sliding vs. tumbling windows?**

- **Tumbling**: data is partitioned into non-overlapping chunks of N. For 100 items with `--window 10`, you get 10 windows. Simple, fewer output rows.
- **Sliding**: window moves one element at a time. For 100 items with `--window 10`, you get 91 windows. Classic moving average, better for trend smoothing but much more output.

These are very different features. The "smoothing" use case strongly suggests sliding windows.

**3. The `N%` variant adds complexity**

`--window 25%` meaning "25% of the dataset" requires reading all data first (already the case here). It's implementable but it's a second mode to parse, validate, and document. Worth asking: does this pull its weight vs. the user just doing the math themselves?

**4. Output format**

The current output is human-readable aligned text. Per-window output would benefit from a tabular or CSV-like format for piping into other tools. This might motivate a `-o csv` or `-o tsv` flag, which is scope creep but worth noting.

**5. Interaction with existing flags**

- `-p` custom percentiles per window? Probably noise.
- `-k` outlier detection per window? Could be interesting but multiplies output.
- The current `Stats` struct and `printStats()` are designed for single-invocation display, not repeated tabular output.

#### Implementation Sketch

The minimal version would be:

1. Add `--window N` flag (integer, no `%` variant initially)
2. Use sliding windows over the input slice
3. Output one line per window: `window_start, window_end, mean` (or a small set of summary stats)
4. Keep the existing full-stats output as-is (no `--window` = current behavior)

This is maybe 50-80 lines of new Go code plus tests.

#### Potential Concerns

- **Conceptual tension**: The tool is currently a "describe this dataset" tool. Moving averages are a "transform this dataset" tool. These serve different analytical purposes. Adding windowing starts pushing toward time-series tooling territory.
- **Order dependence**: Current stats are order-independent (data gets sorted). Moving averages are inherently order-dependent -- they assume the input sequence matters. This is a philosophical shift for the tool.

#### My Take

It's technically easy to implement. The harder question is whether it belongs in this tool or whether it muddies the tool's identity. If you proceed, I'd suggest starting with the simplest version -- sliding window, mean-only output, integer N only -- and seeing if it's actually useful before expanding scope.

### Open Questions

- Sliding vs. tumbling?
- Mean-only vs. a small stat summary per window?
- Do you want the `N%` variant in v1?

---

## Standard Deviation: Sample vs. Population

### Question

A common "gotcha" in stats tools is calculating the wrong type of variance.

    Sample Variance: s2=n−1∑(xi​−xˉ)2​ (Used when you have a subset of data).

    Population Variance: σ2=N∑(xi​−μ)2​ (Used when you have every single data point).

    The Feature: A toggle to switch between n and n−1 denominators would make the tool more academically rigorous.

However, this tool is meant for the layperson, so I am not sure how useful this would be.

### Analysis

This is a good one to think through. A few observations:

**The current behavior is already the right default.** The code at `stats.go:186` uses `N-1` (sample variance), which is the standard choice for virtually all practical use cases. If you're running command-line stats on a data file, you almost certainly have a sample, not a complete population.

**The cases where population variance matters are narrow.** You'd use `N` when you genuinely have every member of a population -- class grades for every student, census data, etc. Even then, the difference shrinks as N grows. For N=100, it's a 1% difference. For N=1000, it's 0.1%. It only matters meaningfully for small N, which is also when people are least likely to know which one they want.

**The "academically rigorous" audience already knows the difference** and likely uses R, Python, or a proper stats package. The layperson audience this tool targets would be confused by the toggle. Adding a `--population` flag means you now need to explain the distinction in the README, which adds conceptual overhead for everyone to serve a minority use case.

**Downstream effects are modest but real.** Switching the denominator would change: variance, standard deviation, CV, skewness (which uses the adjusted Fisher-Pearson formula with `n*(n-1)*(n-2)` correction), and indirectly outlier bounds (since IQR is percentile-based, it's unaffected). So it's not just swapping one denominator -- you'd need to thread the flag through several calculations and verify each.

**My take:** I'd skip this one. The current `N-1` default is correct for the tool's audience and use case. Adding the toggle has a low benefit-to-complexity ratio -- not in implementation effort (it's small), but in documentation burden and user-facing complexity. If someone needs population variance on a small dataset, the mental math to convert is trivial: multiply by `(n-1)/n`.

If you did want to add it despite that, a `-N` or `--population` flag that silently switches the denominator (no extra output, just changes the math) would be the least intrusive approach.

---

## Log Transformation

### Question

For data that spans several orders of magnitude (like file sizes or web traffic), a log transform can make the distribution much easier to read.

    The Feature: A --log flag to transform input data before calculating stats.

### Analysis

This one is interesting because it's simpler than it first appears, but has some sharp edges.

**What it would do:** Apply `math.Log()` (or `math.Log10()`) to every value before passing it to `computeStats()`. Conceptually it's a one-liner transform inserted between `readNumbers()` and `computeStats()` at `stats.go:89-111`.

**The good:**

- Implementation is trivial -- iterate the slice, replace each value with its log. Everything downstream (mean, median, stddev, etc.) just works.
- Genuinely useful for heavy-tailed data. Log-transformed mean and stddev are meaningful for lognormal distributions (file sizes, latencies, income data).
- Doesn't change the tool's identity -- it's still "describe this dataset," just with a pre-processing step.

**The sharp edges:**

- **Zero and negative values.** `log(0)` is `-Inf`, `log(-x)` is `NaN`. You'd need to decide: error out? skip those values with a warning? This is the main implementation question. Silently dropping values would change the count and all derived stats, which could be confusing.
- **Which log base?** `ln` (natural log) is standard in statistics, but `log10` is more intuitive for the "file sizes and web traffic" use case (orders of magnitude map to integer values). You might end up wanting `--log` for ln and `--log10` for base-10, which doubles the surface area.
- **Interpretability of output.** The output would show stats in log-space with no indication of what the original scale was. A user seeing `Mean: 3.4` needs to know that means `e^3.4 ≈ 30` in the original units. The tool doesn't currently label units at all, so this is consistent, but it could trip people up. You could add a note like `(log-transformed)` to the output header.
- **Interaction with outlier detection.** Log transformation compresses the upper tail, which is actually a feature -- outliers in the original space may not be outliers in log-space. But this means the outlier list will differ, and users might not expect that.

**My take:** This is the strongest candidate of the three features discussed so far. It's minimal code, doesn't compromise the tool's identity, and serves a real need. I'd suggest:

- Single flag: `--log` for natural log (the statistical standard)
- Error out if any value is `<= 0`, with a clear message ("log transform requires all positive values")
- Add a header line to output: `(log-transformed)` so it's obvious
- Skip `--log10` unless there's demand

The strict "error on non-positive values" approach is better than silently dropping data. It keeps the tool honest and predictable.

---

## Z-Score Outlier Detection

### Question

While the IQR method is great for skewed data, adding Z-score detection would be better for data that follows a normal distribution.

    The Logic: You flag data points that are more than x standard deviations (σ) from the mean (μ).

    Formula: Z=σx−μ​

    Benefit: This provides a more "scientific" threshold for many engineering and physics applications.

### Analysis

This is a natural complement to what's already there. Some thoughts:

**It fits well.** The tool already computes mean, stddev, and does outlier detection via IQR. All the inputs for Z-score detection are already calculated in `computeStats()`. You'd literally just add a second pass over the data comparing `math.Abs((v - stats.Mean) / stats.StdDev)` against a threshold. The sorted data, the statistical foundations -- it's all in place.

**Design is straightforward.** A `-z` flag taking a float threshold (default 2.0 or 3.0) maps cleanly onto the existing `-k` pattern for IQR multiplier. The output could be an additional line right after the existing `Outliers:` line -- something like `Outliers (Z>3):  [...]`.

**The question is whether two outlier methods creates confusion.** The IQR and Z-score methods will often flag different points:

- IQR is robust against skewed distributions and doesn't assume normality
- Z-score assumes roughly normal data and is sensitive to the very outliers it's trying to detect (since they inflate the mean and stddev)

A layperson seeing two different outlier lists might not understand why they disagree. You'd need to either:

1. Show both and let the user interpret (more information, more confusion)
2. Make them mutually exclusive (`-k` for IQR, `-z` for Z-score, error if both)
3. Always show IQR, optionally add Z-score with `-z`

Option 3 feels right -- IQR stays the default, `-z` is opt-in for users who know they want it.

**Common thresholds are well-established:**

| Threshold | Meaning |
|:----------|:--------|
| **2.0** | ~95% of data in a normal distribution. Liberal, flags more. |
| **2.5** | Common in engineering quality control. |
| **3.0** | ~99.7% of data. Conservative, the typical "three-sigma rule." |

A default of 3.0 would be sensible -- it's the most widely recognized threshold and avoids over-flagging.

**Edge cases are minor.** If stddev is 0 (all values identical), Z-score is undefined -- but `computeStats()` already handles this (stddev is 0 for count <= 1). You'd just skip Z-score detection when stddev is 0, same as skewness already does at `stats.go:291`.

**My take:** This is a good feature. It's low-cost to implement (maybe 15-20 lines of code plus the flag), doesn't change existing behavior, uses data already computed, and genuinely adds value for the engineering/science audience. The `-z 3.0` flag pattern mirrors the existing `-k 1.5` pattern, so it's consistent with the tool's UX.

---

## Visual "Sparkline" Histograms (and ASCII Box Plot)

### Question

Since this is a CLI tool, you can't easily pop up a window with a graph, but you can use Unicode block elements to show a distribution.

    The Feature: A small, text-based histogram (e.g., ▃▆▇▆▃) printed in the terminal.

    Benefit: It allows the user to see at a glance if the data is "Bi-modal" (having two peaks) or heavily skewed, which numbers alone often hide.

Also considered: an ASCII box plot (see `box-plot.jpg` in the repo) showing Q1, median, Q3, whiskers, and outliers in a horizontal text diagram.

### Analysis

These are two related but distinct ideas.

#### Sparkline Histogram

**The idea is sound.** Unicode block characters (`▁▂▃▄▅▆▇█`) map naturally to bin heights. You'd divide the data range into N bins (say 10-20), count values per bin, then scale the counts to 8 levels. The output would be a single line appended to the stats:

```
Distribution:  ▁▂▃▅▇▇▅▃▂▁
```

**Implementation is simple.** Maybe 20-30 lines: bin the sorted data, find the max bin count, scale each bin to 0-7, index into the block character array. No external dependencies.

**The main value is exactly what you said** -- it surfaces bimodality and skew visually. The tool already computes skewness numerically, but a sparkline makes it visceral. A bimodal distribution would jump out as `▅▂▁▁▂▆` in a way that no single number can convey.

**Design choice:** always show it (no flag needed), or gate it behind `--hist` / `-H`? I'd lean toward always showing it -- it's one line, it's non-intrusive, and it's the kind of thing that makes the tool feel polished.

#### ASCII Box Plot

The box plot image shows a different visualization: a structural diagram showing Q1, median, Q3, whiskers, and outlier points. A text-based horizontal box plot is feasible:

```
Box Plot:  |----[=====|=======]--------| o  o
           Min  Q1   Med    Q3        Max (outliers)
```

The tool already computes every value needed (min, max, Q1, median, Q3, IQR, outliers). So the data is there -- it's purely a rendering exercise.

**However**, a box plot works best for *comparing* multiple datasets side by side (as the image shows with Data 1/2/3). For a single dataset, it's less compelling than the sparkline -- it mostly restates Q1/median/Q3 in visual form, information already in the numeric output. The sparkline reveals *shape* (bimodality, skew, gaps) that the box plot doesn't.

#### My Take

These are complementary, not competing:

- **Sparkline histogram** -- shows distribution shape. High value, simple to implement. Do this.
- **ASCII box plot** -- shows quartile structure. Moderate value for single datasets, high value if the tool ever supports comparing multiple datasets. Could be a follow-up.

If you want both, a `-g` (graph) flag that prints both would work. But the sparkline alone delivers most of the value.

---

## Kurtosis

### Question

Kurtosis is dimensionless like skewness, with universal thresholds:

| Excess Kurtosis | Descriptor |
|:----------------|:-----------|
| < -1 | Platykurtic (flat, thin tails) |
| -1 to 1 | Mesokurtic (normal-like) |
| > 1 | Leptokurtic (peaked, heavy tails) |

Kurtosis pairs naturally with skewness -- together they describe the "shape" of a distribution. Skewness tells you about asymmetry; kurtosis tells you about tail weight and peakedness.

Also considered: printing a descriptor table in the terminal output to help laypersons understand the Greek terms.

### Analysis

This is the most straightforward feature of everything discussed. It's a near-copy of the skewness implementation. The pattern is already established end-to-end:

1. `calculateSkewness` at `stats.go:288` sums cubed deviations with an adjustment factor
2. `interpretSkewness` at `stats.go:338` maps ranges to labels
3. Stored as a `float64` in the `Stats` struct
4. Printed as `Skewness: 0.7271 (Moderately Right Skewed)` at `stats.go:415`
5. Tested with data cases and edge cases in `stats_test.go`

Kurtosis follows the identical structure -- the only difference is raising deviations to the 4th power instead of 3rd, and using a different adjustment factor. The excess kurtosis formula (subtracting 3 so a normal distribution = 0) is:

```
Kurt = [n(n+1) / ((n-1)(n-2)(n-3))] * Σ((xi - mean)/stddev)^4  -  3(n-1)² / ((n-2)(n-3))
```

Edge cases are the same as skewness: requires n >= 4 (vs n >= 3 for skewness), return 0 if stddev is 0. The n >= 4 requirement comes from the `(n-3)` term in the denominator.

No design decisions needed. There's no flag -- it just appears in the output alongside skewness, always. No interaction with other features. No ambiguity about the formula (excess kurtosis is the standard).

#### Descriptor Format

Rather than printing a separate table in the output (which would break the visual flow), the descriptors should follow the existing inline pattern used by skewness and CV. The `interpretKurtosis` function would embed the meaning directly in the label:

- `"Platykurtic (flat, thin tails)"`
- `"Mesokurtic (normal-like)"`
- `"Leptokurtic (peaked, heavy tails)"`

Producing output like:

```
Kurtosis:  -1.53 (Platykurtic - flat, thin tails)
Kurtosis:   0.12 (Mesokurtic - normal-like)
Kurtosis:   2.45 (Leptokurtic - peaked, heavy tails)
```

This keeps output scannable (one line, no table), consistent with skewness/CV formatting, and still teaches the user what the Greek term means. The full reference table belongs in the README's "Understanding the Output" section.

#### My Take

This is the easiest win of all the features discussed. It's maybe 30 lines of code mirroring existing patterns, plus tests. It pairs naturally with skewness in the output and adds real descriptive value.
