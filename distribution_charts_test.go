// distribution_charts_test.go
//
// Tests for the section 4 label inputs: each text must equal what the text
// output prints in the matching row (EMA, autocorrelation, the input-order
// warning), each must be absent under the conditions that hide that row, and
// the -T run must carry neither the autocorrelation nor the warning.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// distributionFor computes statistics for data with the given -e span and
// builds the section 4 label inputs.
func distributionFor(t *testing.T, data []float64, emaSpan int) (*Stats, distributionCharts) {
	t.Helper()
	s, err := computeStats(data, nil, 1.5, 16, 0, 0, emaSpan)
	if err != nil {
		t.Fatal(err)
	}
	return s, newDistributionCharts(s)
}

// textRow returns the text output's row of the given kind, failing the test
// when it is missing.
func textRow(t *testing.T, s *Stats, kind string) statRow {
	t.Helper()
	for _, section := range statSections(s) {
		for _, row := range section.Rows {
			if row.Kind == kind {
				return row
			}
		}
	}
	t.Fatalf("text output has no %q row", kind)
	return statRow{}
}

// TestDistributionMatchesTextRows checks every text against the text output's
// own row on ascending and descending input, with and without -e, so the
// charts repeat exactly what the text output prints.
func TestDistributionMatchesTextRows(t *testing.T) {
	ascending := []float64{1, 2, 2, 3, 5, 8, 13, 21}
	descending := []float64{21, 13, 8, 5, 3, 2, 2, 1}
	for _, tc := range []struct {
		name    string
		data    []float64
		emaSpan int
	}{
		{"ascending", ascending, 0},
		{"ascending -e", ascending, 3},
		{"descending -e", descending, 4},
	} {
		s, d := distributionFor(t, tc.data, tc.emaSpan)

		order := textRow(t, s, "inputorder").Value
		if d.SortWarning == "" || order != s.InputOrder+" WARNING: "+d.SortWarning {
			t.Errorf("%s: warning %q does not match the input order row %q", tc.name, d.SortWarning, order)
		}
		if mentionsEMA := strings.Contains(d.SortWarning, "EMA"); mentionsEMA != (tc.emaSpan > 0) {
			t.Errorf("%s: warning %q names EMA = %v, want %v", tc.name, d.SortWarning, mentionsEMA, tc.emaSpan > 0)
		}

		ac := textRow(t, s, "autocorrelation")
		if d.Autocorrelation == nil {
			t.Fatalf("%s: autocorrelation missing", tc.name)
		}
		if got := d.Autocorrelation.Text + " (" + d.Autocorrelation.Reading + ")"; got != ac.Value {
			t.Errorf("%s: autocorrelation %q, text row %q", tc.name, got, ac.Value)
		}
		if d.Autocorrelation.Value != s.Autocorrelation || d.Autocorrelation.Label != ac.Label {
			t.Errorf("%s: autocorrelation mark %+v", tc.name, d.Autocorrelation.chartMark)
		}

		sd := textRow(t, s, "stddev")
		if d.StdDev.Text != sd.Value || d.StdDev.Value != s.StdDev {
			t.Errorf("%s: SD %+v, text row %q", tc.name, d.StdDev, sd.Value)
		}

		if tc.emaSpan == 0 {
			if d.EMA != nil {
				t.Errorf("%s: EMA should be absent without -e, got %+v", tc.name, d.EMA)
			}
			continue
		}
		ema := textRow(t, s, "ema")
		if d.EMA == nil || d.EMA.Label != ema.Label || d.EMA.Text != ema.Value || d.EMA.Value != s.EMA {
			t.Errorf("%s: EMA %+v, text row %q %q", tc.name, d.EMA, ema.Label, ema.Value)
		}
	}
}

// TestDistributionAbsent checks that the warning is present only for
// ascending or descending input, and that the autocorrelation is absent below
// 3 values and at zero standard deviation, where the text output omits it.
func TestDistributionAbsent(t *testing.T) {
	for _, tc := range []struct {
		name     string
		data     []float64
		warning  bool
		autocorr bool
	}{
		{"unordered", []float64{3, 1, 2, 5, 4}, false, true},
		{"constant", []float64{7, 7, 7, 7}, false, false},
		{"two values", []float64{1, 2}, true, false},
	} {
		s, d := distributionFor(t, tc.data, 0)
		if (d.SortWarning != "") != tc.warning {
			t.Errorf("%s (%s): warning %q, want present = %v", tc.name, s.InputOrder, d.SortWarning, tc.warning)
		}
		if (d.Autocorrelation != nil) != tc.autocorr {
			t.Errorf("%s: autocorrelation present = %v, want %v", tc.name, d.Autocorrelation != nil, tc.autocorr)
		}
	}
}

// TestDistributionCLITrim runs --html -T on ascending input and checks that
// the report carries neither the autocorrelation nor the sort warning, since
// -T sorts the data and hides both rows.
func TestDistributionCLITrim(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "data.txt")
	var lines []string
	for i := 1; i <= 40; i++ {
		lines = append(lines, formatFloat(float64(i*i)))
	}
	if err := os.WriteFile(input, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		args []string
		trim bool
	}{
		{[]string{"--html", input}, false},
		{[]string{"--html", "-T", "5", input}, true},
	} {
		if _, stderr, err := runStatsIn(t, dir, "", tc.args...); err != nil {
			t.Fatalf("%v: %v\n%s", tc.args, err, stderr)
		}
		page, err := os.ReadFile(filepath.Join(dir, "data.html"))
		if err != nil {
			t.Fatal(err)
		}
		d, ok := reportJSON(t, string(page))["distribution"].(map[string]any)
		if !ok {
			t.Fatalf("%v: no distribution object", tc.args)
		}
		_, hasWarning := d["sortWarning"]
		_, hasAutocorr := d["autocorrelation"]
		if hasWarning == tc.trim || hasAutocorr == tc.trim {
			t.Errorf("%v: warning %v, autocorrelation %v; want both %v", tc.args, hasWarning, hasAutocorr, !tc.trim)
		}
		if _, ok := d["stdDev"]; !ok {
			t.Errorf("%v: stdDev missing", tc.args)
		}
	}
}
