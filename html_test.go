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
		want, _ := htmlOutputPath(input, time.Now())
		if !strings.Contains(stderr, input) || !strings.Contains(stderr, want) {
			t.Errorf("%s: error should name both paths, got: %s", input, stderr)
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

// TestHTMLReportCLI checks that --html writes a parseable report for a file
// argument and for stdin, and leaves stdout identical to a run without it.
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
		args  []string
		stdin string
	}{
		{"file text", []string{input}, ""},
		{"file json", []string{"-j", input}, ""},
		{"stdin text", nil, string(data)},
		{"stdin dash json", []string{"-j", "-"}, string(data)},
	}
	for _, tc := range cases {
		runDir := t.TempDir()
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

		path, found := strings.CutPrefix(strings.TrimSpace(stderr), "HTML report written to ")
		if !found {
			t.Fatalf("%s: missing report note on stderr: %q", tc.name, stderr)
		}
		if tc.stdin != "" {
			if !strings.HasPrefix(path, "stats--") || filepath.Dir(path) != "." {
				t.Errorf("%s: stdin report path = %q, want stats--<time>.html in the current directory", tc.name, path)
			}
			path = filepath.Join(runDir, path)
		} else if path != filepath.Join(dir, "data.html") {
			t.Errorf("%s: report path = %q, want %q", tc.name, path, filepath.Join(dir, "data.html"))
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
	page, err := buildReportPage(s, "dir/a<b>.txt", now, 20, true)
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
		PgmVersion,
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
	page, err = buildReportPage(s, "-", now, defaultNumBins, false)
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
