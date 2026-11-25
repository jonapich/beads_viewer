package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Dicklesworthstone/beads_viewer/pkg/testutil"
)

// filesFixture is a project directory with a few files, and a handler serving it.
type filesFixture struct {
	root string
	h    http.Handler
}

type fileReply struct {
	Code   int
	Header http.Header
	Body   string
}

func writeFile(t *testing.T, path string, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newFilesFixture(t *testing.T, mutate func(*Config, string)) *filesFixture {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".beads", "captures", "a.png"), "PNGDATA")
	writeFile(t, filepath.Join(root, "shots", "my shot.png"), "SPACED")
	writeFile(t, filepath.Join(root, "shots", "100%20.png"), "LITERAL")
	writeFile(t, filepath.Join(root, "icon.svg"), "<svg xmlns='http://www.w3.org/2000/svg'/>")
	writeFile(t, filepath.Join(root, "notes.md"), "# notes")
	writeFile(t, filepath.Join(root, "doc.pdf"), "%PDF-1.4")
	writeFile(t, filepath.Join(root, "settings.json"), "{}")
	writeFile(t, filepath.Join(root, ".git", "config.png"), "GITCONFIG")
	h := newTestHandler(t, func(c *Config) {
		c.FileBase = root
		c.FileRoots = []string{root}
		if mutate != nil {
			mutate(c, root)
		}
	})
	return &filesFixture{root: root, h: h}
}

func (f *filesFixture) get(path string, opts ...reqOpt) *fileReply {
	return f.do(http.MethodGet, "/api/v1/file?path="+url.QueryEscape(path), append([]reqOpt{withSession()}, opts...)...)
}

func (f *filesFixture) do(method, target string, opts ...reqOpt) *fileReply {
	rec := do(f.h, method, target, opts...)
	return &fileReply{Code: rec.Code, Header: rec.Header(), Body: rec.Body.String()}
}

func (f *filesFixture) stat(t *testing.T, paths ...string) []FileStat {
	t.Helper()
	q := url.Values{}
	for _, p := range paths {
		q.Add("path", p)
	}
	rec := do(f.h, http.MethodGet, "/api/v1/file/stat?"+q.Encode(), withSession())
	if rec.Code != http.StatusOK {
		t.Fatalf("stat: %d %s", rec.Code, rec.Body.String())
	}
	var out FileStatResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out.Files
}

func errCode(t *testing.T, body string) string {
	t.Helper()
	var e errorBody
	if err := json.Unmarshal([]byte(body), &e); err != nil {
		t.Fatalf("not an error body: %q", body)
	}
	return e.Error.Code
}

func TestFileServesAnAllowedFile(t *testing.T) {
	f := newFilesFixture(t, nil)
	rec := f.get(".beads/captures/a.png")
	if rec.Code != http.StatusOK || rec.Body != "PNGDATA" {
		t.Fatalf("got %d %q", rec.Code, rec.Body)
	}
	h := rec.Header
	if h.Get("Content-Type") != "image/png" {
		t.Errorf("Content-Type = %q", h.Get("Content-Type"))
	}
	if h.Get("Cache-Control") != "private, no-cache" {
		t.Errorf("Cache-Control = %q", h.Get("Cache-Control"))
	}
	if csp := h.Get("Content-Security-Policy"); !strings.Contains(csp, "sandbox") || !strings.Contains(csp, "default-src 'none'") {
		t.Errorf("CSP = %q, want the sandboxing file policy", csp)
	}
	if !strings.HasPrefix(h.Get("Content-Disposition"), "inline; filename*=UTF-8''a.png") {
		t.Errorf("Content-Disposition = %q", h.Get("Content-Disposition"))
	}
	etag := h.Get("ETag")
	if !strings.HasPrefix(etag, `"`) || strings.HasPrefix(etag, "W/") {
		t.Fatalf("ETag = %q, want a strong validator (If-Range needs one)", etag)
	}
	if again := f.get(".beads/captures/a.png", func(r *http.Request) { r.Header.Set("If-None-Match", etag) }); again.Code != http.StatusNotModified {
		t.Errorf("matching ETag = %d, want 304", again.Code)
	}
	if hd := f.do(http.MethodHead, "/api/v1/file?path="+url.QueryEscape(".beads/captures/a.png"), withSession()); hd.Code != http.StatusOK || hd.Body != "" {
		t.Errorf("HEAD = %d %q", hd.Code, hd.Body)
	}
	// Range requests work (video seeking needs them).
	rng := f.get(".beads/captures/a.png", func(r *http.Request) { r.Header.Set("Range", "bytes=0-2") })
	if rng.Code != http.StatusPartialContent || rng.Body != "PNG" {
		t.Errorf("range = %d %q", rng.Code, rng.Body)
	}
	// ... and a resumed one with If-Range (the strong ETag) is honoured instead of sent whole.
	resumed := f.get(".beads/captures/a.png", func(r *http.Request) {
		r.Header.Set("Range", "bytes=3-")
		r.Header.Set("If-Range", etag)
	})
	if resumed.Code != http.StatusPartialContent || resumed.Body != "DATA" {
		t.Errorf("If-Range resume = %d %q", resumed.Code, resumed.Body)
	}
}

func TestFileTypesAndPolicies(t *testing.T) {
	f := newFilesFixture(t, nil)
	for path, want := range map[string]string{
		"icon.svg": "image/svg+xml",
		"notes.md": "text/plain; charset=utf-8",
		"doc.pdf":  "application/pdf",
	} {
		rec := f.get(path)
		if rec.Code != http.StatusOK || rec.Header.Get("Content-Type") != want {
			t.Errorf("%s: %d %q, want 200 %q", path, rec.Code, rec.Header.Get("Content-Type"), want)
		}
	}
	// Every served type gets the sandboxing policy, with the directives default-src does not cover.
	for _, p := range []string{"icon.svg", "notes.md", "doc.pdf"} {
		csp := f.get(p).Header.Get("Content-Security-Policy")
		if !strings.HasSuffix(csp, "; sandbox") || !strings.Contains(csp, "form-action 'none'") || !strings.Contains(csp, "base-uri 'none'") || !strings.Contains(csp, "default-src 'none'") {
			t.Errorf("%s CSP = %q", p, csp)
		}
	} // Not on the allow-list: settings and tracker metadata stay off the wire.
	if rec := f.get("settings.json"); rec.Code != http.StatusUnsupportedMediaType || errCode(t, rec.Body) != "unsupported_type" {
		t.Errorf("json: %d %s", rec.Code, rec.Body)
	}
	// .git is never served, whatever the extension.
	if rec := f.get(".git/config.png"); rec.Code != http.StatusForbidden {
		t.Errorf(".git: %d", rec.Code)
	}
}

func TestFileNeedsASessionAndGet(t *testing.T) {
	f := newFilesFixture(t, nil)
	for _, target := range []string{"/api/v1/file?path=icon.svg", "/api/v1/file/stat?path=icon.svg"} {
		if rec := f.do(http.MethodGet, target); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s without a session = %d", target, rec.Code)
		}
		if rec := f.do(http.MethodPost, target, withSession(), withOrigin(localOrigin)); rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("POST %s = %d", target, rec.Code)
		}
	}
	if rec := f.do(http.MethodGet, "/api/v1/file", withSession()); rec.Code != http.StatusBadRequest {
		t.Errorf("no path = %d", rec.Code)
	}
}

func TestFilesDisabledWithoutRoots(t *testing.T) {
	h := newTestHandler(t, nil)
	for _, target := range []string{"/api/v1/file?path=x.png", "/api/v1/file/stat?path=x.png"} {
		rec := do(h, http.MethodGet, target, withSession())
		if rec.Code != http.StatusNotFound || errCode(t, rec.Body.String()) != "files_disabled" {
			t.Errorf("%s: %d %s", target, rec.Code, rec.Body.String())
		}
	}
}

func TestFileRefusesWhatIsOutsideTheRoots(t *testing.T) {
	f := newFilesFixture(t, nil)
	outside := filepath.Join(filepath.Dir(f.root), "outside-"+filepath.Base(f.root)+".png")
	writeFile(t, outside, "SECRET")
	t.Cleanup(func() { _ = os.Remove(outside) })

	for _, p := range []string{
		"../" + filepath.Base(outside),
		`..\` + filepath.Base(outside),
		"shots/../../" + filepath.Base(outside),
		outside,                                    // absolute, exists
		filepath.Join(filepath.Dir(outside), "no"), // absolute, does not exist: same answer, no existence leak
	} {
		rec := f.get(p)
		if rec.Code != http.StatusForbidden || errCode(t, rec.Body) != "outside_roots" {
			t.Errorf("%q: %d %s, want 403 outside_roots", p, rec.Code, rec.Body)
		}
		if strings.Contains(rec.Body, "SECRET") {
			t.Errorf("%q leaked the file", p)
		}
	}
}

func TestFileRefusesASymlinkLeavingTheRoot(t *testing.T) {
	f := newFilesFixture(t, nil)
	outsideDir := t.TempDir()
	writeFile(t, filepath.Join(outsideDir, "secret.png"), "SECRET")
	link := filepath.Join(f.root, "link")
	if err := os.Symlink(outsideDir, link); err != nil {
		// Windows without the symlink privilege: a directory junction is the same escape.
		if runtime.GOOS != "windows" {
			t.Skipf("symlinks are not available here: %v", err)
		}
		if out, jerr := exec.Command("cmd", "/c", "mklink", "/J", link, outsideDir).CombinedOutput(); jerr != nil {
			t.Skipf("neither symlinks nor junctions are available here: %v: %s", jerr, out)
		}
	}
	rec := f.get("link/secret.png")
	if rec.Code == http.StatusOK || strings.Contains(rec.Body, "SECRET") {
		t.Fatalf("a symlink out of the root was followed: %d %q", rec.Code, rec.Body)
	}
	if rec.Code != http.StatusForbidden {
		t.Errorf("symlink escape = %d, want 403", rec.Code)
	}
}

func TestFileRejectsNetworkAndStreamPaths(t *testing.T) {
	f := newFilesFixture(t, nil)
	for _, p := range []string{
		`\\host\share\a.png`, `//host/a.png`, `\\?\C:\a.png`, `\\.\pipe\x`,
		"shots:stream.png", "x:y.png", "C:a.png", "shots/a.png:hidden",
		"file://host/share/a.png", "a\x00.png", "  ", "<>",
		`/\host\share\a.png`, `\/host/share/a.png`, // two separators of either kind
		"C:",                                  // a bare drive
		"file:shots/a.png", "file:..%2Fx.png", // an opaque file: URL has no path
		// The %-decoded form goes through the same checks: these decode to a colon or a UNC path.
		"x%3Ay.png", "C%3Aa.png", "a.png%3Ahidden", "%5C%5Chost%5Cshare%5Ca.png", "%2F%2Fhost%2Fa.png",
	} {
		rec := f.get(p)
		if rec.Code != http.StatusBadRequest || errCode(t, rec.Body) != "invalid_path" {
			t.Errorf("%q: %d %s, want 400 invalid_path", p, rec.Code, rec.Body)
		}
	}
}

func TestFileResolvesTheFormsBeadTextUses(t *testing.T) {
	f := newFilesFixture(t, nil)
	abs := filepath.Join(f.root, ".beads", "captures", "a.png")
	slashes := filepath.ToSlash(abs)
	fileURL := "file:///" + strings.TrimPrefix(slashes, "/")
	for name, p := range map[string]string{
		"relative":              ".beads/captures/a.png",
		"relative, ./":          "./.beads/captures/a.png",
		"relative, backslashes": `.beads\captures\a.png`,
		"absolute":              abs,
		"forward slashes":       slashes,
		"file URL":              fileURL,
		"angle brackets":        "<" + abs + ">",
		"quoted":                `"` + abs + `"`,
		"surrounding space":     "  " + abs + "  ",
	} {
		if rec := f.get(p); rec.Code != http.StatusOK || rec.Body != "PNGDATA" {
			t.Errorf("%s (%q): %d %s", name, p, rec.Code, rec.Body)
		}
	}
	// A name with a space, as %20, and a name that really contains "%20".
	if rec := f.get("shots/my%20shot.png"); rec.Code != http.StatusOK || rec.Body != "SPACED" {
		t.Errorf("%%20 form: %d %s", rec.Code, rec.Body)
	}
	if rec := f.get("shots/100%20.png"); rec.Code != http.StatusOK || rec.Body != "LITERAL" {
		t.Errorf("literal %%20 name: %d %s", rec.Code, rec.Body)
	}
	if rec := f.get("shots/nope.png"); rec.Code != http.StatusNotFound || errCode(t, rec.Body) != "not_found" {
		t.Errorf("missing: %d %s", rec.Code, rec.Body)
	}
}

func TestFileDriveLetterCaseDoesNotMatter(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("drive letters")
	}
	f := newFilesFixture(t, nil)
	abs := filepath.Join(f.root, "notes.md")
	flipped := strings.ToLower(abs[:1])
	if flipped == abs[:1] {
		flipped = strings.ToUpper(abs[:1])
	}
	if rec := f.get(flipped + abs[1:]); rec.Code != http.StatusOK {
		t.Fatalf("drive letter case: %d %s", rec.Code, rec.Body)
	}
	if rec := f.get(strings.ToUpper(abs)); rec.Code != http.StatusOK {
		t.Fatalf("path case: %d %s", rec.Code, rec.Body)
	}
}

func TestFileSizeCap(t *testing.T) {
	f := newFilesFixture(t, func(c *Config, _ string) { c.maxFileBytes = 10 })
	rec := f.get(".beads/captures/a.png") // 7 bytes
	if rec.Code != http.StatusOK {
		t.Fatalf("small file: %d", rec.Code)
	}
	writeFile(t, filepath.Join(f.root, "big.png"), strings.Repeat("x", 11))
	if rec := f.get("big.png"); rec.Code != http.StatusRequestEntityTooLarge || errCode(t, rec.Body) != "too_large" {
		t.Errorf("big file: %d %s", rec.Code, rec.Body)
	}
	if st := f.stat(t, "big.png"); st[0].Status != "too_large" {
		t.Errorf("stat of a big file = %+v", st[0])
	}
}

// A relative path resolves against the project directory; failing that, against the one git
// repository directly inside it that has the file, and two such repositories are ambiguous.
func TestFileRelativePathsAndChildRepositories(t *testing.T) {
	f := newFilesFixture(t, nil)
	for _, repo := range []string{"alpha", "beta"} {
		if err := os.MkdirAll(filepath.Join(f.root, repo, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, filepath.Join(f.root, "alpha", "docs", "only-alpha.png"), "ALPHA")
	writeFile(t, filepath.Join(f.root, "alpha", "docs", "both.png"), "A")
	writeFile(t, filepath.Join(f.root, "beta", "docs", "both.png"), "B")
	writeFile(t, filepath.Join(f.root, "docs", "both.png"), "TOP")

	// "alpha/docs/x.png" works from the project directory; so does "docs/x.png" through the repository.
	if rec := f.get("alpha/docs/only-alpha.png"); rec.Code != http.StatusOK || rec.Body != "ALPHA" {
		t.Errorf("explicit repository: %d %s", rec.Code, rec.Body)
	}
	if rec := f.get("docs/only-alpha.png"); rec.Code != http.StatusOK || rec.Body != "ALPHA" {
		t.Errorf("unique repository: %d %s", rec.Code, rec.Body)
	}
	// The project directory itself wins when it has the file.
	if rec := f.get("docs/both.png"); rec.Code != http.StatusOK || rec.Body != "TOP" {
		t.Errorf("project directory first: %d %s", rec.Code, rec.Body)
	}
	if err := os.Remove(filepath.Join(f.root, "docs", "both.png")); err != nil {
		t.Fatal(err)
	}
	rec := f.get("docs/both.png")
	if rec.Code != http.StatusConflict || errCode(t, rec.Body) != "ambiguous_path" {
		t.Errorf("two repositories: %d %s, want 409 ambiguous_path", rec.Code, rec.Body)
	}
	if st := f.stat(t, "docs/both.png"); st[0].Status != "ambiguous" || st[0].Abs != "" {
		t.Errorf("stat of an ambiguous path = %+v", st[0])
	}
}

func TestFileExtraAllowedRoots(t *testing.T) {
	scratch := t.TempDir()
	writeFile(t, filepath.Join(scratch, "claude", "shot.png"), "SCRATCH")
	f := newFilesFixture(t, func(c *Config, _ string) {
		// A nested root is dropped in favour of its parent; a missing one is skipped, not fatal.
		c.FileRoots = append(c.FileRoots, scratch, filepath.Join(scratch, "claude"), filepath.Join(scratch, "does-not-exist"))
	})
	if rec := f.get(filepath.Join(scratch, "claude", "shot.png")); rec.Code != http.StatusOK || rec.Body != "SCRATCH" {
		t.Fatalf("extra root: %d %s", rec.Code, rec.Body)
	}
	if rec := f.get(filepath.Join(filepath.Dir(scratch), "elsewhere.png")); rec.Code != http.StatusForbidden {
		t.Fatalf("outside every root: %d", rec.Code)
	}
}

func TestNewFileSetRefusesNetworkRoots(t *testing.T) {
	var warned []string
	got := newFileSet("", []string{`\\nonexistent.invalid\share`, `//nonexistent.invalid/share`, `\\?\C:\x`, `\\.\pipe`}, 0, func(s string) { warned = append(warned, s) })
	// Windows spells these as network or device paths; elsewhere "//x" is just a path, and a root
	// that does not exist is skipped anyway. Either way nothing is served and nothing was reached.
	if got != nil {
		t.Fatalf("a network or device root was accepted: %+v", got)
	}
	if runtime.GOOS == "windows" {
		if len(warned) != 4 {
			t.Fatalf("warned %q, want one warning per root", warned)
		}
		for _, w := range warned {
			if !strings.Contains(w, "never served") {
				t.Errorf("warning %q does not say why", w)
			}
		}
	}
}

func TestNewFileSetRoots(t *testing.T) {
	var warned []string
	warn := func(s string) { warned = append(warned, s) }
	if got := newFileSet("", nil, 0, warn); got != nil {
		t.Fatal("no roots must give no file set")
	}
	dir := t.TempDir()
	if got := newFileSet("", []string{filepath.Join(dir, "nope"), "  "}, 0, warn); got != nil || len(warned) != 1 {
		t.Fatalf("a missing root: %v, warned %v", got, warned)
	}
	writeFile(t, filepath.Join(dir, "sub", "x.png"), "x")
	set := newFileSet(dir, []string{filepath.Join(dir, "sub"), dir, dir}, 0, warn)
	if set == nil || len(set.roots) != 1 {
		t.Fatalf("duplicate and nested roots must collapse to one: %+v", set)
	}
}

func TestFileStatReportsEachPath(t *testing.T) {
	f := newFilesFixture(t, nil)
	abs := filepath.Join(f.root, ".beads", "captures", "a.png")
	got := f.stat(t, ".beads/captures/a.png", "shots/gone.png", "../x.png", "settings.json", "x:y.png", "doc.pdf")
	want := []struct {
		status, kind string
		hasURL       bool
	}{
		{"ok", "image", true}, {"missing", "", false}, {"outside", "", false},
		{"unsupported", "", false}, {"invalid", "", false}, {"ok", "pdf", true},
	}
	if len(got) != len(want) {
		t.Fatalf("%d results for %d paths", len(got), len(want))
	}
	for i, w := range want {
		if got[i].Status != w.status || got[i].Kind != w.kind || (got[i].URL != "") != w.hasURL {
			t.Errorf("path %d (%s) = %+v, want %+v", i, got[i].Path, got[i], w)
		}
	}
	if got[0].Abs != abs || got[0].Size != int64(len("PNGDATA")) || got[0].URL != "/api/v1/file?path="+url.QueryEscape(abs) {
		t.Errorf("an ok file reports abs, size and a URL built from abs: %+v", got[0])
	}
	if got[4].Abs != "" {
		t.Errorf("an invalid path has no abs: %+v", got[4])
	}
	// The URL it hands out serves the file.
	if rec := f.do(http.MethodGet, got[0].URL, withSession()); rec.Code != http.StatusOK || rec.Body != "PNGDATA" {
		t.Errorf("following the URL: %d %s", rec.Code, rec.Body)
	}
	rec := f.do(http.MethodGet, "/api/v1/file/stat?path=icon.svg", withSession())
	if rec.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("stat Cache-Control = %q", rec.Header.Get("Cache-Control"))
	}
}

func TestFileStatLimits(t *testing.T) {
	f := newFilesFixture(t, nil)
	q := url.Values{}
	for range 65 {
		q.Add("path", "icon.svg")
	}
	if rec := f.do(http.MethodGet, "/api/v1/file/stat?"+q.Encode(), withSession()); rec.Code != http.StatusBadRequest {
		t.Errorf("65 paths = %d, want 400", rec.Code)
	}
	q = url.Values{}
	for range 64 {
		q.Add("path", "icon.svg")
	}
	if rec := f.do(http.MethodGet, "/api/v1/file/stat?"+q.Encode(), withSession()); rec.Code != http.StatusOK {
		t.Errorf("64 paths = %d, want 200", rec.Code)
	}
	if rec := f.do(http.MethodGet, "/api/v1/file/stat", withSession()); rec.Code != http.StatusBadRequest {
		t.Errorf("no paths = %d, want 400", rec.Code)
	}
}

func TestMetaReportsTheFilesCapability(t *testing.T) {
	path := writeGoldenTracker(t)
	store := goldenStore(t, path)
	waitPhase2(t, store)
	meta := func(mutate func(*Config)) MetaResponse {
		h := newTestHandler(t, func(c *Config) {
			c.Store = store
			if mutate != nil {
				mutate(c)
			}
		})
		return getJSON[MetaResponse](t, h, "/api/v1/meta")
	}
	if meta(nil).Capabilities.Files {
		t.Error("capabilities.files must be false without roots")
	}
	if !meta(func(c *Config) { c.FileRoots = []string{t.TempDir()} }).Capabilities.Files {
		t.Error("capabilities.files must be true with a root")
	}
	if dir := store.ProjectDir(); dir == "" {
		t.Error("Store.ProjectDir is empty")
	}
}

func TestGoldenFileStat(t *testing.T) {
	f := newFilesFixture(t, nil)
	paths := []string{".beads/captures/a.png", "shots/gone.png", "../elsewhere.png", "settings.json", "x:y.png", "doc.pdf"}
	q := url.Values{}
	for _, p := range paths {
		q.Add("path", p)
	}
	rec := do(f.h, http.MethodGet, "/api/v1/file/stat?"+q.Encode(), withSession())
	var got FileStatResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	// Make the fixture portable: the temp directory becomes ROOT and separators slashes.
	portable := func(abs string) string {
		slashed, root := filepath.ToSlash(abs), filepath.ToSlash(f.root)
		if rest, ok := strings.CutPrefix(slashed, root); ok {
			return "ROOT" + rest
		}
		return "OUTSIDE/" + filepath.Base(abs) // a path beside the project directory
	}
	for i := range got.Files {
		if got.Files[i].Abs != "" {
			got.Files[i].Abs = portable(got.Files[i].Abs)
		}
		if got.Files[i].URL != "" {
			got.Files[i].URL = "/api/v1/file?path=" + url.QueryEscape(got.Files[i].Abs)
		}
	}
	data, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatal(err)
	}
	testutil.NewGoldenFile(t, goldenDir, "file-stat.json").AssertJSON(v)
}
