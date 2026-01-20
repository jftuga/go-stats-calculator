package main

import (
	"math"
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
	stats, err := computeStats(testData, nil)
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
	_, err := computeStats([]float64{}, nil)
	if err == nil {
		t.Error("expected error for empty input, got nil")
	}
}

func TestComputeStatsSingleValue(t *testing.T) {
	stats, err := computeStats([]float64{42.5}, nil)
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
	stats, err := computeStats(data, nil)
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
	stats, err := computeStats(data, nil)
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
