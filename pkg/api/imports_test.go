package api

import (
	"bufio"
	"go/parser"
	gotok "go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// bannedImportPrefixes are the packages pkg/api must not reach, directly or
// through another package of this module. charmbracelet/x/ansi is fine: the
// loader already uses it.
func bannedImportPrefixes(module string) []string {
	return []string{
		"github.com/charmbracelet/bubbletea",
		"github.com/charmbracelet/bubbles",
		"github.com/charmbracelet/lipgloss",
		module + "/pkg/ui",
	}
}

// findBannedImports parses the non-test Go files of the package in startDir and
// every in-module package they import, transitively, and reports each import
// that matches a banned prefix as "<package dir> imports <path>".
func findBannedImports(t *testing.T, root, module, startDir string) []string {
	t.Helper()
	banned := bannedImportPrefixes(module)
	var violations []string
	visited := map[string]bool{}
	var visit func(dir string)
	visit = func(dir string) {
		dir = filepath.Clean(dir)
		if visited[dir] {
			return
		}
		visited[dir] = true
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read %s: %v", dir, err)
		}
		fset := gotok.NewFileSet()
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.ImportsOnly)
			if err != nil {
				t.Fatalf("parse %s: %v", filepath.Join(dir, name), err)
			}
			for _, spec := range file.Imports {
				path := strings.Trim(spec.Path.Value, `"`+"`")
				bannedHit := false
				for _, prefix := range banned {
					if path == prefix || strings.HasPrefix(path, prefix+"/") {
						violations = append(violations, filepath.ToSlash(dir)+" imports "+path)
						bannedHit = true
						break
					}
				}
				if bannedHit {
					continue
				}
				if path == module || strings.HasPrefix(path, module+"/") {
					visit(filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(path, module))))
				}
			}
		}
	}
	visit(startDir)
	sort.Strings(violations)
	return violations
}

func modulePath(t *testing.T, goMod string) string {
	t.Helper()
	f, err := os.Open(goMod)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(scanner.Text()), "module "); ok {
			return strings.TrimSpace(rest)
		}
	}
	t.Fatalf("no module line in %s", goMod)
	return ""
}

// TestAPIImportsNoUI: pkg/api is the HTTP side of the GUI and must stay UI-free (ADR 0001
// section 1): neither it nor anything it imports from this module may depend on
// bubbletea, bubbles, lipgloss or pkg/ui.
func TestAPIImportsNoUI(t *testing.T) {
	root := filepath.Join("..", "..")
	module := modulePath(t, filepath.Join(root, "go.mod"))
	if violations := findBannedImports(t, root, module, "."); len(violations) > 0 {
		t.Fatalf("pkg/api must stay UI-free:\n  %s", strings.Join(violations, "\n  "))
	}
}

// TestFindBannedImportsDetectsTransitive proves the convention test can fail: a
// banned import two in-module hops away is reported, a clean tree is not.
func TestFindBannedImportsDetectsTransitive(t *testing.T) {
	const module = "example.com/m"
	root := t.TempDir()
	write := func(rel, src string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("pkg/a/a.go", "package a\n\nimport _ \""+module+"/pkg/b\"\nimport _ \"github.com/charmbracelet/x/ansi\"\n")
	write("pkg/b/b.go", "package b\n\nimport _ \""+module+"/pkg/c\"\n")
	write("pkg/c/c.go", "package c\n\nimport _ \"github.com/charmbracelet/lipgloss/v2\"\n")
	write("pkg/c/c_test.go", "package c\n\nimport _ \"github.com/charmbracelet/bubbletea\"\n")
	got := findBannedImports(t, root, module, filepath.Join(root, "pkg", "a"))
	if len(got) != 1 || !strings.HasSuffix(got[0], "pkg/c imports github.com/charmbracelet/lipgloss/v2") {
		t.Fatalf("violations: %q", got)
	}

	write("pkg/c/c.go", "package c\n\nimport _ \""+module+"/pkg/ui\"\n")
	if got := findBannedImports(t, root, module, filepath.Join(root, "pkg", "a")); len(got) != 1 || !strings.HasSuffix(got[0], "imports "+module+"/pkg/ui") {
		t.Fatalf("pkg/ui violations: %q", got)
	}

	write("pkg/c/c.go", "package c\n")
	if got := findBannedImports(t, root, module, filepath.Join(root, "pkg", "a")); len(got) != 0 {
		t.Fatalf("clean tree reported: %q", got)
	}
}
