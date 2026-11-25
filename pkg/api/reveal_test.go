package api

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
	"github.com/Dicklesworthstone/beads_viewer/pkg/snapshot"
)

// fakeReveal records what the handler asked the file manager to show.
type fakeReveal struct {
	mu    sync.Mutex
	paths []string
	err   error
}

func (f *fakeReveal) reveal(abs string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.paths = append(f.paths, abs)
	return f.err
}

func (f *fakeReveal) calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.paths)
}

type revealFixture struct {
	h    http.Handler
	root string
	fake *fakeReveal
}

// writableOrigin is a complete bd write route (as snapshot.Load gives a tracker bv can write to),
// or a read-only one.
func writableOrigin(dir string, readOnly bool) *model.IssueOrigin {
	o := &model.IssueOrigin{
		LocalID: "fx", Tracker: "bd", Executable: filepath.Join(dir, "bd.exe"),
		WorkingDirectory: dir, TrackerDirectory: filepath.Join(dir, ".beads"), Database: filepath.Join(dir, ".beads", "db"),
	}
	if readOnly {
		o.ReadOnlyReason = "locked"
	}
	return o
}

func newRevealFixture(t *testing.T, readOnly bool, mutate func(*Config)) *revealFixture {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "shots", "a.png"), "PNG")
	writeFile(t, filepath.Join(root, "tool.exe"), "MZ")
	writeFile(t, filepath.Join(root, "settings.json"), "{}")
	writeFile(t, filepath.Join(root, "big.png"), strings.Repeat("x", 20))

	origin := writableOrigin(root, readOnly)
	load := func(ctx context.Context, p string, o snapshot.LoadOptions) (*snapshot.Snapshot, error) {
		snap, err := snapshot.Load(ctx, p, o)
		if err == nil {
			for i := range snap.Issues {
				snap.Issues[i].Origin = origin
			}
		}
		return snap, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	store, err := newStore(ctx, writeGoldenTracker(t), snapshot.LoadOptions{Now: goldenNow}, load, realComplete)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close(); cancel() })

	fake := &fakeReveal{}
	h := newTestHandler(t, func(c *Config) {
		c.Store = store
		c.FileBase = root
		c.FileRoots = []string{root}
		c.reveal = fake.reveal
		c.maxFileBytes = 10
		if mutate != nil {
			mutate(c)
		}
	})
	return &revealFixture{h: h, root: root, fake: fake}
}

func (f *revealFixture) post(path string, opts ...reqOpt) (int, string) {
	body, _ := json.Marshal(RevealRequest{Path: path})
	return f.postBody(string(body), opts...)
}

func (f *revealFixture) postBody(body string, opts ...reqOpt) (int, string) {
	opts = append([]reqOpt{withSession(), withOrigin(localOrigin), withContentType("application/json")}, opts...)
	rec := sendJSON(f.h, http.MethodPost, "/api/v1/file/reveal", body, opts...)
	return rec.Code, rec.Body.String()
}
func TestRevealShowsAFileUnderARoot(t *testing.T) {
	f := newRevealFixture(t, false, nil)
	want := filepath.Join(f.root, "shots", "a.png")
	code, body := f.post("shots/a.png")
	if code != http.StatusOK {
		t.Fatalf("reveal = %d %s", code, body)
	}
	if got := f.fake.calls(); !slices.Equal(got, []string{want}) {
		t.Fatalf("the file manager was asked for %q, want the resolved absolute path %q", got, want)
	}
	var resp RevealResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil || resp.Path != want {
		t.Fatalf("response %q (%v)", body, err)
	}
	// The absolute path from /file/stat works as well.
	if code, _ := f.post(want); code != http.StatusOK || len(f.fake.calls()) != 2 {
		t.Fatalf("absolute path = %d, calls %q", code, f.fake.calls())
	}
}

// A file bv does not preview, or that is over the preview cap, can still be shown: only a file that exists
// under a root is needed.
func TestRevealShowsFilesBvDoesNotPreview(t *testing.T) {
	f := newRevealFixture(t, false, nil)
	for _, p := range []string{"tool.exe", "settings.json", "big.png"} {
		if code, body := f.post(p); code != http.StatusOK {
			t.Errorf("%s: %d %s", p, code, body)
		}
	}
	if len(f.fake.calls()) != 3 {
		t.Fatalf("calls %q", f.fake.calls())
	}
}

func TestRevealRefusesWhatIsNotAFileUnderARoot(t *testing.T) {
	f := newRevealFixture(t, false, nil)
	for name, tc := range map[string]struct {
		path string
		code int
	}{
		"outside":   {"../elsewhere.png", http.StatusForbidden},
		"absolute":  {filepath.Join(filepath.Dir(f.root), "x.png"), http.StatusForbidden},
		"missing":   {"shots/gone.png", http.StatusNotFound},
		"directory": {"shots", http.StatusNotFound},
		"unc":       {`\\host\share\a.png`, http.StatusBadRequest},
		"stream":    {"shots/a.png:hidden", http.StatusBadRequest},
		"decoded":   {"shots%5C%5Chost%5Ca.png", http.StatusNotFound},
		"blank":     {"  ", http.StatusBadRequest},
	} {
		if code, body := f.post(tc.path); code != tc.code {
			t.Errorf("%s (%q): %d %s, want %d", name, tc.path, code, body, tc.code)
		}
	}
	if got := f.fake.calls(); len(got) != 0 {
		t.Fatalf("the file manager was started for a refused path: %q", got)
	}
}

func TestRevealIsGatedLikeOtherActions(t *testing.T) {
	// POST only, from the app's origin, with a session.
	f := newRevealFixture(t, false, nil)
	if rec := do(f.h, http.MethodGet, "/api/v1/file/reveal", withSession()); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET = %d", rec.Code)
	}
	if code, _ := f.post("shots/a.png", withOrigin("http://evil.example")); code != http.StatusForbidden {
		t.Errorf("a foreign origin = %d, want 403", code)
	}
	if code, _ := f.post("shots/a.png", withoutOrigin()); code != http.StatusForbidden {
		t.Errorf("no origin = %d, want 403", code)
	}
	rec := sendJSON(f.h, http.MethodPost, "/api/v1/file/reveal", `{"path":"shots/a.png"}`, func(r *http.Request) { r.Header.Del("Cookie") })
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("no session = %d, want 401", rec.Code)
	}
	for name, body := range map[string]string{"unknown field": `{"path":"shots/a.png","open":true}`, "not json": `x`, "two values": `{"path":"a.png"}{}`} {
		if code, _ := f.postBody(body); code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", name, code)
		}
	}
	if got := f.fake.calls(); len(got) != 0 {
		t.Fatalf("rejected requests reached the file manager: %q", got)
	}
}

func TestRevealNeedsAWritableTracker(t *testing.T) {
	f := newRevealFixture(t, true, nil)
	code, body := f.post("shots/a.png")
	if code != http.StatusForbidden || errCode(t, body) != "read_only" {
		t.Fatalf("read-only tracker: %d %s", code, body)
	}
	if len(f.fake.calls()) != 0 {
		t.Fatal("revealed on a read-only tracker")
	}
	meta := getJSON[MetaResponse](t, f.h, "/api/v1/meta")
	if meta.Capabilities.Reveal || meta.Capabilities.Writes || !meta.Capabilities.Files {
		t.Errorf("a read-only tracker still serves files but cannot reveal: %+v", meta.Capabilities)
	}
}
func TestRevealUnavailableCases(t *testing.T) {
	// No files served.
	f := newRevealFixture(t, false, func(c *Config) { c.FileRoots = nil; c.FileBase = "" })
	code, body := f.post("a.png")
	if code != http.StatusNotFound || errCode(t, body) != "files_disabled" {
		t.Errorf("no roots: %d %s", code, body)
	}
	if getJSON[MetaResponse](t, f.h, "/api/v1/meta").Capabilities.Reveal {
		t.Error("reveal capability without files")
	}
	// A failing file manager is a 500 that does not echo the path.
	g := newRevealFixture(t, false, nil)
	g.fake.err = http.ErrAbortHandler
	code, body = g.post("shots/a.png")
	if code != http.StatusInternalServerError || errCode(t, body) != "reveal_failed" || strings.Contains(body, g.root) {
		t.Errorf("failing revealer: %d %s", code, body)
	}
}
func TestRevealCapabilityInMeta(t *testing.T) {
	f := newRevealFixture(t, false, nil)
	meta := getJSON[MetaResponse](t, f.h, "/api/v1/meta")
	if !meta.Capabilities.Reveal || !meta.Capabilities.Files || !meta.Capabilities.Writes {
		t.Fatalf("capabilities = %+v, want files, writes and reveal", meta.Capabilities)
	}
}

// Off Windows there is no revealer unless one is injected: 501 and no capability.
func TestRevealUnsupportedWithoutARevealer(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has a revealer")
	}
	f := newRevealFixture(t, false, func(c *Config) { c.reveal = nil })
	code, body := f.post("shots/a.png")
	if code != http.StatusNotImplemented || errCode(t, body) != "reveal_unsupported" {
		t.Errorf("no revealer: %d %s", code, body)
	}
	if getJSON[MetaResponse](t, f.h, "/api/v1/meta").Capabilities.Reveal {
		t.Error("reveal capability without a revealer")
	}
}
