// charts.go
//
// Bounded chart aggregates for the HTML report. Each function reduces the data
// to a summary whose size does not depend on n (fine histogram bins, trendline
// buckets, ECDF quantiles, Q-Q order statistics, a lag-1 grid and box plot
// positions), so the page never receives the raw data of a large input.
// A payload that cannot be drawn (too few values, zero spread, an order-based
// chart under -T) is nil and omitted from the JSON, so no NaN or Inf reaches it.
package main

import (
	"cmp"
	"math"
	"slices"
	"sort"
)

// Size limits of the chart payloads.
const (
	fineBins       = 960  // histogram bins; 960 has 28 divisors, so slider counts merge exactly
	maxDefaultBins = 120  // largest slider count chosen as the default
	maxRawPoints   = 1000 // trendline, ECDF and Q-Q send every value up to this n
	trendBuckets   = 1000 // trendline buckets above maxRawPoints
	ecdfSteps      = 1000 // ECDF quantiles at 0.1% steps (ecdfSteps+1 values)
	qqPoints       = 1000 // Q-Q target ranks above maxRawPoints
	maxLagDots     = 5000 // lag-1 pairs drawn as dots; more become a grid
	lagGridSize    = 64   // lag-1 grid is lagGridSize x lagGridSize
	maxOutlierDots = 1000 // distinct box plot outliers; more are binned
)

// histogramChart holds 960 equal-width bin counts over [Min, Max] and the
// slider settings. Bin i covers [Min + i*w, Min + (i+1)*w) with w =
// (Max-Min)/960; the last bin also holds Max. Every offered count divides 960,
// so the client merges 960/count adjacent fine bins per bar.
type histogramChart struct {
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
	Counts  []int   `json:"counts"`
	Offered []int   `json:"offered"`
	Default int     `json:"default"`
}

// trendlineChart is the input-order sequence. Mode "raw" carries every value
// in Values; mode "buckets" carries 1,000 buckets as parallel Mean, Min and
// Max arrays, where bucket i covers input indices [round(i*n/1000),
// round((i+1)*n/1000)). EMA, present only under -e, holds the EMA after each
// raw point or at each bucket's last index.
type trendlineChart struct {
	Mode   string    `json:"mode"`
	Values []float64 `json:"values,omitempty"`
	Mean   []float64 `json:"mean,omitempty"`
	Min    []float64 `json:"min,omitempty"`
	Max    []float64 `json:"max,omitempty"`
	EMA    []float64 `json:"ema,omitempty"`
}

// ecdfChart is the empirical CDF. Mode "exact" carries every sorted value
// (value i sits at cumulative fraction (i+1)/n); mode "quantiles" carries
// 1,001 values, value i being the percentile i/1000 as calculatePercentile
// computes it, so value 950 equals the reported p95.
// Marks gives, for the ladder's percentiles and the median, how many of all
// the values lie at or below each one's value; the page places and labels the
// marks with these counts.
type ecdfChart struct {
	Mode   string     `json:"mode"`
	Values []float64  `json:"values"`
	Marks  []ecdfMark `json:"marks"`
}

// ecdfMark is the count of values at or below percentile P's value.
type ecdfMark struct {
	P         float64 `json:"p"`
	AtOrBelow int     `json:"atOrBelow"`
}

// qqChart pairs order statistics X with their standard normal quantiles Z,
// plus Q1 and Q3 against the normal quartiles for the reference line.
type qqChart struct {
	X   []float64 `json:"x"`
	Z   []float64 `json:"z"`
	Q1  float64   `json:"q1"`
	Q3  float64   `json:"q3"`
	ZQ1 float64   `json:"zq1"`
	ZQ3 float64   `json:"zq3"`
}

// lagChart is the lag-1 scatter. Mode "dots" carries the input-order Values,
// pair i being (Values[i], Values[i+1]); mode "grid" carries Grid instead.
type lagChart struct {
	Mode   string    `json:"mode"`
	Values []float64 `json:"values,omitempty"`
	Grid   *lagGrid  `json:"grid,omitempty"`
}

// lagGrid counts lag-1 pairs in Size x Size equal-width cells over [Min, Max]
// squared. Counts is row-major: the row is the bin of x[i+1] and the column
// the bin of x[i].
type lagGrid struct {
	Size   int     `json:"size"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
	Counts []int   `json:"counts"`
}

// boxOutliers holds the IQR outliers as axis positions with counts. Unbinned,
// Values are the distinct outliers and Counts their multiplicities; binned,
// Values are the centers of the non-empty bins among 1,000 equal-width bins
// spanning the lowest to the highest outlier.
type boxOutliers struct {
	Binned bool      `json:"binned"`
	Values []float64 `json:"values"`
	Counts []int     `json:"counts"`
}

// boxChart is a Tukey box plot: the quartiles, the k*IQR fences, whiskers at
// the most extreme values inside the fences, and the outliers beyond them.
type boxChart struct {
	Q1           float64     `json:"q1"`
	Median       float64     `json:"median"`
	Q3           float64     `json:"q3"`
	LowerFence   float64     `json:"lowerFence"`
	UpperFence   float64     `json:"upperFence"`
	LowerWhisker float64     `json:"lowerWhisker"`
	UpperWhisker float64     `json:"upperWhisker"`
	Outliers     boxOutliers `json:"outliers"`
}

// chartData is the "charts" object of the report data block. A nil payload is
// omitted and means the chart has nothing to draw.
type chartData struct {
	Histogram *histogramChart `json:"histogram,omitempty"`
	Trendline *trendlineChart `json:"trendline,omitempty"`
	ECDF      *ecdfChart      `json:"ecdf,omitempty"`
	QQ        *qqChart        `json:"qq,omitempty"`
	Lag       *lagChart       `json:"lag,omitempty"`
	Box       *boxChart       `json:"box,omitempty"`
}

// newChartData builds every chart payload from data in input order. It sorts
// one copy shared by the order-free payloads. The order-based trendline and
// lag-1 payloads are dropped under -T, where data arrives sorted.
func newChartData(data []float64, s *Stats) chartData {
	sorted := slices.Clone(data)
	sort.Float64s(sorted)
	trimmed := s.TrimDatasetPct > 0

	charts := chartData{
		Histogram: newHistogramChart(sorted),
		ECDF:      newECDFChart(sorted),
		Box:       newBoxChart(sorted, s.IQRMultiplier),
	}
	if charts.ECDF != nil {
		charts.ECDF.Marks = ecdfMarks(sorted, s)
	}
	if !trimmed {
		charts.Trendline = newTrendlineChart(data, s.EMASpan)
	}
	if s.StdDev > 0 {
		charts.QQ = newQQChart(sorted)
	}
	if s.AutocorrValid && !trimmed {
		charts.Lag = newLagChart(data, sorted[0], sorted[len(sorted)-1])
	}
	return charts
}

// hasSpread reports whether sorted holds at least two values and a finite,
// positive range, the condition for every value-axis chart.
func hasSpread(sorted []float64) bool {
	n := len(sorted)
	if n < 2 {
		return false
	}
	r := sorted[n-1] - sorted[0]
	return r > 0 && !math.IsInf(r, 0)
}

// binIndex maps v in [minVal, maxVal] to one of bins equal-width bins. It
// divides by the range rather than a bin width, which can underflow to 0, and
// clamps so the maximum lands in the last bin. The caller ensures a finite,
// positive range.
func binIndex(v, minVal, maxVal float64, bins int) int {
	idx := int(float64(bins) * ((v - minVal) / (maxVal - minVal)))
	return min(max(idx, 0), bins-1)
}

// newHistogramChart counts sorted into 960 equal-width bins over [min, max]
// and chooses the slider's offered and default counts. Unlike binData it
// never reduces the bin count to n. It returns nil without spread.
func newHistogramChart(sorted []float64) *histogramChart {
	if !hasSpread(sorted) {
		return nil
	}
	n := len(sorted)
	minVal, maxVal := sorted[0], sorted[n-1]
	counts := make([]int, fineBins)
	for _, v := range sorted {
		counts[binIndex(v, minVal, maxVal, fineBins)]++
	}
	offered := offeredBinCounts(n)
	iqr := calculatePercentile(sorted, 0.75) - calculatePercentile(sorted, 0.25)
	return &histogramChart{
		Min:     minVal,
		Max:     maxVal,
		Counts:  counts,
		Offered: offered,
		Default: snapBinCount(defaultBinCount(n, maxVal-minVal, iqr), offered),
	}
}

// offeredBinCounts returns the divisors of 960 no larger than n, ascending.
func offeredBinCounts(n int) []int {
	var counts []int
	for d := 1; d <= fineBins && d <= n; d++ {
		if fineBins%d == 0 {
			counts = append(counts, d)
		}
	}
	return counts
}

// defaultBinCount is the Freedman-Diaconis bin count ceil(range /
// (2*IQR*n^(-1/3))), or Sturges' ceil(log2 n) + 1 when the IQR is 0. The
// result is clamped to [1, 960] before conversion, so a tiny IQR (an infinite
// or NaN ratio) never reaches int().
func defaultBinCount(n int, valueRange, iqr float64) int {
	var raw float64
	if iqr > 0 {
		raw = valueRange * math.Cbrt(float64(n)) / (2 * iqr)
	} else {
		raw = math.Log2(float64(n)) + 1
	}
	raw = math.Ceil(raw)
	if !(raw >= 1) { // also catches NaN
		return 1
	}
	if raw > fineBins {
		return fineBins
	}
	return int(raw)
}

// snapBinCount returns the offered count nearest to want among those no
// larger than maxDefaultBins, preferring the smaller on a tie. offered is
// ascending and starts at 1, so a result always exists.
func snapBinCount(want int, offered []int) int {
	best := offered[0]
	for _, c := range offered {
		if c > maxDefaultBins {
			break
		}
		if abs(c-want) < abs(best-want) {
			best = c
		}
	}
	return best
}

// abs returns the absolute value of an int.
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// trendBounds returns the [start, end) index range of bucket i when n values
// are split into buckets chunks, using generateTrendline's floating-point
// boundaries.
func trendBounds(i, n, buckets int) (int, int) {
	step := float64(n) / float64(buckets)
	start := int(math.Round(float64(i) * step))
	end := min(int(math.Round(float64(i+1)*step)), n)
	if end <= start {
		end = start + 1
	}
	return start, end
}

// newTrendlineChart summarizes data in input order: every value up to 1,000,
// otherwise 1,000 buckets of mean, min and max. With emaSpan >= 2 it adds the
// EMA, computed over the full data with calculateEMA's recurrence, after each
// raw point or at each bucket's last index. Like the text trendline it
// returns nil for fewer than 2 values or zero spread.
func newTrendlineChart(data []float64, emaSpan int) *trendlineChart {
	n := len(data)
	if n < 2 || slices.Min(data) == slices.Max(data) {
		return nil
	}
	var ema []float64
	if emaSpan >= 2 {
		ema = emaSeries(data, emaSpan)
	}
	if n <= maxRawPoints {
		return &trendlineChart{Mode: "raw", Values: slices.Clone(data), EMA: ema}
	}

	t := &trendlineChart{
		Mode: "buckets",
		Mean: make([]float64, trendBuckets),
		Min:  make([]float64, trendBuckets),
		Max:  make([]float64, trendBuckets),
	}
	if ema != nil {
		t.EMA = make([]float64, trendBuckets)
	}
	for i := range trendBuckets {
		start, end := trendBounds(i, n, trendBuckets)
		chunk := data[start:end]
		var sum float64
		for _, v := range chunk {
			sum += v
		}
		t.Mean[i] = sum / float64(len(chunk))
		t.Min[i] = slices.Min(chunk)
		t.Max[i] = slices.Max(chunk)
		if ema != nil {
			t.EMA[i] = ema[end-1]
		}
	}
	return t
}

// emaSeries returns the EMA after each value of data, with alpha =
// 2/(span+1) and the recurrence of calculateEMA, so element i equals
// calculateEMA(data[:i+1], span).
func emaSeries(data []float64, span int) []float64 {
	alpha := 2.0 / (float64(span) + 1.0)
	series := make([]float64, len(data))
	ema := data[0]
	series[0] = ema
	for i := 1; i < len(data); i++ {
		ema = alpha*data[i] + (1-alpha)*ema
		series[i] = ema
	}
	return series
}

// ecdfMarks counts the values at or below each ladder percentile and the
// median, the marks the ECDF chart draws, in ascending order of percentile.
func ecdfMarks(sorted []float64, s *Stats) []ecdfMark {
	values := map[float64]float64{50: s.Median}
	for _, m := range ladderMarks(s) {
		values[m.P] = m.Value
	}
	marks := make([]ecdfMark, 0, len(values))
	for p, v := range values {
		n := sort.Search(len(sorted), func(i int) bool { return sorted[i] > v })
		marks = append(marks, ecdfMark{P: p, AtOrBelow: n})
	}
	slices.SortFunc(marks, func(a, b ecdfMark) int { return cmp.Compare(a.P, b.P) })
	return marks
}

// newECDFChart returns every sorted value up to 1,000, otherwise the 1,001
// percentiles at 0.1% steps. It returns nil without spread, where the curve
// is a single vertical step.
func newECDFChart(sorted []float64) *ecdfChart {
	if !hasSpread(sorted) {
		return nil
	}
	n := len(sorted)
	if n <= maxRawPoints {
		return &ecdfChart{Mode: "exact", Values: slices.Clone(sorted)}
	}
	values := make([]float64, ecdfSteps+1)
	for i := range values {
		values[i] = calculatePercentile(sorted, float64(i)/ecdfSteps)
	}
	return &ecdfChart{Mode: "quantiles", Values: values}
}

// normalQuantile is the standard normal quantile of probability p.
func normalQuantile(p float64) float64 {
	return math.Sqrt2 * math.Erfinv(2*p-1)
}

// roundZ rounds a theoretical quantile to 6 decimals, far below a pixel, so
// it marshals in a few characters instead of 17 significant digits.
func roundZ(z float64) float64 {
	return math.Round(z*1e6) / 1e6
}

// qqRanks returns the 1-based ranks plotted on the Q-Q chart: every rank up
// to n = 1,000; above that, ranks nearest to 1,000 points spaced evenly in
// theoretical-quantile space between the quantiles of rank 1 and rank n,
// deduplicated, so the tails stay visible. Ranks 1 and n are always included.
func qqRanks(n int) []int {
	if n <= maxRawPoints {
		ranks := make([]int, n)
		for i := range ranks {
			ranks[i] = i + 1
		}
		return ranks
	}
	zFirst := normalQuantile(0.5 / float64(n))
	zLast := -zFirst
	ranks := []int{1}
	for k := 1; k < qqPoints-1; k++ {
		z := zFirst + float64(k)*(zLast-zFirst)/float64(qqPoints-1)
		p := 0.5 * (1 + math.Erf(z/math.Sqrt2))
		// p = (i-0.5)/n solved for the nearest whole rank i.
		rank := min(max(int(math.Round(p*float64(n)+0.5)), 1), n)
		if rank != ranks[len(ranks)-1] {
			ranks = append(ranks, rank)
		}
	}
	if ranks[len(ranks)-1] != n {
		ranks = append(ranks, n)
	}
	return ranks
}

// newQQChart pairs the order statistics x_(i) of sorted with the normal
// quantiles of p = (i-0.5)/n at the ranks from qqRanks, and adds Q1 and Q3
// against the normal quartiles. The caller drops it when the SD is 0; it is
// also nil without spread.
func newQQChart(sorted []float64) *qqChart {
	if !hasSpread(sorted) {
		return nil
	}
	n := len(sorted)
	ranks := qqRanks(n)
	q := &qqChart{
		X:   make([]float64, len(ranks)),
		Z:   make([]float64, len(ranks)),
		Q1:  calculatePercentile(sorted, 0.25),
		Q3:  calculatePercentile(sorted, 0.75),
		ZQ1: roundZ(normalQuantile(0.25)),
		ZQ3: roundZ(normalQuantile(0.75)),
	}
	for j, rank := range ranks {
		q.X[j] = sorted[rank-1]
		q.Z[j] = roundZ(normalQuantile((float64(rank) - 0.5) / float64(n)))
	}
	return q
}

// newLagChart builds the lag-1 scatter of data in input order: the values
// themselves when there are at most 5,000 pairs, otherwise a 64x64 grid of
// pair counts over [minVal, maxVal] squared. It returns nil for fewer than 2
// values or no finite, positive range.
func newLagChart(data []float64, minVal, maxVal float64) *lagChart {
	n := len(data)
	r := maxVal - minVal
	if n < 2 || !(r > 0) || math.IsInf(r, 0) {
		return nil
	}
	if n-1 <= maxLagDots {
		return &lagChart{Mode: "dots", Values: slices.Clone(data)}
	}
	counts := make([]int, lagGridSize*lagGridSize)
	for i := 0; i < n-1; i++ {
		col := binIndex(data[i], minVal, maxVal, lagGridSize)
		row := binIndex(data[i+1], minVal, maxVal, lagGridSize)
		counts[row*lagGridSize+col]++
	}
	return &lagChart{Mode: "grid", Grid: &lagGrid{Size: lagGridSize, Min: minVal, Max: maxVal, Counts: counts}}
}

// newBoxChart computes the Tukey box plot of sorted with fences at Q1 - k*IQR
// and Q3 + k*IQR, the same rule as the Outliers statistic. It returns nil
// without spread, when a fence overflows, or when rounding in the quartile
// interpolation leaves no value inside the fences.
func newBoxChart(sorted []float64, k float64) *boxChart {
	if !hasSpread(sorted) {
		return nil
	}
	n := len(sorted)
	q1 := calculatePercentile(sorted, 0.25)
	q3 := calculatePercentile(sorted, 0.75)
	lower := q1 - k*(q3-q1)
	upper := q3 + k*(q3-q1)
	if math.IsInf(lower, 0) || math.IsInf(upper, 0) {
		return nil
	}
	// lo is the first value inside the lower fence and hi the last inside the
	// upper. With IQR = 0 the quartiles interpolate between equal values and
	// can land an ulp off them, leaving the fences with nothing between.
	lo := sort.SearchFloat64s(sorted, lower)
	hi := sort.Search(n, func(i int) bool { return sorted[i] > upper }) - 1
	if lo > hi {
		return nil
	}
	outliers := slices.Concat(sorted[:lo], sorted[hi+1:])
	return &boxChart{
		Q1:           q1,
		Median:       calculatePercentile(sorted, 0.5),
		Q3:           q3,
		LowerFence:   lower,
		UpperFence:   upper,
		LowerWhisker: sorted[lo],
		UpperWhisker: sorted[hi],
		Outliers:     newBoxOutliers(outliers),
	}
}

// newBoxOutliers groups sorted outliers into distinct values with counts, or,
// with more than 1,000 distinct values, into the non-empty bins among 1,000
// equal-width bins spanning the lowest to the highest outlier.
func newBoxOutliers(sorted []float64) boxOutliers {
	out := boxOutliers{Values: []float64{}, Counts: []int{}}
	for i, v := range sorted {
		if i > 0 && v == sorted[i-1] {
			out.Counts[len(out.Counts)-1]++
			continue
		}
		out.Values = append(out.Values, v)
		out.Counts = append(out.Counts, 1)
	}
	if len(out.Values) <= maxOutlierDots {
		return out
	}

	minVal, maxVal := sorted[0], sorted[len(sorted)-1]
	width := (maxVal - minVal) / maxOutlierDots
	bins := make([]int, maxOutlierDots)
	for _, v := range sorted {
		bins[binIndex(v, minVal, maxVal, maxOutlierDots)]++
	}
	out = boxOutliers{Binned: true, Values: []float64{}, Counts: []int{}}
	for i, c := range bins {
		if c > 0 {
			out.Values = append(out.Values, minVal+(float64(i)+0.5)*width)
			out.Counts = append(out.Counts, c)
		}
	}
	return out
}
