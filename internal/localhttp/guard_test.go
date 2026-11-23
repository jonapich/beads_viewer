package localhttp

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthority(t *testing.T) {
	for _, tc := range []struct {
		in   string
		port int
		want string
		ok   bool
	}{
		{"127.0.0.1:9000", 9000, "127.0.0.1:9000", true},
		{"localhost:9000", 9000, "localhost:9000", true},
		{"LOCALHOST:9000", 9000, "localhost:9000", true},
		{"[::1]:9000", 9000, "[::1]:9000", true},
		{"localhost", 80, "localhost:80", true},
		{"localhost", 9000, "", false},
		{"127.0.0.1:9001", 9000, "", false},
		{"127.0.0.1:+9000", 9000, "", false},
		{"127.0.0.1:9000", 0, "", false},
		{"localhost.evil.invalid:9000", 9000, "", false},
		{"192.0.2.1:9000", 9000, "", false},
		{"user@localhost:9000", 9000, "", false},
		{"", 9000, "", false},
	} {
		got, ok := Authority(tc.in, tc.port)
		if got != tc.want || ok != tc.ok {
			t.Errorf("Authority(%q, %d) = %q, %v; want %q, %v", tc.in, tc.port, got, ok, tc.want, tc.ok)
		}
	}
}

type guardCase struct {
	name    string
	method  string
	host    string
	origins []string
	want    int
}

func runGuardCases(t *testing.T, port int, opts Options, cases []guardCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			method := tc.method
			if method == "" {
				method = http.MethodGet
			}
			req := httptest.NewRequest(method, "http://example.invalid/x", nil)
			req.Host = tc.host
			for _, o := range tc.origins {
				req.Header.Add("Origin", o)
			}
			rec := httptest.NewRecorder()
			called := false
			Guard(port, opts, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				_, _ = io.WriteString(w, "private")
			})).ServeHTTP(rec, req)
			if rec.Code != tc.want || called != (tc.want == 200) {
				t.Fatalf("status=%d called=%v, want status %d", rec.Code, called, tc.want)
			}
			h := rec.Header()
			if h.Get("X-Content-Type-Options") != "nosniff" ||
				h.Get("Cross-Origin-Opener-Policy") != "same-origin" ||
				h.Get("Cross-Origin-Resource-Policy") != "same-origin" {
				t.Fatalf("missing policy headers: %v", h)
			}
			if got := h.Get("Cross-Origin-Embedder-Policy") != ""; got != opts.EmbedderPolicy {
				t.Fatalf("COEP present=%v, want %v", got, opts.EmbedderPolicy)
			}
			if h.Get("Access-Control-Allow-Origin") != "" {
				t.Fatalf("guard must never emit CORS allow headers: %v", h)
			}
		})
	}
}

func TestGuardDefaultAllowsAllMethods(t *testing.T) {
	runGuardCases(t, 9000, Options{}, []guardCase{
		{name: "GET", host: "127.0.0.1:9000", want: 200},
		{name: "POST same origin", method: "POST", host: "127.0.0.1:9000", origins: []string{"http://127.0.0.1:9000"}, want: 200},
		{name: "POST no origin", method: "POST", host: "127.0.0.1:9000", want: 200},
		{name: "POST foreign origin", method: "POST", host: "127.0.0.1:9000", origins: []string{"https://evil.invalid"}, want: 403},
		{name: "foreign host", host: "rebind.invalid:9000", want: 403},
		{name: "different local origin", host: "127.0.0.1:9000", origins: []string{"http://localhost:9000"}, want: 403},
		{name: "multiple origins", host: "127.0.0.1:9000", origins: []string{"http://127.0.0.1:9000", "http://127.0.0.1:9000"}, want: 403},
		{name: "origin with path", host: "127.0.0.1:9000", origins: []string{"http://127.0.0.1:9000/"}, want: 403},
		{name: "null origin", host: "127.0.0.1:9000", origins: []string{"null"}, want: 403},
	})
}

func TestGuardReadOnly(t *testing.T) {
	runGuardCases(t, 9000, Options{ReadOnly: true, EmbedderPolicy: true}, []guardCase{
		{name: "GET", host: "127.0.0.1:9000", want: 200},
		{name: "HEAD", method: "HEAD", host: "127.0.0.1:9000", want: 200},
		{name: "POST", method: "POST", host: "127.0.0.1:9000", want: 405},
		{name: "foreign host beats method", method: "POST", host: "evil.invalid:9000", want: 403},
	})
}

func TestGuardRequireOriginOnMutating(t *testing.T) {
	runGuardCases(t, 9000, Options{RequireOriginOnMutating: true}, []guardCase{
		{name: "GET without origin", host: "127.0.0.1:9000", want: 200},
		{name: "POST without origin", method: "POST", host: "127.0.0.1:9000", want: 403},
		{name: "PUT without origin", method: "PUT", host: "127.0.0.1:9000", want: 403},
		{name: "PATCH without origin", method: "PATCH", host: "127.0.0.1:9000", want: 403},
		{name: "DELETE without origin", method: "DELETE", host: "127.0.0.1:9000", want: 403},
		{name: "POST matching origin", method: "POST", host: "127.0.0.1:9000", origins: []string{"http://127.0.0.1:9000"}, want: 200},
		{name: "POST wrong origin", method: "POST", host: "127.0.0.1:9000", origins: []string{"http://127.0.0.1:9001"}, want: 403},
	})
}

func TestGuardExtraOrigins(t *testing.T) {
	opts := Options{
		RequireOriginOnMutating: true,
		ExtraOrigins:            []string{"http://localhost:5173", "http://127.0.0.1:5173", "not a url", "http://localhost"},
	}
	runGuardCases(t, 7777, opts, []guardCase{
		{name: "own origin", method: "POST", host: "127.0.0.1:7777", origins: []string{"http://127.0.0.1:7777"}, want: 200},
		{name: "vite origin, api host", method: "POST", host: "127.0.0.1:7777", origins: []string{"http://localhost:5173"}, want: 200},
		{name: "vite origin 127", method: "PUT", host: "127.0.0.1:7777", origins: []string{"http://127.0.0.1:5173"}, want: 200},
		{name: "vite host forwarded by proxy", method: "POST", host: "localhost:5173", origins: []string{"http://localhost:5173"}, want: 200},
		{name: "vite GET no origin", host: "localhost:5173", want: 200},
		{name: "other vite port", method: "POST", host: "127.0.0.1:7777", origins: []string{"http://localhost:5174"}, want: 403},
		{name: "vite https", method: "POST", host: "127.0.0.1:7777", origins: []string{"https://localhost:5173"}, want: 403},
		{name: "foreign host", host: "evil.invalid:5173", want: 403},
		{name: "foreign origin", method: "POST", host: "127.0.0.1:7777", origins: []string{"http://evil.invalid:5173"}, want: 403},
	})

	// Without extras the Vite origin is foreign.
	runGuardCases(t, 7777, Options{}, []guardCase{
		{name: "vite origin not allowed by default", method: "POST", host: "127.0.0.1:7777", origins: []string{"http://localhost:5173"}, want: 403},
		{name: "vite host not allowed by default", host: "localhost:5173", want: 403},
	})
}

func TestGuardDenyHook(t *testing.T) {
	var gotStatus int
	var gotMsg string
	opts := Options{
		ForbiddenMessage: "nope",
		Deny: func(w http.ResponseWriter, r *http.Request, status int, message string) {
			gotStatus, gotMsg = status, message
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_, _ = io.WriteString(w, `{"error":"custom"}`)
		},
	}
	called := false
	h := Guard(1, opts, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://x/", nil)
	req.Host = "evil.invalid:1"
	h.ServeHTTP(rec, req)
	if rec.Code != 403 || gotStatus != 403 || gotMsg != "nope" || rec.Body.String() != `{"error":"custom"}` {
		t.Fatalf("deny hook: code=%d status=%d msg=%q body=%q", rec.Code, gotStatus, gotMsg, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("X-Content-Type-Options") != "nosniff" || called {
		t.Fatalf("headers/handler: %v called=%v", rec.Header(), called)
	}

	// Allowed requests never reach the hook.
	gotStatus = 0
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "http://x/", nil)
	req.Host = "127.0.0.1:1"
	h.ServeHTTP(rec, req)
	if gotStatus != 0 || !called {
		t.Fatalf("allowed request: hook status=%d called=%v", gotStatus, called)
	}
}

func TestGuardForbiddenMessage(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://x/", nil)
	req.Host = "evil.invalid:1"
	Guard(1, Options{ForbiddenMessage: "custom message"}, http.NotFoundHandler()).ServeHTTP(rec, req)
	if rec.Code != 403 || rec.Body.String() != "custom message\n" {
		t.Fatalf("got %d %q", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("denied response must not be cacheable: %v", rec.Header())
	}
}
