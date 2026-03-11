# numstats

Run the `stats` CLI tool to compute descriptive statistics on numeric data.

## Instructions

You are a skill that runs the `stats` command-line tool. This tool computes descriptive statistics from newline-delimited numbers, reading from files or stdin.

When the user asks you to compute statistics, analyze data, detect outliers, visualize distributions, or perform any statistical analysis on numeric data, use the `stats` binary.

### How to invoke

```bash
# From a file
stats data.txt

# From stdin
echo -e "1\n2\n3\n4\n5" | stats

# Generate data inline
printf '%s\n' 10 20 30 40 50 | stats
```

### Available flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-v` | bool | false | Show version |
| `-p` | string | "" | Comma-separated custom percentiles (0.0-100.0) |
| `-k` | float | 1.5 | IQR multiplier for outlier detection |
| `-b` | int | 16 | Number of histogram/trendline bins (5-50) |
| `-z` | float | 0 | Z-score threshold for outlier detection (>= 1.0 to enable) |
| `-l` | bool | false | Log transform (ln) input data (requires all positive values) |
| `-t` | float | 0 | Trimmed mean percentage from each tail (0-50) |
| `-T` | float | 0 | Trim dataset percentage from each tail before all stats (0-50) |
| `-e` | int | 0 | EMA span for exponential moving average (>= 2 to enable) |

**Note:** `-t` and `-T` are mutually exclusive.

### Computed statistics

- **Count, Sum, Min, Max**
- **Mean, Median, Mode**
- **Std Deviation, Variance, Coefficient of Variation**
- **Q1, Q3, IQR, P95, P99** (plus custom percentiles via `-p`)
- **Skewness** (symmetry) and **Kurtosis** (tailedness)
- **Outliers** via IQR method (always) and Z-score method (when `-z` is set)
- **Histogram** (sorted data distribution) and **Trendline** (input order) using Unicode blocks
- **Trimmed Mean** (via `-t`), **EMA** (via `-e`)

### Guidelines

1. **Choosing flags based on user intent:**
   - User mentions outliers → consider `-z 2.0` or adjusting `-k`
   - User wants robust statistics → suggest `-t` or `-T`
   - User has skewed data (file sizes, latencies, salaries) → suggest `-l` for log transform
   - User wants trend analysis → mention the trendline output and `-e` for EMA
   - User wants specific percentiles → use `-p "10,25,50,75,90"`

2. **Preparing data for stats:**
   - Extract numeric columns from CSVs: `awk -F',' '{print $2}' data.csv | stats`
   - Filter and pipe: `grep -v '^#' data.txt | stats`
   - Generate from commands: `wc -l src/*.go | head -n -1 | awk '{print $1}' | stats`

3. **Combining flags for deeper analysis:**
   - Full analysis: `stats -z 2.0 -p "5,10,90,95" -e 10 data.txt`
   - Outlier-resistant: `stats -T 5 -z 2.0 data.txt`
   - Log-scale analysis: `stats -l -p "10,50,90" data.txt`

4. **Interpreting output for the user:**
   - Explain what skewness and kurtosis values mean in context
   - Highlight outliers and suggest whether they matter
   - Compare mean vs median to assess skew impact
   - Note if CV is high (data is highly variable)

5. **Always show the raw `stats` output** to the user, then provide interpretation if the data warrants it or the user asks.
