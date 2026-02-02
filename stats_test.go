package main

import (
	"math"
	"sort"
	"strings"
	"testing"
)

const epsilon = 1e-4

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

func TestComputeStats(t *testing.T) {
	stats, err := computeStats(testData, nil, 1.5, 16, 0, 0)
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
	_, err := computeStats([]float64{}, nil, 1.5, 16, 0, 0)
	if err == nil {
		t.Error("expected error for empty input, got nil")
	}
}

func TestComputeStatsSingleValue(t *testing.T) {
	stats, err := computeStats([]float64{42.5}, nil, 1.5, 16, 0, 0)
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
}

func TestComputeStatsMultipleMode(t *testing.T) {
	// 5 and 10 both appear twice
	data := []float64{5, 5, 10, 10, 15}
	stats, err := computeStats(data, nil, 1.5, 16, 0, 0)
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
	stats, err := computeStats(data, nil, 1.5, 16, 0, 0)
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
	numbers, err := readNumbers(reader)
	if err != nil {
		t.Fatalf("readNumbers returned error: %v", err)
	}

	expected := []float64{10, 20.5, 30.00, 40}
	if !floatSliceEquals(numbers, expected) {
		t.Errorf("readNumbers: got %v, expected %v", numbers, expected)
	}
}

func TestComputeStatsCustomIQRMultiplier(t *testing.T) {
	// With k=3.0 (extreme outliers only), 150 should no longer be an outlier
	// Q1=27.5, Q3=72.625, IQR=45.125
	// lowerBound = 27.5 - 3.0*45.125 = -108.875
	// upperBound = 72.625 + 3.0*45.125 = 208.0
	// 150 < 208.0, so no outliers
	stats, err := computeStats(testData, nil, 3.0, 16, 0, 0)
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
	stats, err = computeStats(testData, nil, 1.0, 16, 0, 0)
	if err != nil {
		t.Fatalf("computeStats returned error: %v", err)
	}
	if len(stats.Outliers) != 1 || !floatEquals(stats.Outliers[0], 150) {
		t.Errorf("Outliers with k=1.0: got %v, expected [150]", stats.Outliers)
	}
}

func TestCVForTestData(t *testing.T) {
	stats, err := computeStats(testData, nil, 1.5, 16, 0, 0)
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
	stats, err := computeStats(data, nil, 1.5, 16, 0, 0)
	if err != nil {
		t.Fatalf("computeStats returned error: %v", err)
	}
	if !stats.HasNegativeData {
		t.Error("HasNegativeData: got false, expected true")
	}
}

func TestCVWithMeanNearZero(t *testing.T) {
	data := []float64{-1, 0, 1}
	stats, err := computeStats(data, nil, 1.5, 16, 0, 0)
	if err != nil {
		t.Fatalf("computeStats returned error: %v", err)
	}
	if stats.CVValid {
		t.Error("CVValid: got true, expected false")
	}
}

func TestCVSingleValue(t *testing.T) {
	stats, err := computeStats([]float64{42.5}, nil, 1.5, 16, 0, 0)
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
	numbers, err := readNumbers(reader)
	if err != nil {
		t.Fatalf("readNumbers returned error: %v", err)
	}
	if len(numbers) != 0 {
		t.Errorf("expected empty slice, got %v", numbers)
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
		stats, err := computeStats(testData, nil, 1.5, 16, 2.0, 0)
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
		stats, err := computeStats(testData, nil, 1.5, 16, 3.0, 0)
		if err != nil {
			t.Fatalf("computeStats returned error: %v", err)
		}
		if len(stats.ZScoreOutliers) != 0 {
			t.Errorf("ZScoreOutliers with z=3.0: got %v, expected none", stats.ZScoreOutliers)
		}
	})
}

func TestZScoreDisabled(t *testing.T) {
	stats, err := computeStats(testData, nil, 1.5, 16, 0, 0)
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
	stats, err := computeStats([]float64{5, 5, 5}, nil, 1.5, 16, 2.0, 0)
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
	stats, err := computeStats(result, nil, 1.5, 16, 0, 0)
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
	stats, err := computeStats(testData, nil, 1.5, 16, 0, 10)
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
	stats, err := computeStats(testData, nil, 1.5, 16, 0, 0)
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
	_, err := computeStats([]float64{1, 2, 3, 4}, nil, 1.5, 16, 0, 50)
	if err == nil {
		t.Error("expected error for dataset too small to trim, got nil")
	}
}

func TestTrimmedMeanSmallTrim(t *testing.T) {
	// 5 values with trim=5%: trimCount = floor(5 * 5/100) = floor(0.25) = 0
	// No trimming occurs, result equals regular mean
	data := []float64{1, 2, 3, 4, 5}
	stats, err := computeStats(data, nil, 1.5, 16, 0, 5)
	if err != nil {
		t.Fatalf("computeStats returned error: %v", err)
	}
	if !floatEquals(stats.TrimmedMean, stats.Mean) {
		t.Errorf("TrimmedMean: got %v, expected %v (same as Mean)", stats.TrimmedMean, stats.Mean)
	}
}
