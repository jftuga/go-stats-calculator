// stats.go
package main

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
)

// Stats holds the computed statistical results.
type Stats struct {
	Count    int
	Mean     float64
	Median   float64
	Mode     []float64 // A dataset can have more than one mode
	Min      float64
	Max      float64
	StdDev   float64 // Standard Deviation
	Variance float64 // Variance = StdDev^2
	Q1       float64 // 1st Quartile (25th percentile)
	Q3       float64 // 3rd Quartile (75th percentile)
	IQR      float64 // Interquartile Range (Q3 - Q1)
	Outliers []float64
	Skewness float64 // Formal skewness value
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(
			os.Stderr,
			"Usage:\n  %s <filename>\n  %s -\n",
			os.Args[0],
			os.Args[0],
		)
		fmt.Fprintf(
			os.Stderr,
			"Description:\n  Computes statistics from a list of numbers.\n",
		)
		fmt.Fprintf(
			os.Stderr,
			"  Provide a filename or use '-' to read from standard input.\n",
		)
		os.Exit(1)
	}

	var reader io.Reader
	arg := os.Args[1]

	if arg == "-" {
		reader = os.Stdin
	} else {
		file, err := os.Open(arg)
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

	// --- Median, Q1, Q3 (Percentiles) ---
	stats.Median = calculatePercentile(sortedData, 0.50)
	stats.Q1 = calculatePercentile(sortedData, 0.25)
	stats.Q3 = calculatePercentile(sortedData, 0.75)

	// --- IQR ---
	stats.IQR = stats.Q3 - stats.Q1

	// --- Mode (single-pass efficient algorithm) ---
	freqs := make(map[float64]int)
	for _, v := range data {
		freqs[v]++
	}

	var modes []float64
	maxFreq := 1 // Start at 1, so if no value repeats, we get an empty slice.
	for val, freq := range freqs {
		if freq > maxFreq {
			maxFreq = freq
			modes = []float64{val} // New max, reset the slice
		} else if freq == maxFreq {
			modes = append(modes, val) // Found another mode
		}
	}
	stats.Mode = modes
	sort.Float64s(stats.Mode) // For consistent output

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
	fmt.Printf("Min:            %.4f\n", s.Min)
	fmt.Printf("Max:            %.4f\n", s.Max)
	fmt.Println("\n--- Measures of Central Tendency ---")
	fmt.Printf("Mean:           %.4f\n", s.Mean)
	fmt.Printf("Median (p50):   %.4f\n", s.Median)
	if len(s.Mode) > 0 {
		fmt.Printf("Mode:           %v\n", s.Mode)
	} else {
		fmt.Println("Mode:           None")
	}
	fmt.Println("\n--- Measures of Spread & Distribution ---")
	fmt.Printf("Std Deviation:  %.4f\n", s.StdDev)
	fmt.Printf("Variance:       %.4f\n", s.Variance)
	fmt.Printf("Quartile 1 (p25): %.4f\n", s.Q1)
	fmt.Printf("Quartile 3 (p75): %.4f\n", s.Q3)
	fmt.Printf("IQR:            %.4f\n", s.IQR)
	fmt.Printf("Skewness:       %.4f (%s)\n", s.Skewness, interpretSkewness(s.Skewness))
	if len(s.Outliers) > 0 {
		fmt.Printf("Outliers:       %v\n", s.Outliers)
	} else {
		fmt.Println("Outliers:       None")
	}
}
