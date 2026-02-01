# Go Stats Calculator

![Code Base: AI Vibes](https://img.shields.io/badge/Code%20Base-AI%20Vibes%20%F0%9F%A4%A0-blue)

A robust, command-line tool written in Go to compute a comprehensive set of descriptive statistics from a list of numbers. It can read data from a file or directly from standard input, making it a flexible utility for data analysis in any shell environment.

## Overview

This program takes a simple, newline-delimited list of numbers (integers or floats) and calculates key statistical properties, including measures of central tendency (mean, median, mode), measures of spread (standard deviation, variance, IQR), and the shape of the distribution (skewness). It also identifies outliers based on the interquartile range method.

## Disclaimer

This program was vibe-coded by `Gemini 2.5` Pro and `Opus 4.5`. As such, the author can't be held responsible for incorrect calculations. Please verify the results for any critical applications. That said, it has been validated through unit tests and independent verification. See [Testing and Correctness](#testing-and-correctness) for details.

## Features

The calculator computes the following statistics:

-   **Count**: Total number of valid data points.
-   **Min / Max**: The minimum and maximum values in the dataset.
-   **Mean**: The arithmetic average.
-   **Median (p50)**: The 50th percentile, or the middle value of the dataset.
-   **Mode**: The value(s) that appear most frequently.
-   **Standard Deviation**: A measure of the amount of variation or dispersion.
-   **Variance**: The square of the standard deviation.
-   **Quartiles (Q1, Q3)**: The 25th (p25) and 75th (p75) percentiles.
-   **Percentiles (p95, p99)**: The 95th and 99th percentiles, useful for understanding tail distributions.
-   **Custom Percentiles**: Compute any percentile(s) between 0 and 100 using the `-p` flag.
-   **Interquartile Range (IQR)**: The range between the first and third quartiles (Q3 - Q1).
-   **Skewness**: A formal measure of the asymmetry of the data distribution.
-   **Kurtosis**: Excess kurtosis measuring the "tailedness" of the distribution. Values near 0 indicate normal-like tails, negative values indicate thin tails, and positive values indicate heavy tails.
-   **Coefficient of Variation (CV)**: The ratio of the standard deviation to the mean, expressed as a percentage. Useful for comparing variability across datasets with different units or scales.
-   **Outliers**: Data points identified as abnormally distant from other values, using the IQR method with a configurable multiplier (`-k` flag).
-   **Z-Score Outliers**: Optional outlier detection using Z-score method, flagging data points more than a configurable number of standard deviations from the mean (`-z` flag). Ideal for normally distributed data.
-   **Histogram**: A single-line Unicode histogram showing the distribution of values across configurable bins (`-b` flag).
-   **Trendline**: A single-line Unicode trendline showing the sequence pattern of values in their original input order, using configurable bins (`-b` flag).

All numeric output uses full decimal notation (no scientific notation) with trailing zeros trimmed for readability.

## Installation

### Homebrew (MacOS / Linux):

* `brew tap jftuga/homebrew-tap; brew update; brew install jftuga/tap/stats`

### Source

* Clone the repository and build:
    ```bash
    git clone https://github.com/jftuga/go-stats-calculator.git
    cd go-stats-calculator
    go build -ldflags="-s -w" -o stats stats.go
    ```

## Usage

The program can be run in two ways: by providing a filename as a command-line argument or by piping data into it. The program automatically detects piped input, so the `-` argument is optional.

### 1. Read from a File

Provide the path to a file containing numbers, one per line.

**Syntax:**
```bash
./stats <filename>
```

**Example:**
```bash
./stats data.txt
```

### 2. Read from Standard Input (stdin)

Pipe data from other commands directly into the program. The program automatically detects piped input, so no special argument is needed. You can optionally use the `-` argument for explicit stdin reading.

**Syntax:**
```bash
<command> | ./stats
```

**Examples:**
```bash
# Pipe the contents of a file
cat data.txt | ./stats

# Pipe output from another command (e.g., extracting a column from a CSV)
awk -F',' '{print $3}' metrics.csv | ./stats

# Explicit stdin reading (also works)
cat data.txt | ./stats -

# Manually enter numbers (press Ctrl+D when finished)
./stats -
10
20
30
^D
```

### 3. Custom Percentiles

Use the `-p` flag to compute additional percentiles. Provide a comma-separated list of values between 0 and 100.

**Syntax:**
```bash
./stats -p <percentiles> <filename>
```

**Examples:**
```bash
# Compute 10th and 90th percentiles
./stats -p "10,90" data.txt

# Compute multiple percentiles including decimals
./stats -p "5,10,90,99.9" data.txt

# Combined with stdin
cat data.txt | ./stats -p "10,50,90"
```

### 4. Outlier Sensitivity

Use the `-k` flag to adjust the IQR multiplier used for outlier detection. The default is `1.5`, which corresponds to Tukey's inner fences. A smaller value flags more data points as outliers; a larger value flags fewer.

**Syntax:**
```bash
./stats -k <multiplier> <filename>
```

**Examples:**
```bash
# Default behavior (k=1.5, Tukey's inner fences)
./stats data.txt

# Extreme outliers only (k=3.0, Tukey's outer fences)
./stats -k 3.0 data.txt

# High sensitivity (k=1.0, useful for fraud detection, quality control, or manual inspection)
./stats -k 1.0 data.txt

# Combined with other flags
./stats -k 2.0 -p "10,90" data.txt
```

### 5. Histogram / Trendline Bins

Use the `-b` flag to control the number of bins in the histogram and trendline. The default is `16`, and valid values range from `5` to `50`.

**Syntax:**
```bash
./stats -b <bins> <filename>
```

**Examples:**
```bash
# Default 16 bins
./stats data.txt

# Fewer bins for a coarser view
./stats -b 8 data.txt

# More bins for finer detail
./stats -b 32 data.txt

# Combined with other flags
./stats -b 10 -k 2.0 -p "10,90" data.txt
```

### 6. Z-Score Outlier Detection

Use the `-z` flag to enable Z-score based outlier detection. A data point is flagged when its Z-score (number of standard deviations from the mean) exceeds the given threshold. This method is disabled by default and complements the always-shown IQR method.

**IQR vs Z-Score — when to use which:**

The default IQR method uses quartiles and makes no assumptions about the shape of your data, so it works well for skewed distributions, small samples, or data you haven't explored yet. The Z-score method measures distance from the mean in standard deviations and assumes data is roughly normally distributed.

Consider using `-z` when:
- Your data is approximately bell-shaped (e.g., measurement errors, test scores, sensor readings).
- You want outlier detection tied to a specific confidence level (Z>2 ≈ 95%, Z>3 ≈ 99.7%).
- You want to cross-check IQR results with a second method — values flagged by both are strong outlier candidates.

Stick with the default IQR method when:
- Your data is skewed, heavy-tailed, or you're unsure of the distribution.
- The sample size is small (Z-scores rely on mean and standard deviation, which are sensitive to outliers themselves).
- You need a single, assumption-free approach.

**Syntax:**
```bash
./stats -z <threshold> <filename>
```

**Examples:**
```bash
# Flag values more than 2 standard deviations from the mean
./stats -z 2.0 data.txt

# Stricter threshold (fewer outliers flagged)
./stats -z 3.0 data.txt

# Combined with other flags
./stats -z 2.5 -k 2.0 -p "10,90" data.txt
```

**Common Z-score thresholds:**

| Threshold | Description |
| :-------- | :---------- |
| **2.0** | Flags values beyond ~95.4% of a normal distribution. Good for exploratory analysis. |
| **2.5** | A moderate threshold, balancing sensitivity and specificity. |
| **3.0** | Flags values beyond ~99.7% of a normal distribution. Conservative, suitable for large datasets. |

**Common multiplier values:**

| Multiplier | Description |
| :--------- | :---------- |
| **1.0** | High sensitivity. Useful for scenarios such as fraud detection or quality control where you want to manually inspect anything even slightly suspicious. Will produce more false positives. |
| **1.5** | Standard (default). Tukey's inner fences, the widely accepted general-purpose threshold. |
| **3.0** | Tukey's outer fences, the standard threshold for extreme or "far out" outliers. Only the most anomalous points are flagged. |

## Example

Given a file named `sample_data.txt` with the following content:

**`sample_data.txt`**
```
14.52
16.81
13.99
15.05
17.33
21.40
15.05
18.92
24.78
19.61
38.95
22.13
16.42
35.88
20.11
```

Running the command `./stats -z 2.0 sample_data.txt` will produce the following output:

```
--- Descriptive Statistics ---
Count:          15
Sum:            310.95
Min:            13.99
Max:            38.95

--- Measures of Central Tendency ---
Mean:           20.73
Median (p50):   18.92
Mode:           15.05

--- Measures of Spread & Distribution ---
Std Deviation:    7.4605
Variance:         55.6597
CV:               35.9891% (High Variability)
Quartile 1 (p25): 15.735
Quartile 3 (p75): 21.765
Percentile (p95): 36.801
Percentile (p99): 38.5202
IQR:              6.03
Skewness:         1.6862 (Highly Right Skewed)
Kurtosis:         2.2437 (Leptokurtic - peaked, heavy tails)
Outliers:         [35.88 38.95]
Z-Outliers (Z>2): [35.88 38.95]

--- Distribution ---
Histogram:        █▄▂▆▂▂▂▁▁▁▁▁▁▁▂▂
Trendline:        ▁▁▁▁▁▃▁▂▄▂█▃▁▇▂
```

The **Histogram** shows *distribution* — how values are spread across bins from sorted data. The **Trendline** shows *sequence* — how values trend over their original input order. Together they give a fuller picture of the dataset.

## Understanding the Output

| Statistic         | Description                                                                                                                                                                |
| :---------------- |:---------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Count**         | The total number of valid numeric entries processed.                                                                                                                       |
| **Min**           | The smallest number in the dataset.                                                                                                                                        |
| **Max**           | The largest number in the dataset.                                                                                                                                         |
| **Mean**          | The "average" value. Highly sensitive to outliers.                                                                                                                         |
| **Median (p50)**  | The middle value of the sorted dataset. Represents the "typical" value and is robust against outliers.                                                                     |
| **Mode**          | The number(s) that occur most frequently. If no number repeats, the mode is "None".                                                                                        |
| **Std Deviation** | Measures how spread out the numbers are from the mean. A low value indicates data is clustered tightly; a high value indicates data is spread out.                         |
| **Variance**      | The square of the standard deviation.                                                                                                                                      |
| **CV**            | The ratio of the standard deviation to the mean, expressed as a percentage. CV < 15% indicates low variability, 15–30% moderate variability, and ≥ 30% high variability. Shows "N/A" when the mean is near zero, and displays a warning if the dataset contains negative values. |
| **Quartile 1 (p25)** | The value below which 25% of the data falls.                                                                                                                            |
| **Quartile 3 (p75)** | The value below which 75% of the data falls.                                                                                                                            |
| **Percentile (p95)** | The value below which 95% of the data falls. Useful for understanding the upper tail of the distribution.                                                              |
| **Percentile (p99)** | The value below which 99% of the data falls. Useful for identifying extreme values and tail behavior.                                                                   |
| **Percentile (pN)** | Custom percentiles requested via the `-p` flag. The value below which N% of the data falls.                                                                              |
| **IQR**           | The Interquartile Range (`Q3 - Q1`). It represents the middle 50% of the data and is a robust measure of spread.                                                           |
| **Skewness**      | A measure of asymmetry. A value near 0 is symmetrical. A positive value indicates a "right skew" (a long tail of high values). A negative value indicates a "left skew".   |
| **Kurtosis**      | Excess kurtosis measuring the "tailedness" of the distribution. Values < -1 are platykurtic (flat, thin tails), between -1 and 1 are mesokurtic (normal-like), and > 1 are leptokurtic (peaked, heavy tails). |
| **Outliers**      | Values that fall outside the range of `Q1 - k*IQR` and `Q3 + k*IQR`, where `k` defaults to 1.5 and can be adjusted with the `-k` flag.                                      |
| **Z-Score Outliers** | Values whose Z-score (number of standard deviations from the mean) exceeds the threshold set with the `-z` flag. Only shown when `-z` is provided. Ideal for normally distributed data. |
| **Histogram**     | A single-line Unicode histogram showing data distribution across bins. Each character represents a bin, with taller blocks indicating more values. Bin count is configurable with the `-b` flag (default 16). |
| **Trendline**     | A single-line Unicode trendline showing the sequence pattern of values in their original input order. Data is divided into equal chunks, each averaged and mapped to a block character. Bin count is configurable with the `-b` flag (default 16). |

## Testing and Correctness

The program includes two layers of verification:

### Unit Tests (`stats_test.go`)

Standard Go unit tests cover the core statistical functions:

- `computeStats` - verifies all computed statistics against a 31-number dataset
- `calculatePercentile` - tests percentile interpolation at various points (p0, p25, p50, p75, p100)
- `calculateSkewness` - validates skewness calculations for symmetric and skewed distributions

Run the tests with:
```bash
go test -v
```

### Independent Verification (`verify_stats.sh`)

A shell script independently calculates statistics using `bc` (arbitrary precision calculator) and compares the results against the program's output. This provides external validation that the Go implementation produces correct results.

The script was developed and tested on `MacOS Sequoia 15.7.3` using:
- `bc` - arbitrary precision calculator for sum, mean, variance, standard deviation, and percentile calculations
- `sort` - for ordering the dataset to verify percentile indices

Run the verification with:
```bash
./verify_stats.sh
```

The script exits with code `0` if all values match, or code `1` if any discrepancies are found.

### Test Data Characteristics

The test dataset consists of 31 numbers designed to exercise common scenarios:
- A mix of integers, decimals, and numbers with trailing zeros (e.g., `25.00`, `35.0`)
- Repeated values to produce a defined mode
- An outlier value to verify outlier detection

The tests focus on typical usage patterns and do not cover exotic edge cases, extreme values, or adversarial inputs. Users requiring high-assurance results for critical applications should perform additional validation appropriate to their use case.

## Acknowledgements

- [John Tukey's 1977 *Exploratory Data Analysis*](https://archive.org/details/exploratorydataa0000tuke_7616)
    - Used as the source for the outlier sensitivity multiplier values (inner fences at 1.5×IQR and outer fences at 3.0×IQR)

## Personal Project Disclosure

This program is my own original idea, conceived and developed entirely:

* On my own personal time, outside of work hours
* For my own personal benefit and use
* On my personally owned equipment
* Without using any employer resources, proprietary information, or trade secrets
* Without any connection to my employer's business, products, or services
* Independent of any duties or responsibilities of my employment

This project does not relate to my employer's actual or demonstrably
anticipated research, development, or business activities. No
confidential or proprietary information from any employer was used
in its creation.
