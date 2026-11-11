package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// runStatsIn executes the compiled binary in dir with the given stdin and
// returns stdout and stderr separately. Running in a temp directory keeps
// reports written for stdin out of the repository.
func runStatsIn(t *testing.T, dir, stdin string, args ...string) (string, string, error) {
	t.Helper()
	cmd := exec.Command(statsBin, args...)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// reportJSON extracts and decodes the report-data block from a rendered page.
func reportJSON(t *testing.T, page string) map[string]any {
	t.Helper()
	_, rest, found := strings.Cut(page, `<script type="application/json" id="report-data">`)
	if !found {
		t.Fatalf("report-data block not found in page")
	}
	block, _, found := strings.Cut(rest, "</script>")
	if !found {
		t.Fatalf("report-data block is not closed")
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(block), &data); err != nil {
		t.Fatalf("report-data block is not valid JSON: %v\n%s", err, block)
	}
	return data
}

// dirEntries returns the sorted names in dir.
func dirEntries(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	return names
}

// TestHTMLOutputPath checks how the report path is derived from the input.
func TestHTMLOutputPath(t *testing.T) {
	now := time.Date(2026, 11, 10, 14, 30, 5, 0, time.Local)
	tests := []struct {
		input string
		want  string
	}{
		{"data.txt", "data.html"},
		{"/path/to/data.csv", "/path/to/data.html"},
		{"data.tar.gz", "data.tar.html"},
		{"data", "data.html"},
		{".data", ".data.html"},
		{"dir/.data", "dir/.data.html"},
		{".data.txt", ".data.html"},
		{"data.", "data.html"},
		{"dir.v2/data", "dir.v2/data.html"},
		{"report.html", "report.html"},
		{"", "stats--20261110.143005.html"},
		{"-", "stats--20261110.143005.html"},
	}
	for _, tt := range tests {
		got, err := htmlOutputPath(tt.input, now)
		if err != nil {
			t.Errorf("htmlOutputPath(%q) returned error: %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("htmlOutputPath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// TestHTMLOutputPathInvalid checks that inputs without a file name are rejected.
func TestHTMLOutputPathInvalid(t *testing.T) {
	for _, input := range []string{"dir/", ".", "..", "dir/.."} {
		if got, err := htmlOutputPath(input, time.Now()); err == nil {
			t.Errorf("htmlOutputPath(%q) = %q, want an error", input, got)
		}
	}
}

// TestHTMLSameFileRefusal checks that the report never overwrites its input,
// whether the input is the report path itself or reached through a symlink.
func TestHTMLSameFileRefusal(t *testing.T) {
	dir := t.TempDir()
	content := []byte("1\n2\n3\n")

	direct := filepath.Join(dir, "numbers.html")
	if err := os.WriteFile(direct, content, 0o644); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "real.txt")
	if err := os.WriteFile(target, content, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "linked.html")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "linked.txt")); err != nil {
		t.Fatal(err)
	}

	for _, input := range []string{direct, filepath.Join(dir, "linked.txt")} {
		stdout, stderr, err := runStatsIn(t, dir, "", "--html", input)
		if err == nil {
			t.Errorf("%s: expected exit 1, got success", input)
		}
		output, _ := htmlOutputPath(input, time.Now())
		want := "Error: not writing the HTML report: " + output + " would overwrite the input file " + input + "\n"
		if stderr != want {
			t.Errorf("%s: stderr = %q, want %q", input, stderr, want)
		}
		if stdout != "" {
			t.Errorf("%s: expected no stdout before refusal, got: %s", input, stdout)
		}
	}
	for _, path := range []string{direct, target} {
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, content) {
			t.Errorf("%s was modified: %q, %v", path, got, err)
		}
	}
}

// TestHTMLMarshalFailure checks that an unencodable value (variance overflows
// to +Inf for values near 1e200) exits 1 and writes no report or temp file.
func TestHTMLMarshalFailure(t *testing.T) {
	for _, mode := range [][]string{{"--html"}, {"-j", "--html"}} {
		dir := t.TempDir()
		input := filepath.Join(dir, "big.txt")
		if err := os.WriteFile(input, []byte("1e200\n2e200\n3e200\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		stdout, stderr, err := runStatsIn(t, dir, "", append(mode, input)...)
		if err == nil {
			t.Errorf("%v: expected exit 1, got success", mode)
		}
		if !strings.Contains(stderr, "Inf") {
			t.Errorf("%v: expected an Inf encoding error, got: %s", mode, stderr)
		}
		if stdout != "" {
			t.Errorf("%v: expected no stdout, got: %s", mode, stdout)
		}
		if names := dirEntries(t, dir); len(names) != 1 {
			t.Errorf("%v: expected only the input in %s, got %v", mode, dir, names)
		}
	}
}

// resolvedPath returns path with symlinks resolved (macOS temp directories
// sit behind /var -> /private/var), so two spellings of one file compare equal.
func resolvedPath(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("resolving %s: %v", path, err)
	}
	return resolved
}

// TestHTMLReportCLI checks that --html writes a parseable report for a file
// argument and for stdin, announces it on stderr by its absolute path after
// the stdout output and an empty line, and leaves stdout identical to a run
// without it.
func TestHTMLReportCLI(t *testing.T) {
	data, err := os.ReadFile("test_data.txt")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	input := filepath.Join(dir, "data.txt")
	if err := os.WriteFile(input, data, 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name  string
		dir   string // working directory; empty means a fresh temp directory
		args  []string
		stdin string
	}{
		{"file text", "", []string{input}, ""},
		{"file json", "", []string{"-j", input}, ""},
		{"relative file", dir, []string{"data.txt"}, ""},
		{"stdin text", "", nil, string(data)},
		{"stdin dash json", "", []string{"-j", "-"}, string(data)},
	}
	for _, tc := range cases {
		runDir := tc.dir
		if runDir == "" {
			runDir = t.TempDir()
		}
		plain, _, err := runStatsIn(t, runDir, tc.stdin, tc.args...)
		if err != nil {
			t.Fatalf("%s: run without --html failed: %v", tc.name, err)
		}
		withHTML, stderr, err := runStatsIn(t, runDir, tc.stdin, append([]string{"--html"}, tc.args...)...)
		if err != nil {
			t.Fatalf("%s: run with --html failed: %v\n%s", tc.name, err, stderr)
		}
		if withHTML != plain {
			t.Errorf("%s: stdout changed with --html:\n--- without\n%s\n--- with\n%s", tc.name, plain, withHTML)
		}

		path, found := strings.CutPrefix(strings.TrimSuffix(stderr, "\n"), "\nHTML report written to ")
		if !found {
			t.Fatalf("%s: stderr is not an empty line and the report note: %q", tc.name, stderr)
		}

		// With stdout and stderr on one pipe, the note comes last, after the
		// complete text or JSON output and an empty line.
		combined := exec.Command(statsBin, append([]string{"--html"}, tc.args...)...)
		combined.Dir = runDir
		combined.Stdin = strings.NewReader(tc.stdin)
		both, err := combined.CombinedOutput()
		if err != nil {
			t.Fatalf("%s: combined run failed: %v\n%s", tc.name, err, both)
		}
		// The stdin report name carries the time, so only its shape is compared.
		note, found := strings.CutPrefix(string(both), plain+"\nHTML report written to ")
		if !found || !strings.HasSuffix(note, ".html\n") || strings.Count(note, "\n") != 1 {
			t.Errorf("%s: combined output does not end with an empty line and the note:\n%s", tc.name, both)
		}
		if !filepath.IsAbs(path) {
			t.Fatalf("%s: report path %q is not absolute", tc.name, path)
		}
		wantDir := dir
		if tc.stdin != "" {
			if !strings.HasPrefix(filepath.Base(path), "stats--") {
				t.Errorf("%s: stdin report path = %q, want stats--<time>.html", tc.name, path)
			}
			wantDir = runDir
		} else if filepath.Base(path) != "data.html" {
			t.Errorf("%s: report path = %q, want data.html", tc.name, path)
		}
		if got := resolvedPath(t, filepath.Dir(path)); got != resolvedPath(t, wantDir) {
			t.Errorf("%s: report directory = %q, want %q", tc.name, got, resolvedPath(t, wantDir))
		}

		page, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: reading report: %v", tc.name, err)
		}
		if !strings.HasPrefix(string(page), "<!DOCTYPE html>") || !strings.Contains(string(page), "<title>") {
			t.Errorf("%s: report is not an HTML5 page", tc.name)
		}
		stats := reportJSON(t, string(page))["stats"].(map[string]any)
		if stats["count"] != float64(len(testData)) {
			t.Errorf("%s: report count = %v, want %d", tc.name, stats["count"], len(testData))
		}
	}
}

// TestRenderReportHeader checks the header contents and that the input name
// is escaped rather than interpreted as markup.
func TestRenderReportHeader(t *testing.T) {
	s, err := computeStats(testData, []float64{90, 10}, 2, 16, 2.5, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 10, 14, 30, 5, 0, time.UTC)
	page, err := buildReportPage(s, testData, "dir/a<b>.txt", now, 20, true)
	if err != nil {
		t.Fatal(err)
	}
	out, err := renderReport(page)
	if err != nil {
		t.Fatal(err)
	}
	html := string(out)

	for _, want := range []string{
		"a&lt;b&gt;.txt",
		"2026-11-10 14:30:05 UTC",
		`<a href="` + PgmUrl + `" target="_blank" rel="noopener noreferrer">stats ` + PgmVersion + `</a>`,
		`<li class="chip">-k 2</li>`,
		`<li class="chip">-b 20</li>`,
		`<li class="chip">-z 2.5</li>`,
		`<li class="chip">-p 10,90</li>`,
		`<li class="chip">-l</li>`,
		"(log-transformed, base e)",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered page missing %q", want)
		}
	}
	for _, unwanted := range []string{"<b>", `class="chip">-t`, `class="chip">-T`, `class="chip">-e`, "trimmed dataset"} {
		if strings.Contains(html, unwanted) {
			t.Errorf("rendered page should not contain %q", unwanted)
		}
	}
	reportJSON(t, html)

	// Defaults produce no chips and a stdin input is labeled as such.
	s, err = computeStats(testData, nil, defaultIQRMultiplier, defaultNumBins, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	page, err = buildReportPage(s, testData, "-", now, defaultNumBins, false)
	if err != nil {
		t.Fatal(err)
	}
	out, err = renderReport(page)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), `class="chips"`) || !strings.Contains(string(out), "<h1>stdin</h1>") {
		t.Errorf("default stdin header rendered incorrectly:\n%s", out)
	}
}

// reportRows flattens the rows of every section in a decoded data block.
func reportRows(t *testing.T, data map[string]any) []map[string]any {
	t.Helper()
	var rows []map[string]any
	for _, section := range data["sections"].([]any) {
		for _, row := range section.(map[string]any)["rows"].([]any) {
			rows = append(rows, row.(map[string]any))
		}
	}
	return rows
}

// findRow returns the first row of the given kind, or nil when there is none.
func findRow(rows []map[string]any, kind string) map[string]any {
	for _, row := range rows {
		if row["kind"] == kind {
			return row
		}
	}
	return nil
}

// renderTestReport computes statistics for data and renders a report page.
func renderTestReport(t *testing.T, data []float64, percentiles []float64, zThreshold, trimPct float64, emaSpan int) (*Stats, string) {
	t.Helper()
	s, err := computeStats(data, percentiles, defaultIQRMultiplier, defaultNumBins, zThreshold, trimPct, emaSpan)
	if err != nil {
		t.Fatal(err)
	}
	page, err := buildReportPage(s, data, "data.txt", time.Date(2026, 11, 10, 14, 30, 5, 0, time.UTC), defaultNumBins, false)
	if err != nil {
		t.Fatal(err)
	}
	out, err := renderReport(page)
	if err != nil {
		t.Fatal(err)
	}
	return s, string(out)
}

// TestRenderReportSections renders a known dataset, parses the data block
// back, and checks that values are formatted exactly as the text output
// formats them and that every row has a unique, defined popover.
func TestRenderReportSections(t *testing.T) {
	s, html := renderTestReport(t, testData, []float64{90, 10}, 2, 10, 5)
	data := reportJSON(t, html)

	var titles []string
	for _, section := range data["sections"].([]any) {
		titles = append(titles, section.(map[string]any)["title"].(string))
	}
	wantTitles := []string{"Descriptive Statistics", "Measures of Central Tendency", "Measures of Spread & Distribution", "Distribution"}
	if strings.Join(titles, "|") != strings.Join(wantTitles, "|") {
		t.Errorf("section titles = %v, want %v", titles, wantTitles)
	}

	rows := reportRows(t, data)
	wantValues := map[string]string{
		"mean":        formatFloat(s.Mean),
		"stddev":      formatFloat(s.StdDev),
		"median":      formatFloat(s.Median),
		"iqr":         formatFloat(s.IQR),
		"trimmedmean": formatFloat(s.TrimmedMean),
		"ema":         formatFloat(s.EMA),
		"ci95":        "[" + formatFloat(s.CI95Lower) + ", " + formatFloat(s.CI95Upper) + "]",
		"skewness":    formatFloat(s.Skewness) + " (" + interpretSkewness(s.Skewness) + ")",
		"cv":          formatFloat(s.CV) + "% (" + interpretCV(s.CV) + ")",
	}
	for kind, want := range wantValues {
		row := findRow(rows, kind)
		if row == nil {
			t.Errorf("row %q missing", kind)
			continue
		}
		if row["value"] != want {
			t.Errorf("row %q value = %v, want %q", kind, row["value"], want)
		}
		// The value is the dd's whole text; a gauge container may follow it.
		if !strings.Contains(html, `<dd class="value value-`+kind+`">`+want+"<") {
			t.Errorf("rendered page does not show %q for %q", want, kind)
		}
	}
	wantReadings := map[string]string{
		"skewness":        interpretSkewness(s.Skewness),
		"kurtosis":        interpretKurtosis(s.Kurtosis),
		"cv":              interpretCV(s.CV),
		"autocorrelation": interpretAutocorrelation(s.Autocorrelation),
	}
	for kind, want := range wantReadings {
		if row := findRow(rows, kind); row == nil || row["reading"] != want {
			t.Errorf("row %q reading = %v, want %q", kind, row["reading"], want)
		}
	}
	if stats := data["stats"].(map[string]any); stats["mean"] != s.Mean || stats["skewness"] != s.Skewness {
		t.Errorf("raw stats not carried: mean %v, skewness %v", stats["mean"], stats["skewness"])
	}

	var pctLabels, pctIDs []string
	ids := make(map[string]bool)
	for _, row := range rows {
		id := row["id"].(string)
		if ids[id] {
			t.Errorf("duplicate row id %q", id)
		}
		ids[id] = true
		if !strings.Contains(html, `popovertarget="def-`+id+`"`) {
			t.Errorf("row %q has no label button", id)
		}
		_, popover, found := strings.Cut(html, `<div popover="auto" id="def-`+id+`" class="def">`)
		popover, _, _ = strings.Cut(popover, "</div>")
		if !found || !strings.Contains(popover, `class="formula"`) {
			t.Errorf("row %q has no popover with a definition and formula", id)
		}
		if row["kind"] == "percentile" {
			pctLabels = append(pctLabels, row["label"].(string))
			pctIDs = append(pctIDs, id)
		}
	}
	if got := strings.Join(pctLabels, ","); got != "Percentile (p10),Percentile (p90),Percentile (p95),Percentile (p99)" {
		t.Errorf("percentile rows = %s", got)
	}
	if got := strings.Join(pctIDs, ","); got != "percentile,percentile-2,percentile-3,percentile-4" {
		t.Errorf("percentile ids = %s", got)
	}
	if strings.Contains(html, "innerHTML") {
		t.Errorf("report script must not use innerHTML")
	}

	// Summary chart inputs parse back with their formatted texts, and every
	// chart has its container: inline in its row, or after its rows.
	summary := data["summary"].(map[string]any)
	if d := summary["distinct"].(map[string]any); d["distinct"] != float64(s.Distinct) || d["count"] != float64(s.Count) {
		t.Errorf("summary distinct = %v", d)
	}
	center := summary["center"].(map[string]any)
	var centerKeys []string
	for _, m := range center["marks"].([]any) {
		mark := m.(map[string]any)
		centerKeys = append(centerKeys, mark["key"].(string))
		if mark["key"] == "mean" && mark["text"] != formatFloat(s.Mean) {
			t.Errorf("mean mark text = %v, want %s", mark["text"], formatFloat(s.Mean))
		}
	}
	if got := strings.Join(centerKeys, ","); strings.Contains(got, "ema") || !strings.HasPrefix(got, "mean,") {
		t.Errorf("center marks = %s; EMA must not be drawn", got)
	}
	if ci := center["ci"].(map[string]any); ci["text"] != wantValues["ci95"] {
		t.Errorf("center CI text = %v, want %s", ci["text"], wantValues["ci95"])
	}
	if n := len(summary["ladder"].([]any)); n != 6 {
		t.Errorf("ladder has %d rungs, want 6 (Q1, Q3, p10, p90, p95, p99)", n)
	}
	if len(summary["range"].([]any)) != 5 {
		t.Errorf("range strip should have 5 marks")
	}
	for _, name := range []string{"skewness", "kurtosis", "cv"} {
		g := summary[name].(map[string]any)
		if g["na"] != nil || g["reading"] != wantReadings[name] {
			t.Errorf("%s gauge = %v", name, g)
		}
	}
	for _, kind := range []string{"distinct", "skewness", "kurtosis", "cv"} {
		if !strings.Contains(html, `<div class="chart chart-inline" data-chart="`+kind+`"></div></dd>`) {
			t.Errorf("row %q has no inline chart container", kind)
		}
	}
	for _, name := range []string{"range", "center", "box", "ladder"} {
		if strings.Count(html, `<div class="chart" data-chart="`+name+`"></div>`) != 1 {
			t.Errorf("expected exactly one %q chart container", name)
		}
	}
	// The box plot and ladder sit right after the IQR row, before skewness.
	iqr, box, skew := strings.Index(html, `value-iqr"`), strings.Index(html, `data-chart="box"`), strings.Index(html, `value-skewness"`)
	if !(iqr < box && box < skew) {
		t.Errorf("box plot container is not between the IQR and skewness rows")
	}
}

// TestRenderReportGaugeNA checks through the rendered page that undefined
// gauges carry their N/A reasons for a two-value dataset and a zero-mean one.
func TestRenderReportGaugeNA(t *testing.T) {
	_, html := renderTestReport(t, []float64{4, 9}, nil, 0, 0, 0)
	summary := reportJSON(t, html)["summary"].(map[string]any)
	for name, want := range map[string]string{"skewness": "requires at least 3 values", "kurtosis": "requires at least 4 values"} {
		if g := summary[name].(map[string]any); g["na"] != want || g["text"] != nil {
			t.Errorf("n=2: %s gauge = %v, want N/A %q", name, g, want)
		}
	}
	_, html = renderTestReport(t, []float64{-2, -1, 1, 2}, nil, 0, 0, 0)
	if cv := reportJSON(t, html)["summary"].(map[string]any)["cv"].(map[string]any); cv["na"] != "mean near zero" {
		t.Errorf("zero mean: CV gauge = %v", cv)
	}
}

// TestReportPayloadBounded checks that modes and every outlier list are
// capped in the data block and the page, so the report stays small when
// every value repeats and outliers number in the thousands.
func TestReportPayloadBounded(t *testing.T) {
	// 20,000 central values and 600 far values on each side, each appearing
	// twice: every value is a mode, and the far values are outliers by all
	// three methods.
	var data []float64
	for i := range 20000 {
		data = append(data, float64(i), float64(i))
	}
	for i := range 600 {
		data = append(data, 1e6+float64(i), 1e6+float64(i), -1e6-float64(i), -1e6-float64(i))
	}
	s, html := renderTestReport(t, data, nil, 2, 0, 0)
	if len(s.Mode) != 21200 || len(s.Outliers) != 2400 || len(s.ZScoreOutliers) != 2400 || len(s.ModZOutliers) != 2400 {
		t.Fatalf("unexpected dataset shape: %d modes, %d/%d/%d outliers", len(s.Mode), len(s.Outliers), len(s.ZScoreOutliers), len(s.ModZOutliers))
	}
	if len(html) > 200_000 {
		t.Errorf("report is %d bytes, want under 200000", len(html))
	}

	stats := reportJSON(t, html)["stats"].(map[string]any)
	mode := stats["mode"].(map[string]any)
	if mode["count"] != float64(21200) || len(mode["head"].([]any)) != reportListCap || mode["tail"] != nil {
		t.Errorf("mode not capped to the first %d: count %v, head %d, tail %v", reportListCap, mode["count"], len(mode["head"].([]any)), mode["tail"])
	}
	sources := map[string][]float64{"outliers": s.Outliers, "zScoreOutliers": s.ZScoreOutliers, "modZOutliers": s.ModZOutliers}
	for _, key := range []string{"outliers", "zScoreOutliers", "modZOutliers"} {
		src := sources[key]
		list := stats[key].(map[string]any)
		head, tail := list["head"].([]any), list["tail"].([]any)
		if list["count"] != float64(2400) || len(head) != reportListCap || len(tail) != reportListCap {
			t.Errorf("%s not capped: count %v, head %d, tail %d", key, list["count"], len(head), len(tail))
			continue
		}
		if head[0] != src[0] || tail[len(tail)-1] != src[len(src)-1] {
			t.Errorf("%s does not keep the lowest and highest values: %v ... %v", key, head[0], tail[len(tail)-1])
		}
	}
	for _, want := range []string{"21200 values (showing first 50 of 21200)", "2400 values (showing 50 lowest and 50 highest of 2400)"} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered page missing %q", want)
		}
	}
	if len(s.Mode) != 21200 || len(s.Outliers) != 2400 {
		t.Errorf("building the report modified the statistics")
	}

	// A detector that never ran stays null; one that found nothing is empty.
	_, html = renderTestReport(t, testData, nil, 0, 0, 0)
	stats = reportJSON(t, html)["stats"].(map[string]any)
	if stats["zScoreOutliers"] != nil || stats["modZOutliers"] != nil {
		t.Errorf("disabled detectors should be null, got %v and %v", stats["zScoreOutliers"], stats["modZOutliers"])
	}
	_, html = renderTestReport(t, []float64{1, 2, 3, 4}, nil, 0, 0, 0)
	stats = reportJSON(t, html)["stats"].(map[string]any)
	if outliers := stats["outliers"].(map[string]any); outliers["count"] != float64(0) || len(outliers["head"].([]any)) != 0 {
		t.Errorf("empty outlier list should be count 0 with an empty head, got %v", outliers)
	}
}

// TestReportConditionalRows checks through the CLI that rows appear and
// disappear with the flags exactly as they do in the text output.
func TestReportConditionalRows(t *testing.T) {
	data, err := os.ReadFile("test_data.txt")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	input := filepath.Join(dir, "data.txt")
	if err := os.WriteFile(input, append(data, []byte("abc\n")...), 0o644); err != nil {
		t.Fatal(err)
	}

	always := []string{"count", "skipped", "distinct", "mean", "median", "mode", "stddev", "cv", "skewness", "symmetry", "outliers", "histogram"}
	cases := []struct {
		args        []string
		present     []string
		absent      []string
		percentiles string
		footnote    bool
	}{
		{nil, []string{"ci95", "geomean", "trendline", "autocorrelation", "inputorder"}, []string{"zoutliers", "modzoutliers", "trimmedmean", "ema"}, "p95,p99", false},
		{[]string{"-z", "2"}, []string{"zoutliers", "modzoutliers"}, nil, "p95,p99", false},
		{[]string{"-t", "10"}, []string{"trimmedmean"}, []string{"ema"}, "p95,p99", false},
		{[]string{"-e", "3"}, []string{"ema"}, []string{"trimmedmean"}, "p95,p99", false},
		{[]string{"-p", "99.5,10,95"}, nil, nil, "p10,p95,p99,p99.5", false},
		{[]string{"-T", "10"}, nil, []string{"trendline", "autocorrelation", "inputorder"}, "p95,p99", true},
		{[]string{"-l"}, nil, []string{"geomean"}, "p95,p99", false},
	}
	for _, tc := range cases {
		_, stderr, err := runStatsIn(t, dir, "", append(append([]string{"--html"}, tc.args...), input)...)
		if err != nil {
			t.Fatalf("%v: run failed: %v\n%s", tc.args, err, stderr)
		}
		page, err := os.ReadFile(filepath.Join(dir, "data.html"))
		if err != nil {
			t.Fatal(err)
		}
		report := reportJSON(t, string(page))
		rows := reportRows(t, report)
		for _, kind := range append(always, tc.present...) {
			if findRow(rows, kind) == nil {
				t.Errorf("%v: row %q missing", tc.args, kind)
			}
		}
		for _, kind := range tc.absent {
			if findRow(rows, kind) != nil {
				t.Errorf("%v: row %q should not appear", tc.args, kind)
			}
		}
		var pcts []string
		for _, row := range rows {
			if row["kind"] == "percentile" {
				label := strings.TrimSuffix(strings.TrimPrefix(row["label"].(string), "Percentile ("), ")")
				pcts = append(pcts, label)
			}
		}
		if got := strings.Join(pcts, ","); got != tc.percentiles {
			t.Errorf("%v: percentiles = %s, want %s", tc.args, got, tc.percentiles)
		}
		hasFootnote := strings.Contains(string(page), `<p class="footnote">* `+trimFootnote+"</p>")
		if hasFootnote != tc.footnote || (report["footnote"] != nil) != tc.footnote {
			t.Errorf("%v: footnote shown = %v, want %v", tc.args, hasFootnote, tc.footnote)
		}
	}
}

// TestRenderReportDistributionCharts checks that every section 4 chart
// container appears exactly once, under its row when the row exists, at the
// end of the section when the row is absent, and in a Distribution card of its
// own when the text output has no Distribution section at all.
func TestRenderReportDistributionCharts(t *testing.T) {
	charts := []string{"histogram", "ecdf", "qq", "trendline", "lag"}
	tenTenths := make([]float64, 10)
	for i := range tenTenths {
		tenTenths[i] = 0.1
	}
	datasets := map[string][]float64{
		"full":       testData,
		"n=1":        {5},
		"constant":   {3, 3, 3, 3},
		"ten tenths": tenTenths,
	}
	for name, data := range datasets {
		_, html := renderTestReport(t, data, nil, 0, 0, 0)
		if n := strings.Count(html, "<h2>Distribution</h2>"); n != 1 {
			t.Errorf("%s: %d Distribution sections, want 1", name, n)
		}
		for _, chart := range charts {
			if n := strings.Count(html, `<div class="chart" data-chart="`+chart+`"></div>`); n != 1 {
				t.Errorf("%s: %d %q chart containers, want 1", name, n, chart)
			}
		}
	}

	// With every row present, each chart follows its own row in order.
	_, html := renderTestReport(t, testData, nil, 0, 0, 0)
	order := []string{
		`value-histogram"`, `data-chart="histogram"`, `data-chart="ecdf"`, `data-chart="qq"`,
		`value-trendline"`, `data-chart="trendline"`,
		`value-autocorrelation"`, `data-chart="lag"`,
	}
	last := -1
	for _, marker := range order {
		i := strings.Index(html, marker)
		if i <= last {
			t.Errorf("%s is missing or out of order (at %d, previous at %d)", marker, i, last)
		}
		last = i
	}
}
