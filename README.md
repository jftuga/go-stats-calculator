# Go Stats Calculator

A robust, command-line tool written in Go to compute a comprehensive set of descriptive statistics from a list of numbers. It can read data from a file or directly from standard input, making it a flexible utility for data analysis in any shell environment.

## Overview

This program takes a simple, newline-delimited list of numbers (integers or floats) and calculates key statistical properties, including measures of central tendency (mean, median, mode), measures of spread (standard deviation, variance, IQR), and the shape of the distribution (skewness). It also identifies outliers based on the interquartile range method.

## Disclaimer

This program was 100% vibe-coded by Gemini 2.5 Pro. As such, the author can't be held responsible for incorrect calculations. Please verify the results for any critical applications.

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
-   **Interquartile Range (IQR)**: The range between the first and third quartiles (Q3 - Q1).
-   **Skewness**: A formal measure of the asymmetry of the data distribution.
-   **Outliers**: Data points identified as abnormally distant from other values.

## Installation

To use this program, you need to have Go installed on your system.

1.  **Clone the repository** (or save the `stats.go` file to a new directory):
    ```bash
    # Example using git
    git clone https://github.com/your-username/go-stats-calculator.git
    cd go-stats-calculator
    ```

2.  **Build the executable:**
    ```bash
    # This creates an executable file named 'stats' in the current directory
    go build -ldflags="-s -w" -o stats stats.go
    ```

## Usage

The program can be run in two ways: by providing a filename as a command-line argument or by piping data into it using the `-` argument.

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

Use the `-` argument to tell the program to read from standard input. This allows you to pipe data from other commands.

**Syntax:**
```bash
<command> | ./stats -
```

**Examples:**
```bash
# Pipe the contents of a file
cat data.txt | ./stats -

# Pipe output from another command (e.g., extracting a column from a CSV)
awk -F',' '{print $3}' metrics.csv | ./stats -

# Manually enter numbers (press Ctrl+D when finished)
./stats -
10
20
30
^D
```

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

Running the command `./stats sample_data.txt` will produce the following output:

```
--- Descriptive Statistics ---
Count:          15
Sum:            310.9500
Min:            13.9900
Max:            38.9500

--- Measures of Central Tendency ---
Mean:           20.7300
Median (p50):   18.9200
Mode:           15.0500

--- Measures of Spread & Distribution ---
Std Deviation:  7.4605
Variance:       55.6597
Quartile 1 (p25): 15.7350
Quartile 3 (p75): 21.7650
IQR:            6.0300
Skewness:       1.6862 (Highly Right Skewed)
Outliers:       [35.88 38.95]
```

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
| **Quartile 1 (p25)** | The value below which 25% of the data falls.                                                                                                                            |
| **Quartile 3 (p75)** | The value below which 75% of the data falls.                                                                                                                            |
| **IQR**           | The Interquartile Range (`Q3 - Q1`). It represents the middle 50% of the data and is a robust measure of spread.                                                           |
| **Skewness**      | A measure of asymmetry. A value near 0 is symmetrical. A positive value indicates a "right skew" (a long tail of high values). A negative value indicates a "left skew".   |
| **Outliers**      | Values that fall outside the range of `Q1 - 1.5*IQR` and `Q3 + 1.5*IQR`. These are statistically unusual data points.                                                      |
