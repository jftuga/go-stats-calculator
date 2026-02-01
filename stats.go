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

const PgmVersion string = "1.5.0"

// Stats holds the computed statistical results.
type Stats struct {
	Count             int
	Sum               float64
	Mean              float64
	Median            float64
	Mode              []float64 // A dataset can have more than one mode
	Min               float64
	Max               float64
	StdDev            float64 // Standard Deviation
	Variance          float64 // Variance = StdDev^2
	Q1                float64 // 1st Quartile (25th percentile)
	Q3                float64 // 3rd Quartile (75th percentile)
	P95               float64 // 95th percentile
	P99               float64 // 99th percentile
	IQR               float64 // Interquartile Range (Q3 - Q1)
	Outliers          []float64
	Skewness          float64             // Formal skewness value
	Kurtosis          float64             // Excess kurtosis
	CV                float64             // Coefficient of Variation as a percentage
	HasNegativeData   bool                // Flag for negative value warning
	CVValid           bool                // False when mean is near zero
	CustomPercentiles map[float64]float64 // User-requested percentiles
	Sparkline         string              // Unicode sparkline histogram
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] <filename | ->\n", os.Args[0])
		fmt.Fprintln(os.Stderr, "Computes statistics from a list of numbers.")
		fmt.Fprintln(os.Stderr, "\nOptions:")
		flag.PrintDefaults()
	}
	version := flag.Bool("v", false, "show version")
	percentileFlag := flag.String("p", "", "comma-separated percentiles to compute (0.0-100.0)")
	iqrMultiplier := flag.Float64("k", 1.5, "IQR multiplier for outlier detection (default: 1.5)")
	numBins := flag.Int("b", 16, "number of bins for sparkline histogram (5-50)")
	flag.Parse()

	if *numBins < 5 || *numBins > 50 {
		fmt.Fprintf(os.Stderr, "Error: number of bins must be between 5 and 50, got %d\n", *numBins)
		os.Exit(1)
	}

	if *version {
		fmt.Printf("%s version %s\n%s\n\n%s\n%s\n", PgmName, PgmVersion, PgmUrl, PgmDisclaimer, PgmSeeAlso)
		os.Exit(0)
	}
	args := flag.Args()
	// Determine whether stdin is a terminal
	inputIsTerminal := term.IsTerminal(int(os.Stdin.Fd()))

	if len(args) < 1 && inputIsTerminal {
		flag.Usage()
		os.Exit(0)
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

	var customPercentiles []float64
	if *percentileFlag != "" {
		for _, s := range strings.Split(*percentileFlag, ",") {
			p, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: invalid percentile value '%s'\n", s)
				os.Exit(1)
			}
			if p < 0 || p > 100 {
				fmt.Fprintf(os.Stderr, "Error: percentile %v must be between 0 and 100\n", p)
				os.Exit(1)
			}
			customPercentiles = append(customPercentiles, p)
		}
	}

	stats, err := computeStats(numbers, customPercentiles, *iqrMultiplier, *numBins)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error computing stats: %v\n", err)
		os.Exit(1)
	}

	labelWidth := 18
	if len(customPercentiles) > 0 {
		labelWidth = 22
	}
	printStats(stats, labelWidth)
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
func computeStats(data []float64, customPercentiles []float64, iqrMultiplier float64, numBins int) (*Stats, error) {
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

	// --- Custom Percentiles ---
	if len(customPercentiles) > 0 {
		stats.CustomPercentiles = make(map[float64]float64)
		for _, p := range customPercentiles {
			stats.CustomPercentiles[p] = calculatePercentile(sortedData, p/100.0)
		}
	}

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

	// --- Outliers (using the k * IQR rule) ---
	lowerBound := stats.Q1 - iqrMultiplier*stats.IQR
	upperBound := stats.Q3 + iqrMultiplier*stats.IQR

	for _, v := range data {
		if v < lowerBound || v > upperBound {
			stats.Outliers = append(stats.Outliers, v)
		}
	}
	sort.Float64s(stats.Outliers) // For consistent output

	// --- Skewness (formal calculation) ---
	stats.Skewness = calculateSkewness(data, stats.Mean, stats.StdDev)

	// --- Kurtosis (excess kurtosis) ---
	stats.Kurtosis = calculateKurtosis(data, stats.Mean, stats.StdDev)

	// --- Check for negative data ---
	for _, v := range data {
		if v < 0 {
			stats.HasNegativeData = true
			break
		}
	}

	// --- Coefficient of Variation ---
	if math.Abs(stats.Mean) < 1e-10 {
		stats.CVValid = false
	} else {
		stats.CVValid = true
		stats.CV = (stats.StdDev / math.Abs(stats.Mean)) * 100
	}

	// --- Sparkline ---
	stats.Sparkline = generateSparkline(sortedData, numBins)

	return stats, nil
}

// generateSparkline creates a Unicode sparkline histogram from sorted data.
func generateSparkline(sortedData []float64, numBins int) string {
	n := len(sortedData)
	if n < 2 {
		return ""
	}
	minVal := sortedData[0]
	maxVal := sortedData[n-1]
	if minVal == maxVal {
		return ""
	}

	binWidth := (maxVal - minVal) / float64(numBins)
	bins := make([]int, numBins)

	for _, v := range sortedData {
		idx := int((v - minVal) / binWidth)
		if idx >= numBins {
			idx = numBins - 1
		}
		bins[idx]++
	}

	maxCount := 0
	for _, c := range bins {
		if c > maxCount {
			maxCount = c
		}
	}

	blocks := []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}
	runes := make([]rune, numBins)
	for i, c := range bins {
		if c == 0 {
			runes[i] = blocks[0]
		} else {
			level := (c * 7) / maxCount
			runes[i] = blocks[level]
		}
	}
	return string(runes)
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

// calculateKurtosis computes the sample excess kurtosis.
func calculateKurtosis(data []float64, mean, stdDev float64) float64 {
	n := float64(len(data))
	if n < 4 || stdDev == 0 {
		return 0
	}
	var sumOfFourthDeviations float64
	for _, v := range data {
		sumOfFourthDeviations += math.Pow((v-mean)/stdDev, 4)
	}
	// Excess kurtosis using the sample formula
	return (n*(n+1))/((n-1)*(n-2)*(n-3))*sumOfFourthDeviations - 3*(n-1)*(n-1)/((n-2)*(n-3))
}

// interpretKurtosis provides a human-readable label for a kurtosis value.
func interpretKurtosis(k float64) string {
	if k < -1 {
		return "Platykurtic - flat, thin tails"
	}
	if k <= 1 {
		return "Mesokurtic - normal-like"
	}
	return "Leptokurtic - peaked, heavy tails"
}

// interpretCV provides a human-readable label for a coefficient of variation value.
func interpretCV(cv float64) string {
	if cv < 15 {
		return "Low Variability"
	}
	if cv < 30 {
		return "Moderate Variability"
	}
	return "High Variability"
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

// padLabel pads a label to at least labelWidth characters, ensuring at least one trailing space.
func padLabel(label string, labelWidth int) string {
	padded := fmt.Sprintf("%-*s", labelWidth, label)
	if len(label) >= labelWidth {
		padded = label + " "
	}
	return padded
}

// printStats displays the results in a readable format.
func printStats(s *Stats, labelWidth int) {
	fmt.Println("--- Descriptive Statistics ---")
	fmt.Printf("%s%d\n", padLabel("Count:", labelWidth), s.Count)
	fmt.Printf("%s%s\n", padLabel("Sum:", labelWidth), formatFloat(s.Sum))
	fmt.Printf("%s%s\n", padLabel("Min:", labelWidth), formatFloat(s.Min))
	fmt.Printf("%s%s\n", padLabel("Max:", labelWidth), formatFloat(s.Max))
	fmt.Println("\n--- Measures of Central Tendency ---")
	fmt.Printf("%s%s\n", padLabel("Mean:", labelWidth), formatFloat(s.Mean))
	fmt.Printf("%s%s\n", padLabel("Median (p50):", labelWidth), formatFloat(s.Median))

	switch len(s.Mode) {
	case 0:
		fmt.Printf("%s%s\n", padLabel("Mode:", labelWidth), "None")
	case 1:
		// If there's only one mode, print it as a clean number.
		fmt.Printf("%s%s\n", padLabel("Mode:", labelWidth), formatFloat(s.Mode[0]))
	default:
		// If there are multiple modes, label it and print the slice.
		fmt.Printf("%s%s\n", padLabel("Mode (multi):", labelWidth), formatFloatSlice(s.Mode))
	}

	fmt.Println("\n--- Measures of Spread & Distribution ---")
	fmt.Printf("%s%s\n", padLabel("Std Deviation:", labelWidth), formatFloat(s.StdDev))
	fmt.Printf("%s%s\n", padLabel("Variance:", labelWidth), formatFloat(s.Variance))
	if !s.CVValid {
		fmt.Printf("%s%s\n", padLabel("CV:", labelWidth), "N/A - mean near zero")
	} else {
		cvStr := fmt.Sprintf("%s%% (%s)", formatFloat(s.CV), interpretCV(s.CV))
		if s.HasNegativeData {
			cvStr += " WARNING: data set contains negative data"
		}
		fmt.Printf("%s%s\n", padLabel("CV:", labelWidth), cvStr)
	}
	fmt.Printf("%s%s\n", padLabel("Quartile 1 (p25):", labelWidth), formatFloat(s.Q1))
	fmt.Printf("%s%s\n", padLabel("Quartile 3 (p75):", labelWidth), formatFloat(s.Q3))
	allPercentiles := map[float64]float64{95: s.P95, 99: s.P99}
	for k, v := range s.CustomPercentiles {
		allPercentiles[k] = v
	}
	pctKeys := make([]float64, 0, len(allPercentiles))
	for k := range allPercentiles {
		pctKeys = append(pctKeys, k)
	}
	sort.Float64s(pctKeys)
	for _, k := range pctKeys {
		label := fmt.Sprintf("Percentile (p%s):", formatFloat(k))
		fmt.Printf("%s%s\n", padLabel(label, labelWidth), formatFloat(allPercentiles[k]))
	}
	fmt.Printf("%s%s\n", padLabel("IQR:", labelWidth), formatFloat(s.IQR))
	fmt.Printf("%s%s (%s)\n", padLabel("Skewness:", labelWidth), formatFloat(s.Skewness), interpretSkewness(s.Skewness))
	fmt.Printf("%s%s (%s)\n", padLabel("Kurtosis:", labelWidth), formatFloat(s.Kurtosis), interpretKurtosis(s.Kurtosis))
	if len(s.Outliers) > 0 {
		fmt.Printf("%s%s\n", padLabel("Outliers:", labelWidth), formatFloatSlice(s.Outliers))
	} else {
		fmt.Printf("%s%s\n", padLabel("Outliers:", labelWidth), "None")
	}
	if s.Sparkline != "" {
		fmt.Printf("\n--- Distribution ---\n")
		fmt.Printf("%s%s\n", padLabel("Sparkline:", labelWidth), s.Sparkline)
	}
}
