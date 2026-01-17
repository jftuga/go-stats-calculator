// stats.go
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/term"
)

const PgmName string = "stats"
const PgmUrl string = "https://github.com/jftuga/go-stats-calculator"
const PgmDisclaimer string = "DISCLAIMER: This program is vibe-coded. Use at your own risk."
const PgmSeeAlso string = "SEE ALSO: " + PgmUrl + "/tree/main?tab=readme-ov-file#testing-and-correctness"

const PgmVersion string = "1.0.0"

// Stats holds the computed statistical results.
type Stats struct {
	Count    int
	Sum      float64
	Mean     float64
	Median   float64
	Mode     []float64 // A dataset can have more than one mode
	Min      float64
	Max      float64
	StdDev   float64 // Standard Deviation
	Variance float64 // Variance = StdDev^2
	Q1       float64 // 1st Quartile (25th percentile)
	Q3       float64 // 3rd Quartile (75th percentile)
	P95      float64 // 95th percentile
	P99      float64 // 99th percentile
	IQR      float64 // Interquartile Range (Q3 - Q1)
	Outliers []float64
	Skewness float64 // Formal skewness value
}

func main() {
	version := flag.Bool("v", false, "show version")
	flag.Parse()

	if *version {
		fmt.Printf("%s version %s\n%s\n\n%s\n%s\n", PgmName, PgmVersion, PgmUrl, PgmDisclaimer, PgmSeeAlso)
		os.Exit(0)
	}
	args := flag.Args()
	// Determine whether stdin is a terminal
	inputIsTerminal := term.IsTerminal(int(os.Stdin.Fd()))

	if len(args) < 1 && inputIsTerminal {
		fmt.Fprintf(os.Stderr, "Usage:\n  %s <filename>\n  %s -\n", os.Args[0], os.Args[0])
		fmt.Fprintln(os.Stderr, "Description:\n  Computes statistics from a list of numbers.")
		fmt.Fprintln(os.Stderr, "  Provide a filename or use '-' to read from standard input.")
		os.Exit(1)
	}

	var reader io.Reader

	if len(args) == 0 || args[0] == "-" {
		// No args with piped input, or explicit "-" flag
		reader = os.Stdin
	} else {
		file, err := os.Open(args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening file: %v\n", err)
			os.Exit(1)
		}
		defer file.Close()
		reader = file
	}

	numbers, err := readNumbers(reader)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading numbers: %v\n", err)
		os.Exit(1)
	}

	stats, err := computeStats(numbers)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error computing stats: %v\n", err)
		os.Exit(1)
	}

	printStats(stats)
}

// readNumbers reads floating-point numbers (one per line) from an io.Reader.
func readNumbers(reader io.Reader) ([]float64, error) {
	var numbers []float64
	scanner := bufio.NewScanner(reader)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue // Skip empty lines
		}

		num, err := strconv.ParseFloat(line, 64)
		if err != nil {
			// Log invalid lines but continue processing
			fmt.Fprintf(
				os.Stderr,
				"Warning: skipping invalid number on line %d: '%s'\n",
				lineNum,
				scanner.Text(),
			)
			continue
		}
		numbers = append(numbers, num)
	}
	return numbers, scanner.Err()
}

// computeStats calculates all the desired statistics for a slice of numbers.
func computeStats(data []float64) (*Stats, error) {
	count := len(data)
	if count == 0 {
		return nil, fmt.Errorf("input contains no valid numbers")
	}

	// Create a sorted copy for calculations that require it (median, quartiles).
	sortedData := make([]float64, count)
	copy(sortedData, data)
	sort.Float64s(sortedData)

	// --- Basic Stats ---
	stats := &Stats{
		Count: count,
		Min:   sortedData[0],
		Max:   sortedData[count-1],
	}

	// --- Mean ---
	var sum float64
	for _, v := range data {
		sum += v
	}
	stats.Sum = sum
	stats.Mean = sum / float64(count)

	// --- Variance and Standard Deviation ---
	if count > 1 {
		var sumOfSquares float64
		for _, v := range data {
			sumOfSquares += math.Pow(v-stats.Mean, 2)
		}
		// Using sample variance (N-1), which is more common.
		stats.Variance = sumOfSquares / float64(count-1)
		stats.StdDev = math.Sqrt(stats.Variance)
	}

	// --- Median, Q1, Q3, P95, P99 (Percentiles) ---
	stats.Median = calculatePercentile(sortedData, 0.50)
	stats.Q1 = calculatePercentile(sortedData, 0.25)
	stats.Q3 = calculatePercentile(sortedData, 0.75)
	stats.P95 = calculatePercentile(sortedData, 0.95)
	stats.P99 = calculatePercentile(sortedData, 0.99)

	// --- IQR ---
	stats.IQR = stats.Q3 - stats.Q1

	// --- Mode (single-pass efficient algorithm) ---
	freqs := make(map[float64]int)
	for _, v := range data {
		freqs[v]++
	}

	var modes []float64
	maxFreq := 0 // Start at 0 to correctly find the max frequency
	for val, freq := range freqs {
		if freq > maxFreq {
			maxFreq = freq
			modes = []float64{val} // New max, reset the slice
		} else if freq == maxFreq {
			modes = append(modes, val) // Found another mode
		}
	}

	// If the max frequency is 1, it means no number repeated, so there is no mode.
	if maxFreq <= 1 {
		stats.Mode = []float64{} // Return an empty slice
	} else {
		stats.Mode = modes
		sort.Float64s(stats.Mode) // For consistent output
	}

	// --- Outliers (using the 1.5 * IQR rule) ---
	lowerBound := stats.Q1 - 1.5*stats.IQR
	upperBound := stats.Q3 + 1.5*stats.IQR

	for _, v := range data {
		if v < lowerBound || v > upperBound {
			stats.Outliers = append(stats.Outliers, v)
		}
	}
	sort.Float64s(stats.Outliers) // For consistent output

	// --- Skewness (formal calculation) ---
	stats.Skewness = calculateSkewness(data, stats.Mean, stats.StdDev)

	return stats, nil
}

// calculatePercentile finds the value at a given percentile (p) in sorted data.
func calculatePercentile(sortedData []float64, p float64) float64 {
	n := len(sortedData)
	if n == 0 {
		return 0
	}
	if n == 1 {
		return sortedData[0]
	}

	rank := p * float64(n-1)
	lowerIndex := math.Floor(rank)
	upperIndex := math.Ceil(rank)

	if lowerIndex == upperIndex {
		return sortedData[int(rank)]
	}

	weight := rank - lowerIndex
	return sortedData[int(lowerIndex)]*(1-weight) + sortedData[int(upperIndex)]*weight
}

// calculateSkewness computes the adjusted Fisher-Pearson standardized moment coefficient.
func calculateSkewness(data []float64, mean, stdDev float64) float64 {
	n := float64(len(data))
	if n < 3 || stdDev == 0 {
		return 0 // Skewness is not defined for less than 3 points or zero std dev
	}

	var sumOfCubedDeviations float64
	for _, v := range data {
		sumOfCubedDeviations += math.Pow(v-mean, 3)
	}

	// Formula for sample skewness
	return (n / ((n - 1) * (n - 2))) * (sumOfCubedDeviations / math.Pow(stdDev, 3))
}

// formatFloat formats a float64 without scientific notation, trimming unnecessary trailing zeros.
func formatFloat(v float64) string {
	if v == math.Trunc(v) {
		return strconv.FormatFloat(v, 'f', 0, 64)
	}
	s := strconv.FormatFloat(v, 'f', 4, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimSuffix(s, ".")
	return s
}

// formatFloatSlice formats a slice of float64 values without scientific notation.
func formatFloatSlice(values []float64) string {
	if len(values) == 0 {
		return "[]"
	}
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = formatFloat(v)
	}
	return "[" + strings.Join(parts, " ") + "]"
}

// interpretSkewness provides a human-readable label for a skewness value.
func interpretSkewness(s float64) string {
	absS := math.Abs(s)
	if absS < 0.5 {
		return "Fairly Symmetrical"
	}
	if absS < 1.0 {
		if s > 0 {
			return "Moderately Right Skewed"
		}
		return "Moderately Left Skewed"
	}
	if s > 0 {
		return "Highly Right Skewed"
	}
	return "Highly Left Skewed"
}

// printStats displays the results in a readable format.
func printStats(s *Stats) {
	fmt.Println("--- Descriptive Statistics ---")
	fmt.Printf("Count:          %d\n", s.Count)
	fmt.Printf("Sum:            %s\n", formatFloat(s.Sum))
	fmt.Printf("Min:            %s\n", formatFloat(s.Min))
	fmt.Printf("Max:            %s\n", formatFloat(s.Max))
	fmt.Println("\n--- Measures of Central Tendency ---")
	fmt.Printf("Mean:           %s\n", formatFloat(s.Mean))
	fmt.Printf("Median (p50):   %s\n", formatFloat(s.Median))

	switch len(s.Mode) {
	case 0:
		fmt.Println("Mode:           None")
	case 1:
		// If there's only one mode, print it as a clean number.
		fmt.Printf("Mode:           %s\n", formatFloat(s.Mode[0]))
	default:
		// If there are multiple modes, label it and print the slice.
		fmt.Printf("Mode (multi):   %s\n", formatFloatSlice(s.Mode))
	}

	fmt.Println("\n--- Measures of Spread & Distribution ---")
	fmt.Printf("Std Deviation:  %s\n", formatFloat(s.StdDev))
	fmt.Printf("Variance:       %s\n", formatFloat(s.Variance))
	fmt.Printf("Quartile 1 (p25): %s\n", formatFloat(s.Q1))
	fmt.Printf("Quartile 3 (p75): %s\n", formatFloat(s.Q3))
	fmt.Printf("Percentile (p95): %s\n", formatFloat(s.P95))
	fmt.Printf("Percentile (p99): %s\n", formatFloat(s.P99))
	fmt.Printf("IQR:            %s\n", formatFloat(s.IQR))
	fmt.Printf("Skewness:       %s (%s)\n", formatFloat(s.Skewness), interpretSkewness(s.Skewness))
	if len(s.Outliers) > 0 {
		fmt.Printf("Outliers:       %s\n", formatFloatSlice(s.Outliers))
	} else {
		fmt.Println("Outliers:       None")
	}
}
