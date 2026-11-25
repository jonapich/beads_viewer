package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestServeAllowDirsMergesFlagsAndEnv(t *testing.T) {
	sep := string(os.PathListSeparator)
	abs := func(name string) string { return filepath.Join(t.TempDir(), name) }
	a, b, c, d := abs("a"), abs("b"), abs("c"), abs("d")
	var warned []string
	warn := func(s string) { warned = append(warned, s) }
	got := serveAllowDirs([]string{a, " " + b + " ", "", a}, strings.Join([]string{c, "", b, d}, sep), warn)
	if want := []string{a, b, c, d}; !slices.Equal(got, want) {
		t.Fatalf("serveAllowDirs = %q, want %q", got, want)
	}
	if got := serveAllowDirs(nil, "", warn); len(got) != 0 || len(warned) != 0 {
		t.Fatalf("nothing allowed = %q, warned %q", got, warned)
	}
}

// A relative entry in BV_SERVE_ALLOW would mean a different directory depending on the working
// directory: it is skipped with a warning. A relative --serve-allow value is the user's to mean.
func TestServeAllowDirsSkipsRelativeEnvEntries(t *testing.T) {
	sep := string(os.PathListSeparator)
	abs := t.TempDir()
	var warned []string
	got := serveAllowDirs([]string{"captures"}, strings.Join([]string{".", "shots", abs}, sep), func(s string) { warned = append(warned, s) })
	if want := []string{"captures", abs}; !slices.Equal(got, want) {
		t.Fatalf("serveAllowDirs = %q, want %q", got, want)
	}
	if len(warned) != 2 || !strings.Contains(warned[0], `"."`) || !strings.Contains(warned[1], `"shots"`) {
		t.Fatalf("warnings = %q", warned)
	}
}

func TestServeAllowIsAServeModifier(t *testing.T) {
	fs := newServeTestFlagSet()
	fs.StringArray("serve-allow", nil, "")
	if err := fs.Parse([]string{"--serve-allow", "/a", "--serve-allow", "/b"}); err != nil {
		t.Fatal(err)
	}
	if err := validateModifierFlags(fs, serveModifierRules()); err == nil || !strings.Contains(err.Error(), "requires --serve") {
		t.Fatalf("--serve-allow without --serve: %v", err)
	}
	fs = newServeTestFlagSet()
	fs.StringArray("serve-allow", nil, "")
	if err := fs.Parse([]string{"--serve", "--serve-allow", "/a", "--serve-allow", "/b"}); err != nil {
		t.Fatal(err)
	}
	if err := validateModifierFlags(fs, serveModifierRules()); err != nil {
		t.Fatalf("--serve with --serve-allow: %v", err)
	}
	if vals, _ := fs.GetStringArray("serve-allow"); !slices.Equal(vals, []string{"/a", "/b"}) {
		t.Fatalf("repeatable flag = %q", vals)
	}
	if !knownValueTakingCLIFlags["serve-allow"] {
		t.Fatal("--serve-allow takes a separate value")
	}
}
