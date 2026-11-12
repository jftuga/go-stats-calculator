//go:build unix

// Permission tests for the HTML report. They set the process umask, which
// only Unix has, so they live apart from the portable report tests.

package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// withUmask runs fn with the process umask set to mask, restoring it after.
// The tests in this package do not run in parallel, so nothing else creates
// files meanwhile.
func withUmask(mask int, fn func()) {
	old := syscall.Umask(mask)
	defer syscall.Umask(old)
	fn()
}

// mode returns the permission bits of path.
func mode(t *testing.T, path string) fs.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}

// TestWriteFileAtomicNewFileUmask checks that a new report gets 0666 less the
// umask rather than a fixed mode.
func TestWriteFileAtomicNewFileUmask(t *testing.T) {
	for _, tc := range []struct {
		mask int
		want fs.FileMode
	}{{0o022, 0o644}, {0o077, 0o600}, {0o002, 0o664}} {
		path := filepath.Join(t.TempDir(), "report.html")
		withUmask(tc.mask, func() {
			if err := writeFileAtomic(path, []byte("x")); err != nil {
				t.Fatal(err)
			}
		})
		if got := mode(t, path); got != tc.want {
			t.Errorf("umask %03o: mode %03o, want %03o", tc.mask, got, tc.want)
		}
	}
}

// TestWriteFileAtomicKeepsMode checks that replacing a regular file keeps its
// permissions, private or not, whatever the umask, and that the content is
// replaced.
func TestWriteFileAtomicKeepsMode(t *testing.T) {
	for _, want := range []fs.FileMode{0o600, 0o640, 0o644} {
		path := filepath.Join(t.TempDir(), "report.html")
		if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, want); err != nil {
			t.Fatal(err)
		}
		withUmask(0o022, func() {
			if err := writeFileAtomic(path, []byte("new")); err != nil {
				t.Fatal(err)
			}
		})
		if got := mode(t, path); got != want {
			t.Errorf("replacing a %03o file: mode %03o", want, got)
		}
		if b, _ := os.ReadFile(path); string(b) != "new" {
			t.Errorf("replacing a %03o file: content %q", want, b)
		}
	}
}

// TestHTMLReportUmaskCLI runs the program under umask 077 on a private input,
// the case where a fixed 0644 exposed the values to other users.
func TestHTMLReportUmaskCLI(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "private.txt")
	if err := os.WriteFile(input, []byte("1\n2\n3\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	withUmask(0o077, func() {
		if _, stderr, err := runStatsIn(t, dir, "", "--html", input); err != nil {
			t.Fatalf("run failed: %v\n%s", err, stderr)
		}
	})
	if got := mode(t, filepath.Join(dir, "private.html")); got != 0o600 {
		t.Errorf("report mode %03o under umask 077, want 600", got)
	}
}
