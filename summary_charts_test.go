// summary_charts_test.go
//
// Tests for the summary chart inputs of report sections 1 to 3: which marks
// each chart receives under which conditions, that their texts match
// formatFloat, and that each gauge reports N/A exactly when its statistic's
// code returns a placeholder 0.
package main

import (
	"strings"
	"testing"
)

// summaryFor computes statistics for data with the given -p, -t and -b
// settings and builds the summary chart inputs.
func summaryFor(t *testing.T, data, percentiles []float64, trimPct float64, numBins int) (*Stats, summaryCharts) {
	t.Helper()
	s, err := computeStats(data, percentiles, 1.5, numBins, 0, trimPct, 0)
	if err != nil {
		t.Fatal(err)
	}
	return s, newSummaryCharts(s, numBins)
}

// markKeys joins the keys of marks with commas.
func markKeys(marks []chartMark) string {
	keys := make([]string, len(marks))
	for i, m := range marks {
		keys[i] = m.Key
	}
	return strings.Join(keys, ",")
}

// TestSummaryDistinctAndRange checks the duplicate bar counts and the range
// strip's marks, and that the strip is absent without spread.
func TestSummaryDistinctAndRange(t *testing.T) {
	s, sc := summaryFor(t, []float64{1, 2, 2, 3, 3, 3}, nil, 0, 16)
	if sc.Distinct != (distinctChart{Distinct: 3, Count: 6, DuplicatedText: "50"}) {
		t.Errorf("distinct = %+v", sc.Distinct)
	}
	if got := markKeys(sc.Range); got != "min,q1,median,q3,max" {
		t.Errorf("range keys = %s", got)
	}
	want := []float64{s.Min, s.Q1, s.Median, s.Q3, s.Max}
	for i, m := range sc.Range {
		if m.Value != want[i] || m.Text != formatFloat(want[i]) {
			t.Errorf("range mark %s = %v %q, want %v", m.Key, m.Value, m.Text, want[i])
		}
	}
	_, sc = summaryFor(t, []float64{3, 3, 3, 3}, nil, 0, 16)
	if sc.Range != nil {
		t.Errorf("constant data: range should be absent, got %v", sc.Range)
	}
	if sc.Distinct.DuplicatedText != "75" {
		t.Errorf("constant data: duplicated = %q, want 75", sc.Distinct.DuplicatedText)
	}
}

// TestSummaryCenter checks that the number line gets the centers under the
// text output's conditions, never EMA, and the modal bin with its real bin
// count.
func TestSummaryCenter(t *testing.T) {
	// testData: positive, repeated values, -t on: mean, geomean, trimmed
	// mean, median and (when single) the mode; CI valid.
	s, sc := summaryFor(t, testData, nil, 10, 16)
	c := sc.Center
	wantKeys := "mean,geomean,trimmedmean,median"
	if len(s.Mode) == 1 {
		wantKeys += ",mode"
	}
	if got := markKeys(c.Marks); got != wantKeys {
		t.Errorf("center keys = %s, want %s", got, wantKeys)
	}
	if c.ModeCount != len(s.Mode) {
		t.Errorf("mode count = %d, want %d", c.ModeCount, len(s.Mode))
	}
	if c.Marks[2].Label != "Trimmed Mean (10%)" || c.Marks[2].Text != formatFloat(s.TrimmedMean) {
		t.Errorf("trimmed mean mark = %+v", c.Marks[2])
	}
	if c.CI == nil || c.CI.Text != "["+formatFloat(s.CI95Lower)+", "+formatFloat(s.CI95Upper)+"]" {
		t.Errorf("CI = %+v", c.CI)
	}
	if c.Quartiles.Low != s.Q1 || c.Quartiles.High != s.Q3 {
		t.Errorf("quartiles = %+v", c.Quartiles)
	}

	// Two modes: no mode mark, the count says why. Negative data: no geomean.
	_, sc = summaryFor(t, []float64{-1, 2, 2, 5, 5, 9}, nil, 0, 16)
	if got := markKeys(sc.Center.Marks); got != "mean,median" || sc.Center.ModeCount != 2 || sc.Center.ModalBin != nil {
		t.Errorf("two modes: keys %s, mode count %d, modal bin %v", got, sc.Center.ModeCount, sc.Center.ModalBin)
	}

	// No repeats: the modal bin, with -b capped at n as binData caps it.
	s, sc = summaryFor(t, []float64{1, 2, 3, 4, 5, 6, 7, 8, 10, 20}, nil, 0, 16)
	b := sc.Center.ModalBin
	if b == nil || b.Bins != 10 || b.Count != s.ModalBinCount || b.Low != s.ModalBinLow || b.High != s.ModalBinHigh {
		t.Fatalf("modal bin = %+v, want 10 bins over [%v, %v] holding %d", b, s.ModalBinLow, s.ModalBinHigh, s.ModalBinCount)
	}
	if strings.Contains(markKeys(sc.Center.Marks), "mode") {
		t.Errorf("no repeats: unexpected mode mark")
	}
	_, sc = summaryFor(t, shuffledData(100), nil, 0, 20)
	if sc.Center.ModalBin == nil || sc.Center.ModalBin.Bins != 20 {
		t.Errorf("n=100, -b 20: modal bin = %+v", sc.Center.ModalBin)
	}

	// A single value: no CI.
	if _, sc = summaryFor(t, []float64{4}, nil, 0, 16); sc.Center.CI != nil {
		t.Errorf("n=1: CI should be absent")
	}
}

// TestSummaryLadder checks the percentile ladder's order and labels, and
// that a -p 25 merges into the Q1 rung.
func TestSummaryLadder(t *testing.T) {
	s, sc := summaryFor(t, testData, []float64{99.5, 25, 10, 95}, 0, 16)
	if got := markKeys(sc.Ladder); got != "p10,p25,p75,p95,p99,p99.5" {
		t.Fatalf("ladder keys = %s", got)
	}
	want := map[string]struct {
		label string
		value float64
	}{
		"p10":   {"p10", s.CustomPercentiles[10]},
		"p25":   {"Q1 (p25)", s.Q1},
		"p75":   {"Q3 (p75)", s.Q3},
		"p95":   {"p95", s.P95},
		"p99":   {"p99", s.P99},
		"p99.5": {"p99.5", s.CustomPercentiles[99.5]},
	}
	for _, m := range sc.Ladder {
		w := want[m.Key]
		if m.Label != w.label || m.Value != w.value || m.Text != formatFloat(w.value) {
			t.Errorf("rung %s = %+v, want label %q value %v", m.Key, m, w.label, w.value)
		}
	}
}

// TestGaugeThresholds checks that each threshold a gauge sends is where its
// interpret* reading actually changes, so the page's bands cannot drift from
// the text output's labels. Skewness is checked on both sides of zero.
func TestGaugeThresholds(t *testing.T) {
	_, sc := summaryFor(t, testData, nil, 0, 16)
	const eps = 1e-9
	tests := []struct {
		name       string
		thresholds [2]float64
		interpret  func(float64) string
		mirror     bool
	}{
		{"skewness", sc.Skewness.Thresholds, interpretSkewness, true},
		{"kurtosis", sc.Kurtosis.Thresholds, interpretKurtosis, false},
		{"cv", sc.CV.Thresholds, interpretCV, false},
	}
	for _, tt := range tests {
		if !(tt.thresholds[0] < tt.thresholds[1]) {
			t.Errorf("%s: thresholds %v not increasing", tt.name, tt.thresholds)
		}
		for _, v := range tt.thresholds {
			signs := []float64{1}
			if tt.mirror {
				signs = append(signs, -1)
			}
			for _, sign := range signs {
				below, above := tt.interpret(sign*(v-eps)), tt.interpret(sign*(v+eps))
				if below == above {
					t.Errorf("%s: reading does not change at %v: %q", tt.name, sign*v, below)
				}
			}
		}
	}
}

// TestSummaryGauges checks each gauge's value, reading and N/A reason
// against the conditions under which its statistic is defined.
func TestSummaryGauges(t *testing.T) {
	s, sc := summaryFor(t, testData, nil, 0, 16)
	if sc.Skewness != (gaugeChart{Value: s.Skewness, Thresholds: [2]float64{skewModerate, skewHigh}, Text: formatFloat(s.Skewness), Reading: interpretSkewness(s.Skewness)}) {
		t.Errorf("skewness = %+v", sc.Skewness)
	}
	if sc.Kurtosis != (gaugeChart{Value: s.Kurtosis, Thresholds: [2]float64{kurtosisLow, kurtosisHigh}, Text: formatFloat(s.Kurtosis), Reading: interpretKurtosis(s.Kurtosis)}) {
		t.Errorf("kurtosis = %+v", sc.Kurtosis)
	}
	if sc.CV != (gaugeChart{Value: s.CV, Thresholds: [2]float64{cvModerate, cvHigh}, Text: formatFloat(s.CV) + "%", Reading: interpretCV(s.CV)}) {
		t.Errorf("cv = %+v", sc.CV)
	}

	tests := []struct {
		name           string
		data           []float64
		skew, kurt, cv string
		cvWarning      bool
	}{
		{"n=2", []float64{4, 9}, "requires at least 3 values", "requires at least 4 values", "", false},
		{"n=3", []float64{1, 2, 7}, "", "requires at least 4 values", "", false},
		{"constant", []float64{3, 3, 3, 3}, "standard deviation is zero", "standard deviation is zero", "", false},
		{"mean zero", []float64{-2, -1, 1, 2}, "", "", "mean near zero", false},
		{"negative", []float64{-1, 2, 3, 4, 5}, "", "", "", true},
	}
	for _, tt := range tests {
		_, sc := summaryFor(t, tt.data, nil, 0, 16)
		if sc.Skewness.NA != tt.skew || sc.Kurtosis.NA != tt.kurt || sc.CV.NA != tt.cv {
			t.Errorf("%s: N/A reasons = %q, %q, %q", tt.name, sc.Skewness.NA, sc.Kurtosis.NA, sc.CV.NA)
		}
		for _, g := range []gaugeChart{sc.Skewness, sc.Kurtosis, sc.CV} {
			if (g.NA != "") == (g.Text != "") || (g.NA != "") == (g.Reading != "") {
				t.Errorf("%s: a gauge must have either an N/A reason or a text and reading: %+v", tt.name, g)
			}
		}
		if (sc.CV.Warning != "") != tt.cvWarning {
			t.Errorf("%s: CV warning = %q", tt.name, sc.CV.Warning)
		}
	}
	// Constant data with a non-zero mean has a defined CV of 0.
	if _, sc := summaryFor(t, []float64{3, 3, 3, 3}, nil, 0, 16); sc.CV.Text != "0%" || sc.CV.Reading != interpretCV(0) {
		t.Errorf("constant data: CV = %+v", sc.CV)
	}
}
