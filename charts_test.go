// charts_test.go
//
// Tests for the chart aggregates in charts.go: each payload function at small
// and large n and on degenerate data, the histogram slider settings, the
// gating under -T and zero spread, and a 1,000,000-value build that keeps the
// report data block under 1MB.
package main

import (
	"encoding/json"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"testing"
	"time"
)

// chartSizes are the counts every chart function is exercised at.
var chartSizes = []int{1, 2, 3, 20, 1001}

// shuffledData returns n distinct values 1..n in a fixed pseudo-random order.
func shuffledData(n int) []float64 {
	r := rand.New(rand.NewPCG(1, uint64(n)))
	data := make([]float64, n)
	for i, j := range r.Perm(n) {
		data[i] = float64(j + 1)
	}
	return data
}

// sortedCopy returns an ascending copy of data.
func sortedCopy(data []float64) []float64 {
	s := slices.Clone(data)
	sort.Float64s(s)
	return s
}

// sum returns the total of counts.
func sum(counts []int) int {
	total := 0
	for _, c := range counts {
		total += c
	}
	return total
}

// emaClose reports whether two EMA values agree to 1e-12 relative. The same
// recurrence can differ in the last bit between call sites, because Go may
// fuse its multiply-add into an FMA differently where it is inlined.
func emaClose(got, want float64) bool {
	return math.Abs(got-want) <= 1e-12*math.Max(1, math.Abs(want))
}

// TestHistogramChart checks the fine bins at every size and the cases with
// nothing to draw.
func TestHistogramChart(t *testing.T) {
	for _, n := range chartSizes {
		h := newHistogramChart(sortedCopy(shuffledData(n)))
		if n < 2 {
			if h != nil {
				t.Errorf("n=%d: expected no histogram", n)
			}
			continue
		}
		if h == nil || len(h.Counts) != fineBins || sum(h.Counts) != n {
			t.Fatalf("n=%d: histogram %+v", n, h)
		}
		// Values 1..n: the first bin holds those below 1 + w, the last those
		// from n - w up, with w = (n-1)/960.
		w := float64(n-1) / fineBins
		first, last := 0, 0
		for v := 1.0; v <= float64(n); v++ {
			if v < 1+w {
				first++
			}
			if v >= float64(n)-w {
				last++
			}
		}
		if h.Min != 1 || h.Max != float64(n) || h.Counts[0] != first || h.Counts[fineBins-1] != last {
			t.Errorf("n=%d: edge bins %v..%v hold %d and %d, want %d and %d", n, h.Min, h.Max, h.Counts[0], h.Counts[fineBins-1], first, last)
		}
		if !slices.Equal(h.Offered, offeredBinCounts(n)) || !slices.Contains(h.Offered, h.Default) || h.Default > maxDefaultBins {
			t.Errorf("n=%d: offered %v, default %d", n, h.Offered, h.Default)
		}
	}
	if h := newHistogramChart([]float64{4, 4, 4}); h != nil {
		t.Errorf("constant data: expected no histogram")
	}
	// IQR = 0 with spread falls back to Sturges: ceil(log2 8) + 1 = 4.
	h := newHistogramChart([]float64{1, 5, 5, 5, 5, 5, 5, 9})
	if h == nil || h.Default != 4 || h.Counts[0] != 1 || h.Counts[480] != 6 || h.Counts[959] != 1 {
		t.Errorf("IQR=0: histogram %+v", h)
	}
}

// TestOfferedBinCounts checks the slider counts are the divisors of 960 no
// larger than n.
func TestOfferedBinCounts(t *testing.T) {
	tests := []struct {
		n    int
		want []int
	}{
		{1, []int{1}},
		{2, []int{1, 2}},
		{4, []int{1, 2, 3, 4}},
		{7, []int{1, 2, 3, 4, 5, 6}},
		{20, []int{1, 2, 3, 4, 5, 6, 8, 10, 12, 15, 16, 20}},
	}
	for _, tt := range tests {
		if got := offeredBinCounts(tt.n); !slices.Equal(got, tt.want) {
			t.Errorf("offeredBinCounts(%d) = %v, want %v", tt.n, got, tt.want)
		}
	}
	all := offeredBinCounts(1_000_000)
	if len(all) != 28 || all[len(all)-1] != fineBins {
		t.Errorf("offeredBinCounts(1e6) = %v, want the 28 divisors of 960", all)
	}
}

// TestDefaultBinCount checks Freedman-Diaconis, the Sturges fallback and the
// clamps that keep an infinite ratio away from int().
func TestDefaultBinCount(t *testing.T) {
	tests := []struct {
		name       string
		n          int
		valueRange float64
		iqr        float64
		want       int
	}{
		{"FD", 1000, 100, 10, 50},                   // 100 * 10 / 20
		{"FD rounds up", 8, 10, 3, 4},               // 10 * 2 / 6 = 3.33
		{"Sturges", 1000, 100, 0, 11},               // ceil(9.97) + 1
		{"Sturges n=2", 2, 1, 0, 2},                 // 1 + 1
		{"tiny IQR", 1000, 1e300, 1e-300, fineBins}, // ratio overflows
		{"subnormal IQR", 1000, 1, math.SmallestNonzeroFloat64, fineBins},
		{"huge IQR", 1000, 1, math.Inf(1), 1}, // ratio is 0
	}
	for _, tt := range tests {
		if got := defaultBinCount(tt.n, tt.valueRange, tt.iqr); got != tt.want {
			t.Errorf("%s: defaultBinCount = %d, want %d", tt.name, got, tt.want)
		}
	}
}

// TestSnapBinCount checks snapping to the nearest offered count no larger
// than 120, ties going to the smaller, and limits for small n.
func TestSnapBinCount(t *testing.T) {
	tests := []struct {
		want  int
		n     int
		snaps int
	}{
		{7, 1000, 6},     // 6 and 8 tie
		{9, 1000, 8},     // 8 and 10 tie
		{11, 1000, 10},   // 10 and 12 tie
		{14, 1000, 15},   // nearest
		{50, 1000, 48},   // nearest
		{110, 1000, 120}, // nearest
		{500, 1000, 120}, // capped at 120
		{960, 1000, 120},
		{1, 1000, 1},
		{4, 2, 2},    // only 1 and 2 offered
		{4, 3, 3},    // 1, 2, 3
		{11, 4, 4},   // 1..4
		{2, 1, 1},    // only 1
		{50, 40, 40}, // offered stops at n
	}
	for _, tt := range tests {
		got := snapBinCount(tt.want, offeredBinCounts(tt.n))
		if got != tt.snaps {
			t.Errorf("snapBinCount(%d, n=%d) = %d, want %d", tt.want, tt.n, got, tt.snaps)
		}
	}
}

// TestTrendlineChart checks raw points, bucket bounds and statistics, and
// that the EMA at each point or bucket end matches calculateEMA on prefixes.
func TestTrendlineChart(t *testing.T) {
	for _, n := range chartSizes {
		data := shuffledData(n)
		tr := newTrendlineChart(data, 5)
		if n < 2 {
			if tr != nil {
				t.Errorf("n=%d: expected no trendline", n)
			}
			continue
		}
		if n <= maxRawPoints {
			if tr.Mode != "raw" || !slices.Equal(tr.Values, data) || len(tr.EMA) != n || tr.Mean != nil {
				t.Errorf("n=%d: raw trendline %+v", n, tr)
				continue
			}
			for i := range n {
				if !emaClose(tr.EMA[i], calculateEMA(data[:i+1], 5)) {
					t.Errorf("n=%d: EMA[%d] = %v, want %v", n, i, tr.EMA[i], calculateEMA(data[:i+1], 5))
				}
			}
			continue
		}
		if tr.Mode != "buckets" || tr.Values != nil || len(tr.Mean) != trendBuckets || len(tr.Min) != trendBuckets || len(tr.Max) != trendBuckets || len(tr.EMA) != trendBuckets {
			t.Fatalf("n=%d: bucketed trendline has wrong shape", n)
		}
		covered := 0
		for i := range trendBuckets {
			start, end := trendBounds(i, n, trendBuckets)
			if start != covered {
				t.Fatalf("n=%d: bucket %d starts at %d, want %d", n, i, start, covered)
			}
			covered = end
			chunk := data[start:end]
			var total float64
			for _, v := range chunk {
				total += v
			}
			if tr.Mean[i] != total/float64(len(chunk)) || tr.Min[i] != slices.Min(chunk) || tr.Max[i] != slices.Max(chunk) {
				t.Errorf("n=%d: bucket %d = %v/%v/%v", n, i, tr.Mean[i], tr.Min[i], tr.Max[i])
			}
			if want := calculateEMA(data[:end], 5); !emaClose(tr.EMA[i], want) {
				t.Errorf("n=%d: bucket %d EMA = %v, want %v", n, i, tr.EMA[i], want)
			}
		}
		if covered != n {
			t.Errorf("n=%d: buckets cover %d values", n, covered)
		}
		if !emaClose(tr.EMA[trendBuckets-1], calculateEMA(data, 5)) {
			t.Errorf("n=%d: last EMA is not the reported EMA", n)
		}
	}
	if tr := newTrendlineChart(shuffledData(20), 0); tr.EMA != nil {
		t.Errorf("EMA present without -e")
	}
	if tr := newTrendlineChart([]float64{3, 3, 3}, 5); tr != nil {
		t.Errorf("constant data: expected no trendline")
	}
	sorted := sortedCopy(shuffledData(1001))
	if tr := newTrendlineChart(sorted, 0); tr.Mean[0] >= tr.Mean[trendBuckets-1] {
		t.Errorf("sorted input: bucket means should rise")
	}
}

// TestECDFChart checks exact values up to 1,000 and quantiles above, which
// must match the reported percentiles exactly.
func TestECDFChart(t *testing.T) {
	for _, n := range chartSizes {
		sorted := sortedCopy(shuffledData(n))
		e := newECDFChart(sorted)
		switch {
		case n < 2:
			if e != nil {
				t.Errorf("n=%d: expected no ECDF", n)
			}
		case n <= maxRawPoints:
			if e.Mode != "exact" || !slices.Equal(e.Values, sorted) {
				t.Errorf("n=%d: ECDF %+v", n, e)
			}
		default:
			if e.Mode != "quantiles" || len(e.Values) != ecdfSteps+1 {
				t.Fatalf("n=%d: ECDF mode %s with %d values", n, e.Mode, len(e.Values))
			}
			if e.Values[0] != sorted[0] || e.Values[ecdfSteps] != sorted[n-1] || e.Values[950] != calculatePercentile(sorted, 0.95) || e.Values[500] != calculatePercentile(sorted, 0.5) {
				t.Errorf("n=%d: quantiles do not match the percentiles", n)
			}
			if !slices.IsSorted(e.Values) {
				t.Errorf("n=%d: quantiles not ascending", n)
			}
		}
	}
	if e := newECDFChart([]float64{2, 2}); e != nil {
		t.Errorf("constant data: expected no ECDF")
	}
}

// TestECDFMarks checks the count of values at or below each mark against a
// direct count, on both sides of the exact/quantile boundary, with repeated
// values where every percentile shares one value and the share is far from p.
func TestECDFMarks(t *testing.T) {
	ties := func(zeros int) []float64 {
		return append(make([]float64, zeros), 1)
	}
	for _, tc := range []struct {
		name string
		data []float64
		mode string
	}{
		{"999 zeros and a one", ties(999), "exact"},
		{"1000 zeros and a one", ties(1000), "quantiles"},
		{"shuffled 5000", shuffledData(5000), "quantiles"},
		{"shuffled 20", shuffledData(20), "exact"},
	} {
		s, err := computeStats(tc.data, []float64{10, 50, 90}, 1.5, 16, 0, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		e := newChartData(tc.data, s).ECDF
		if e == nil || e.Mode != tc.mode {
			t.Fatalf("%s: ECDF %+v, want mode %s", tc.name, e, tc.mode)
		}
		values := map[float64]float64{10: s.CustomPercentiles[10], 25: s.Q1, 50: s.Median, 75: s.Q3, 90: s.CustomPercentiles[90], 95: s.P95, 99: s.P99}
		var ps []float64
		for _, m := range e.Marks {
			ps = append(ps, m.P)
			want := 0
			for _, v := range tc.data {
				if v <= values[m.P] {
					want++
				}
			}
			if m.AtOrBelow != want {
				t.Errorf("%s: p%v has %d at or below, want %d", tc.name, m.P, m.AtOrBelow, want)
			}
		}
		if !slices.Equal(ps, []float64{10, 25, 50, 75, 90, 95, 99}) {
			t.Errorf("%s: mark percentiles %v", tc.name, ps)
		}
	}
	for _, zeros := range []int{999, 1000} {
		s, _ := computeStats(ties(zeros), nil, 1.5, 16, 0, 0, 0)
		for _, m := range newChartData(ties(zeros), s).ECDF.Marks {
			if m.AtOrBelow != zeros {
				t.Errorf("%d zeros: p%v has %d at or below, want %d", zeros, m.P, m.AtOrBelow, zeros)
			}
		}
	}
}

// TestQQChart checks the ranks, the order statistics and normal quantiles,
// and the quartile reference points.
func TestQQChart(t *testing.T) {
	for _, n := range chartSizes {
		sorted := sortedCopy(shuffledData(n))
		q := newQQChart(sorted)
		if n < 2 {
			if q != nil {
				t.Errorf("n=%d: expected no Q-Q", n)
			}
			continue
		}
		ranks := qqRanks(n)
		if ranks[0] != 1 || ranks[len(ranks)-1] != n || !slices.IsSorted(ranks) || len(slices.Compact(slices.Clone(ranks))) != len(ranks) {
			t.Errorf("n=%d: ranks not ascending and distinct from 1 to n", n)
		}
		if n <= maxRawPoints && len(ranks) != n {
			t.Errorf("n=%d: %d ranks, want every rank", n, len(ranks))
		}
		if n > maxRawPoints && len(ranks) > qqPoints {
			t.Errorf("n=%d: %d ranks, want at most %d", n, len(ranks), qqPoints)
		}
		for j, rank := range ranks {
			if q.X[j] != sorted[rank-1] {
				t.Errorf("n=%d: X[%d] = %v, want x_(%d) = %v", n, j, q.X[j], rank, sorted[rank-1])
			}
			want := math.Sqrt2 * math.Erfinv(2*(float64(rank)-0.5)/float64(n)-1)
			if math.Abs(q.Z[j]-want) > 1e-6 {
				t.Errorf("n=%d: Z[%d] = %v, want %v", n, j, q.Z[j], want)
			}
		}
		// Independently of qqRanks' own arithmetic, each evenly spaced target
		// must be served by the rank whose plotting position (i-0.5)/n is nearest
		// to it, so a rank formula off by one fails here.
		if n > maxRawPoints {
			inSet := make(map[int]bool, len(ranks))
			for _, r := range ranks {
				inSet[r] = true
			}
			pos := func(r int) float64 { return (float64(r) - 0.5) / float64(n) }
			zFirst := math.Sqrt2 * math.Erfinv(2*pos(1)-1)
			for k := 1; k < qqPoints-1; k++ {
				z := zFirst + float64(k)*(-2*zFirst)/float64(qqPoints-1)
				target := 0.5 * (1 + math.Erf(z/math.Sqrt2))
				best := 1
				for r := max(1, int(target*float64(n))-2); r <= min(n, int(target*float64(n))+3); r++ {
					if math.Abs(pos(r)-target) < math.Abs(pos(best)-target) {
						best = r
					}
				}
				if !inSet[best] {
					t.Errorf("n=%d: target %d (p=%v) wants rank %d, which is missing", n, k, target, best)
					break
				}
			}
		}
		if q.Z[0] != -q.Z[len(q.Z)-1] {
			t.Errorf("n=%d: Z not symmetric: %v, %v", n, q.Z[0], q.Z[len(q.Z)-1])
		}
		if q.Q1 != calculatePercentile(sorted, 0.25) || q.Q3 != calculatePercentile(sorted, 0.75) || q.ZQ1 != -0.67449 || q.ZQ3 != 0.67449 {
			t.Errorf("n=%d: reference points %v@%v, %v@%v", n, q.Q1, q.ZQ1, q.Q3, q.ZQ3)
		}
	}
	if q := newQQChart([]float64{1, 1, 1}); q != nil {
		t.Errorf("constant data: expected no Q-Q")
	}
}

// TestLagChart checks dots, the switch to the grid, grid placement and that
// grid counts sum to n-1.
func TestLagChart(t *testing.T) {
	for _, n := range append(chartSizes, maxLagDots+1, maxLagDots+2) {
		data := shuffledData(n)
		l := newLagChart(data, 1, float64(n))
		switch {
		case n < 2:
			if l != nil {
				t.Errorf("n=%d: expected no lag chart", n)
			}
		case n-1 <= maxLagDots:
			if l.Mode != "dots" || !slices.Equal(l.Values, data) || l.Grid != nil {
				t.Errorf("n=%d: lag chart mode %s", n, l.Mode)
			}
		default:
			if l.Mode != "grid" || l.Values != nil || l.Grid.Size != lagGridSize || len(l.Grid.Counts) != lagGridSize*lagGridSize || sum(l.Grid.Counts) != n-1 {
				t.Errorf("n=%d: grid %+v", n, l.Grid)
			}
		}
	}
	// Alternating min and max puts every pair in the top-left or bottom-right
	// cell: low x[i] with high x[i+1] is row 63, column 0.
	alt := make([]float64, 6001)
	for i := range alt {
		alt[i] = float64(i % 2)
	}
	g := newLagChart(alt, 0, 1).Grid
	last := lagGridSize - 1
	if g.Counts[last*lagGridSize] != 3000 || g.Counts[last] != 3000 || sum(g.Counts) != 6000 {
		t.Errorf("alternating grid: [63][0]=%d [0][63]=%d", g.Counts[last*lagGridSize], g.Counts[last])
	}
	// A 0, 1, 0.5 cycle is asymmetric: (0,1) lands in row 63, column 0, and
	// nothing ever lands in row 0, column 63, so a transposed grid fails.
	cyc := make([]float64, 6001)
	for i := range cyc {
		cyc[i] = []float64{0, 1, 0.5}[i%3]
	}
	g = newLagChart(cyc, 0, 1).Grid
	if g.Counts[last*lagGridSize] != 2000 || g.Counts[last] != 0 {
		t.Errorf("cycle grid: [63][0]=%d, want 2000; [0][63]=%d, want 0", g.Counts[last*lagGridSize], g.Counts[last])
	}
	if l := newLagChart([]float64{5, 5, 5}, 5, 5); l != nil {
		t.Errorf("constant data: expected no lag chart")
	}
}

// TestBoxChart checks the fences, the whiskers against a brute-force search,
// and that the outliers match the Outliers statistic.
func TestBoxChart(t *testing.T) {
	data := append(shuffledData(20), -40, 90, 90, 200)
	s, err := computeStats(data, nil, 1.5, 16, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	b := newBoxChart(sortedCopy(data), 1.5)
	if b.Q1 != s.Q1 || b.Q3 != s.Q3 || b.Median != s.Median || b.LowerFence != s.Q1-1.5*s.IQR || b.UpperFence != s.Q3+1.5*s.IQR {
		t.Errorf("box quartiles or fences differ from the statistics: %+v", b)
	}
	lowW, highW := math.Inf(1), math.Inf(-1)
	for _, v := range data {
		if v >= b.LowerFence && v < lowW {
			lowW = v
		}
		if v <= b.UpperFence && v > highW {
			highW = v
		}
	}
	if b.LowerWhisker != lowW || b.UpperWhisker != highW {
		t.Errorf("whiskers %v..%v, want %v..%v", b.LowerWhisker, b.UpperWhisker, lowW, highW)
	}
	if b.Outliers.Binned || !slices.Equal(b.Outliers.Values, []float64{-40, 90, 200}) || !slices.Equal(b.Outliers.Counts, []int{1, 2, 1}) {
		t.Errorf("outliers %+v, statistic %v", b.Outliers, s.Outliers)
	}

	// IQR = 0 with spread: the whiskers collapse onto the box.
	b = newBoxChart([]float64{1, 5, 5, 5, 5, 5, 5, 9}, 1.5)
	if b.LowerWhisker != 5 || b.UpperWhisker != 5 || !slices.Equal(b.Outliers.Values, []float64{1, 9}) {
		t.Errorf("IQR=0: %+v", b)
	}
	// No outliers: whiskers at the extremes, empty (not null) lists.
	for _, n := range chartSizes {
		b := newBoxChart(sortedCopy(shuffledData(n)), 1.5)
		if n < 2 {
			if b != nil {
				t.Errorf("n=%d: expected no box plot", n)
			}
			continue
		}
		if b.LowerWhisker != 1 || b.UpperWhisker != float64(n) || b.Outliers.Values == nil || len(b.Outliers.Values) != 0 {
			t.Errorf("n=%d: %+v", n, b)
		}
	}
	if b := newBoxChart([]float64{7, 7}, 1.5); b != nil {
		t.Errorf("constant data: expected no box plot")
	}
}

// TestBoxOutliersBinned checks that more than 1,000 distinct outliers are
// binned into at most 1,000 positions within the outlier range, keeping
// every outlier counted.
func TestBoxOutliersBinned(t *testing.T) {
	var data []float64
	for i := range 20000 {
		data = append(data, float64(i%10))
	}
	for i := range 1500 {
		data = append(data, 1000+float64(i), -1000-float64(i)/2)
	}
	sorted := sortedCopy(data)
	b := newBoxChart(sorted, 1.5)
	o := b.Outliers
	if !o.Binned || len(o.Values) > maxOutlierDots || len(o.Values) != len(o.Counts) || sum(o.Counts) != 3000 {
		t.Fatalf("binned outliers: binned %v, %d positions, %d counted", o.Binned, len(o.Values), sum(o.Counts))
	}
	if o.Values[0] < sorted[0] || o.Values[len(o.Values)-1] > sorted[len(sorted)-1] || !slices.IsSorted(o.Values) {
		t.Errorf("bin centers outside the outlier range: %v..%v", o.Values[0], o.Values[len(o.Values)-1])
	}
}

// chartsFor computes statistics for data and builds its chart payloads.
func chartsFor(t *testing.T, data []float64, emaSpan int) chartData {
	t.Helper()
	s, err := computeStats(data, nil, 1.5, 16, 0, 0, emaSpan)
	if err != nil {
		t.Fatal(err)
	}
	return newChartData(data, s)
}

// TestChartDataGating checks which payloads newChartData emits for
// degenerate data, zero SD and -T.
func TestChartDataGating(t *testing.T) {
	if c := chartsFor(t, []float64{42}, 0); c != (chartData{}) {
		t.Errorf("n=1: expected no charts, got %+v", c)
	}
	if c := chartsFor(t, []float64{3, 3, 3, 3}, 0); c != (chartData{}) {
		t.Errorf("constant data: expected no charts, got %+v", c)
	}
	c := chartsFor(t, []float64{1, 2}, 0)
	if c.Histogram == nil || c.Trendline == nil || c.ECDF == nil || c.QQ == nil || c.Box == nil || c.Lag != nil {
		t.Errorf("n=2: expected every chart but lag-1, got %+v", c)
	}
	c = chartsFor(t, shuffledData(20), 4)
	if c.Histogram == nil || c.Trendline == nil || c.Trendline.EMA == nil || c.ECDF == nil || c.QQ == nil || c.Box == nil || c.Lag == nil {
		t.Errorf("n=20: expected every chart, got %+v", c)
	}
	// Ten copies of 0.1 sum to just under 1, leaving a non-zero SD with no
	// spread; nothing may divide by the zero range.
	tenths := slices.Repeat([]float64{0.1}, 10)
	if c := chartsFor(t, tenths, 0); c != (chartData{}) {
		t.Errorf("repeated 0.1: expected no charts, got %+v", c)
	}
	if _, err := json.Marshal(chartsFor(t, tenths, 0)); err != nil {
		t.Errorf("repeated 0.1: %v", err)
	}

	// -T: the order-based payloads are dropped, the rest stay.
	s, err := computeStats(sortedCopy(shuffledData(20)), nil, 1.5, 16, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	s.TrimDatasetPct = 10
	c = newChartData(sortedCopy(shuffledData(20)), s)
	if c.Trendline != nil || c.Lag != nil || c.Histogram == nil || c.QQ == nil {
		t.Errorf("-T: trendline %v, lag %v, histogram %v, qq %v", c.Trendline, c.Lag, c.Histogram, c.QQ)
	}
}

// TestChartsCLITrim checks through the CLI that -T removes the trendline and
// lag-1 payloads from the report while the other charts remain.
func TestChartsCLITrim(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "data.txt")
	content := ""
	for _, v := range shuffledData(50) {
		content += formatFloat(v) + "\n"
	}
	if err := os.WriteFile(input, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		args    []string
		ordered bool
	}{{nil, true}, {[]string{"-T", "10"}, false}} {
		if _, stderr, err := runStatsIn(t, dir, "", append(append([]string{"--html"}, tc.args...), input)...); err != nil {
			t.Fatalf("%v: %v\n%s", tc.args, err, stderr)
		}
		page, err := os.ReadFile(filepath.Join(dir, "data.html"))
		if err != nil {
			t.Fatal(err)
		}
		charts := reportJSON(t, string(page))["charts"].(map[string]any)
		for _, key := range []string{"histogram", "ecdf", "qq", "box"} {
			if charts[key] == nil {
				t.Errorf("%v: %s missing", tc.args, key)
			}
		}
		for _, key := range []string{"trendline", "lag"} {
			if (charts[key] != nil) != tc.ordered {
				t.Errorf("%v: %s present = %v, want %v", tc.args, key, charts[key] != nil, tc.ordered)
			}
		}
	}
}

// TestChartPayloadOneMillion runs the full report build on 1,000,000 normal
// values with full-precision floats, the largest payload, and checks the
// data block stays under 1MB and the Q-Q tails reach the extremes. The
// elapsed time is logged, not asserted.
func TestChartPayloadOneMillion(t *testing.T) {
	if testing.Short() {
		t.Skip("1M-value report build skipped in -short mode")
	}
	r := rand.New(rand.NewPCG(7, 11))
	data := make([]float64, 1_000_000)
	for i := range data {
		data[i] = r.NormFloat64()*100 + 500
	}
	start := time.Now()
	s, err := computeStats(data, nil, 1.5, 16, 0, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	statsElapsed := time.Since(start)
	page, err := buildReportPage(s, data, "data.txt", time.Now(), 16, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("computeStats %v, buildReportPage %v, data block %d bytes", statsElapsed, time.Since(start)-statsElapsed, len(page.Data))
	if len(page.Data) >= 1_000_000 {
		t.Errorf("data block is %d bytes, want under 1MB", len(page.Data))
	}

	c := page.Report.Charts
	q := c.QQ
	if q.X[0] != s.Min || q.X[len(q.X)-1] != s.Max || math.Abs(q.Z[0]) <= 4.5 || math.Abs(q.Z[len(q.Z)-1]) <= 4.5 {
		t.Errorf("Q-Q tails: x %v..%v, z %v..%v", q.X[0], q.X[len(q.X)-1], q.Z[0], q.Z[len(q.Z)-1])
	}
	if c.Trendline.Mode != "buckets" || len(c.Trendline.EMA) != trendBuckets || c.Trendline.EMA[trendBuckets-1] != s.EMA {
		t.Errorf("trendline: mode %s, last EMA %v, want %v", c.Trendline.Mode, c.Trendline.EMA[trendBuckets-1], s.EMA)
	}
	if c.Lag.Mode != "grid" || sum(c.Lag.Grid.Counts) != len(data)-1 {
		t.Errorf("lag grid: mode %s, %d pairs", c.Lag.Mode, sum(c.Lag.Grid.Counts))
	}
	if !c.Box.Outliers.Binned || sum(c.Box.Outliers.Counts) != len(s.Outliers) {
		t.Errorf("box outliers: binned %v, %d counted, %d outliers", c.Box.Outliers.Binned, sum(c.Box.Outliers.Counts), len(s.Outliers))
	}
	if sum(c.Histogram.Counts) != len(data) || c.ECDF.Mode != "quantiles" {
		t.Errorf("histogram total %d, ECDF mode %s", sum(c.Histogram.Counts), c.ECDF.Mode)
	}
}
