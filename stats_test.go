package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const epsilon = 1e-4

// statsBin is the path to the binary compiled once for the CLI-level tests. Building
// the whole package (rather than "go run stats.go") keeps these tests working if the
// program is ever split across multiple files.
var statsBin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "stats-cli")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}
	statsBin = filepath.Join(dir, "stats")
	if out, err := exec.Command("go", "build", "-o", statsBin, ".").CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build test binary: %v\n%s", err, out)
		os.RemoveAll(dir)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// runStats executes the compiled binary and returns its combined output.
func runStats(args ...string) (string, error) {
	out, err := exec.Command(statsBin, args...).CombinedOutput()
	return string(out), err
}

func floatEquals(a, b float64) bool {
	return math.Abs(a-b) < epsilon
}

func floatSliceEquals(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !floatEquals(a[i], b[i]) {
			return false
		}
	}
	return true
}

// Test dataset with 31 numbers:
// - Contains mode (50 appears 4 times)
// - Mix of whole numbers, floats, and numbers ending in .0/.00
// - Contains an outlier (150)
var testData = []float64{
	5, 10, 15.5, 20, 25.00, 30, 35.0, 40, 45, 50,
	55, 60, 65, 70, 75.25, 80, 85, 90, 95, 100,
	12.5, 37.5, 62.50, 87.5, 50, 50, 50, 3, 150, 7.75, 42.0,
}

// Symmetric test dataset with 40 numbers:
// - 20 disjoint pairs, each summing to 1000 (symmetric about 500)
// - All values distinct; 500 itself is deliberately absent
// - Order is scrambled so the property is not visible positionally
var symmetricTestData = []float64{
	612, 137, 827, 495, 958, 264, 709, 19, 882, 455,
	349, 736, 88, 981, 545, 233, 61, 694, 42, 767,
	388, 118, 912, 651, 306, 994, 173, 588, 505, 377,
	863, 220, 939, 6, 623, 780, 151, 849, 291, 412,
}

func TestComputeStats(t *testing.T) {
	stats, err := computeStats(testData, nil, 1.5, 16, 0, 0, 0)
	if err != nil {
		t.Fatalf("computeStats returned error: %v", err)
	}

	tests := []struct {
		name     string
		got      float64
		expected float64
	}{
		{"Count", float64(stats.Count), 31},
		{"Sum", stats.Sum, 1603.5},
		{"Min", stats.Min, 3},
		{"Max", stats.Max, 150},
		{"Mean", stats.Mean, 51.7258},
		{"Median", stats.Median, 50},
		{"StdDev", stats.StdDev, 33.5751},
		{"Variance", stats.Variance, 1127.2848},
		{"Q1", stats.Q1, 27.5},
		{"Q3", stats.Q3, 72.625},
		{"P95", stats.P95, 97.5},
		{"P99", stats.P99, 135},
		{"IQR", stats.IQR, 45.125},
		{"Skewness", stats.Skewness, 0.7271},
		{"Kurtosis", stats.Kurtosis, 0.8884},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if !floatEquals(tc.got, tc.expected) {
				t.Errorf("%s: got %v, expected %v", tc.name, tc.got, tc.expected)
			}
		})
	}

	// Test Mode separately (should be [50])
	t.Run("Mode", func(t *testing.T) {
		expectedMode := []float64{50}
		if !floatSliceEquals(stats.Mode, expectedMode) {
			t.Errorf("Mode: got %v, expected %v", stats.Mode, expectedMode)
		}
	})

	// Test Outliers separately (should be [150])
	t.Run("Outliers", func(t *testing.T) {
		expectedOutliers := []float64{150}
		if !floatSliceEquals(stats.Outliers, expectedOutliers) {
			t.Errorf("Outliers: got %v, expected %v", stats.Outliers, expectedOutliers)
		}
	})
}

func TestComputeStatsEmptyInput(t *testing.T) {
	_, err := computeStats([]float64{}, nil, 1.5, 16, 0, 0, 0)
	if err == nil {
		t.Error("expected error for empty input, got nil")
	}
}

func TestComputeStatsSingleValue(t *testing.T) {
	stats, err := computeStats([]float64{42.5}, nil, 1.5, 16, 0, 0, 0)
	if err != nil {
		t.Fatalf("computeStats returned error: %v", err)
	}

	if stats.Count != 1 {
		t.Errorf("Count: got %d, expected 1", stats.Count)
	}
	if !floatEquals(stats.Mean, 42.5) {
		t.Errorf("Mean: got %v, expected 42.5", stats.Mean)
	}
	if !floatEquals(stats.Median, 42.5) {
		t.Errorf("Median: got %v, expected 42.5", stats.Median)
	}
	if !floatEquals(stats.Min, 42.5) {
		t.Errorf("Min: got %v, expected 42.5", stats.Min)
	}
	if !floatEquals(stats.Max, 42.5) {
		t.Errorf("Max: got %v, expected 42.5", stats.Max)
	}
	// StdDev and Variance should be 0 for single value
	if !floatEquals(stats.StdDev, 0) {
		t.Errorf("StdDev: got %v, expected 0", stats.StdDev)
	}
	if !floatEquals(stats.Variance, 0) {
		t.Errorf("Variance: got %v, expected 0", stats.Variance)
	}
	// A confidence interval needs at least two observations
	if stats.CIValid {
		t.Error("CIValid: got true, expected false for a single value")
	}
}

func TestComputeStatsMultipleMode(t *testing.T) {
	// 5 and 10 both appear twice
	data := []float64{5, 5, 10, 10, 15}
	stats, err := computeStats(data, nil, 1.5, 16, 0, 0, 0)
	if err != nil {
		t.Fatalf("computeStats returned error: %v", err)
	}

	expectedMode := []float64{5, 10}
	if !floatSliceEquals(stats.Mode, expectedMode) {
		t.Errorf("Mode: got %v, expected %v", stats.Mode, expectedMode)
	}
}

func TestComputeStatsNoMode(t *testing.T) {
	// All values unique - no mode
	data := []float64{1, 2, 3, 4, 5}
	stats, err := computeStats(data, nil, 1.5, 16, 0, 0, 0)
	if err != nil {
		t.Fatalf("computeStats returned error: %v", err)
	}

	if len(stats.Mode) != 0 {
		t.Errorf("Mode: got %v, expected empty slice", stats.Mode)
	}
}

func TestCalculatePercentile(t *testing.T) {
	// Simple sorted dataset for easy manual verification
	sortedData := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}

	tests := []struct {
		name       string
		percentile float64
		expected   float64
	}{
		{"Minimum (p0)", 0.0, 1},
		{"Q1 (p25)", 0.25, 3.25},
		{"Median (p50)", 0.50, 5.5},
		{"Q3 (p75)", 0.75, 7.75},
		{"Maximum (p100)", 1.0, 10},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := calculatePercentile(sortedData, tc.percentile)
			if !floatEquals(got, tc.expected) {
				t.Errorf("calculatePercentile(%v): got %v, expected %v", tc.percentile, got, tc.expected)
			}
		})
	}
}

func TestCalculatePercentileSingleElement(t *testing.T) {
	sortedData := []float64{42}
	got := calculatePercentile(sortedData, 0.5)
	if !floatEquals(got, 42) {
		t.Errorf("calculatePercentile with single element: got %v, expected 42", got)
	}
}

func TestCalculatePercentileEmpty(t *testing.T) {
	sortedData := []float64{}
	got := calculatePercentile(sortedData, 0.5)
	if !floatEquals(got, 0) {
		t.Errorf("calculatePercentile with empty data: got %v, expected 0", got)
	}
}

func TestCalculateSkewness(t *testing.T) {
	tests := []struct {
		name     string
		data     []float64
		mean     float64
		stdDev   float64
		expected float64
	}{
		{
			name:     "Right skewed data",
			data:     testData,
			mean:     51.7258,
			stdDev:   33.5751,
			expected: 0.7271,
		},
		{
			name:     "Symmetric data",
			data:     []float64{1, 2, 3, 4, 5, 6, 7, 8, 9},
			mean:     5,
			stdDev:   2.7386,
			expected: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := calculateSkewness(tc.data, tc.mean, tc.stdDev)
			if !floatEquals(got, tc.expected) {
				t.Errorf("calculateSkewness: got %v, expected %v", got, tc.expected)
			}
		})
	}
}

func TestCalculateSkewnessEdgeCases(t *testing.T) {
	// Less than 3 data points - should return 0
	t.Run("TwoElements", func(t *testing.T) {
		got := calculateSkewness([]float64{1, 2}, 1.5, 0.5)
		if got != 0 {
			t.Errorf("expected 0 for less than 3 elements, got %v", got)
		}
	})

	// Zero standard deviation - should return 0
	t.Run("ZeroStdDev", func(t *testing.T) {
		got := calculateSkewness([]float64{5, 5, 5}, 5, 0)
		if got != 0 {
			t.Errorf("expected 0 for zero std dev, got %v", got)
		}
	})
}

func TestCalculateKurtosis(t *testing.T) {
	tests := []struct {
		name     string
		data     []float64
		mean     float64
		stdDev   float64
		expected float64
	}{
		{
			name:     "Right skewed data",
			data:     testData,
			mean:     51.7258,
			stdDev:   33.5751,
			expected: 0.8884,
		},
		{
			name:     "Symmetric data",
			data:     []float64{1, 2, 3, 4, 5, 6, 7, 8, 9},
			mean:     5,
			stdDev:   2.7386,
			expected: -1.2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := calculateKurtosis(tc.data, tc.mean, tc.stdDev)
			if !floatEquals(got, tc.expected) {
				t.Errorf("calculateKurtosis: got %v, expected %v", got, tc.expected)
			}
		})
	}
}

func TestCalculateKurtosisEdgeCases(t *testing.T) {
	// Less than 4 data points - should return 0
	t.Run("ThreeElements", func(t *testing.T) {
		got := calculateKurtosis([]float64{1, 2, 3}, 2, 1)
		if got != 0 {
			t.Errorf("expected 0 for less than 4 elements, got %v", got)
		}
	})

	// Zero standard deviation - should return 0
	t.Run("ZeroStdDev", func(t *testing.T) {
		got := calculateKurtosis([]float64{5, 5, 5, 5}, 5, 0)
		if got != 0 {
			t.Errorf("expected 0 for zero std dev, got %v", got)
		}
	})
}

func TestInterpretKurtosis(t *testing.T) {
	tests := []struct {
		kurtosis float64
		expected string
	}{
		{-2, "Platykurtic - flat, thin tails"},
		{0, "Mesokurtic - normal-like"},
		{1, "Mesokurtic - normal-like"},
		{2, "Leptokurtic - peaked, heavy tails"},
	}
	for _, tc := range tests {
		got := interpretKurtosis(tc.kurtosis)
		if got != tc.expected {
			t.Errorf("interpretKurtosis(%v): got %q, expected %q", tc.kurtosis, got, tc.expected)
		}
	}
}

func TestReadNumbers(t *testing.T) {
	input := `10
20.5
30.00

invalid
40
`
	reader := strings.NewReader(input)
	numbers, skipped, err := readNumbers(reader)
	if err != nil {
		t.Fatalf("readNumbers returned error: %v", err)
	}

	expected := []float64{10, 20.5, 30.00, 40}
	if !floatSliceEquals(numbers, expected) {
		t.Errorf("readNumbers: got %v, expected %v", numbers, expected)
	}
	// One unparseable line ("invalid"); the blank line must not count
	if skipped != 1 {
		t.Errorf("skipped: got %d, expected 1", skipped)
	}
}

func TestComputeStatsCustomIQRMultiplier(t *testing.T) {
	// With k=3.0 (extreme outliers only), 150 should no longer be an outlier
	// Q1=27.5, Q3=72.625, IQR=45.125
	// lowerBound = 27.5 - 3.0*45.125 = -108.875
	// upperBound = 72.625 + 3.0*45.125 = 208.0
	// 150 < 208.0, so no outliers
	stats, err := computeStats(testData, nil, 3.0, 16, 0, 0, 0)
	if err != nil {
		t.Fatalf("computeStats returned error: %v", err)
	}
	if len(stats.Outliers) != 0 {
		t.Errorf("Outliers with k=3.0: got %v, expected none", stats.Outliers)
	}

	// With k=1.0 (narrower), more values should be flagged
	// lowerBound = 27.5 - 1.0*45.125 = -17.625
	// upperBound = 72.625 + 1.0*45.125 = 117.75
	// 150 > 117.75, so 150 is an outlier (same as default for this dataset)
	stats, err = computeStats(testData, nil, 1.0, 16, 0, 0, 0)
	if err != nil {
		t.Fatalf("computeStats returned error: %v", err)
	}
	if len(stats.Outliers) != 1 || !floatEquals(stats.Outliers[0], 150) {
		t.Errorf("Outliers with k=1.0: got %v, expected [150]", stats.Outliers)
	}
}

func TestCVForTestData(t *testing.T) {
	stats, err := computeStats(testData, nil, 1.5, 16, 0, 0, 0)
	if err != nil {
		t.Fatalf("computeStats returned error: %v", err)
	}
	// Mean=51.7258, StdDev=33.5751 → CV≈64.91%
	expectedCV := 64.9097
	if !floatEquals(stats.CV, expectedCV) {
		t.Errorf("CV: got %v, expected %v", stats.CV, expectedCV)
	}
	if !stats.CVValid {
		t.Error("CVValid: got false, expected true")
	}
	if stats.HasNegativeData {
		t.Error("HasNegativeData: got true, expected false")
	}
}

func TestInterpretCV(t *testing.T) {
	tests := []struct {
		cv       float64
		expected string
	}{
		{10, "Low Variability"},
		{20, "Moderate Variability"},
		{50, "High Variability"},
	}
	for _, tc := range tests {
		got := interpretCV(tc.cv)
		if got != tc.expected {
			t.Errorf("interpretCV(%v): got %q, expected %q", tc.cv, got, tc.expected)
		}
	}
}

func TestCVWithNegativeData(t *testing.T) {
	data := []float64{-10, -5, 0, 5, 10, 20, 30}
	stats, err := computeStats(data, nil, 1.5, 16, 0, 0, 0)
	if err != nil {
		t.Fatalf("computeStats returned error: %v", err)
	}
	if !stats.HasNegativeData {
		t.Error("HasNegativeData: got false, expected true")
	}
}

func TestCVWithMeanNearZero(t *testing.T) {
	data := []float64{-1, 0, 1}
	stats, err := computeStats(data, nil, 1.5, 16, 0, 0, 0)
	if err != nil {
		t.Fatalf("computeStats returned error: %v", err)
	}
	if stats.CVValid {
		t.Error("CVValid: got true, expected false")
	}
}

func TestCVSingleValue(t *testing.T) {
	stats, err := computeStats([]float64{42.5}, nil, 1.5, 16, 0, 0, 0)
	if err != nil {
		t.Fatalf("computeStats returned error: %v", err)
	}
	// StdDev=0, Mean=42.5 → CV=0%
	if !stats.CVValid {
		t.Error("CVValid: got false, expected true")
	}
	if !floatEquals(stats.CV, 0) {
		t.Errorf("CV: got %v, expected 0", stats.CV)
	}
}

func TestReadNumbersEmpty(t *testing.T) {
	reader := strings.NewReader("")
	numbers, skipped, err := readNumbers(reader)
	if err != nil {
		t.Fatalf("readNumbers returned error: %v", err)
	}
	if len(numbers) != 0 {
		t.Errorf("expected empty slice, got %v", numbers)
	}
	if skipped != 0 {
		t.Errorf("skipped: got %d, expected 0", skipped)
	}
}

func TestGenerateHistogram(t *testing.T) {
	sorted := make([]float64, len(testData))
	copy(sorted, testData)
	sort.Float64s(sorted)
	result := generateHistogram(sorted, 16)
	if len([]rune(result)) != 16 {
		t.Errorf("expected 16 runes, got %d", len([]rune(result)))
	}
	blocks := "▁▂▃▄▅▆▇█"
	for _, r := range result {
		if !strings.ContainsRune(blocks, r) {
			t.Errorf("invalid histogram character: %c", r)
		}
	}
}

func TestGenerateHistogramUniform(t *testing.T) {
	data := make([]float64, 16)
	for i := range data {
		data[i] = float64(i + 1)
	}
	result := generateHistogram(data, 16)
	expected := "████████████████"
	if result != expected {
		t.Errorf("expected all full blocks, got %q", result)
	}
}

func TestGenerateHistogramSingleValue(t *testing.T) {
	result := generateHistogram([]float64{42}, 16)
	if result != "" {
		t.Errorf("expected empty string for single value, got %q", result)
	}
}

func TestGenerateHistogramAllIdentical(t *testing.T) {
	result := generateHistogram([]float64{5, 5, 5, 5}, 16)
	if result != "" {
		t.Errorf("expected empty string for identical values, got %q", result)
	}
}

func TestGenerateHistogramCustomBins(t *testing.T) {
	sorted := make([]float64, len(testData))
	copy(sorted, testData)
	sort.Float64s(sorted)
	result := generateHistogram(sorted, 8)
	if len([]rune(result)) != 8 {
		t.Errorf("expected 8 runes, got %d", len([]rune(result)))
	}
}

func TestGenerateTrendline(t *testing.T) {
	result := generateTrendline(testData, 16)
	if len([]rune(result)) != 16 {
		t.Errorf("expected 16 runes, got %d", len([]rune(result)))
	}
	blocks := "▁▂▃▄▅▆▇█"
	for _, r := range result {
		if !strings.ContainsRune(blocks, r) {
			t.Errorf("invalid trendline character: %c", r)
		}
	}
}

func TestGenerateTrendlinePreservesOrder(t *testing.T) {
	// Ascending input should produce ascending blocks
	data := []float64{1, 2, 3, 4, 5, 6, 7, 8}
	result := generateTrendline(data, 8)
	runes := []rune(result)
	for i := 1; i < len(runes); i++ {
		if runes[i] < runes[i-1] {
			t.Errorf("expected ascending trendline, but position %d (%c) < position %d (%c)", i, runes[i], i-1, runes[i-1])
		}
	}
}

func TestGenerateTrendlineSingleValue(t *testing.T) {
	result := generateTrendline([]float64{42}, 16)
	if result != "" {
		t.Errorf("expected empty string for single value, got %q", result)
	}
}

func TestGenerateTrendlineAllIdentical(t *testing.T) {
	result := generateTrendline([]float64{5, 5, 5, 5}, 16)
	if result != "" {
		t.Errorf("expected empty string for identical values, got %q", result)
	}
}

func TestGenerateTrendlineCustomBins(t *testing.T) {
	result := generateTrendline(testData, 8)
	if len([]rune(result)) != 8 {
		t.Errorf("expected 8 runes, got %d", len([]rune(result)))
	}
}

func TestZScoreOutliers(t *testing.T) {
	// With z=2.0: 150 has Z=(150-51.7258)/33.5751=2.926 > 2.0, so flagged
	t.Run("Threshold2.0", func(t *testing.T) {
		stats, err := computeStats(testData, nil, 1.5, 16, 2.0, 0, 0)
		if err != nil {
			t.Fatalf("computeStats returned error: %v", err)
		}
		if !floatEquals(stats.ZScoreThreshold, 2.0) {
			t.Errorf("ZScoreThreshold: got %v, expected 2.0", stats.ZScoreThreshold)
		}
		expectedOutliers := []float64{150}
		if !floatSliceEquals(stats.ZScoreOutliers, expectedOutliers) {
			t.Errorf("ZScoreOutliers: got %v, expected %v", stats.ZScoreOutliers, expectedOutliers)
		}
	})

	// With z=3.0: 150 has Z=2.926 < 3.0, so no outliers
	t.Run("Threshold3.0", func(t *testing.T) {
		stats, err := computeStats(testData, nil, 1.5, 16, 3.0, 0, 0)
		if err != nil {
			t.Fatalf("computeStats returned error: %v", err)
		}
		if len(stats.ZScoreOutliers) != 0 {
			t.Errorf("ZScoreOutliers with z=3.0: got %v, expected none", stats.ZScoreOutliers)
		}
	})
}

func TestZScoreDisabled(t *testing.T) {
	stats, err := computeStats(testData, nil, 1.5, 16, 0, 0, 0)
	if err != nil {
		t.Fatalf("computeStats returned error: %v", err)
	}
	if stats.ZScoreOutliers != nil {
		t.Errorf("ZScoreOutliers: got %v, expected nil", stats.ZScoreOutliers)
	}
	if stats.ZScoreThreshold != 0 {
		t.Errorf("ZScoreThreshold: got %v, expected 0", stats.ZScoreThreshold)
	}
}

func TestZScoreZeroStdDev(t *testing.T) {
	stats, err := computeStats([]float64{5, 5, 5}, nil, 1.5, 16, 2.0, 0, 0)
	if err != nil {
		t.Fatalf("computeStats returned error: %v", err)
	}
	if stats.ZScoreOutliers != nil {
		t.Errorf("ZScoreOutliers with zero stddev: got %v, expected nil", stats.ZScoreOutliers)
	}
}

func TestApplyLogTransformPositiveValues(t *testing.T) {
	data := []float64{1, 10, 100, 1000}
	result, err := applyLogTransform(data)
	if err != nil {
		t.Fatalf("applyLogTransform returned error: %v", err)
	}
	// ln(1)=0, ln(10)=2.302585, ln(100)=4.605170, ln(1000)=6.907755
	expected := []float64{0, 2.302585, 4.605170, 6.907755}
	if !floatSliceEquals(result, expected) {
		t.Errorf("applyLogTransform: got %v, expected %v", result, expected)
	}

	// Verify stats on transformed data
	stats, err := computeStats(result, nil, 1.5, 16, 0, 0, 0)
	if err != nil {
		t.Fatalf("computeStats returned error: %v", err)
	}
	// Mean of ln values: (0 + 2.302585 + 4.605170 + 6.907755) / 4 = 3.4539
	if !floatEquals(stats.Mean, 3.4539) {
		t.Errorf("Mean of log-transformed data: got %v, expected 3.4539", stats.Mean)
	}
	if stats.Count != 4 {
		t.Errorf("Count: got %d, expected 4", stats.Count)
	}
}

func TestApplyLogTransformErrorOnZero(t *testing.T) {
	data := []float64{1, 2, 0, 4}
	_, err := applyLogTransform(data)
	if err == nil {
		t.Error("expected error for zero value, got nil")
	}
}

func TestApplyLogTransformErrorOnNegative(t *testing.T) {
	data := []float64{1, 2, -5, 4}
	_, err := applyLogTransform(data)
	if err == nil {
		t.Error("expected error for negative value, got nil")
	}
}

func TestTrimmedMean(t *testing.T) {
	// testData has 31 values, trim=10%
	// trimCount = floor(31 * 10 / 100) = 3, remaining = 25
	// sorted[3:28] sum = 1242.75, mean = 49.71
	stats, err := computeStats(testData, nil, 1.5, 16, 0, 10, 0)
	if err != nil {
		t.Fatalf("computeStats returned error: %v", err)
	}
	if !floatEquals(stats.TrimmedMean, 49.71) {
		t.Errorf("TrimmedMean: got %v, expected 49.71", stats.TrimmedMean)
	}
	if !floatEquals(stats.TrimmedMeanPct, 10) {
		t.Errorf("TrimmedMeanPct: got %v, expected 10", stats.TrimmedMeanPct)
	}
}

func TestTrimmedMeanDisabled(t *testing.T) {
	stats, err := computeStats(testData, nil, 1.5, 16, 0, 0, 0)
	if err != nil {
		t.Fatalf("computeStats returned error: %v", err)
	}
	if stats.TrimmedMeanPct != 0 {
		t.Errorf("TrimmedMeanPct: got %v, expected 0", stats.TrimmedMeanPct)
	}
	if stats.TrimmedMean != 0 {
		t.Errorf("TrimmedMean: got %v, expected 0", stats.TrimmedMean)
	}
}

func TestTrimmedMeanDatasetTooSmall(t *testing.T) {
	// 4 values with trim=50%: trimCount = floor(4 * 50/100) = 2, remaining = 0 → error
	_, err := computeStats([]float64{1, 2, 3, 4}, nil, 1.5, 16, 0, 50, 0)
	if err == nil {
		t.Error("expected error for dataset too small to trim, got nil")
	}
}

func TestTrimmedMeanSmallTrim(t *testing.T) {
	// 5 values with trim=5%: trimCount = floor(5 * 5/100) = floor(0.25) = 0
	// No trimming occurs, result equals regular mean
	data := []float64{1, 2, 3, 4, 5}
	stats, err := computeStats(data, nil, 1.5, 16, 0, 5, 0)
	if err != nil {
		t.Fatalf("computeStats returned error: %v", err)
	}
	if !floatEquals(stats.TrimmedMean, stats.Mean) {
		t.Errorf("TrimmedMean: got %v, expected %v (same as Mean)", stats.TrimmedMean, stats.Mean)
	}
}

func TestTrimDataset(t *testing.T) {
	// Manually trim testData at 10%: sort, remove 3 from each end (floor(31*10/100)=3)
	sorted := make([]float64, len(testData))
	copy(sorted, testData)
	sort.Float64s(sorted)
	trimmed := sorted[3 : len(sorted)-3] // 25 values

	stats, err := computeStats(trimmed, nil, 1.5, 16, 0, 0, 0)
	if err != nil {
		t.Fatalf("computeStats returned error: %v", err)
	}
	if stats.Count != 25 {
		t.Errorf("Count: got %d, expected 25", stats.Count)
	}

	// Mean of trimmed data should differ from full data mean
	fullStats, err := computeStats(testData, nil, 1.5, 16, 0, 0, 0)
	if err != nil {
		t.Fatalf("computeStats returned error: %v", err)
	}
	if floatEquals(stats.Mean, fullStats.Mean) {
		t.Errorf("Trimmed dataset mean (%v) should differ from full dataset mean (%v)", stats.Mean, fullStats.Mean)
	}

	// Verify the expected trimmed mean
	if !floatEquals(stats.Mean, 49.71) {
		t.Errorf("Trimmed dataset Mean: got %v, expected 49.71", stats.Mean)
	}
}

func TestTrimDatasetTooSmall(t *testing.T) {
	// 2 values with trim=50%: trimCount = floor(2*50/100) = 1, remaining = 0 → error
	tooSmall := writeTempData(t, []float64{1, 2})
	output, err := runStats("-T", "50", tooSmall)
	if err == nil {
		t.Fatalf("expected error trimming 50%% from 2 values, got none:\n%s", output)
	}
	if !strings.Contains(output, "dataset too small") {
		t.Errorf("expected 'dataset too small' error, got: %s", output)
	}

	// 3 values with trim=50%: trimCount = floor(3*50/100) = 1, remaining = 1 → succeeds
	justEnough := writeTempData(t, []float64{1, 2, 3})
	output, err = runStats("-T", "50", justEnough)
	if err != nil {
		t.Fatalf("expected success trimming 50%% from 3 values: %v\n%s", err, output)
	}
	if !strings.Contains(output, "3 → 1 values") {
		t.Errorf("expected header showing 3 → 1 values, got:\n%s", output)
	}
}

func TestTrimDatasetMutualExclusion(t *testing.T) {
	output, err := runStats("-t", "10", "-T", "10", "test_data.txt")
	if err == nil {
		t.Fatal("Expected error when using both -t and -T, but got none")
	}
	if !strings.Contains(output, "mutually exclusive") {
		t.Errorf("Expected mutual exclusion error message, got: %s", output)
	}
}

// writeTempData writes values one per line to a temp file and returns its path.
func writeTempData(t *testing.T, values []float64) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "data.txt")
	var sb strings.Builder
	for _, v := range values {
		sb.WriteString(strconv.FormatFloat(v, 'f', -1, 64))
		sb.WriteString("\n")
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	return path
}

func TestCalculateEMA(t *testing.T) {
	// Simple ascending data: [1, 2, 3, 4, 5] with span=3
	// α = 2/(3+1) = 0.5
	// EMA[0] = 1
	// EMA[1] = 0.5*2 + 0.5*1 = 1.5
	// EMA[2] = 0.5*3 + 0.5*1.5 = 2.25
	// EMA[3] = 0.5*4 + 0.5*2.25 = 3.125
	// EMA[4] = 0.5*5 + 0.5*3.125 = 4.0625
	t.Run("AscendingSpan3", func(t *testing.T) {
		data := []float64{1, 2, 3, 4, 5}
		got := calculateEMA(data, 3)
		if !floatEquals(got, 4.0625) {
			t.Errorf("calculateEMA: got %v, expected 4.0625", got)
		}
	})

	// Constant data: EMA should equal the constant
	t.Run("ConstantData", func(t *testing.T) {
		data := []float64{10, 10, 10, 10}
		got := calculateEMA(data, 5)
		if !floatEquals(got, 10) {
			t.Errorf("calculateEMA: got %v, expected 10", got)
		}
	})

	// Single spike: [0, 0, 100, 0, 0] with span=3 (α=0.5)
	// EMA[0] = 0
	// EMA[1] = 0.5*0 + 0.5*0 = 0
	// EMA[2] = 0.5*100 + 0.5*0 = 50
	// EMA[3] = 0.5*0 + 0.5*50 = 25
	// EMA[4] = 0.5*0 + 0.5*25 = 12.5
	t.Run("SingleSpike", func(t *testing.T) {
		data := []float64{0, 0, 100, 0, 0}
		got := calculateEMA(data, 3)
		if !floatEquals(got, 12.5) {
			t.Errorf("calculateEMA: got %v, expected 12.5", got)
		}
	})
}

func TestEMAViaComputeStats(t *testing.T) {
	data := []float64{1, 2, 3, 4, 5}
	stats, err := computeStats(data, nil, 1.5, 16, 0, 0, 3)
	if err != nil {
		t.Fatalf("computeStats returned error: %v", err)
	}
	if stats.EMASpan != 3 {
		t.Errorf("EMASpan: got %d, expected 3", stats.EMASpan)
	}
	if !floatEquals(stats.EMA, 4.0625) {
		t.Errorf("EMA: got %v, expected 4.0625", stats.EMA)
	}
}

func TestEMADisabled(t *testing.T) {
	stats, err := computeStats(testData, nil, 1.5, 16, 0, 0, 0)
	if err != nil {
		t.Fatalf("computeStats returned error: %v", err)
	}
	if stats.EMASpan != 0 {
		t.Errorf("EMASpan: got %d, expected 0", stats.EMASpan)
	}
	if stats.EMA != 0 {
		t.Errorf("EMA: got %v, expected 0", stats.EMA)
	}
}

func TestDetectSymmetry(t *testing.T) {
	// Near-miss copy of the symmetric fixture: one value changed by 1 breaks the 612+388 pair
	nearMiss := make([]float64, len(symmetricTestData))
	copy(nearMiss, symmetricTestData)
	nearMiss[0] = 613

	tests := []struct {
		name      string
		data      []float64
		symmetric bool
		center    float64
		pairs     int
	}{
		{"Even symmetric", symmetricTestData, true, 500, 20},
		{"Odd symmetric", []float64{2, 4, 6, 8, 10, 12, 14}, true, 8, 3},
		{"Consecutive integers", []float64{1, 2, 3, 4, 5, 6, 7, 8, 9}, true, 5, 4},
		{"Asymmetric", testData, false, 0, 0},
		{"Mean equals median but asymmetric", []float64{1, 2, 6, 9, 12}, false, 0, 0},
		{"All identical", []float64{5, 5, 5}, true, 5, 1},
		{"With duplicates", []float64{1, 2, 2, 3, 4, 4, 5}, true, 3, 3},
		{"Negative and mixed sign", []float64{-10, -5, 0, 5, 10}, true, 0, 2},
		{"Floats", []float64{1.5, 2.5, 3.5}, true, 2.5, 1},
		{"Near miss", nearMiss, false, 0, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// detectSymmetry requires sorted input; sort a copy so fixtures are untouched
			sorted := make([]float64, len(tc.data))
			copy(sorted, tc.data)
			sort.Float64s(sorted)
			symmetric, center, pairs := detectSymmetry(sorted)
			if symmetric != tc.symmetric {
				t.Errorf("symmetric: got %v, expected %v", symmetric, tc.symmetric)
			}
			// center and pairs are undefined when symmetric is false
			if tc.symmetric {
				if !floatEquals(center, tc.center) {
					t.Errorf("center: got %v, expected %v", center, tc.center)
				}
				if pairs != tc.pairs {
					t.Errorf("pairs: got %v, expected %v", pairs, tc.pairs)
				}
			}
		})
	}
}

func TestDetectSymmetryEdgeCases(t *testing.T) {
	// Fewer than 3 values: symmetry is not evaluated, signaled by symmetric == false
	t.Run("Empty", func(t *testing.T) {
		symmetric, _, _ := detectSymmetry([]float64{})
		if symmetric {
			t.Error("expected not symmetric for empty input")
		}
	})

	t.Run("OneElement", func(t *testing.T) {
		symmetric, _, _ := detectSymmetry([]float64{42})
		if symmetric {
			t.Error("expected not symmetric for one element")
		}
	})

	t.Run("TwoElements", func(t *testing.T) {
		symmetric, _, _ := detectSymmetry([]float64{5, 7})
		if symmetric {
			t.Error("expected not symmetric for two elements")
		}
	})
}

func TestDetectSymmetryFloatTolerance(t *testing.T) {
	// Repeated addition of 0.1 accumulates representation error, but pair sums
	// stay well within the scale-relative tolerance
	t.Run("AccumulatedFloatError", func(t *testing.T) {
		data := make([]float64, 9)
		v := 0.0
		for i := range data {
			v += 0.1
			data[i] = v
		}
		symmetric, center, _ := detectSymmetry(data)
		if !symmetric {
			t.Error("expected symmetric despite accumulated float error")
		}
		if !floatEquals(center, 0.5) {
			t.Errorf("center: got %v, expected 0.5", center)
		}
	})

	// A shift of 1e-5 is below the test epsilon of 1e-4 but far above the
	// detection tolerance, so it must be reported as not symmetric
	t.Run("GenuineSmallDifference", func(t *testing.T) {
		data := []float64{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9 + 1e-5}
		symmetric, _, _ := detectSymmetry(data)
		if symmetric {
			t.Error("expected not symmetric for value shifted by 1e-5")
		}
	})
}

func TestSymmetryViaComputeStats(t *testing.T) {
	stats, err := computeStats(symmetricTestData, nil, 1.5, 16, 0, 0, 0)
	if err != nil {
		t.Fatalf("computeStats returned error: %v", err)
	}

	// These four are consequences of symmetry, equally true of many asymmetric datasets
	if !floatEquals(stats.Sum, 20000) {
		t.Errorf("Sum: got %v, expected 20000", stats.Sum)
	}
	if !floatEquals(stats.Mean, 500) {
		t.Errorf("Mean: got %v, expected 500", stats.Mean)
	}
	if !floatEquals(stats.Median, 500) {
		t.Errorf("Median: got %v, expected 500", stats.Median)
	}
	if !floatEquals(stats.Skewness, 0) {
		t.Errorf("Skewness: got %v, expected 0", stats.Skewness)
	}

	// The pairwise check identifies the actual property
	if !stats.IsSymmetric {
		t.Error("IsSymmetric: got false, expected true")
	}
	if !floatEquals(stats.SymmetryCenter, 500) {
		t.Errorf("SymmetryCenter: got %v, expected 500", stats.SymmetryCenter)
	}
	if stats.SymmetryPairs != 20 {
		t.Errorf("SymmetryPairs: got %d, expected 20", stats.SymmetryPairs)
	}
}

func TestSymmetryNotDetectedForTestData(t *testing.T) {
	stats, err := computeStats(testData, nil, 1.5, 16, 0, 0, 0)
	if err != nil {
		t.Fatalf("computeStats returned error: %v", err)
	}
	if stats.IsSymmetric {
		t.Error("IsSymmetric: got true, expected false for asymmetric testData")
	}
}

func TestSymmetryIndependentOfInputOrder(t *testing.T) {
	// Deterministic permutation: stride through the fixture with step 7, coprime with 40
	n := len(symmetricTestData)
	shuffled := make([]float64, n)
	for i := 0; i < n; i++ {
		shuffled[i] = symmetricTestData[(i*7)%n]
	}

	original, err := computeStats(symmetricTestData, nil, 1.5, 16, 0, 0, 0)
	if err != nil {
		t.Fatalf("computeStats returned error: %v", err)
	}
	reordered, err := computeStats(shuffled, nil, 1.5, 16, 0, 0, 0)
	if err != nil {
		t.Fatalf("computeStats returned error: %v", err)
	}

	if !original.IsSymmetric {
		t.Error("IsSymmetric (original order): got false, expected true")
	}
	if reordered.IsSymmetric != original.IsSymmetric {
		t.Errorf("IsSymmetric: got %v for shuffled, expected %v", reordered.IsSymmetric, original.IsSymmetric)
	}
	if !floatEquals(reordered.SymmetryCenter, original.SymmetryCenter) {
		t.Errorf("SymmetryCenter: got %v for shuffled, expected %v", reordered.SymmetryCenter, original.SymmetryCenter)
	}
	if reordered.SymmetryPairs != original.SymmetryPairs {
		t.Errorf("SymmetryPairs: got %d for shuffled, expected %d", reordered.SymmetryPairs, original.SymmetryPairs)
	}
}

func TestSymmetryOutputLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "symmetric.txt")
	var sb strings.Builder
	for _, v := range symmetricTestData {
		sb.WriteString(strconv.FormatFloat(v, 'f', -1, 64))
		sb.WriteString("\n")
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	output, err := runStats(path)
	if err != nil {
		t.Fatalf("stats failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "Symmetric about 500 (20 pairs)") {
		t.Errorf("expected symmetric output line, got:\n%s", output)
	}

	output, err = runStats("test_data.txt")
	if err != nil {
		t.Fatalf("stats failed: %v\n%s", err, output)
	}
	found := false
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "Symmetry:") {
			found = true
			if !strings.Contains(line, "None") {
				t.Errorf("expected Symmetry line to read None, got: %s", line)
			}
		}
	}
	if !found {
		t.Errorf("Symmetry line not found in output:\n%s", output)
	}
}

func TestReadNumbersSkipped(t *testing.T) {
	input := `10

not-a-number
20

abc
30
`
	reader := strings.NewReader(input)
	numbers, skipped, err := readNumbers(reader)
	if err != nil {
		t.Fatalf("readNumbers returned error: %v", err)
	}
	expected := []float64{10, 20, 30}
	if !floatSliceEquals(numbers, expected) {
		t.Errorf("readNumbers: got %v, expected %v", numbers, expected)
	}
	// Two unparseable lines; the two blank lines must not count toward skipped
	if skipped != 2 {
		t.Errorf("skipped: got %d, expected 2", skipped)
	}
}

func TestDistinctCount(t *testing.T) {
	tests := []struct {
		name        string
		data        []float64
		distinct    int
		expectedPct float64
	}{
		{"All unique", []float64{1, 2, 3, 4, 5}, 5, 0},
		// testData has 31 values; 50 appears 4 times, so 3 duplicates: 3/31*100 = 9.6774%
		{"Repeated 50s", testData, 28, 9.6774},
		{"All identical", []float64{7, 7, 7, 7}, 1, 75},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stats, err := computeStats(tc.data, nil, 1.5, 16, 0, 0, 0)
			if err != nil {
				t.Fatalf("computeStats returned error: %v", err)
			}
			if stats.Distinct != tc.distinct {
				t.Errorf("Distinct: got %d, expected %d", stats.Distinct, tc.distinct)
			}
			dupPct := float64(stats.Count-stats.Distinct) / float64(stats.Count) * 100
			if !floatEquals(dupPct, tc.expectedPct) {
				t.Errorf("duplicate percentage: got %v, expected %v", dupPct, tc.expectedPct)
			}
		})
	}
}

func TestCalculateMAD(t *testing.T) {
	// Hand-computed case for {1..9}:
	// median = 5
	// |x - 5| = {4,3,2,1,0,1,2,3,4}, sorted = {0,1,1,2,2,3,3,4,4}
	// median of deviations = 2
	// scaled MAD = 1.4826 * 2 = 2.9652
	t.Run("HandComputed", func(t *testing.T) {
		data := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9}
		got := calculateMAD(data, 5)
		if !floatEquals(got, 2.9652) {
			t.Errorf("calculateMAD: got %v, expected 2.9652", got)
		}
	})

	// More than half the values are identical:
	// median = 5, |x - 5| = {0,0,0,2}, median of deviations = 0, MAD = 0
	t.Run("ZeroMAD", func(t *testing.T) {
		data := []float64{5, 5, 5, 7}
		got := calculateMAD(data, 5)
		if !floatEquals(got, 0) {
			t.Errorf("calculateMAD: got %v, expected 0", got)
		}
	})

	t.Run("ViaComputeStats", func(t *testing.T) {
		stats, err := computeStats(testData, nil, 1.5, 16, 0, 0, 0)
		if err != nil {
			t.Fatalf("computeStats returned error: %v", err)
		}
		// median = 50, median of |x - 50| = 25, scaled: 1.4826 * 25 = 37.065
		if !floatEquals(stats.MAD, 37.065) {
			t.Errorf("MAD: got %v, expected 37.065", stats.MAD)
		}
	})
}

// maskingData holds a tight cluster with two adjacent outliers on the same tail.
// The outliers inflate the mean (17.775) and standard deviation (15.541) enough to
// suppress their own classic z-scores: z(50) = 2.0736, z(52) = 2.2022 — both below
// a 2.5 threshold. The modified z-scores use median (11.35) and raw MAD (0.75),
// which the outliers cannot inflate: modZ(50) = 34.76, modZ(52) = 36.56.
var maskingData = []float64{10, 10.2, 10.5, 10.8, 11, 11.2, 11.5, 11.8, 12, 12.3, 50, 52}

func TestModifiedZScoreOutliers(t *testing.T) {
	stats, err := computeStats(maskingData, nil, 1.5, 16, 2.5, 0, 0)
	if err != nil {
		t.Fatalf("computeStats returned error: %v", err)
	}

	// Classic z-score detection is masked: neither outlier reaches |z| > 2.5
	if len(stats.ZScoreOutliers) != 0 {
		t.Errorf("ZScoreOutliers: got %v, expected none (masking)", stats.ZScoreOutliers)
	}

	// Modified z-score detection catches both at the same threshold
	expected := []float64{50, 52}
	if !floatSliceEquals(stats.ModZOutliers, expected) {
		t.Errorf("ModZOutliers: got %v, expected %v", stats.ModZOutliers, expected)
	}
}

func TestModifiedZScoreZeroMAD(t *testing.T) {
	// More than half identical: MAD = 0, so modified z-scores are undefined
	stats, err := computeStats([]float64{5, 5, 5, 5, 9}, nil, 1.5, 16, 2.0, 0, 0)
	if err != nil {
		t.Fatalf("computeStats returned error: %v", err)
	}
	if !floatEquals(stats.MAD, 0) {
		t.Errorf("MAD: got %v, expected 0", stats.MAD)
	}
	if stats.ModZOutliers != nil {
		t.Errorf("ModZOutliers with zero MAD: got %v, expected nil", stats.ModZOutliers)
	}
}

func TestStandardError(t *testing.T) {
	stats, err := computeStats(testData, nil, 1.5, 16, 0, 0, 0)
	if err != nil {
		t.Fatalf("computeStats returned error: %v", err)
	}
	// StdDev 33.5751 / sqrt(31) = 6.0303
	if !floatEquals(stats.StdErr, 6.0303) {
		t.Errorf("StdErr: got %v, expected 6.0303", stats.StdErr)
	}

	single, err := computeStats([]float64{42.5}, nil, 1.5, 16, 0, 0, 0)
	if err != nil {
		t.Fatalf("computeStats returned error: %v", err)
	}
	if !floatEquals(single.StdErr, 0) {
		t.Errorf("StdErr for single value: got %v, expected 0", single.StdErr)
	}
}

func TestGeometricMean(t *testing.T) {
	// Closed-form case: geometric mean of {1,2,4,8} is (1*2*4*8)^(1/4) = 2^1.5 = 2.8284
	t.Run("ClosedForm", func(t *testing.T) {
		got := calculateGeometricMean([]float64{1, 2, 4, 8})
		if !floatEquals(got, 2.8284) {
			t.Errorf("calculateGeometricMean: got %v, expected 2.8284", got)
		}
	})

	// Geometric mean never exceeds the arithmetic mean for positive data
	t.Run("OrderingVsArithmetic", func(t *testing.T) {
		stats, err := computeStats(testData, nil, 1.5, 16, 0, 0, 0)
		if err != nil {
			t.Fatalf("computeStats returned error: %v", err)
		}
		if !stats.GeoMeanValid {
			t.Fatal("GeoMeanValid: got false, expected true for all-positive data")
		}
		if !floatEquals(stats.GeometricMean, 38.2248) {
			t.Errorf("GeometricMean: got %v, expected 38.2248", stats.GeometricMean)
		}
		if stats.GeometricMean > stats.Mean {
			t.Errorf("GeometricMean (%v) should not exceed Mean (%v)", stats.GeometricMean, stats.Mean)
		}
	})

	// Suppressed when data contains zero or negative values
	t.Run("SuppressedOnNonPositive", func(t *testing.T) {
		withZero, err := computeStats([]float64{0, 1, 2, 3}, nil, 1.5, 16, 0, 0, 0)
		if err != nil {
			t.Fatalf("computeStats returned error: %v", err)
		}
		if withZero.GeoMeanValid {
			t.Error("GeoMeanValid: got true, expected false for data containing zero")
		}
		withNegative, err := computeStats([]float64{-1, 1, 2, 3}, nil, 1.5, 16, 0, 0, 0)
		if err != nil {
			t.Fatalf("computeStats returned error: %v", err)
		}
		if withNegative.GeoMeanValid {
			t.Error("GeoMeanValid: got true, expected false for data containing a negative value")
		}
	})
}

func TestCalculateAutocorrelation(t *testing.T) {
	ascending := make([]float64, 20)
	for i := range ascending {
		ascending[i] = float64(i + 1)
	}
	// Deterministic shuffle of 1..20 chosen for near-zero lag-1 autocorrelation
	shuffled := []float64{5, 3, 6, 12, 10, 2, 7, 14, 19, 9, 18, 1, 11, 20, 8, 13, 4, 17, 16, 15}

	tests := []struct {
		name     string
		data     []float64
		expected float64
	}{
		{"Ascending", ascending, 0.85},
		{"Alternating", []float64{1, -1, 1, -1, 1, -1, 1, -1, 1, -1}, -0.9},
		{"Shuffled", shuffled, -0.0064},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var sum float64
			for _, v := range tc.data {
				sum += v
			}
			mean := sum / float64(len(tc.data))
			got := calculateAutocorrelation(tc.data, mean)
			if !floatEquals(got, tc.expected) {
				t.Errorf("calculateAutocorrelation: got %v, expected %v", got, tc.expected)
			}
		})
	}

	t.Run("FewerThanThree", func(t *testing.T) {
		got := calculateAutocorrelation([]float64{1, 2}, 1.5)
		if got != 0 {
			t.Errorf("expected 0 for fewer than 3 values, got %v", got)
		}
	})

	t.Run("ZeroVariance", func(t *testing.T) {
		got := calculateAutocorrelation([]float64{5, 5, 5, 5}, 5)
		if got != 0 {
			t.Errorf("expected 0 for zero variance, got %v", got)
		}
	})
}

func TestDetectMonotonicity(t *testing.T) {
	tests := []struct {
		name     string
		data     []float64
		expected string
	}{
		{"Ascending with ties", []float64{1, 2, 2, 3, 4}, "ascending"},
		{"Descending with ties", []float64{9, 7, 7, 2}, "descending"},
		{"Unordered", testData, "unordered"},
		{"All identical", []float64{5, 5, 5}, "constant"},
		{"Two ascending", []float64{1, 2}, "ascending"},
		{"Two descending", []float64{2, 1}, "descending"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := detectMonotonicity(tc.data)
			if got != tc.expected {
				t.Errorf("detectMonotonicity: got %q, expected %q", got, tc.expected)
			}
		})
	}
}

func TestOrderStatisticsSuppressedUnderTrim(t *testing.T) {
	output, err := runStats("-T", "10", "test_data.txt")
	if err != nil {
		t.Fatalf("stats failed: %v\n%s", err, output)
	}
	for _, label := range []string{"Trendline:", "Autocorrelation:", "Input Order:"} {
		if strings.Contains(output, label) {
			t.Errorf("expected no %q line under -T, got:\n%s", label, output)
		}
	}
	if !strings.Contains(output, "(trimmed dataset: 10% from each tail, 31 → 25 values)") {
		t.Errorf("expected trimmed-dataset header under -T, got:\n%s", output)
	}
	if !strings.Contains(output, "all statistics above are computed on the trimmed dataset") {
		t.Errorf("expected trimmed-dataset footnote under -T, got:\n%s", output)
	}
}

// The star convention was dropped: under -T every statistic is computed on the
// trimmed dataset, so marking a subset implied the rest were unaffected.
func TestNoStarMarkersUnderTrim(t *testing.T) {
	output, err := runStats("-T", "10", "test_data.txt")
	if err != nil {
		t.Fatalf("stats failed: %v\n%s", err, output)
	}
	for _, line := range strings.Split(output, "\n") {
		label, _, found := strings.Cut(line, ":")
		if found && strings.HasSuffix(label, "*") {
			t.Errorf("unexpected star marker on label %q", label)
		}
	}
}

func TestGeometricSuppressedUnderLogTransform(t *testing.T) {
	output, err := runStats("-l", "test_data.txt")
	if err != nil {
		t.Fatalf("stats failed: %v\n%s", err, output)
	}
	if strings.Contains(output, "Geometric Mean") {
		t.Errorf("expected no Geometric Mean line under -l, got:\n%s", output)
	}
}

func TestEMATrimDatasetMutualExclusion(t *testing.T) {
	output, err := runStats("-e", "5", "-T", "10", "test_data.txt")
	if err == nil {
		t.Fatal("Expected error when using both -e and -T, but got none")
	}
	if !strings.Contains(output, "mutually exclusive") {
		t.Errorf("Expected mutual exclusion error message, got: %s", output)
	}
}

// --- Input validation: NaN and infinities parse but must be rejected ---

func TestReadNumbersRejectsNonFinite(t *testing.T) {
	input := "1\nNaN\n2\nInf\n3\n+Inf\n-Inf\n4\n"
	numbers, skipped, err := readNumbers(strings.NewReader(input))
	if err != nil {
		t.Fatalf("readNumbers returned error: %v", err)
	}
	expected := []float64{1, 2, 3, 4}
	if !floatSliceEquals(numbers, expected) {
		t.Errorf("readNumbers: got %v, expected %v", numbers, expected)
	}
	// NaN, Inf, +Inf, -Inf all count as invalid lines
	if skipped != 4 {
		t.Errorf("skipped: got %d, expected 4", skipped)
	}
}

func TestNonFiniteInputDoesNotPoisonOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nan.txt")
	if err := os.WriteFile(path, []byte("1\n2\nNaN\n4\n"), 0o644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	output, err := runStats(path)
	if err != nil {
		t.Fatalf("stats failed: %v\n%s", err, output)
	}
	if strings.Contains(output, "NaN") && !strings.Contains(output, "invalid number") {
		t.Errorf("NaN leaked into computed output:\n%s", output)
	}
	for _, want := range []string{"Count:             3", "Skipped:           1 invalid line", "Min:               1", "Max:               4"} {
		if !strings.Contains(output, want) {
			t.Errorf("expected %q in output, got:\n%s", want, output)
		}
	}
}

// --- Number formatting: small magnitudes must not collapse to zero ---

func TestFormatFloatAdaptiveDecimals(t *testing.T) {
	tests := []struct {
		value    float64
		expected string
	}{
		{0, "0"},
		{42, "42"},
		{-42, "-42"},
		{20.73, "20.73"},
		{55.659728571, "55.6597"},
		{0.5, "0.5"},
		// Below the old fixed 4-decimal floor these all used to print as "0"
		{0.00004, "0.00004"},
		{0.00006, "0.00006"},
		{0.000012345, "0.00001234"},
		{-0.00004, "-0.00004"},
		{0.0000000001234, "0.0000000001234"},
	}
	for _, tc := range tests {
		if got := formatFloat(tc.value); got != tc.expected {
			t.Errorf("formatFloat(%v): got %q, expected %q", tc.value, got, tc.expected)
		}
	}
}

func TestFormatFloatNeverUsesScientificNotation(t *testing.T) {
	for _, v := range []float64{1e-12, 5e-7, 1.5e9, -3.25e-8} {
		got := formatFloat(v)
		if strings.ContainsAny(got, "eE") {
			t.Errorf("formatFloat(%v) used scientific notation: %q", v, got)
		}
	}
}

func TestSmallMagnitudeStatsSurviveFormatting(t *testing.T) {
	path := writeTempData(t, []float64{0.00004, 0.00006})
	output, err := runStats(path)
	if err != nil {
		t.Fatalf("stats failed: %v\n%s", err, output)
	}
	for _, want := range []string{"Min:               0.00004", "Max:               0.00006", "Mean:              0.00005"} {
		if !strings.Contains(output, want) {
			t.Errorf("expected %q in output, got:\n%s", want, output)
		}
	}
}

// --- Flag validation ---

func TestIQRMultiplierValidation(t *testing.T) {
	for _, k := range []string{"-1", "0"} {
		output, err := runStats("-k", k, "test_data.txt")
		if err == nil {
			t.Errorf("expected error for -k %s, got none:\n%s", k, output)
		}
		if !strings.Contains(output, "IQR multiplier must be greater than 0") {
			t.Errorf("expected IQR multiplier error for -k %s, got: %s", k, output)
		}
	}

	if output, err := runStats("-k", "0.5", "test_data.txt"); err != nil {
		t.Errorf("expected -k 0.5 to succeed: %v\n%s", err, output)
	}
}

func TestExtraArgumentsRejected(t *testing.T) {
	// Go's flag package stops at the first non-flag argument, so these options
	// used to be accepted and silently ignored.
	output, err := runStats("test_data.txt", "-z", "2.0")
	if err == nil {
		t.Fatalf("expected error for options after the filename, got none:\n%s", output)
	}
	if !strings.Contains(output, "expected at most one input file") {
		t.Errorf("expected extra-argument error, got: %s", output)
	}
	if !strings.Contains(output, "options must appear before the filename") {
		t.Errorf("expected hint about option ordering, got: %s", output)
	}

	output, err = runStats("test_data.txt", "symmetric_data.txt")
	if err == nil {
		t.Fatalf("expected error for two input files, got none:\n%s", output)
	}
}

func TestVersionFlagIgnoresOtherValidation(t *testing.T) {
	// -v is handled before flag validation, so an invalid bin count must not mask it
	output, err := runStats("-v", "-b", "3")
	if err != nil {
		t.Fatalf("expected -v to succeed regardless of other flags: %v\n%s", err, output)
	}
	if !strings.Contains(output, "version "+PgmVersion) {
		t.Errorf("expected version output, got: %s", output)
	}
}

// --- Z-score reporting on degenerate data ---

func TestZScoreReportedWhenStdDevIsZero(t *testing.T) {
	stats, err := computeStats([]float64{5, 5, 5}, nil, 1.5, 16, 2.0, 0, 0)
	if err != nil {
		t.Fatalf("computeStats returned error: %v", err)
	}
	// The request is recorded even though it cannot be satisfied
	if !floatEquals(stats.ZScoreThreshold, 2.0) {
		t.Errorf("ZScoreThreshold: got %v, expected 2.0", stats.ZScoreThreshold)
	}
	if stats.ZScoreValid {
		t.Error("ZScoreValid: got true, expected false for zero standard deviation")
	}
	if stats.ModZValid {
		t.Error("ModZValid: got true, expected false for zero MAD")
	}
	if stats.ZScoreOutliers != nil {
		t.Errorf("ZScoreOutliers: got %v, expected nil", stats.ZScoreOutliers)
	}
}

func TestZScoreSectionNotSilentlyDropped(t *testing.T) {
	path := writeTempData(t, []float64{5, 5, 5, 5})
	output, err := runStats("-z", "2.0", path)
	if err != nil {
		t.Fatalf("stats failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "N/A - standard deviation is zero") {
		t.Errorf("expected Z-outlier N/A line, got:\n%s", output)
	}
	if !strings.Contains(output, "N/A - MAD is zero") {
		t.Errorf("expected Mod Z-outlier N/A line, got:\n%s", output)
	}
}

// --- Custom percentiles ---

func TestCustomPercentiles(t *testing.T) {
	stats, err := computeStats(testData, []float64{10, 90, 99.9}, 1.5, 16, 0, 0, 0)
	if err != nil {
		t.Fatalf("computeStats returned error: %v", err)
	}
	if len(stats.CustomPercentiles) != 3 {
		t.Fatalf("CustomPercentiles: got %d entries, expected 3", len(stats.CustomPercentiles))
	}

	sorted := make([]float64, len(testData))
	copy(sorted, testData)
	sort.Float64s(sorted)
	for _, p := range []float64{10, 90, 99.9} {
		want := calculatePercentile(sorted, p/100.0)
		if got := stats.CustomPercentiles[p]; !floatEquals(got, want) {
			t.Errorf("CustomPercentiles[%v]: got %v, expected %v", p, got, want)
		}
	}

	// p0 and p100 are the min and max
	edges, err := computeStats(testData, []float64{0, 100}, 1.5, 16, 0, 0, 0)
	if err != nil {
		t.Fatalf("computeStats returned error: %v", err)
	}
	if !floatEquals(edges.CustomPercentiles[0], edges.Min) {
		t.Errorf("p0: got %v, expected Min %v", edges.CustomPercentiles[0], edges.Min)
	}
	if !floatEquals(edges.CustomPercentiles[100], edges.Max) {
		t.Errorf("p100: got %v, expected Max %v", edges.CustomPercentiles[100], edges.Max)
	}
}

func TestCustomPercentilesDisabled(t *testing.T) {
	stats, err := computeStats(testData, nil, 1.5, 16, 0, 0, 0)
	if err != nil {
		t.Fatalf("computeStats returned error: %v", err)
	}
	if stats.CustomPercentiles != nil {
		t.Errorf("CustomPercentiles: got %v, expected nil", stats.CustomPercentiles)
	}
}

func TestCustomPercentilesCLI(t *testing.T) {
	output, err := runStats("-p", "10,90", "test_data.txt")
	if err != nil {
		t.Fatalf("stats failed: %v\n%s", err, output)
	}
	for _, want := range []string{"Percentile (p10):", "Percentile (p90):"} {
		if !strings.Contains(output, want) {
			t.Errorf("expected %q in output, got:\n%s", want, output)
		}
	}

	if output, err := runStats("-p", "101", "test_data.txt"); err == nil {
		t.Errorf("expected error for out-of-range percentile, got none:\n%s", output)
	}
	if output, err := runStats("-p", "abc", "test_data.txt"); err == nil {
		t.Errorf("expected error for unparseable percentile, got none:\n%s", output)
	}
}

// --- 95% confidence interval for the mean ---

func TestConfidenceInterval(t *testing.T) {
	stats, err := computeStats(testData, nil, 1.5, 16, 0, 0, 0)
	if err != nil {
		t.Fatalf("computeStats returned error: %v", err)
	}
	if !stats.CIValid {
		t.Fatal("CIValid: got false, expected true")
	}
	// mean 51.7258 ± 1.96 * 6.0303
	if !floatEquals(stats.CI95Lower, 51.7258-1.96*6.0303) {
		t.Errorf("CI95Lower: got %v", stats.CI95Lower)
	}
	if !floatEquals(stats.CI95Upper, 51.7258+1.96*6.0303) {
		t.Errorf("CI95Upper: got %v", stats.CI95Upper)
	}
	// The interval is centered on the mean and its half-width is 1.96 standard errors
	if !floatEquals((stats.CI95Lower+stats.CI95Upper)/2, stats.Mean) {
		t.Errorf("interval is not centered on the mean")
	}
	if !floatEquals((stats.CI95Upper-stats.CI95Lower)/2, ci95Z*stats.StdErr) {
		t.Errorf("interval half-width is not 1.96 standard errors")
	}
}

func TestConfidenceIntervalSuppressedForZeroSpread(t *testing.T) {
	stats, err := computeStats([]float64{7, 7, 7}, nil, 1.5, 16, 0, 0, 0)
	if err != nil {
		t.Fatalf("computeStats returned error: %v", err)
	}
	if stats.CIValid {
		t.Error("CIValid: got true, expected false when the standard error is zero")
	}

	output, err := runStats(writeTempData(t, []float64{7, 7, 7}))
	if err != nil {
		t.Fatalf("stats failed: %v\n%s", err, output)
	}
	if strings.Contains(output, "95% CI") {
		t.Errorf("expected no CI line for zero-spread data, got:\n%s", output)
	}
}

// --- Histogram binning ---

func TestGenerateHistogramCapsBinsToDataLength(t *testing.T) {
	// 7 values with 20 requested bins must produce 7 bins, matching the trendline
	data := []float64{1, 2, 3, 4, 5, 8, 9}
	hist := generateHistogram(data, 20)
	if got := len([]rune(hist)); got != 7 {
		t.Errorf("expected 7 runes when bins exceed data length, got %d (%q)", got, hist)
	}
	trend := generateTrendline(data, 20)
	if len([]rune(hist)) != len([]rune(trend)) {
		t.Errorf("histogram (%d) and trendline (%d) lengths must match", len([]rune(hist)), len([]rune(trend)))
	}
}

func TestGenerateHistogramSparseBinsAreVisible(t *testing.T) {
	// One dominant bin plus nine bins holding a single value each. Sparse bins must
	// not render as the same glyph as an empty bin.
	data := make([]float64, 0, 110)
	for i := 0; i < 100; i++ {
		data = append(data, 0)
	}
	for i := 1; i <= 9; i++ {
		data = append(data, float64(i*10))
	}
	data = append(data, 100)
	sort.Float64s(data)

	hist := []rune(generateHistogram(data, 11))
	if hist[0] != '█' {
		t.Errorf("expected a full block for the dominant bin, got %c", hist[0])
	}
	for i, r := range hist[1:] {
		if r == '▁' {
			t.Errorf("occupied bin %d rendered as the empty-bin glyph", i+1)
		}
	}
}

// --- Modal bin fallback for continuous data ---

func TestModalBinReportedWhenNoModeExists(t *testing.T) {
	stats, err := computeStats(symmetricTestData, nil, 1.5, 16, 0, 0, 0)
	if err != nil {
		t.Fatalf("computeStats returned error: %v", err)
	}
	if len(stats.Mode) != 0 {
		t.Fatalf("Mode: got %v, expected none for all-distinct data", stats.Mode)
	}
	if !stats.ModalBinValid {
		t.Fatal("ModalBinValid: got false, expected true")
	}
	if stats.ModalBinCount < 1 {
		t.Errorf("ModalBinCount: got %d, expected at least 1", stats.ModalBinCount)
	}
	if stats.ModalBinLow >= stats.ModalBinHigh {
		t.Errorf("modal bin edges are not ordered: [%v, %v]", stats.ModalBinLow, stats.ModalBinHigh)
	}
	if stats.ModalBinLow < stats.Min || stats.ModalBinHigh > stats.Max+1e-9 {
		t.Errorf("modal bin [%v, %v] falls outside the data range [%v, %v]",
			stats.ModalBinLow, stats.ModalBinHigh, stats.Min, stats.Max)
	}

	// The reported count must match the number of values actually inside the bin
	inside := 0
	for _, v := range symmetricTestData {
		if v >= stats.ModalBinLow && v < stats.ModalBinHigh {
			inside++
		}
	}
	if inside != stats.ModalBinCount {
		t.Errorf("ModalBinCount: got %d, but %d values fall in [%v, %v)",
			stats.ModalBinCount, inside, stats.ModalBinLow, stats.ModalBinHigh)
	}
}

func TestModalBinSuppressedWhenModeExists(t *testing.T) {
	stats, err := computeStats(testData, nil, 1.5, 16, 0, 0, 0)
	if err != nil {
		t.Fatalf("computeStats returned error: %v", err)
	}
	if stats.ModalBinValid {
		t.Error("ModalBinValid: got true, expected false when a real mode exists")
	}
}

func TestModalBinSuppressedForZeroSpread(t *testing.T) {
	// All identical: there is a mode, so no fallback. All distinct but only one
	// value: nothing to bin.
	single, err := computeStats([]float64{42}, nil, 1.5, 16, 0, 0, 0)
	if err != nil {
		t.Fatalf("computeStats returned error: %v", err)
	}
	if single.ModalBinValid {
		t.Error("ModalBinValid: got true, expected false for a single value")
	}
}

func TestModalBinOutputLine(t *testing.T) {
	output, err := runStats("symmetric_data.txt")
	if err != nil {
		t.Fatalf("stats failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "modal bin:") {
		t.Errorf("expected modal bin on the Mode line, got:\n%s", output)
	}

	output, err = runStats("test_data.txt")
	if err != nil {
		t.Fatalf("stats failed: %v\n%s", err, output)
	}
	if strings.Contains(output, "modal bin:") {
		t.Errorf("expected no modal bin when a mode exists, got:\n%s", output)
	}
}

// --- Order-dependence warning ---

func TestSortedInputWarningNamesEMA(t *testing.T) {
	path := writeTempData(t, []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10})

	output, err := runStats("-e", "3", path)
	if err != nil {
		t.Fatalf("stats failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "trendline, autocorrelation, and EMA reflect this sort order") {
		t.Errorf("expected EMA named in the sort-order warning, got:\n%s", output)
	}

	// Without -e there is no EMA line, so it must not be named
	output, err = runStats(path)
	if err != nil {
		t.Fatalf("stats failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "trendline and autocorrelation reflect this sort order") {
		t.Errorf("expected the two-statistic warning without -e, got:\n%s", output)
	}
}

// --- JSON output ---

func TestJSONOutput(t *testing.T) {
	output, err := runStats("-j", "-z", "2.0", "-p", "10,90", "test_data.txt")
	if err != nil {
		t.Fatalf("stats failed: %v\n%s", err, output)
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(output), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, output)
	}

	want := map[string]float64{
		"count":    31,
		"sum":      1603.5,
		"mean":     51.7258,
		"median":   50,
		"stdDev":   33.5751,
		"variance": 1127.2848,
		"q1":       27.5,
		"q3":       72.625,
		"p95":      97.5,
		"p99":      135,
		"iqr":      45.125,
		"skewness": 0.7271,
		"kurtosis": 0.8884,
		"mad":      37.065,
		"stdErr":   6.0303,
	}
	for key, expected := range want {
		v, ok := got[key].(float64)
		if !ok {
			t.Errorf("JSON key %q missing or not a number", key)
			continue
		}
		if !floatEquals(v, expected) {
			t.Errorf("JSON %q: got %v, expected %v", key, v, expected)
		}
	}

	if got["version"] != PgmVersion {
		t.Errorf("JSON version: got %v, expected %v", got["version"], PgmVersion)
	}
	if got["inputOrder"] != "unordered" {
		t.Errorf("JSON inputOrder: got %v, expected unordered", got["inputOrder"])
	}

	pcts, ok := got["customPercentiles"].(map[string]any)
	if !ok {
		t.Fatalf("JSON customPercentiles missing: %v", got["customPercentiles"])
	}
	for _, key := range []string{"10", "90"} {
		if _, ok := pcts[key]; !ok {
			t.Errorf("JSON customPercentiles missing key %q", key)
		}
	}

	// Outlier lists must be arrays, never null, when they were computed
	for _, key := range []string{"outliers", "zScoreOutliers", "modZOutliers"} {
		if _, ok := got[key].([]any); !ok {
			t.Errorf("JSON %q: got %v, expected an array", key, got[key])
		}
	}
}

func TestJSONMatchesTextOutput(t *testing.T) {
	jsonOut, err := runStats("-j", "sample_data.txt")
	if err != nil {
		t.Fatalf("stats failed: %v\n%s", err, jsonOut)
	}
	textOut, err := runStats("sample_data.txt")
	if err != nil {
		t.Fatalf("stats failed: %v\n%s", err, textOut)
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	// Every JSON scalar the text output also prints must format identically
	pairs := []struct {
		key   string
		label string
	}{
		{"sum", "Sum:"},
		{"min", "Min:"},
		{"max", "Max:"},
		{"mean", "Mean:"},
		{"median", "Median (p50):"},
		{"stdDev", "Std Deviation:"},
		{"variance", "Variance:"},
		{"iqr", "IQR:"},
	}
	for _, p := range pairs {
		v, ok := parsed[p.key].(float64)
		if !ok {
			t.Errorf("JSON key %q missing", p.key)
			continue
		}
		want := p.label + " " + formatFloat(v)
		found := false
		for _, line := range strings.Split(textOut, "\n") {
			if strings.HasPrefix(line, p.label) && strings.Join(strings.Fields(line), " ") == want {
				found = true
			}
		}
		if !found {
			t.Errorf("text output has no line matching %q", want)
		}
	}
}

func TestJSONUnderTrimAndLogTransform(t *testing.T) {
	output, err := runStats("-j", "-T", "10", "test_data.txt")
	if err != nil {
		t.Fatalf("stats failed: %v\n%s", err, output)
	}
	var trimmed map[string]any
	if err := json.Unmarshal([]byte(output), &trimmed); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, output)
	}
	if trimmed["count"].(float64) != 25 {
		t.Errorf("JSON count under -T: got %v, expected 25", trimmed["count"])
	}
	if trimmed["trimDatasetOrigN"].(float64) != 31 {
		t.Errorf("JSON trimDatasetOrigN: got %v, expected 31", trimmed["trimDatasetOrigN"])
	}
	if trimmed["inputOrder"] != "" {
		t.Errorf("JSON inputOrder under -T: got %v, expected empty", trimmed["inputOrder"])
	}
	if trimmed["autocorrValid"] != false {
		t.Errorf("JSON autocorrValid under -T: got %v, expected false", trimmed["autocorrValid"])
	}

	output, err = runStats("-j", "-l", "test_data.txt")
	if err != nil {
		t.Fatalf("stats failed: %v\n%s", err, output)
	}
	var logged map[string]any
	if err := json.Unmarshal([]byte(output), &logged); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, output)
	}
	if logged["logTransformed"] != true {
		t.Errorf("JSON logTransformed: got %v, expected true", logged["logTransformed"])
	}
	if logged["geoMeanValid"] != false {
		t.Errorf("JSON geoMeanValid under -l: got %v, expected false", logged["geoMeanValid"])
	}
}
