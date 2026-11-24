package api

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

const (
	testPort    = 4321
	testToken   = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	testVersion = "v9.9.9-test"
	testIndex   = "<html><body>spa index</body></html>"
	localHost   = "127.0.0.1:4321"
	localOrigin = "http://127.0.0.1:4321"
)

func testAssets() fstest.MapFS {
	return fstest.MapFS{
		"index.html":          {Data: []byte(testIndex)},
		"assets/app-abc12.js": {Data: []byte("console.log('app')")},
		"favicon.svg":         {Data: []byte("<svg/>")},
	}
}

func newTestHandler(t *testing.T, mutate func(*Config)) http.Handler {
	t.Helper()
	cfg := Config{Port: testPort, Token: testToken, Version: testVersion, Assets: testAssets()}
	if mutate != nil {
		mutate(&cfg)
	}
	h, err := NewHandler(cfg)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	return h
}

type reqOpt func(*http.Request)

func withHost(host string) reqOpt { return func(r *http.Request) { r.Host = host } }
func withOrigin(o string) reqOpt  { return func(r *http.Request) { r.Header.Set("Origin", o) } }
func withSession() reqOpt {
	return func(r *http.Request) {
		r.AddCookie(&http.Cookie{Name: cookieNamePrefix + strconv.Itoa(testPort), Value: testToken})
	}
}
func withCookie(name, value string) reqOpt {
	return func(r *http.Request) { r.AddCookie(&http.Cookie{Name: name, Value: value}) }
}

func do(h http.Handler, method, target string, opts ...reqOpt) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	req.Host = localHost
	for _, o := range opts {
		o(req)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) errorDetail {
	t.Helper()
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q, want JSON (body %q)", ct, rec.Body.String())
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("error body is not JSON: %v: %q", err, rec.Body.String())
	}
	if body.Error.Code == "" || body.Error.Message == "" {
		t.Fatalf("error body missing code/message: %q", rec.Body.String())
	}
	return body.Error
}

func TestHealthRequiresSession(t *testing.T) {
	h := newTestHandler(t, nil)

	for name, opts := range map[string][]reqOpt{
		"no cookie":          nil,
		"wrong value":        {withCookie(cookieNamePrefix+strconv.Itoa(testPort), "nope")},
		"wrong cookie name":  {withCookie("bv_session", testToken)},
		"other port cookie":  {withCookie(cookieNamePrefix+"9999", testToken)},
		"empty cookie value": {withCookie(cookieNamePrefix+strconv.Itoa(testPort), "")},
	} {
		t.Run(name, func(t *testing.T) {
			rec := do(h, "GET", "/api/v1/health", opts...)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
			if e := decodeError(t, rec); e.Code != "unauthorized" {
				t.Fatalf("code = %q", e.Code)
			}
		})
	}

	rec := do(h, "GET", "/api/v1/health", withSession())
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["status"] != "ok" || got["version"] != testVersion {
		t.Fatalf("health = %v", got)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("API responses must be no-store, got %q", rec.Header().Get("Cache-Control"))
	}

	if rec := do(h, "HEAD", "/api/v1/health", withSession()); rec.Code != http.StatusOK {
		t.Fatalf("HEAD status = %d", rec.Code)
	}
	rec = do(h, "POST", "/api/v1/health", withSession(), withOrigin(localOrigin))
	if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") == "" {
		t.Fatalf("POST health: status=%d allow=%q", rec.Code, rec.Header().Get("Allow"))
	}
	decodeError(t, rec)
}

func TestAuthRejectsBadToken(t *testing.T) {
	h := newTestHandler(t, nil)
	for name, target := range map[string]string{
		"missing":       "/auth",
		"empty":         "/auth?token=",
		"wrong":         "/auth?token=wrong",
		"prefix":        "/auth?token=" + testToken[:len(testToken)-1],
		"extended":      "/auth?token=" + testToken + "x",
		"case changed":  "/auth?token=" + strings.ToUpper(testToken),
		"second param":  "/auth?x=" + testToken,
		"repeated bad":  "/auth?token=bad&token=" + testToken,
		"url-encoded x": "/auth?token=%00",
	} {
		t.Run(name, func(t *testing.T) {
			rec := do(h, "GET", target)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
			if rec.Header().Get("Set-Cookie") != "" {
				t.Fatalf("bad token must not set a cookie: %q", rec.Header().Get("Set-Cookie"))
			}
			if rec.Header().Get("Location") != "" {
				t.Fatal("bad token must not redirect")
			}
			decodeError(t, rec)
		})
	}
	if rec := do(h, "POST", "/auth?token="+testToken, withOrigin(localOrigin)); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /auth status = %d, want 405", rec.Code)
	}
}

func TestAuthGoodTokenSetsCookieAndRedirects(t *testing.T) {
	h := newTestHandler(t, nil)
	rec := do(h, "GET", "/auth?token="+testToken)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/" {
		t.Fatalf("Location = %q, want /", loc)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %v", cookies)
	}
	c := cookies[0]
	if c.Name != cookieNamePrefix+strconv.Itoa(testPort) || c.Value != testToken {
		t.Fatalf("cookie = %s=%s", c.Name, c.Value)
	}
	if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Path != "/" {
		t.Fatalf("cookie attributes: HttpOnly=%v SameSite=%v Path=%q", c.HttpOnly, c.SameSite, c.Path)
	}
	if c.MaxAge != 0 || !c.Expires.IsZero() {
		t.Fatalf("session cookie must not be persistent: %+v", c)
	}
	if rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("auth response headers: %v", rec.Header())
	}

	// The cookie the server just issued opens the API.
	if rec := do(h, "GET", "/api/v1/health", withCookie(c.Name, c.Value)); rec.Code != http.StatusOK {
		t.Fatalf("issued cookie rejected: %d", rec.Code)
	}
}

func TestForeignHostAndOriginRejected(t *testing.T) {
	h := newTestHandler(t, nil)

	for _, target := range []string{"/", "/auth?token=" + testToken, "/api/v1/health", "/some/spa/route"} {
		for _, host := range []string{"evil.invalid:4321", "127.0.0.1:9999", "localhost.evil.invalid:4321", "192.0.2.1:4321", "127.0.0.1"} {
			rec := do(h, "GET", target, withHost(host), withSession())
			if rec.Code != http.StatusForbidden {
				t.Errorf("GET %s with Host %q: status %d, want 403", target, host, rec.Code)
			}
			if rec.Header().Get("Set-Cookie") != "" || strings.Contains(rec.Body.String(), testIndex) {
				t.Errorf("GET %s with Host %q leaked content", target, host)
			}
		}
		// A foreign Origin is rejected even on GET.
		rec := do(h, "GET", target, withOrigin("http://evil.invalid"), withSession())
		if rec.Code != http.StatusForbidden {
			t.Errorf("GET %s with foreign Origin: status %d, want 403", target, rec.Code)
		}
	}

	// Same-origin variants are fine.
	if rec := do(h, "GET", "/api/v1/health", withHost("localhost:4321"), withSession()); rec.Code != http.StatusOK {
		t.Fatalf("localhost host: %d", rec.Code)
	}
	if rec := do(h, "GET", "/api/v1/health", withOrigin(localOrigin), withSession()); rec.Code != http.StatusOK {
		t.Fatalf("same-origin GET: %d", rec.Code)
	}
}

// Guard denials on /api and elsewhere use the API error shape, not text/plain.
func TestGuardDenialIsJSON(t *testing.T) {
	h := newTestHandler(t, nil)
	for _, rec := range []*httptest.ResponseRecorder{
		do(h, "GET", "/api/v1/health", withHost("evil.invalid:4321"), withSession()),
		do(h, "POST", "/api/v1/health", withSession(), withOrigin("http://evil.invalid")),
		do(h, "POST", "/api/v1/health", withSession()),
		do(h, "GET", "/", withHost("evil.invalid:4321")),
	} {
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", rec.Code)
		}
		if e := decodeError(t, rec); e.Code != "forbidden" {
			t.Fatalf("code = %q, want forbidden", e.Code)
		}
		if rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
		}
	}
}

func TestMutatingMethodsNeedExactOrigin(t *testing.T) {
	h := newTestHandler(t, nil)
	for _, method := range []string{"POST", "PUT", "PATCH", "DELETE"} {
		for name, opts := range map[string][]reqOpt{
			"no origin":      {withSession()},
			"foreign origin": {withSession(), withOrigin("https://evil.invalid")},
			"other port":     {withSession(), withOrigin("http://127.0.0.1:4322")},
			"other loopback": {withSession(), withOrigin("http://localhost:4321")},
			"null origin":    {withSession(), withOrigin("null")},
			"vite (non-dev)": {withSession(), withOrigin("http://localhost:5173")},
		} {
			rec := do(h, method, "/api/v1/health", opts...)
			if rec.Code != http.StatusForbidden {
				t.Errorf("%s with %s: status %d, want 403", method, name, rec.Code)
			}
		}
		// An exact Origin passes the guard (health then refuses the method).
		rec := do(h, method, "/api/v1/health", withSession(), withOrigin(localOrigin))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s with exact Origin: status %d, want 405 from the endpoint", method, rec.Code)
		}
	}
}

func TestSPAFallbackAndStaticCaching(t *testing.T) {
	h := newTestHandler(t, nil)

	for _, target := range []string{"/", "/index.html", "/issues/bd-42", "/deep/unknown/route", "/assets/", "/assets", "/missing.js"} {
		rec := do(h, "GET", target)
		if rec.Code != http.StatusOK || rec.Body.String() != testIndex {
			t.Errorf("GET %s: status %d body %q, want index.html", target, rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("Cache-Control"); got != "no-store" {
			t.Errorf("GET %s: Cache-Control %q, want no-store", target, got)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
			t.Errorf("GET %s: Content-Type %q", target, ct)
		}
	}

	rec := do(h, "GET", "/assets/app-abc12.js")
	if rec.Code != http.StatusOK || rec.Body.String() != "console.log('app')" {
		t.Fatalf("hashed asset: %d %q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("hashed asset Cache-Control = %q", got)
	}

	// A stale hashed-asset reference fails loudly instead of returning HTML.
	for _, target := range []string{"/assets/app-gone.js", "/assets/nested/missing.css"} {
		rec := do(h, "GET", target)
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s: status %d, want 404", target, rec.Code)
			continue
		}
		if e := decodeError(t, rec); e.Code != "not_found" {
			t.Errorf("GET %s: code %q", target, e.Code)
		}
		if strings.Contains(rec.Body.String(), "spa index") {
			t.Errorf("GET %s fell through to the SPA", target)
		}
	}

	rec = do(h, "GET", "/favicon.svg")
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("plain asset: %d cache=%q", rec.Code, rec.Header().Get("Cache-Control"))
	}

	if rec := do(h, "HEAD", "/issues/bd-42"); rec.Code != http.StatusOK || rec.Body.Len() != 0 {
		t.Fatalf("HEAD fallback: %d body %q", rec.Code, rec.Body.String())
	}

	// Static content needs no session: the SPA must load to show its own
	// "not signed in" state.
	if rec := do(h, "GET", "/"); rec.Code != http.StatusOK {
		t.Fatalf("GET / without cookie: %d", rec.Code)
	}

	// Path traversal cannot leave the asset tree.
	for _, target := range []string{"/../../go.mod", "/%2e%2e/%2e%2e/go.mod", "/assets/../../go.mod"} {
		rec := do(h, "GET", target)
		if rec.Code == http.StatusOK && rec.Body.String() != testIndex && rec.Body.String() != "console.log('app')" {
			t.Errorf("GET %s served unexpected content %q", target, rec.Body.String())
		}
	}

	if rec := do(h, "POST", "/", withOrigin(localOrigin)); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST / status = %d, want 405", rec.Code)
	}
}

func TestNoAssetsConfigured(t *testing.T) {
	h := newTestHandler(t, func(c *Config) { c.Assets = nil })
	rec := do(h, "GET", "/")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	decodeError(t, rec)

	h = newTestHandler(t, func(c *Config) { c.Assets = fstest.MapFS{"other.txt": {Data: []byte("x")}} })
	if rec := do(h, "GET", "/anything"); rec.Code != http.StatusNotFound {
		t.Fatalf("missing index.html: status = %d, want 404", rec.Code)
	}
}

func TestUnknownAPIPathIs404JSON(t *testing.T) {
	h := newTestHandler(t, nil)
	for _, target := range []string{"/api/unknown", "/api/v1/nope", "/api/v1/health/extra", "/api/", "/api", "/api/v2/health"} {
		rec := do(h, "GET", target, withSession())
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s: status %d, want 404", target, rec.Code)
			continue
		}
		if e := decodeError(t, rec); e.Code != "not_found" {
			t.Errorf("GET %s: code %q", target, e.Code)
		}
		if strings.Contains(rec.Body.String(), "spa index") {
			t.Errorf("GET %s fell through to the SPA", target)
		}
	}
	// Without a session even unknown API paths are 401: no probing.
	if rec := do(h, "GET", "/api/unknown"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated unknown API path: %d, want 401", rec.Code)
	}
}

func TestSecurityHeaders(t *testing.T) {
	h := newTestHandler(t, nil)
	for name, rec := range map[string]*httptest.ResponseRecorder{
		"index":                do(h, "GET", "/"),
		"asset":                do(h, "GET", "/assets/app-abc12.js"),
		"api ok":               do(h, "GET", "/api/v1/health", withSession()),
		"api 401":              do(h, "GET", "/api/v1/health"),
		"api 404":              do(h, "GET", "/api/zzz", withSession()),
		"auth 401":             do(h, "GET", "/auth"),
		"auth ok":              do(h, "GET", "/auth?token="+testToken),
		"forbidden":            do(h, "GET", "/", withHost("evil.invalid:4321")),
		"forbidden bad origin": do(h, "POST", "/api/v1/health", withSession(), withOrigin("http://evil.invalid")),
	} {
		hdr := rec.Header()
		want := map[string]string{
			"X-Content-Type-Options":       "nosniff",
			"Cross-Origin-Opener-Policy":   "same-origin",
			"Cross-Origin-Resource-Policy": "same-origin",
		}
		if name != "forbidden" && name != "forbidden bad origin" {
			want["Content-Security-Policy"] = "default-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'"
		}
		for k, v := range want {
			if got := hdr.Get(k); got != v {
				t.Errorf("%s: %s = %q, want %q", name, k, got, v)
			}
		}
		if hdr.Get("Access-Control-Allow-Origin") != "" {
			t.Errorf("%s: unexpected CORS header", name)
		}
	}
}

func TestDevModeServesAPIOnly(t *testing.T) {
	h := newTestHandler(t, func(c *Config) { c.Dev = true })

	rec := do(h, "GET", "/")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("dev GET /: status %d, want 404", rec.Code)
	}
	decodeError(t, rec)
	if strings.Contains(rec.Body.String(), "spa index") {
		t.Fatal("dev mode must not serve embedded assets")
	}

	if rec := do(h, "GET", "/api/v1/health", withSession()); rec.Code != http.StatusOK {
		t.Fatalf("dev health: %d", rec.Code)
	}
	if rec := do(h, "GET", "/auth?token="+testToken); rec.Code != http.StatusSeeOther {
		t.Fatalf("dev auth: %d", rec.Code)
	}

	// Vite origins are allowed, only in dev mode, only those two.
	for _, origin := range []string{"http://localhost:5173", "http://127.0.0.1:5173"} {
		if rec := do(h, "POST", "/api/v1/health", withSession(), withOrigin(origin)); rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("dev POST with Origin %s: status %d, want 405 from the endpoint", origin, rec.Code)
		}
		if rec := do(h, "GET", "/api/v1/health", withSession(), withOrigin(origin)); rec.Code != http.StatusOK {
			t.Errorf("dev GET with Origin %s: status %d", origin, rec.Code)
		}
	}
	for _, origin := range []string{"http://localhost:5174", "https://localhost:5173", "http://evil.invalid:5173"} {
		if rec := do(h, "POST", "/api/v1/health", withSession(), withOrigin(origin)); rec.Code != http.StatusForbidden {
			t.Errorf("dev POST with Origin %s: status %d, want 403", origin, rec.Code)
		}
	}
	// The Vite proxy forwards the browser's Host header by default.
	if rec := do(h, "GET", "/api/v1/health", withSession(), withHost("localhost:5173"), withOrigin("http://localhost:5173")); rec.Code != http.StatusOK {
		t.Errorf("dev request proxied with Vite Host: status %d", rec.Code)
	}
	if rec := do(h, "GET", "/api/v1/health", withSession(), withHost("evil.invalid:5173")); rec.Code != http.StatusForbidden {
		t.Errorf("dev foreign Host: status %d, want 403", rec.Code)
	}
}

func TestNewHandlerValidatesConfig(t *testing.T) {
	for name, cfg := range map[string]Config{
		"no port":       {Token: testToken},
		"negative port": {Port: -1, Token: testToken},
		"huge port":     {Port: 70000, Token: testToken},
		"no token":      {Port: testPort},
	} {
		if _, err := NewHandler(cfg); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestNewToken(t *testing.T) {
	a, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := NewToken()
	if len(a) != 64 || a == b {
		t.Fatalf("tokens must be 32 random bytes, hex encoded and unique: %q %q", a, b)
	}
}

// TestServeEndToEnd runs the real server on a kernel-chosen loopback port,
// follows the /auth redirect with a cookie jar, and checks graceful shutdown.
func TestServeEndToEnd(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot listen on loopback: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	token, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	h, err := NewHandler(Config{Port: port, Token: token, Version: testVersion, Assets: testAssets()})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	served := make(chan error, 1)
	go func() { served <- Serve(ctx, ln, h) }()

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 5 * time.Second}
	base := "http://127.0.0.1:" + strconv.Itoa(port)

	resp, err := client.Get(base + "/api/v1/health")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated health: %d", resp.StatusCode)
	}

	resp, err = client.Get(base + "/auth?token=" + url.QueryEscape(token))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || string(body) != testIndex || resp.Request.URL.Path != "/" {
		t.Fatalf("after /auth: status %d path %q body %q", resp.StatusCode, resp.Request.URL.Path, body)
	}

	resp, err = client.Get(base + "/api/v1/health")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("authenticated health: %d", resp.StatusCode)
	}

	// A request that spoofs a foreign Host is refused by the real server too.
	req, _ := http.NewRequest("GET", base+"/", nil)
	req.Host = "evil.invalid:" + strconv.Itoa(port)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("foreign Host over the wire: %d", resp.StatusCode)
	}

	cancel()
	select {
	case err := <-served:
		if err != nil {
			t.Fatalf("Serve returned %v after graceful shutdown", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Serve did not return after cancel")
	}
	client.CloseIdleConnections()
	if conn, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(port), time.Second); err == nil {
		conn.Close()
		t.Fatal("listener still accepting after shutdown")
	}
}

// TestServeShutdownWithIdlePreconnect: a socket that connected but never sent a
// request (what browsers preconnect) must not make Ctrl+C slow or noisy.
func TestServeShutdownWithIdlePreconnect(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot listen on loopback: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	served := make(chan error, 1)
	go func() { served <- Serve(ctx, ln, http.NotFoundHandler()) }()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	time.Sleep(50 * time.Millisecond) // let the server accept it

	start := time.Now()
	cancel()
	select {
	case err := <-served:
		if err != nil {
			t.Fatalf("Serve = %v, want nil after a requested stop", err)
		}
		if d := time.Since(start); d > 4*time.Second {
			t.Fatalf("shutdown took %v", d)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Serve did not return")
	}
}

func TestServeReturnsListenerError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot listen on loopback: %v", err)
	}
	ln.Close() // Serve on a closed listener fails immediately
	err = Serve(context.Background(), ln, http.NotFoundHandler())
	if err == nil {
		t.Fatal("expected an error serving on a closed listener")
	}
}

// TestReadEndpointsRejectWrites pins the 405 contract of every read endpoint:
// a POST that passes the guard (session and own Origin) is rejected with the
// method_not_allowed error and an Allow header, and never reaches the store.
func TestReadEndpointsRejectWrites(t *testing.T) {
	env := newWriteEnv(t, nil)
	for _, tc := range []struct{ target, allow string }{
		{"/api/v1/health", "GET, HEAD"},
		{"/api/v1/meta", "GET, HEAD"},
		{"/api/v1/events", "GET"},
		{"/api/v1/issues", "GET, HEAD"},
		{"/api/v1/search?q=a", "GET, HEAD"},
		{"/api/v1/signoff", "GET, HEAD"},
		{"/api/v1/triage", "GET, HEAD"},
		{"/api/v1/insights", "GET, HEAD"},
		{"/api/v1/issues/fx-a", "GET, HEAD"},
	} {
		t.Run(tc.target, func(t *testing.T) {
			rec := do(env.h, http.MethodPost, tc.target, withSession(), withOrigin(localOrigin))
			if rec.Code != http.StatusMethodNotAllowed {
				t.Fatalf("status = %d, want 405: %s", rec.Code, rec.Body.String())
			}
			if got := rec.Header().Get("Allow"); got != tc.allow {
				t.Fatalf("Allow = %q, want %q", got, tc.allow)
			}
			if e := decodeError(t, rec); e.Code != "method_not_allowed" {
				t.Fatalf("code = %q", e.Code)
			}
		})
	}
	expectNoCalls(t, env.fw)
}
