// html.go
package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

// reportFS holds the report's markup, styles and scripts. All HTML lives in
// these files; Go only supplies data to the template.
//
//go:embed report/*
var reportFS embed.FS

// reportTemplate is the parsed page template, checked once at startup.
var reportTemplate = template.Must(template.ParseFS(reportFS, "report/report.tmpl"))

// Default option values; a chip is shown only for an option that differs.
const (
	defaultIQRMultiplier = 1.5
	defaultNumBins       = 16
)

// reportOption is one active command-line option shown as a chip in the
// report header. Value is empty for boolean flags.
type reportOption struct {
	Flag  string
	Value string
}

// reportListCap is how many values of a long list the report keeps at each
// end, so the file size stays bounded however many modes or outliers exist.
const reportListCap = 50

// cappedList is a list cut down to at most a fixed number of values at each
// end. Head holds the first (lowest) values and Tail the highest; Tail is
// empty unless values were dropped from the middle.
type cappedList[T any] struct {
	Count int `json:"count"`
	Head  []T `json:"head"`
	Tail  []T `json:"tail,omitempty"`
}

// reportStats is the -j statistics object with its four unbounded lists
// replaced by capped copies. The capped fields shadow the embedded fields of
// the same JSON name, since encoding/json emits the least nested one, and the
// embedded copy has those lists cleared. Every raw number stays, so a NaN or
// Inf still fails json.Marshal.
type reportStats struct {
	statsOutput
	Mode           *cappedList[float64] `json:"mode"`
	Outliers       *cappedList[float64] `json:"outliers"`
	ZScoreOutliers *cappedList[float64] `json:"zScoreOutliers"`
	ModZOutliers   *cappedList[float64] `json:"modZOutliers"`
}

// reportRow is one statistic as the report shows it. Value matches the text
// output exactly; a list-valued row carries List instead. ID is unique within
// the page and names the row's definition popover.
type reportRow struct {
	ID      string              `json:"id"`
	Kind    string              `json:"kind"`
	Label   string              `json:"label"`
	Value   string              `json:"value,omitempty"`
	Reading string              `json:"reading,omitempty"`
	List    *cappedList[string] `json:"list,omitempty"`
}

// reportSection is a titled group of report rows.
type reportSection struct {
	Title string      `json:"title"`
	Rows  []reportRow `json:"rows"`
}

// reportData is the payload marshaled into the page's JSON data block and
// rendered by the template. Everything in it is bounded in size.
type reportData struct {
	Stats        reportStats        `json:"stats"`
	Sections     []reportSection    `json:"sections"`
	Footnote     string             `json:"footnote,omitempty"`
	Charts       chartData          `json:"charts"`
	Summary      summaryCharts      `json:"summary"`
	Distribution distributionCharts `json:"distribution"`
}

// reportPage is the value passed to the report template. CSS and JS carry
// only the embedded report files; Data is the already-marshaled Report.
type reportPage struct {
	CSS            template.CSS
	JS             template.JS
	Report         reportData
	Data           json.RawMessage
	Input          string
	Generated      string
	Version        string
	URL            string
	Options        []reportOption
	LogTransformed bool
	TrimDatasetPct string // empty when -T is not active
	TrimOrigN      int
	Count          int
}

// htmlOutputPath derives the report path from the input argument. A file
// argument has its extension replaced with ".html"; a name with no extension,
// or a leading-dot name such as ".data", has ".html" appended. Stdin ("" or
// "-") yields a timestamped name in the current directory.
func htmlOutputPath(inputArg string, now time.Time) (string, error) {
	if inputArg == "" || inputArg == "-" {
		return "stats--" + now.Format("20060102.150405") + ".html", nil
	}
	dir, base := filepath.Split(inputArg)
	if base == "" || base == "." || base == ".." {
		return "", fmt.Errorf("cannot derive an HTML report name from %q", inputArg)
	}
	ext := filepath.Ext(base)
	// filepath.Ext(".data") is ".data", so a leading-dot name has no extension.
	if ext != base {
		base = strings.TrimSuffix(base, ext)
	}
	return filepath.Join(dir, base+".html"), nil
}

// checkNotSameFile returns an error when the output path already exists and
// refers to the same file as the input, which writing would destroy.
func checkNotSameFile(inputPath, outputPath string) error {
	outInfo, err := os.Stat(outputPath)
	if err != nil {
		return nil // nothing there to overwrite
	}
	inInfo, err := os.Stat(inputPath)
	if err != nil {
		return nil // opening the input reports this error
	}
	if os.SameFile(inInfo, outInfo) {
		return fmt.Errorf("not writing the HTML report: %s would overwrite the input file %s", outputPath, inputPath)
	}
	return nil
}

// reportWrittenNote returns the stderr line announcing the report at path,
// made absolute so it can be opened from any directory. Abs fails only when
// the working directory cannot be found; the path is then shown as given.
func reportWrittenNote(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	return "HTML report written to " + path
}

// reportOptions lists the options that differ from their defaults, in the
// order the flags are documented.
func reportOptions(s *Stats, numBins int, logTransformed bool) []reportOption {
	var opts []reportOption
	if s.IQRMultiplier != defaultIQRMultiplier {
		opts = append(opts, reportOption{"-k", formatFloat(s.IQRMultiplier)})
	}
	if numBins != defaultNumBins {
		opts = append(opts, reportOption{"-b", strconv.Itoa(numBins)})
	}
	if s.ZScoreThreshold > 0 {
		opts = append(opts, reportOption{"-z", formatFloat(s.ZScoreThreshold)})
	}
	if len(s.CustomPercentiles) > 0 {
		keys := make([]float64, 0, len(s.CustomPercentiles))
		for p := range s.CustomPercentiles {
			keys = append(keys, p)
		}
		sort.Float64s(keys)
		parts := make([]string, len(keys))
		for i, p := range keys {
			parts[i] = formatFloat(p)
		}
		opts = append(opts, reportOption{"-p", strings.Join(parts, ",")})
	}
	if s.TrimmedMeanPct > 0 {
		opts = append(opts, reportOption{"-t", formatFloat(s.TrimmedMeanPct)})
	}
	if s.TrimDatasetPct > 0 {
		opts = append(opts, reportOption{"-T", formatFloat(s.TrimDatasetPct)})
	}
	if s.EMASpan > 0 {
		opts = append(opts, reportOption{"-e", strconv.Itoa(s.EMASpan)})
	}
	if logTransformed {
		opts = append(opts, reportOption{"-l", ""})
	}
	return opts
}

// capList keeps the first head and last tail values of a sorted list, copying
// them so the result does not pin the full list. A nil list, meaning the
// detector never ran, stays nil.
func capList(values []float64, head, tail int) *cappedList[float64] {
	if values == nil {
		return nil
	}
	c := &cappedList[float64]{Count: len(values)}
	if len(values) <= head+tail {
		c.Head = slices.Clone(values)
		return c
	}
	c.Head = slices.Clone(values[:head])
	if tail > 0 {
		c.Tail = slices.Clone(values[len(values)-tail:])
	}
	return c
}

// formatList formats every kept value of a capped list with formatFloat.
func formatList(c *cappedList[float64]) *cappedList[string] {
	f := &cappedList[string]{Count: c.Count, Head: make([]string, len(c.Head))}
	for i, v := range c.Head {
		f.Head[i] = formatFloat(v)
	}
	for _, v := range c.Tail {
		f.Tail = append(f.Tail, formatFloat(v))
	}
	return f
}

// listTail is how many highest values a list row keeps: modes keep only
// the first reportListCap, outlier lists also keep the highest.
func listTail(kind string) int {
	if kind == "mode" {
		return 0
	}
	return reportListCap
}

// newReportStats builds the bounded statistics object for the data block from
// a copy of s, leaving s itself untouched.
func newReportStats(s *Stats, logTransformed bool) reportStats {
	bounded := *s
	out := newStatsOutput(&bounded, logTransformed)
	r := reportStats{
		statsOutput:    out,
		Mode:           capList(bounded.Mode, reportListCap, 0),
		Outliers:       capList(bounded.Outliers, reportListCap, reportListCap),
		ZScoreOutliers: capList(bounded.ZScoreOutliers, reportListCap, reportListCap),
		ModZOutliers:   capList(bounded.ModZOutliers, reportListCap, reportListCap),
	}
	bounded.Mode, bounded.Outliers, bounded.ZScoreOutliers, bounded.ModZOutliers = nil, nil, nil, nil
	return r
}

// newReportSections converts the text output's rows into report rows. A
// repeated kind (percentiles) gets a numeric suffix so every ID is unique, and
// a list-valued row is capped and formatted.
func newReportSections(s *Stats) []reportSection {
	seen := make(map[string]int)
	var sections []reportSection
	for _, section := range statSections(s) {
		rs := reportSection{Title: section.Title, Rows: make([]reportRow, len(section.Rows))}
		for i, row := range section.Rows {
			seen[row.Kind]++
			id := row.Kind
			if n := seen[row.Kind]; n > 1 {
				id += "-" + strconv.Itoa(n)
			}
			rr := reportRow{ID: id, Kind: row.Kind, Label: row.Label, Value: row.Value, Reading: row.Reading}
			if len(row.Values) > 0 {
				rr.List = formatList(capList(row.Values, reportListCap, listTail(row.Kind)))
			}
			rs.Rows[i] = rr
		}
		sections = append(sections, rs)
	}
	return sections
}

// buildReportPage assembles the template value for a report from the
// statistics s and the numbers they were computed from, in input order (data
// is sorted under -T). It marshals the chart data itself, because
// html/template hides a marshal error (such as a NaN or Inf) inside a JSON
// script block and renders null instead.
func buildReportPage(s *Stats, data []float64, inputArg string, now time.Time, numBins int, logTransformed bool) (*reportPage, error) {
	report := reportData{
		Stats:        newReportStats(s, logTransformed),
		Sections:     newReportSections(s),
		Charts:       newChartData(data, s),
		Summary:      newSummaryCharts(s, numBins),
		Distribution: newDistributionCharts(s),
	}
	if s.TrimDatasetPct > 0 {
		report.Footnote = trimFootnote
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		return nil, fmt.Errorf("encoding report data: %w", err)
	}
	css, err := reportFS.ReadFile("report/report.css")
	if err != nil {
		return nil, err
	}
	js, err := reportFS.ReadFile("report/report.js")
	if err != nil {
		return nil, err
	}

	input := "stdin"
	if inputArg != "" && inputArg != "-" {
		input = filepath.Base(inputArg)
	}
	page := &reportPage{
		CSS:            template.CSS(css),
		JS:             template.JS(js),
		Report:         report,
		Data:           encoded,
		Input:          input,
		Generated:      now.Format("2006-01-02 15:04:05 MST"),
		Version:        PgmVersion,
		URL:            PgmUrl,
		Options:        reportOptions(s, numBins, logTransformed),
		LogTransformed: logTransformed,
		TrimOrigN:      s.TrimDatasetOrigN,
		Count:          s.Count,
	}
	if s.TrimDatasetPct > 0 {
		page.TrimDatasetPct = formatFloat(s.TrimDatasetPct)
	}
	return page, nil
}

// renderReport executes the report template into a byte slice.
func renderReport(page *reportPage) ([]byte, error) {
	var buf bytes.Buffer
	if err := reportTemplate.Execute(&buf, page); err != nil {
		return nil, fmt.Errorf("rendering report: %w", err)
	}
	return buf.Bytes(), nil
}

// writeFileAtomic writes content to a temporary file in the target's directory
// and renames it over the target, so a failure never leaves a truncated file
// or destroys the previous one, and a symlink at the target is replaced
// rather than followed.
func writeFileAtomic(path string, content []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	_, writeErr := tmp.Write(content)
	closeErr := tmp.Close()
	err = writeErr
	if err == nil {
		err = closeErr
	}
	if err == nil {
		// CreateTemp uses 0600; a report is an ordinary readable file.
		err = os.Chmod(tmpName, 0o644)
	}
	if err == nil {
		err = os.Rename(tmpName, path)
	}
	if err != nil {
		os.Remove(tmpName)
	}
	return err
}

// writeHTMLReport builds, renders and writes the report for s and its input
// data to outPath. Nothing is written unless every earlier step succeeds.
func writeHTMLReport(outPath string, s *Stats, data []float64, inputArg string, now time.Time, numBins int, logTransformed bool) error {
	page, err := buildReportPage(s, data, inputArg, now, numBins, logTransformed)
	if err != nil {
		return err
	}
	content, err := renderReport(page)
	if err != nil {
		return err
	}
	if err := writeFileAtomic(outPath, content); err != nil {
		return fmt.Errorf("writing HTML report: %w", err)
	}
	return nil
}
