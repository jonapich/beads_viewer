package api

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Local files in the GUI (beads-c2o.64): bead text names screenshots and other
// files by path, and the GUI shows them. Two authenticated GETs serve that:
//
//	GET /api/v1/file?path=<raw>              streams one file
//	GET /api/v1/file/stat?path=a&path=b ...  says what each path is, without reading it
//
// Only files under the allowed roots are served: the tracker's project
// directory plus each --serve-allow directory. Every file is opened through
// os.Root, so ".." and symlinks that leave the root are refused by the operating
// system layer, not by string checks. Only an allow-list of extensions is
// served, with a content type of our own (the registry's mappings vary on
// Windows), and SVG and text get a sandboxing CSP so a file opened top level has
// no script, cookie or API access. The session cookie reaches <img src> as it
// is: there is no token in these URLs.

// maxFileBytes is the largest file served.
const maxFileBytes = 50 << 20

// maxStatPaths bounds one /file/stat request.
const maxStatPaths = 64

// File kinds, as FileStat.Kind says them.
const (
	fileKindImage = "image"
	fileKindVideo = "video"
	fileKindPDF   = "pdf"
	fileKindText  = "text"
)

// File statuses, as FileStat.Status says them.
const (
	fileOK          = "ok"
	fileMissing     = "missing"
	fileOutside     = "outside"
	fileAmbiguous   = "ambiguous"
	fileUnsupported = "unsupported"
	fileTooLarge    = "too_large"
	fileInvalid     = "invalid"
)

type fileType struct {
	mime string
	kind string
}

// fileTypes is the extension allow-list. Deliberately no json, yaml, html or
// source files: that keeps settings, tracker metadata and mocks off the wire.
var fileTypes = map[string]fileType{
	".png":  {"image/png", fileKindImage},
	".jpg":  {"image/jpeg", fileKindImage},
	".jpeg": {"image/jpeg", fileKindImage},
	".gif":  {"image/gif", fileKindImage},
	".webp": {"image/webp", fileKindImage},
	".avif": {"image/avif", fileKindImage},
	".bmp":  {"image/bmp", fileKindImage},
	".svg":  {"image/svg+xml", fileKindImage},
	".mp4":  {"video/mp4", fileKindVideo},
	".webm": {"video/webm", fileKindVideo},
	".pdf":  {"application/pdf", fileKindPDF},
	".txt":  {"text/plain; charset=utf-8", fileKindText},
	".log":  {"text/plain; charset=utf-8", fileKindText},
	".md":   {"text/plain; charset=utf-8", fileKindText},
	".csv":  {"text/plain; charset=utf-8", fileKindText},
}

// fileCSP is the Content-Security-Policy of every served file. sandbox gives an SVG
// or text document opened in its own tab an opaque origin (no script, no cookie,
// so no way to call the API); base-uri and form-action are listed because
// default-src does not cover them. PDFs get the same policy: checked in Edge
// (Chromium, driven by Playwright, beads-c2o.64 S3), a PDF served with the sandbox
// token still renders in the built-in viewer. Chromium's PDF viewer page is
// created by the browser, so the token does not make that page's origin opaque
// either; what isolates the document is the viewer itself.
const fileCSP = "default-src 'none'; img-src 'self'; media-src 'self'; style-src 'unsafe-inline'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'; sandbox"

// fileRoot is one allowed directory. abs is the absolute path as given (the
// form a candidate path is compared with); real is it with symlinks resolved
// (the directory os.Root is opened on, per request: a long-lived os.Root would
// keep the directory open for the life of the server). They differ when the root
// is reached through a link or an 8.3 short name.
type fileRoot struct {
	abs  string
	real string
}

// fileSet is the allowed roots, and what relative paths resolve against.
type fileSet struct {
	roots    []fileRoot
	base     string // relative paths resolve against this (the tracker's project directory)
	maxBytes int64
}

// newFileSet opens the roots. A root that cannot be opened is reported to warn
// and skipped; duplicates and roots nested in another root are dropped.
// Returns nil when no root is left.
func newFileSet(base string, dirs []string, maxBytes int64, warn func(string)) *fileSet {
	if maxBytes <= 0 {
		maxBytes = maxFileBytes
	}
	fsys := &fileSet{maxBytes: maxBytes}
	if base != "" {
		if abs, err := filepath.Abs(base); err == nil {
			fsys.base = abs
		}
	}
	type cand struct{ abs, real string }
	var cands []cand
	for _, d := range dirs {
		if strings.TrimSpace(d) == "" {
			continue
		}
		abs, err := filepath.Abs(d)
		if err != nil {
			warn(fmt.Sprintf("%s: %v", d, err))
			continue
		}
		// Checked before anything touches the file system: EvalSymlinks and OpenRoot on \\host\share
		// would reach out to that host at startup and again on every request.
		if isNetworkOrDevicePath(abs) {
			warn(fmt.Sprintf("%s: network and device paths are never served", d))
			continue
		}
		real, err := filepath.EvalSymlinks(abs)
		if err != nil {
			warn(fmt.Sprintf("%s: %v", d, err))
			continue
		}
		if isNetworkOrDevicePath(real) { // a junction or link whose target is a network path
			warn(fmt.Sprintf("%s: resolves to a network or device path, which is never served", d))
			continue
		}
		if info, err := os.Stat(real); err != nil || !info.IsDir() {
			warn(fmt.Sprintf("%s: not a directory", d))
			continue
		}
		cands = append(cands, cand{abs, real})
	}
	// Shortest first, so a nested root is dropped in favour of its parent.
	sort.SliceStable(cands, func(i, j int) bool { return len(cands[i].real) < len(cands[j].real) })
	for _, c := range cands {
		nested := false
		for _, have := range fsys.roots {
			if _, ok := relInside(have.real, c.real); ok {
				nested = true
				break
			}
		}
		if nested {
			continue
		}
		root, err := os.OpenRoot(c.real)
		if err != nil {
			warn(fmt.Sprintf("%s: %v", c.abs, err))
			continue
		}
		_ = root.Close() // only to prove it can be opened
		fsys.roots = append(fsys.roots, fileRoot{abs: c.abs, real: c.real})
	}
	if len(fsys.roots) == 0 {
		return nil
	}
	return fsys
}

// isNetworkOrDevicePath reports a path that starts with two separators: \\host\share,
// \\?\ and \\.\ forms, and //host. Opening one would reach the network or a device.
func isNetworkOrDevicePath(s string) bool {
	return len(s) >= 2 && os.IsPathSeparator(s[0]) && os.IsPathSeparator(s[1])
}

// relInside returns p relative to root when p is root or below it.
func relInside(root, p string) (string, bool) {
	rel, err := filepath.Rel(root, p)
	if err != nil || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return rel, true
}

// fileResult is what resolving one path found. file is open only when resolve
// was asked to open it, and the caller closes it.
type fileResult struct {
	status  string
	abs     string
	kind    string
	mime    string
	size    int64
	modTime time.Time
	file    *os.File
}

// resolve turns the raw text of a path (as bead text writes it) into a file
// under an allowed root. It never touches the network and reports "outside" for
// anything not under a root without saying whether it exists.
func (f *fileSet) resolve(raw string, open bool) fileResult {
	p, ok := normalizeRawPath(raw)
	if !ok {
		return fileResult{status: fileInvalid}
	}
	res := f.resolveOnce(p, open)
	// A path copied from a URL may carry %20 and friends; a literal "%20" in a
	// real file name wins because it is tried first.
	if res.status == fileMissing && strings.Contains(p, "%") {
		if q, err := url.PathUnescape(p); err == nil && q != p {
			// The decoded form is checked like the original: "x%3Ay.png" must not become a stream name.
			if !checkRawPath(q) {
				return fileResult{status: fileInvalid}
			}
			if again := f.resolveOnce(q, open); again.status != fileMissing {
				return again
			}
		}
	}
	return res
}

// normalizeRawPath trims the text, unwraps a file: URL, and rejects what must
// never be opened (see checkRawPath).
func normalizeRawPath(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	for len(s) >= 2 {
		first, last := s[0], s[len(s)-1]
		if (first == '<' && last == '>') || (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			s = strings.TrimSpace(s[1 : len(s)-1])
			continue
		}
		break
	}
	if len(s) >= 5 && strings.EqualFold(s[:5], "file:") {
		u, err := url.Parse(s)
		if err != nil || (u.Host != "" && !strings.EqualFold(u.Host, "localhost")) {
			return "", false
		}
		s = u.Path
		// file:///C:/x carries the drive after a slash.
		if len(s) >= 3 && s[0] == '/' && isDriveAt(s, 1) {
			s = s[1:]
		}
	}
	// An opaque file: URL (file:shots/a.png) has no path: invalid, not the base directory.
	if !checkRawPath(s) {
		return "", false
	}
	return s, true
}

// checkRawPath rejects what must never reach resolution, for the path as written and again for
// its %-decoded form: an empty path, NUL, UNC and device paths (two leading separators of either
// kind: opening \\host\share would send credentials to that host) and any colon that is not the
// drive colon of "X:\" or "X:/" (NTFS alternate streams, drive-relative "C:a", a bare "C:").
func checkRawPath(s string) bool {
	if s == "" || strings.ContainsRune(s, 0) || isNetworkOrDevicePath(s) {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] != ':' {
			continue
		}
		if i != 1 || len(s) < 3 || !isDriveAt(s, 0) {
			return false
		}
	}
	return true
}

// isDriveAt reports whether s[i:] starts with a drive prefix "X:" followed by a
// separator (X:\ or X:/) or the end.
func isDriveAt(s string, i int) bool {
	if len(s) < i+2 || s[i+1] != ':' {
		return false
	}
	c := s[i]
	if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') {
		return false
	}
	return len(s) == i+2 || s[i+2] == '\\' || s[i+2] == '/'
}

func (f *fileSet) resolveOnce(p string, open bool) fileResult {
	if filepath.IsAbs(p) {
		res, _ := f.lookup(filepath.Clean(p), open)
		return res
	}
	if f.base == "" {
		return fileResult{status: fileMissing}
	}
	rel := filepath.Clean(filepath.FromSlash(p))
	first, found := f.lookup(filepath.Join(f.base, rel), open)
	if found || first.status == fileOutside || first.status == fileInvalid {
		return first
	}
	// Not under the project directory itself: try the git repositories that sit directly in it,
	// which is how "beads_viewer/docs/x.png" and "docs/x.png" both work in a workspace. Only a
	// unique hit counts.
	var hits []fileResult
	for _, dir := range f.childRepos() {
		res, ok := f.lookup(filepath.Join(dir, rel), open)
		if ok {
			hits = append(hits, res)
		} else if res.file != nil {
			_ = res.file.Close()
		}
	}
	switch len(hits) {
	case 0:
		return first
	case 1:
		return hits[0]
	}
	for _, h := range hits {
		if h.file != nil {
			_ = h.file.Close()
		}
	}
	return fileResult{status: fileAmbiguous}
}

// childRepos lists the direct child directories of the base that hold a .git entry.
func (f *fileSet) childRepos() []string {
	entries, err := os.ReadDir(f.base)
	if err != nil {
		return nil
	}
	var dirs []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(f.base, e.Name())
		if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
			dirs = append(dirs, dir)
		}
	}
	return dirs
}

// lookup resolves one absolute candidate. found says whether the path names
// something that exists (anything but missing).
func (f *fileSet) lookup(abs string, open bool) (res fileResult, found bool) {
	res.abs = abs
	var root *fileRoot
	var rel string
	for i := range f.roots {
		for _, base := range []string{f.roots[i].abs, f.roots[i].real} {
			if r, ok := relInside(base, abs); ok {
				root, rel = &f.roots[i], r
				break
			}
		}
		if root != nil {
			break
		}
	}
	if root == nil {
		res.status = fileOutside
		return res, true
	}
	if slices.ContainsFunc(strings.Split(filepath.ToSlash(rel), "/"), func(seg string) bool {
		return strings.EqualFold(seg, ".git") || strings.EqualFold(seg, ".dolt")
	}) {
		res.status = fileOutside
		return res, true
	}
	// The .git and .dolt refusal above is lexical, so a short (8.3) alias of ".git" would get past
	// it; what keeps those directories off the wire is the extension allow-list below.
	rootDir, err := os.OpenRoot(root.real)
	if err != nil {
		res.status = fileMissing
		return res, false
	}
	defer rootDir.Close() // a file opened through it stays valid
	// Stat before Open: opening a FIFO or device named x.png would block, and /file/stat never
	// needs the content. Root.Stat follows links only inside the root.
	info, err := rootDir.Stat(rel)
	if err != nil {
		return rootError(res, err)
	}
	if !info.Mode().IsRegular() {
		res.status = fileMissing // a directory or device is not a file we serve
		return res, true
	}
	t, supported := fileTypes[strings.ToLower(filepath.Ext(abs))]
	switch {
	case !supported:
		res.status = fileUnsupported
		return res, true
	case info.Size() > f.maxBytes:
		res.status = fileTooLarge
		res.size = info.Size()
		return res, true
	}
	res.status = fileOK
	res.kind, res.mime, res.size, res.modTime = t.kind, t.mime, info.Size(), info.ModTime()
	if !open {
		return res, true
	}
	file, err := rootDir.Open(rel)
	if err != nil {
		return rootError(res, err)
	}
	// The handle is what is served: check it again, so the cap holds even if the file changed
	// between the Stat and the Open.
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || opened.Size() > f.maxBytes {
		_ = file.Close()
		res.status = fileTooLarge
		if err != nil || !opened.Mode().IsRegular() {
			res.status = fileMissing
		}
		return res, true
	}
	res.size, res.modTime = opened.Size(), opened.ModTime()
	res.file = file
	return res, true
}

// rootError turns an os.Root error into a status. A missing file is not found. os.Root reports a
// path or symlink that leaves the root with an unexported "path escapes from parent" error, so
// it is matched by message: should that text ever change, the fallback is invalid, which also
// refuses the request.
func rootError(res fileResult, err error) (fileResult, bool) {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		res.status = fileMissing
		return res, false
	case strings.Contains(err.Error(), "escapes"):
		res.status = fileOutside
	default:
		res.status = fileInvalid
	}
	return res, true
}

// ---- handlers

func (s *server) files() *fileSet { return s.fileSet }

func fileMethodOK(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "use GET")
		return false
	}
	return true
}

// handleFile serves GET /api/v1/file?path=.
func (s *server) handleFile(w http.ResponseWriter, r *http.Request) {
	if !fileMethodOK(w, r) {
		return
	}
	fsys := s.files()
	if fsys == nil {
		writeError(w, http.StatusNotFound, "files_disabled", "this server serves no local files")
		return
	}
	raw := r.URL.Query().Get("path")
	if raw == "" {
		writeError(w, http.StatusBadRequest, "invalid_path", "path is required")
		return
	}
	res := fsys.resolve(raw, true)
	if res.file != nil {
		defer res.file.Close()
	}
	if res.status != fileOK {
		writeFileStatusError(w, res.status, fsys.maxBytes)
		return
	}

	h := w.Header()
	h.Set("Content-Type", res.mime)
	// Captures are overwritten under the same name: always revalidate.
	h.Set("Cache-Control", "private, no-cache")
	// A strong validator (size and mtime in nanoseconds): a weak one never matches If-Range, so a
	// resumed Range request would get the whole file again.
	h.Set("ETag", fmt.Sprintf(`"%d-%x"`, res.size, res.modTime.UnixNano()))
	h.Set("Content-Disposition", "inline; filename*=UTF-8''"+url.PathEscape(filepath.Base(res.abs)))
	h.Set("Content-Security-Policy", fileCSP)
	http.ServeContent(w, r, filepath.Base(res.abs), res.modTime, res.file)
}

// writeFileStatusError answers a path that is not a servable file with the status the endpoints share.
func writeFileStatusError(w http.ResponseWriter, status string, maxBytes int64) {
	switch status {
	case fileInvalid:
		writeError(w, http.StatusBadRequest, "invalid_path", "that path cannot be served")
	case fileOutside:
		writeError(w, http.StatusForbidden, "outside_roots", "that path is outside the directories this server may serve")
	case fileMissing:
		writeError(w, http.StatusNotFound, "not_found", "no such file")
	case fileAmbiguous:
		writeError(w, http.StatusConflict, "ambiguous_path", "that relative path matches files in more than one repository")
	case fileTooLarge:
		writeError(w, http.StatusRequestEntityTooLarge, "too_large", fmt.Sprintf("the file is larger than %d MiB", maxBytes>>20))
	default:
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_type", "this kind of file is not served")
	}
}

// handleFileStat serves GET /api/v1/file/stat?path=a&path=b: what each path is.
func (s *server) handleFileStat(w http.ResponseWriter, r *http.Request) {
	if !fileMethodOK(w, r) {
		return
	}
	fsys := s.files()
	if fsys == nil {
		writeError(w, http.StatusNotFound, "files_disabled", "this server serves no local files")
		return
	}
	paths := r.URL.Query()["path"]
	if len(paths) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_path", "at least one path is required")
		return
	}
	if len(paths) > maxStatPaths {
		writeError(w, http.StatusBadRequest, "too_many_paths", "at most "+strconv.Itoa(maxStatPaths)+" paths per request")
		return
	}
	out := FileStatResponse{Files: make([]FileStat, 0, len(paths))}
	for _, p := range paths {
		res := fsys.resolve(p, false)
		st := FileStat{Path: p, Status: res.status}
		if res.status != fileInvalid && res.status != fileAmbiguous {
			st.Abs = res.abs
		}
		if res.status == fileOK {
			st.Kind, st.Size = res.kind, res.size
			st.URL = "/api/v1/file?path=" + url.QueryEscape(res.abs)
		}
		out.Files = append(out.Files, st)
	}
	w.Header().Set("Cache-Control", "no-store") // the file system is not part of the snapshot ETag
	writeJSON(w, http.StatusOK, out)
}

// ProjectDir is the tracker's project directory: the base relative paths in
// bead text resolve against. Empty before any data is loaded.
func (s *Store) ProjectDir() string {
	if v := s.Current(); v != nil && v.Snap != nil {
		return v.Snap.ProjectDir
	}
	return ""
}
