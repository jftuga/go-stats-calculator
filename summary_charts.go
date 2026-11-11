// summary_charts.go
//
// Inputs for the summary charts of report sections 1 to 3: the duplicate bar,
// the min-to-max range strip, the central-tendency number line, the percentile
// ladder and the skewness, kurtosis and CV gauges. Each value is a raw number
// for geometry plus its formatFloat text, so chart labels match the text
// output, and each gauge carries the N/A reason the statistic's own code
// implies (it reports 0 where the statistic is undefined).
package main

import (
	"sort"
	"strconv"
)

// chartMark is one labeled value drawn on a summary chart. Text is the value
// as the text output formats it.
type chartMark struct {
	Key   string  `json:"key"`
	Label string  `json:"label"`
	Value float64 `json:"value"`
	Text  string  `json:"text"`
}

// percentileMark is a percentile drawn on a chart: its value, and P, the
// percentile itself (25 for Q1), which places it on a cumulative scale.
type percentileMark struct {
	chartMark
	P float64 `json:"p"`
}

// chartInterval is a labeled range drawn on a summary chart. Text is the
// range as "[low, high]" with both ends formatted like the text output.
type chartInterval struct {
	Label string  `json:"label"`
	Low   float64 `json:"low"`
	High  float64 `json:"high"`
	Text  string  `json:"text"`
}

// modalBinMark is the densest of the -b equal-width bins, reported in place
// of a mode when no value repeats. Bins is the bin count actually used, which
// binData caps at n.
type modalBinMark struct {
	chartInterval
	Bins  int `json:"bins"`
	Count int `json:"count"`
}

// distinctChart is the duplicate bar: distinct values against values that
// repeat an earlier one. DuplicatedText is the duplicated share in percent.
type distinctChart struct {
	Distinct       int    `json:"distinct"`
	Count          int    `json:"count"`
	DuplicatedText string `json:"duplicatedText"`
}

// centerChart is the central-tendency number line. Marks holds the centers
// that the text output shows as single values; a multi-mode dataset has no
// Mode mark and ModeCount says how many modes it has. CI and ModalBin are nil
// when the text output omits them. Quartiles is shaded behind for scale.
type centerChart struct {
	Marks     []chartMark    `json:"marks"`
	CI        *chartInterval `json:"ci,omitempty"`
	ModalBin  *modalBinMark  `json:"modalBin,omitempty"`
	ModeCount int            `json:"modeCount"`
	Quartiles chartInterval  `json:"quartiles"`
}

// gaugeChart is a statistic drawn on a gauge banded by its interpret*
// thresholds, which Thresholds carries so the page never restates them. NA is
// the reason the statistic is undefined, in which case Text and Reading are
// empty; Warning qualifies a defined value.
type gaugeChart struct {
	Value      float64    `json:"value"`
	Thresholds [2]float64 `json:"thresholds"`
	Text       string     `json:"text,omitempty"`
	Reading    string     `json:"reading,omitempty"`
	NA         string     `json:"na,omitempty"`
	Warning    string     `json:"warning,omitempty"`
}

// summaryCharts is the "summary" object of the report data block. Range is
// omitted when the data has no spread.
type summaryCharts struct {
	Distinct distinctChart    `json:"distinct"`
	Range    []chartMark      `json:"range,omitempty"`
	Center   centerChart      `json:"center"`
	Ladder   []percentileMark `json:"ladder"`
	Skewness gaugeChart       `json:"skewness"`
	Kurtosis gaugeChart       `json:"kurtosis"`
	CV       gaugeChart       `json:"cv"`
}

// newSummaryCharts builds every summary chart input from s. numBins is the -b
// value, needed to report how many bins the modal bin came from.
func newSummaryCharts(s *Stats, numBins int) summaryCharts {
	return summaryCharts{
		Distinct: distinctChart{
			Distinct:       s.Distinct,
			Count:          s.Count,
			DuplicatedText: formatFloat(float64(s.Count-s.Distinct) / float64(s.Count) * 100),
		},
		Range:    rangeMarks(s),
		Center:   newCenterChart(s, numBins),
		Ladder:   ladderMarks(s),
		Skewness: skewnessGauge(s),
		Kurtosis: kurtosisGauge(s),
		CV:       cvGauge(s),
	}
}

// mark builds a chartMark whose text is formatFloat(value).
func mark(key, label string, value float64) chartMark {
	return chartMark{Key: key, Label: label, Value: value, Text: formatFloat(value)}
}

// interval builds a chartInterval whose text is "[low, high]".
func interval(label string, low, high float64) chartInterval {
	return chartInterval{Label: label, Low: low, High: high, Text: "[" + formatFloat(low) + ", " + formatFloat(high) + "]"}
}

// rangeMarks returns min, Q1, median, Q3 and max for the range strip, or nil
// when every value is equal and there is no range to draw.
func rangeMarks(s *Stats) []chartMark {
	if s.Min == s.Max {
		return nil
	}
	return []chartMark{
		mark("min", "Min", s.Min),
		mark("q1", "Q1", s.Q1),
		mark("median", "Median", s.Median),
		mark("q3", "Q3", s.Q3),
		mark("max", "Max", s.Max),
	}
}

// newCenterChart collects the centers under the same conditions the text
// output uses: geometric mean when valid, trimmed mean under -t, a single
// mode, the modal bin when no value repeats, and the CI when valid. EMA is
// left out because a recency-weighted endpoint is not a center.
func newCenterChart(s *Stats, numBins int) centerChart {
	c := centerChart{
		Marks:     []chartMark{mark("mean", "Mean", s.Mean)},
		ModeCount: len(s.Mode),
		Quartiles: interval("Q1 to Q3", s.Q1, s.Q3),
	}
	if s.GeoMeanValid {
		c.Marks = append(c.Marks, mark("geomean", "Geometric Mean", s.GeometricMean))
	}
	if s.TrimmedMeanPct > 0 {
		c.Marks = append(c.Marks, mark("trimmedmean", "Trimmed Mean ("+formatFloat(s.TrimmedMeanPct)+"%)", s.TrimmedMean))
	}
	c.Marks = append(c.Marks, mark("median", "Median", s.Median))
	if len(s.Mode) == 1 {
		c.Marks = append(c.Marks, mark("mode", "Mode", s.Mode[0]))
	}
	if s.CIValid {
		ci := interval("95% CI", s.CI95Lower, s.CI95Upper)
		c.CI = &ci
	}
	if s.ModalBinValid {
		c.ModalBin = &modalBinMark{
			chartInterval: interval("Modal bin", s.ModalBinLow, s.ModalBinHigh),
			Bins:          min(numBins, s.Count),
			Count:         s.ModalBinCount,
		}
	}
	return c
}

// ladderMarks returns the quartiles, p95, p99 and any -p percentiles in
// ascending order of percentile. A -p value equal to 25 or 75 merges into the
// quartile rung, since both are the same percentile of the same data.
func ladderMarks(s *Stats) []percentileMark {
	values := map[float64]float64{25: s.Q1, 75: s.Q3, 95: s.P95, 99: s.P99}
	for p, v := range s.CustomPercentiles {
		if p != 25 && p != 75 {
			values[p] = v
		}
	}
	keys := make([]float64, 0, len(values))
	for p := range values {
		keys = append(keys, p)
	}
	sort.Float64s(keys)
	marks := make([]percentileMark, len(keys))
	for i, p := range keys {
		key := "p" + formatFloat(p)
		label := key
		switch p {
		case 25:
			label = "Q1 (p25)"
		case 75:
			label = "Q3 (p75)"
		}
		marks[i] = percentileMark{chartMark: mark(key, label, values[p]), P: p}
	}
	return marks
}

// gaugeNA returns the reason a moment statistic needing at least minCount
// values and a non-zero standard deviation is undefined, or "" when it is
// defined. calculateSkewness and calculateKurtosis return 0 in these cases.
func gaugeNA(s *Stats, minCount int) string {
	if s.Count < minCount {
		return "requires at least " + strconv.Itoa(minCount) + " values"
	}
	if !(s.StdDev > 0) {
		return "standard deviation is zero"
	}
	return ""
}

// skewnessGauge is the skewness gauge input; skewness needs n >= 3 and SD > 0.
func skewnessGauge(s *Stats) gaugeChart {
	g := gaugeChart{Value: s.Skewness, Thresholds: [2]float64{skewModerate, skewHigh}, NA: gaugeNA(s, 3)}
	if g.NA == "" {
		g.Text = formatFloat(s.Skewness)
		g.Reading = interpretSkewness(s.Skewness)
	}
	return g
}

// kurtosisGauge is the kurtosis gauge input; kurtosis needs n >= 4 and SD > 0.
func kurtosisGauge(s *Stats) gaugeChart {
	g := gaugeChart{Value: s.Kurtosis, Thresholds: [2]float64{kurtosisLow, kurtosisHigh}, NA: gaugeNA(s, 4)}
	if g.NA == "" {
		g.Text = formatFloat(s.Kurtosis)
		g.Reading = interpretKurtosis(s.Kurtosis)
	}
	return g
}

// cvGauge is the CV gauge input. CV is undefined when the mean is near zero
// and carries the text output's warning when the data has negative values.
func cvGauge(s *Stats) gaugeChart {
	g := gaugeChart{Value: s.CV, Thresholds: [2]float64{cvModerate, cvHigh}}
	if !s.CVValid {
		g.NA = "mean near zero"
		return g
	}
	g.Text = formatFloat(s.CV) + "%"
	g.Reading = interpretCV(s.CV)
	if s.HasNegativeData {
		g.Warning = "data set contains negative data"
	}
	return g
}
