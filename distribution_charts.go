// distribution_charts.go
//
// Text for the labels of report section 4 (Distribution), whose geometry comes
// from the bounded payloads in charts.go: the standard deviation for the
// histogram's normal curve, the final EMA for the trendline, the
// autocorrelation and its reading for the lag-1 chart, and the text output's
// sort-order warning, which the trendline and lag-1 charts repeat.
package main

// readingMark is a chart mark with the interpret* reading of its value.
type readingMark struct {
	chartMark
	Reading string `json:"reading"`
}

// distributionCharts is the "distribution" object of the report data block.
// EMA is nil without -e and Autocorrelation is nil when the text output omits
// it; SortWarning is empty unless the input arrived ascending or descending.
type distributionCharts struct {
	StdDev          chartMark    `json:"stdDev"`
	EMA             *chartMark   `json:"ema,omitempty"`
	Autocorrelation *readingMark `json:"autocorrelation,omitempty"`
	SortWarning     string       `json:"sortWarning,omitempty"`
}

// newDistributionCharts builds the section 4 label inputs from s under the
// same conditions the text output uses for the matching rows.
func newDistributionCharts(s *Stats) distributionCharts {
	d := distributionCharts{
		StdDev:      mark("stddev", "Std Deviation", s.StdDev),
		SortWarning: sortOrderWarning(s),
	}
	if s.EMASpan > 0 {
		ema := mark("ema", emaLabel(s.EMASpan), s.EMA)
		d.EMA = &ema
	}
	if s.AutocorrValid {
		d.Autocorrelation = &readingMark{
			chartMark: mark("autocorrelation", "Autocorrelation", s.Autocorrelation),
			Reading:   interpretAutocorrelation(s.Autocorrelation),
		}
	}
	return d
}
