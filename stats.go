// stats.go
package main

import (
	"bufio"
	"encoding/json"
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

const PgmVersion string = "1.14.0"

// madScale is the normal-consistency constant that makes the MAD directly
// comparable to the standard deviation for normally distributed data.
const madScale = 1.4826

// modZScale is the Iglewicz-Hoaglin constant for modified z-scores,
// approximately the reciprocal of madScale.
const modZScale = 0.6745

// ci95Z is the standard normal critical value for a two-sided 95% interval.
const ci95Z = 1.96

// baseDecimals is the number of decimal places used for values of magnitude 1
// or greater. Smaller magnitudes gain additional places so that they keep
// roughly the same number of significant digits instead of rounding to zero.
const baseDecimals = 4

// Stats holds the computed statistical results.
type Stats struct {
	Count             int                 `json:"count"`
	SkippedLines      int                 `json:"skippedLines"` // Input lines that failed to parse (blank lines excluded)
	Distinct          int                 `json:"distinct"`     // Number of distinct values
	Sum               float64             `json:"sum"`
	Mean              float64             `json:"mean"`
	StdErr            float64             `json:"stdErr"`        // Standard error of the mean (StdDev / sqrt(n))
	CI95Lower         float64             `json:"ci95Lower"`     // Lower bound of the 95% confidence interval for the mean
	CI95Upper         float64             `json:"ci95Upper"`     // Upper bound of the 95% confidence interval for the mean
	CIValid           bool                `json:"ciValid"`       // False when n < 2 or the standard error is zero
	GeometricMean     float64             `json:"geometricMean"` // Geometric mean; valid only when GeoMeanValid
	GeoMeanValid      bool                `json:"geoMeanValid"`  // False when any value is non-positive or under log transform
	Median            float64             `json:"median"`
	Mode              []float64           `json:"mode"`          // A dataset can have more than one mode
	ModalBinLow       float64             `json:"modalBinLow"`   // Lower edge of the densest histogram bin; valid only when ModalBinValid
	ModalBinHigh      float64             `json:"modalBinHigh"`  // Upper edge of the densest histogram bin
	ModalBinCount     int                 `json:"modalBinCount"` // Number of values in the densest bin
	ModalBinValid     bool                `json:"modalBinValid"` // True only when no value repeats and the data has spread
	Min               float64             `json:"min"`
	Max               float64             `json:"max"`
	StdDev            float64             `json:"stdDev"`   // Standard Deviation
	Variance          float64             `json:"variance"` // Variance = StdDev^2
	MAD               float64             `json:"mad"`      // Median absolute deviation, scaled by madScale
	Q1                float64             `json:"q1"`       // 1st Quartile (25th percentile)
	Q3                float64             `json:"q3"`       // 3rd Quartile (75th percentile)
	P95               float64             `json:"p95"`      // 95th percentile
	P99               float64             `json:"p99"`      // 99th percentile
	IQR               float64             `json:"iqr"`      // Interquartile Range (Q3 - Q1)
	IQRMultiplier     float64             `json:"iqrMultiplier"`
	Outliers          []float64           `json:"outliers"`
	ZScoreOutliers    []float64           `json:"zScoreOutliers"`  // Outliers detected via Z-score method; null when not computed
	ZScoreThreshold   float64             `json:"zScoreThreshold"` // Z-score threshold used (0 = disabled)
	ZScoreValid       bool                `json:"zScoreValid"`     // False when -z is given but the standard deviation is zero
	ModZOutliers      []float64           `json:"modZOutliers"`    // Outliers via modified z-score (Iglewicz-Hoaglin); null when not computed
	ModZValid         bool                `json:"modZValid"`       // False when -z is given but the MAD is zero
	Skewness          float64             `json:"skewness"`        // Formal skewness value
	Kurtosis          float64             `json:"kurtosis"`        // Excess kurtosis
	IsSymmetric       bool                `json:"isSymmetric"`     // Dataset is a mirror image of itself about SymmetryCenter
	SymmetryCenter    float64             `json:"symmetryCenter"`  // Center of reflection; valid only when IsSymmetric
	SymmetryPairs     int                 `json:"symmetryPairs"`   // Number of mirrored pairs (excludes an odd-n center element)
	CV                float64             `json:"cv"`              // Coefficient of Variation as a percentage
	HasNegativeData   bool                `json:"hasNegativeData"` // Flag for negative value warning
	CVValid           bool                `json:"cvValid"`         // False when mean is near zero
	CustomPercentiles map[float64]float64 `json:"-"`               // User-requested percentiles; see statsOutput
	Histogram         string              `json:"histogram"`       // Unicode histogram showing distribution
	Trendline         string              `json:"trendline"`       // Unicode trendline showing sequence pattern
	Autocorrelation   float64             `json:"autocorrelation"` // Lag-1 autocorrelation in input order; valid only when AutocorrValid
	AutocorrValid     bool                `json:"autocorrValid"`   // False when n < 3, zero variance, or dataset trimmed (-T)
	InputOrder        string              `json:"inputOrder"`      // ascending, descending, constant, or unordered; empty when suppressed
	TrimmedMean       float64             `json:"trimmedMean"`
	TrimmedMeanPct    float64             `json:"trimmedMeanPct"`   // 0 = disabled
	TrimDatasetPct    float64             `json:"trimDatasetPct"`   // 0 = disabled; trim dataset before all stats
	TrimDatasetOrigN  int                 `json:"trimDatasetOrigN"` // original count before dataset trimming
	EMA               float64             `json:"ema"`
	EMASpan           int                 `json:"emaSpan"` // 0 = disabled
}

// statsOutput is the JSON view of a Stats value. It embeds Stats so every field
// is emitted without duplication, and adds the run metadata plus the string-keyed
// percentile map that encoding/json requires.
type statsOutput struct {
	*Stats
	Version           string             `json:"version"`
	LogTransformed    bool               `json:"logTransformed"`
	DuplicatePct      float64            `json:"duplicatePct"`
	CustomPercentiles map[string]float64 `json:"customPercentiles,omitempty"`
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
	numBins := flag.Int("b", 16, "number of bins for histogram and trendline (5-50)")
	zScoreThreshold := flag.Float64("z", 0, "Z-score threshold for outlier detection (e.g., 2.0, 2.5, 3.0; disabled by default)")
	logTransform := flag.Bool("l", false, "apply natural log (ln) transform to input data")
	trimPct := flag.Float64("t", 0, "trimmed mean percentage to remove from each tail (0-50)")
	trimDatasetPct := flag.Float64("T", 0, "trim dataset: remove percentage from each tail before computing all statistics (0-50)")
	emaSpan := flag.Int("e", 0, "EMA span (number of periods) for exponential moving average (>= 2)")
	jsonOutput := flag.Bool("j", false, "emit results as JSON instead of formatted text")
	flag.Parse()

	// Handled before validation so the version is always reachable.
	if *version {
		fmt.Printf("%s version %s\n%s\n\n%s\n%s\n", PgmName, PgmVersion, PgmUrl, PgmDisclaimer, PgmSeeAlso)
		os.Exit(0)
	}

	if *numBins < 5 || *numBins > 50 {
		fmt.Fprintf(os.Stderr, "Error: number of bins must be between 5 and 50, got %d\n", *numBins)
		os.Exit(1)
	}

	if *iqrMultiplier <= 0 {
		fmt.Fprintf(os.Stderr, "Error: IQR multiplier must be greater than 0, got %v\n", *iqrMultiplier)
		os.Exit(1)
	}

	if *zScoreThreshold != 0 && *zScoreThreshold < 1.0 {
		fmt.Fprintf(os.Stderr, "Error: Z-score threshold must be >= 1.0, got %v\n", *zScoreThreshold)
		os.Exit(1)
	}

	if *trimPct < 0 || *trimPct > 50 {
		fmt.Fprintf(os.Stderr, "Error: trim percentage must be between 0 and 50, got %v\n", *trimPct)
		os.Exit(1)
	}

	if *trimDatasetPct < 0 || *trimDatasetPct > 50 {
		fmt.Fprintf(os.Stderr, "Error: trim dataset percentage must be between 0 and 50, got %v\n", *trimDatasetPct)
		os.Exit(1)
	}

	if *emaSpan != 0 && *emaSpan < 2 {
		fmt.Fprintf(os.Stderr, "Error: EMA span must be >= 2, got %d\n", *emaSpan)
		os.Exit(1)
	}

	if *trimPct > 0 && *trimDatasetPct > 0 {
		fmt.Fprintf(os.Stderr, "Error: -t and -T are mutually exclusive; use -t for trimmed mean only, or -T to trim the entire dataset\n")
		os.Exit(1)
	}

	if *emaSpan != 0 && *trimDatasetPct > 0 {
		fmt.Fprintf(os.Stderr, "Error: -e and -T are mutually exclusive; EMA is order-dependent and -T sorts the dataset\n")
		os.Exit(1)
	}

	args := flag.Args()
	// Go's flag package stops parsing at the first non-flag argument, so anything
	// after the filename would otherwise be accepted and silently ignored.
	if len(args) > 1 {
		fmt.Fprintf(os.Stderr, "Error: expected at most one input file, got %d arguments: %s\n", len(args), strings.Join(args, " "))
		fmt.Fprintf(os.Stderr, "Note: options must appear before the filename, e.g. %s -z 2.0 data.txt\n", PgmName)
		os.Exit(1)
	}

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

	numbers, skippedLines, err := readNumbers(reader)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading numbers: %v\n", err)
		os.Exit(1)
	}

	if *logTransform {
		numbers, err = applyLogTransform(numbers)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}

	originalCount := len(numbers)
	if *trimDatasetPct > 0 {
		sorted := make([]float64, len(numbers))
		copy(sorted, numbers)
		sort.Float64s(sorted)
		trimCount := int(math.Floor(float64(len(sorted)) * *trimDatasetPct / 100.0))
		remaining := len(sorted) - 2*trimCount
		if remaining < 1 {
			fmt.Fprintf(os.Stderr, "Error: dataset too small (%d values) to trim %.4g%% from each end\n", len(sorted), *trimDatasetPct)
			os.Exit(1)
		}
		numbers = sorted[trimCount : len(sorted)-trimCount]
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

	stats, err := computeStats(numbers, customPercentiles, *iqrMultiplier, *numBins, *zScoreThreshold, *trimPct, *emaSpan)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error computing stats: %v\n", err)
		os.Exit(1)
	}

	stats.SkippedLines = skippedLines

	if *trimDatasetPct > 0 {
		stats.TrimDatasetPct = *trimDatasetPct
		stats.TrimDatasetOrigN = originalCount
		// Order-aware statistics are artifacts of the -T sort, not trimmed statistics
		stats.Trendline = ""
		stats.AutocorrValid = false
		stats.InputOrder = ""
	}

	if *logTransform {
		// The arithmetic mean of logged values already is the log of the geometric mean
		stats.GeoMeanValid = false
	}

	if *jsonOutput {
		if err := printJSON(stats, *logTransform); err != nil {
			fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
			os.Exit(1)
		}
		return
	}

	labelWidth := 18 // one more than len("Quartile 1 (p25):")
	for _, p := range customPercentiles {
		label := fmt.Sprintf("Percentile (p%s):", formatFloat(p))
		if len(label) > labelWidth {
			labelWidth = len(label)
		}
	}
	if *zScoreThreshold > 0 {
		label := fmt.Sprintf("Z-Outliers (Z>%s):", formatFloat(*zScoreThreshold))
		if len(label) > labelWidth {
			labelWidth = len(label)
		}
		label = fmt.Sprintf("Mod Z-Outliers (Z>%s):", formatFloat(*zScoreThreshold))
		if len(label) > labelWidth {
			labelWidth = len(label)
		}
	}
	if *trimPct > 0 {
		label := fmt.Sprintf("Trimmed Mean (%s%%):", formatFloat(*trimPct))
		if len(label) > labelWidth {
			labelWidth = len(label)
		}
	}
	if *emaSpan > 0 {
		label := fmt.Sprintf("EMA (span %d):", *emaSpan)
		if len(label) > labelWidth {
			labelWidth = len(label)
		}
	}
	labelWidth++ // ensure padding via fmt.Sprintf, not the label+space fallback in padLabel
	if *logTransform {
		fmt.Println("(log-transformed, base e)")
		fmt.Println()
	}
	if *trimDatasetPct > 0 {
		fmt.Printf("(trimmed dataset: %s%% from each tail, %d → %d values)\n", formatFloat(*trimDatasetPct), originalCount, stats.Count)
		fmt.Println()
	}
	printStats(stats, labelWidth)
}

// printJSON writes the statistics to stdout as an indented JSON object.
// Outlier lists follow a two-state convention: an empty array means the detector
// ran and found nothing, while null means it never ran.
func printJSON(s *Stats, logTransformed bool) error {
	if s.Outliers == nil {
		s.Outliers = []float64{}
	}
	if s.ZScoreValid && s.ZScoreOutliers == nil {
		s.ZScoreOutliers = []float64{}
	}
	if s.ModZValid && s.ModZOutliers == nil {
		s.ModZOutliers = []float64{}
	}

	out := statsOutput{
		Stats:          s,
		Version:        PgmVersion,
		LogTransformed: logTransformed,
		DuplicatePct:   float64(s.Count-s.Distinct) / float64(s.Count) * 100,
	}
	if len(s.CustomPercentiles) > 0 {
		out.CustomPercentiles = make(map[string]float64, len(s.CustomPercentiles))
		for k, v := range s.CustomPercentiles {
			out.CustomPercentiles[formatFloat(k)] = v
		}
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(out)
}

// readNumbers reads floating-point numbers (one per line) from an io.Reader.
// It also returns the count of invalid lines that were skipped; blank lines
// are skipped silently and do not count toward that total. NaN and infinities
// parse successfully but are rejected, because a single one propagates through
// every downstream statistic.
func readNumbers(reader io.Reader) ([]float64, int, error) {
	var numbers []float64
	skipped := 0
	scanner := bufio.NewScanner(reader)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue // Skip empty lines
		}

		num, err := strconv.ParseFloat(line, 64)
		if err != nil || math.IsNaN(num) || math.IsInf(num, 0) {
			// Log invalid lines but continue processing
			fmt.Fprintf(
				os.Stderr,
				"Warning: skipping invalid number on line %d: '%s'\n",
				lineNum,
				scanner.Text(),
			)
			skipped++
			continue
		}
		numbers = append(numbers, num)
	}
	return numbers, skipped, scanner.Err()
}

// applyLogTransform applies natural log to all values, returning an error if any value is <= 0.
func applyLogTransform(numbers []float64) ([]float64, error) {
	result := make([]float64, len(numbers))
	for i, v := range numbers {
		if v <= 0 {
			return nil, fmt.Errorf("log transform requires all positive values, but got %v", v)
		}
		result[i] = math.Log(v)
	}
	return result, nil
}

// computeStats calculates all the desired statistics for a slice of numbers.
func computeStats(data []float64, customPercentiles []float64, iqrMultiplier float64, numBins int, zScoreThreshold float64, trimPct float64, emaSpan int) (*Stats, error) {
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
		Count:         count,
		Min:           sortedData[0],
		Max:           sortedData[count-1],
		IQRMultiplier: iqrMultiplier,
	}

	// --- Mean ---
	var sum float64
	for _, v := range data {
		sum += v
	}
	stats.Sum = sum
	stats.Mean = sum / float64(count)

	// --- Trimmed Mean ---
	if trimPct > 0 {
		trimCount := int(math.Floor(float64(count) * trimPct / 100.0))
		remaining := count - 2*trimCount
		if remaining < 1 {
			return nil, fmt.Errorf("dataset too small (%d values) to trim %.4g%% from each end", count, trimPct)
		}
		trimmed := sortedData[trimCount : count-trimCount]
		var trimSum float64
		for _, v := range trimmed {
			trimSum += v
		}
		stats.TrimmedMean = trimSum / float64(remaining)
		stats.TrimmedMeanPct = trimPct
	}

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

	// --- Standard Error of the Mean ---
	stats.StdErr = stats.StdDev / math.Sqrt(float64(count))

	// --- 95% Confidence Interval for the Mean (normal approximation) ---
	if count > 1 && stats.StdErr > 0 {
		stats.CI95Lower = stats.Mean - ci95Z*stats.StdErr
		stats.CI95Upper = stats.Mean + ci95Z*stats.StdErr
		stats.CIValid = true
	}

	// --- Geometric Mean ---
	if sortedData[0] > 0 {
		stats.GeometricMean = calculateGeometricMean(data)
		stats.GeoMeanValid = true
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

	// --- MAD (Median Absolute Deviation) ---
	stats.MAD = calculateMAD(sortedData, stats.Median)

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

	// --- Distinct Count ---
	stats.Distinct = len(freqs)

	// If the max frequency is 1, it means no number repeated, so there is no mode.
	if maxFreq <= 1 {
		stats.Mode = []float64{} // Return an empty slice
		// Exact repeats are vanishingly rare in continuous measurements, so fall
		// back to the densest histogram bin, which is what "most common" means there.
		low, high, binCount, ok := findModalBin(sortedData, numBins)
		if ok {
			stats.ModalBinLow = low
			stats.ModalBinHigh = high
			stats.ModalBinCount = binCount
			stats.ModalBinValid = true
		}
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

	// --- Z-Score and Modified Z-Score Outliers ---
	// The threshold is recorded whenever it was requested, so a degenerate
	// dataset reports "N/A" rather than silently dropping the whole section.
	if zScoreThreshold > 0 {
		stats.ZScoreThreshold = zScoreThreshold
		stats.ZScoreValid = stats.StdDev > 0
		stats.ModZValid = stats.MAD > 0

		if stats.ZScoreValid {
			for _, v := range data {
				z := math.Abs((v - stats.Mean) / stats.StdDev)
				if z > zScoreThreshold {
					stats.ZScoreOutliers = append(stats.ZScoreOutliers, v)
				}
			}
			sort.Float64s(stats.ZScoreOutliers)
		}

		// --- Modified Z-Score Outliers (Iglewicz-Hoaglin) ---
		if stats.ModZValid {
			rawMAD := stats.MAD / madScale // unscaled median absolute deviation
			for _, v := range data {
				m := math.Abs(modZScale * (v - stats.Median) / rawMAD)
				if m > zScoreThreshold {
					stats.ModZOutliers = append(stats.ModZOutliers, v)
				}
			}
			sort.Float64s(stats.ModZOutliers)
		}
	}

	// --- Skewness (formal calculation) ---
	stats.Skewness = calculateSkewness(data, stats.Mean, stats.StdDev)

	// --- Kurtosis (excess kurtosis) ---
	stats.Kurtosis = calculateKurtosis(data, stats.Mean, stats.StdDev)

	// --- Symmetry ---
	stats.IsSymmetric, stats.SymmetryCenter, stats.SymmetryPairs = detectSymmetry(sortedData)

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

	// --- EMA ---
	if emaSpan >= 2 {
		stats.EMA = calculateEMA(data, emaSpan)
		stats.EMASpan = emaSpan
	}

	// --- Histogram ---
	stats.Histogram = generateHistogram(sortedData, numBins)

	// --- Trendline ---
	stats.Trendline = generateTrendline(data, numBins)

	// --- Lag-1 Autocorrelation ---
	if count >= 3 && stats.StdDev > 0 {
		stats.Autocorrelation = calculateAutocorrelation(data, stats.Mean)
		stats.AutocorrValid = true
	}

	// --- Input Order Monotonicity ---
	if count >= 2 {
		stats.InputOrder = detectMonotonicity(data)
	}

	return stats, nil
}

// binData buckets sortedData into equal-width bins spanning [min, max] and returns
// the bin counts, the minimum value, and the bin width. numBins is capped at the
// number of values so bins can never outnumber observations. It returns a nil slice
// when the data has fewer than two values or no spread, in which case binning is
// meaningless.
func binData(sortedData []float64, numBins int) ([]int, float64, float64) {
	n := len(sortedData)
	if n < 2 {
		return nil, 0, 0
	}
	minVal := sortedData[0]
	maxVal := sortedData[n-1]
	if minVal == maxVal {
		return nil, 0, 0
	}
	if numBins > n {
		numBins = n
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
	return bins, minVal, binWidth
}

// generateHistogram creates a Unicode histogram from sorted data.
func generateHistogram(sortedData []float64, numBins int) string {
	bins, _, _ := binData(sortedData, numBins)
	if bins == nil {
		return ""
	}

	maxCount := 0
	for _, c := range bins {
		if c > maxCount {
			maxCount = c
		}
	}

	blocks := []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}
	runes := make([]rune, len(bins))
	for i, c := range bins {
		if c == 0 {
			runes[i] = blocks[0]
			continue
		}
		// Floor occupied bins one level above empty so a sparse bin is visually
		// distinct from a bin holding nothing at all.
		level := (c * 7) / maxCount
		if level == 0 {
			level = 1
		}
		runes[i] = blocks[level]
	}
	return string(runes)
}

// findModalBin returns the edges and count of the densest histogram bin. It is used
// in place of the mode for continuous data, where exact repeats almost never occur.
// The ok result is false when the data cannot be binned.
func findModalBin(sortedData []float64, numBins int) (low float64, high float64, count int, ok bool) {
	bins, minVal, binWidth := binData(sortedData, numBins)
	if bins == nil {
		return 0, 0, 0, false
	}
	best := 0
	for i, c := range bins {
		if c > bins[best] {
			best = i
		}
	}
	low = minVal + float64(best)*binWidth
	high = low + binWidth
	return low, high, bins[best], true
}

// generateTrendline creates a Unicode trendline from data in its original input order.
func generateTrendline(data []float64, numBins int) string {
	n := len(data)
	if n < 2 {
		return ""
	}

	minVal := data[0]
	maxVal := data[0]
	for _, v := range data {
		if v < minVal {
			minVal = v
		}
		if v > maxVal {
			maxVal = v
		}
	}
	if minVal == maxVal {
		return ""
	}

	// Cap numBins to data length
	if numBins > n {
		numBins = n
	}

	// Divide data into numBins equal chunks using floating-point boundaries and average each
	step := float64(n) / float64(numBins)
	averages := make([]float64, numBins)
	for i := 0; i < numBins; i++ {
		start := int(math.Round(float64(i) * step))
		end := int(math.Round(float64(i+1) * step))
		if end > n {
			end = n
		}
		if end <= start {
			end = start + 1
		}
		var sum float64
		for j := start; j < end; j++ {
			sum += data[j]
		}
		averages[i] = sum / float64(end-start)
	}

	blocks := []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}
	runes := make([]rune, numBins)
	for i, avg := range averages {
		normalized := (avg - minVal) / (maxVal - minVal)
		level := int(math.Round(normalized * 7))
		if level < 0 {
			level = 0
		}
		if level > 7 {
			level = 7
		}
		runes[i] = blocks[level]
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

// detectSymmetry reports whether sorted data is a mirror image of itself about a center value.
// Symmetry is not evaluated for fewer than 3 values; center and pairs are meaningful only when symmetric is true.
func detectSymmetry(sortedData []float64) (symmetric bool, center float64, pairs int) {
	n := len(sortedData)
	if n < 3 {
		return false, 0, 0
	}
	// If the data is symmetric at all, it is symmetric about the midpoint of min and max.
	c := (sortedData[0] + sortedData[n-1]) / 2
	// Scale-relative tolerance for float64 representation error in the pair sums.
	scale := math.Max(math.Abs(sortedData[0]), math.Abs(sortedData[n-1]))
	tol := 1e-9 * math.Max(1.0, scale)
	// The i <= n-1-i condition lets an odd-n middle element self-check against the center.
	for i := 0; i <= n-1-i; i++ {
		if math.Abs(sortedData[i]+sortedData[n-1-i]-2*c) > tol {
			return false, 0, 0
		}
	}
	return true, c, n / 2
}

// calculateEMA computes the final exponential moving average value for the given span.
// EMA uses the multiplier α = 2/(span+1), starting from the first data point.
func calculateEMA(data []float64, span int) float64 {
	alpha := 2.0 / (float64(span) + 1.0)
	ema := data[0]
	for i := 1; i < len(data); i++ {
		ema = alpha*data[i] + (1-alpha)*ema
	}
	return ema
}

// calculateMAD computes the median absolute deviation of sortedData about the given
// median, scaled by madScale so it is directly comparable to the standard deviation
// for normally distributed data. Divide the result by madScale to recover the raw
// (unscaled) median absolute deviation.
func calculateMAD(sortedData []float64, median float64) float64 {
	deviations := make([]float64, len(sortedData))
	for i, v := range sortedData {
		deviations[i] = math.Abs(v - median)
	}
	sort.Float64s(deviations)
	return madScale * calculatePercentile(deviations, 0.50)
}

// calculateGeometricMean computes the geometric mean as exp(mean(ln x)) to avoid
// the overflow risk of a direct product. The caller must ensure all values are positive.
func calculateGeometricMean(data []float64) float64 {
	var logSum float64
	for _, v := range data {
		logSum += math.Log(v)
	}
	return math.Exp(logSum / float64(len(data)))
}

// calculateAutocorrelation computes the lag-1 autocorrelation of data in its original
// input order. Returns 0 for fewer than 3 values or when the data has zero variance.
func calculateAutocorrelation(data []float64, mean float64) float64 {
	n := len(data)
	if n < 3 {
		return 0
	}
	var numerator, denominator float64
	for i, v := range data {
		d := v - mean
		denominator += d * d
		if i < n-1 {
			numerator += d * (data[i+1] - mean)
		}
	}
	if denominator == 0 {
		return 0
	}
	return numerator / denominator
}

// detectMonotonicity reports whether data arrived in ascending, descending, constant,
// or unordered input order. Ascending and descending are non-strict (ties allowed).
func detectMonotonicity(data []float64) string {
	nonDecreasing := true
	nonIncreasing := true
	for i := 1; i < len(data); i++ {
		if data[i] < data[i-1] {
			nonDecreasing = false
		}
		if data[i] > data[i-1] {
			nonIncreasing = false
		}
	}
	if nonDecreasing && nonIncreasing {
		return "constant"
	}
	if nonDecreasing {
		return "ascending"
	}
	if nonIncreasing {
		return "descending"
	}
	return "unordered"
}

// interpretAutocorrelation provides a human-readable label for a lag-1 autocorrelation value.
func interpretAutocorrelation(r float64) string {
	absR := math.Abs(r)
	if absR < 0.2 {
		return "no serial dependence"
	}
	direction := "positive"
	if r < 0 {
		direction = "negative"
	}
	if absR < 0.5 {
		return "weak " + direction + " serial dependence"
	}
	if absR < 0.8 {
		return "moderate " + direction + " serial dependence"
	}
	return "strong " + direction + " serial dependence"
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

// decimalsFor reports how many decimal places formatFloat should use for a value.
// Magnitudes of 1 or greater use baseDecimals; smaller magnitudes gain one place per
// leading zero so that a value like 0.00004 keeps its significant digits instead of
// rounding away to 0.
func decimalsFor(v float64) int {
	abs := math.Abs(v)
	if abs == 0 || abs >= 1 || math.IsNaN(abs) || math.IsInf(abs, 0) {
		return baseDecimals
	}
	return baseDecimals - int(math.Floor(math.Log10(abs))) - 1
}

// formatFloat formats a float64 without scientific notation, trimming unnecessary
// trailing zeros. The decimal width adapts to the magnitude of the value, so small
// numbers survive formatting rather than collapsing to 0.
func formatFloat(v float64) string {
	if v == math.Trunc(v) {
		return strconv.FormatFloat(v, 'f', 0, 64)
	}
	s := strconv.FormatFloat(v, 'f', decimalsFor(v), 64)
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

// formatMode renders the mode line value, falling back to the densest histogram bin
// when no value repeats.
func formatMode(s *Stats) string {
	switch {
	case len(s.Mode) == 1:
		return formatFloat(s.Mode[0])
	case len(s.Mode) > 1:
		return formatFloatSlice(s.Mode)
	case s.ModalBinValid:
		valueWord := "values"
		if s.ModalBinCount == 1 {
			valueWord = "value"
		}
		return fmt.Sprintf("None (modal bin: %s-%s, %d %s)",
			formatFloat(s.ModalBinLow), formatFloat(s.ModalBinHigh), s.ModalBinCount, valueWord)
	default:
		return "None"
	}
}

// printStats displays the results in a readable format.
func printStats(s *Stats, labelWidth int) {
	fmt.Println("--- Descriptive Statistics ---")
	fmt.Printf("%s%d\n", padLabel("Count:", labelWidth), s.Count)
	if s.SkippedLines > 0 {
		lineWord := "lines"
		if s.SkippedLines == 1 {
			lineWord = "line"
		}
		fmt.Printf("%s%d invalid %s\n", padLabel("Skipped:", labelWidth), s.SkippedLines, lineWord)
	}
	dupPct := float64(s.Count-s.Distinct) / float64(s.Count) * 100
	fmt.Printf("%s%d of %d (%s%% duplicated)\n", padLabel("Distinct:", labelWidth), s.Distinct, s.Count, formatFloat(dupPct))
	fmt.Printf("%s%s\n", padLabel("Sum:", labelWidth), formatFloat(s.Sum))
	fmt.Printf("%s%s\n", padLabel("Min:", labelWidth), formatFloat(s.Min))
	fmt.Printf("%s%s\n", padLabel("Max:", labelWidth), formatFloat(s.Max))
	fmt.Println("\n--- Measures of Central Tendency ---")
	fmt.Printf("%s%s\n", padLabel("Mean:", labelWidth), formatFloat(s.Mean))
	fmt.Printf("%s%s\n", padLabel("Std Error:", labelWidth), formatFloat(s.StdErr))
	if s.CIValid {
		ci := fmt.Sprintf("[%s, %s]", formatFloat(s.CI95Lower), formatFloat(s.CI95Upper))
		fmt.Printf("%s%s\n", padLabel("95% CI (mean):", labelWidth), ci)
	}
	if s.GeoMeanValid {
		fmt.Printf("%s%s\n", padLabel("Geometric Mean:", labelWidth), formatFloat(s.GeometricMean))
	}
	if s.TrimmedMeanPct > 0 {
		label := fmt.Sprintf("Trimmed Mean (%s%%):", formatFloat(s.TrimmedMeanPct))
		fmt.Printf("%s%s\n", padLabel(label, labelWidth), formatFloat(s.TrimmedMean))
	}
	if s.EMASpan > 0 {
		label := fmt.Sprintf("EMA (span %d):", s.EMASpan)
		fmt.Printf("%s%s\n", padLabel(label, labelWidth), formatFloat(s.EMA))
	}
	fmt.Printf("%s%s\n", padLabel("Median (p50):", labelWidth), formatFloat(s.Median))

	modeLabel := "Mode:"
	if len(s.Mode) > 1 {
		modeLabel = "Mode (multi):"
	}
	fmt.Printf("%s%s\n", padLabel(modeLabel, labelWidth), formatMode(s))

	fmt.Println("\n--- Measures of Spread & Distribution ---")
	fmt.Printf("%s%s\n", padLabel("Std Deviation:", labelWidth), formatFloat(s.StdDev))
	fmt.Printf("%s%s\n", padLabel("Variance:", labelWidth), formatFloat(s.Variance))
	fmt.Printf("%s%s\n", padLabel("MAD:", labelWidth), formatFloat(s.MAD))
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
	symmetryStr := "N/A - requires at least 3 values"
	if s.Count >= 3 {
		if s.IsSymmetric {
			pairWord := "pairs"
			if s.SymmetryPairs == 1 {
				pairWord = "pair"
			}
			symmetryStr = fmt.Sprintf("Symmetric about %s (%d %s", formatFloat(s.SymmetryCenter), s.SymmetryPairs, pairWord)
			if s.Count%2 == 1 {
				symmetryStr += " + center value"
			}
			symmetryStr += ")"
		} else {
			symmetryStr = "None"
		}
	}
	fmt.Printf("%s%s\n", padLabel("Symmetry:", labelWidth), symmetryStr)
	if len(s.Outliers) > 0 {
		fmt.Printf("%s%s\n", padLabel("Outliers:", labelWidth), formatFloatSlice(s.Outliers))
	} else {
		fmt.Printf("%s%s\n", padLabel("Outliers:", labelWidth), "None")
	}
	if s.ZScoreThreshold > 0 {
		label := fmt.Sprintf("Z-Outliers (Z>%s):", formatFloat(s.ZScoreThreshold))
		switch {
		case !s.ZScoreValid:
			fmt.Printf("%s%s\n", padLabel(label, labelWidth), "N/A - standard deviation is zero")
		case len(s.ZScoreOutliers) > 0:
			fmt.Printf("%s%s\n", padLabel(label, labelWidth), formatFloatSlice(s.ZScoreOutliers))
		default:
			fmt.Printf("%s%s\n", padLabel(label, labelWidth), "None")
		}
		label = fmt.Sprintf("Mod Z-Outliers (Z>%s):", formatFloat(s.ZScoreThreshold))
		switch {
		case !s.ModZValid:
			fmt.Printf("%s%s\n", padLabel(label, labelWidth), "N/A - MAD is zero")
		case len(s.ModZOutliers) > 0:
			fmt.Printf("%s%s\n", padLabel(label, labelWidth), formatFloatSlice(s.ModZOutliers))
		default:
			fmt.Printf("%s%s\n", padLabel(label, labelWidth), "None")
		}
	}
	if s.Histogram != "" || s.Trendline != "" || s.AutocorrValid || s.InputOrder != "" {
		fmt.Printf("\n--- Distribution ---\n")
		if s.Histogram != "" {
			fmt.Printf("%s%s\n", padLabel("Histogram:", labelWidth), s.Histogram)
		}
		if s.Trendline != "" {
			fmt.Printf("%s%s\n", padLabel("Trendline:", labelWidth), s.Trendline)
		}
		if s.AutocorrValid {
			fmt.Printf("%s%s (%s)\n", padLabel("Autocorrelation:", labelWidth), formatFloat(s.Autocorrelation), interpretAutocorrelation(s.Autocorrelation))
		}
		if s.InputOrder != "" {
			orderStr := s.InputOrder
			if s.InputOrder == "ascending" || s.InputOrder == "descending" {
				// EMA is order-dependent too, so name it whenever it is on display.
				affected := "trendline and autocorrelation"
				if s.EMASpan > 0 {
					affected = "trendline, autocorrelation, and EMA"
				}
				orderStr += " WARNING: " + affected + " reflect this sort order, not a property of the data"
			}
			fmt.Printf("%s%s\n", padLabel("Input Order:", labelWidth), orderStr)
		}
	}
	if s.TrimDatasetPct > 0 {
		fmt.Println("\n* all statistics above are computed on the trimmed dataset; compare against the full-data output")
	}
}
