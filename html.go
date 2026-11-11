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

// reportData is the payload marshaled into the page's JSON data block.
type reportData struct {
	Stats statsOutput `json:"stats"`
}

// reportPage is the value passed to the report template. CSS and JS carry
// only the embedded report files; Data is the already-marshaled reportData.
type reportPage struct {
	CSS            template.CSS
	JS             template.JS
	Data           json.RawMessage
	Input          string
	Generated      string
	Version        string
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
		return fmt.Errorf("HTML report path %s is the input file %s", outputPath, inputPath)
	}
	return nil
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

// buildReportPage assembles the template value for a report. It marshals the
// chart data itself, because html/template hides a marshal error (such as a
// NaN or Inf) inside a JSON script block and renders null instead.
func buildReportPage(s *Stats, inputArg string, now time.Time, numBins int, logTransformed bool) (*reportPage, error) {
	data, err := json.Marshal(reportData{Stats: newStatsOutput(s, logTransformed)})
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
		Data:           data,
		Input:          input,
		Generated:      now.Format("2006-01-02 15:04:05 MST"),
		Version:        PgmVersion,
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

// writeHTMLReport builds, renders and writes the report for s to outPath.
// Nothing is written unless every earlier step succeeds.
func writeHTMLReport(outPath string, s *Stats, inputArg string, now time.Time, numBins int, logTransformed bool) error {
	page, err := buildReportPage(s, inputArg, now, numBins, logTransformed)
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
