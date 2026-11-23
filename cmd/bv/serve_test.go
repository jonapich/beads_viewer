package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	flag "github.com/spf13/pflag"
)

func TestServeArgRewrite(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want []string
	}{
		{"bare word", []string{"serve"}, []string{"--serve"}},
		{"upper case", []string{"SERVE"}, []string{"--serve"}},
		{"flags pass through", []string{"serve", "--serve-no-open", "--serve-port", "8080", "--serve-dev"},
			[]string{"--serve", "--serve-no-open", "--serve-port", "8080", "--serve-dev"}},
		{"equals form", []string{"serve", "--serve-port=0"}, []string{"--serve", "--serve-port=0"}},
		{"unknown flags left for cobra", []string{"serve", "--bogus"}, []string{"--serve", "--bogus"}},
		{"flag form is untouched", []string{"--serve", "--serve-port", "9000"}, []string{"--serve", "--serve-port", "9000"}},
		{"a flag value named serve is not the command", []string{"--search", "serve"}, []string{"--search", "serve"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requireArgs(t, rewriteAgentIntentArgs(tc.args), tc.want)
		})
	}

	if !hasNonRobotPrimaryArg([]string{"--serve"}) {
		t.Fatal("--serve must count as a primary non-robot mode")
	}
	if !knownValueTakingCLIFlags["serve-port"] {
		t.Fatal("--serve-port takes a separate value")
	}
	found := false
	for _, name := range agentIntentCommandNames() {
		if name == "serve" {
			found = true
		}
	}
	if !found {
		t.Fatal("serve must be offered as a did-you-mean command")
	}
}

func newServeTestFlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet("serve-test", flag.ContinueOnError)
	fs.Bool("serve", false, "")
	fs.Int("serve-port", 0, "")
	fs.Bool("serve-no-open", false, "")
	fs.Bool("serve-dev", false, "")
	fs.Bool("robot-triage", false, "")
	fs.String("export-pages", "", "")
	fs.Bool("pages", false, "")
	return fs
}

func TestServeFlagValidation(t *testing.T) {
	parse := func(args ...string) *flag.FlagSet {
		fs := newServeTestFlagSet()
		if err := fs.Parse(args); err != nil {
			t.Fatal(err)
		}
		return fs
	}

	// Modifiers need --serve.
	for _, args := range [][]string{{"--serve-port", "8080"}, {"--serve-no-open"}, {"--serve-dev"}} {
		err := validateModifierFlags(parse(args...), serveModifierRules())
		if err == nil || !strings.Contains(err.Error(), "requires --serve") {
			t.Errorf("%v without --serve: err = %v", args, err)
		}
	}
	for _, args := range [][]string{
		{"--serve"},
		{"--serve", "--serve-port", "8080", "--serve-no-open", "--serve-dev"},
	} {
		if err := validateModifierFlags(parse(args...), serveModifierRules()); err != nil {
			t.Errorf("%v: unexpected error %v", args, err)
		}
	}

	// --serve is its own primary command.
	groups := []primaryCommandGroup{{flags: []string{"robot-triage"}}, {flags: []string{"serve"}}}
	if err := validateExclusivePrimaryCommands(parse("--serve", "--robot-triage"), groups); err == nil {
		t.Error("--serve with --robot-triage must be rejected")
	}
	if err := validateExclusivePrimaryCommands(parse("--serve"), groups); err != nil {
		t.Errorf("--serve alone: %v", err)
	}

	// Other headless modes, and a bad port.
	if err := validateServeFlags(parse("--serve", "--export-pages", "out")); err == nil || !strings.Contains(err.Error(), "--export-pages") {
		t.Errorf("--serve with --export-pages: %v", err)
	}
	if err := validateServeFlags(parse("--serve", "--pages")); err == nil {
		t.Error("--serve with --pages must be rejected")
	}
	for _, port := range []string{"-1", "65536", "99999"} {
		if err := validateServeFlags(parse("--serve", "--serve-port="+port)); err == nil || !strings.Contains(err.Error(), "out of range") {
			t.Errorf("--serve-port=%s: %v", port, err)
		}
	}
	for _, port := range []string{"0", "1", "65535"} {
		if err := validateServeFlags(parse("--serve", "--serve-port="+port)); err != nil {
			t.Errorf("--serve-port=%s: %v", port, err)
		}
	}
	if err := validateServeFlags(parse("--export-pages", "out")); err != nil {
		t.Errorf("validation must be inert without --serve: %v", err)
	}
}

// lockedBuffer is a goroutine-safe io.Writer the tests poll for output.
type lockedBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// findURL returns the first output line that starts with prefix.
func findURL(out, prefix string) string {
	for _, line := range strings.Split(out, "\n") {
		if line = strings.TrimSpace(line); strings.HasPrefix(line, prefix) {
			return line
		}
	}
	return ""
}

func waitForURL(t *testing.T, out *lockedBuffer, prefix string) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if u := findURL(out.String(), prefix); u != "" {
			return u
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no %q line in output:\n%s", prefix, out.String())
	return ""
}

func startServeGUI(t *testing.T, opts serveOptions) (*lockedBuffer, context.CancelFunc, <-chan error) {
	t.Helper()
	out := &lockedBuffer{}
	opts.Stdout = out
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serveGUI(ctx, opts) }()
	t.Cleanup(cancel)
	return out, cancel, done
}

func TestServeGUIPrintsURLAndServesPlaceholder(t *testing.T) {
	opened := make(chan string, 1)
	out, cancel, done := startServeGUI(t, serveOptions{
		NoOpen: true,
		Open:   func(u string) error { opened <- u; return nil },
	})
	signIn := waitForURL(t, out, "http://127.0.0.1:")
	if !strings.Contains(signIn, "/auth?token=") {
		t.Fatalf("printed URL lacks the token path: %q", signIn)
	}

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 5 * time.Second}

	// Before signing in the API is closed.
	u := strings.SplitN(signIn, "/auth", 2)[0]
	resp, err := client.Get(u + "/api/v1/health")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("health before sign-in: %d", resp.StatusCode)
	}

	// Visiting the printed URL signs in and lands on the placeholder page.
	resp, err = client.Get(signIn)
	if err != nil {
		t.Fatal(err)
	}
	page, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(strings.ToLower(string(page)), "<html") {
		t.Fatalf("landing page: %d %q", resp.StatusCode, page)
	}

	resp, err = client.Get(u + "/api/v1/health")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var health map[string]string
	if resp.StatusCode != http.StatusOK || json.Unmarshal(body, &health) != nil || health["status"] != "ok" || health["version"] == "" {
		t.Fatalf("health after sign-in: %d %q", resp.StatusCode, body)
	}

	select {
	case got := <-opened:
		t.Fatalf("--serve-no-open still opened %q", got)
	default:
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serveGUI: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("serveGUI did not stop after cancel")
	}
	if !strings.Contains(out.String(), "stopped") {
		t.Fatalf("missing shutdown notice:\n%s", out.String())
	}
}

func TestServeGUIOpensBrowserUnlessSuppressed(t *testing.T) {
	opened := make(chan string, 1)
	out, cancel, done := startServeGUI(t, serveOptions{
		Open: func(u string) error { opened <- u; return nil },
	})
	signIn := waitForURL(t, out, "http://127.0.0.1:")
	select {
	case got := <-opened:
		if got != signIn {
			t.Fatalf("opened %q, printed %q", got, signIn)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("browser was not opened")
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestServeGUIDevModeIsAPIOnly(t *testing.T) {
	opened := make(chan string, 1)
	out, cancel, done := startServeGUI(t, serveOptions{
		Dev:  true,
		Open: func(u string) error { opened <- u; return nil },
	})
	vite := waitForURL(t, out, viteDevURL+"/auth?token=")
	token := strings.TrimPrefix(vite, viteDevURL+"/auth?token=")
	base := strings.TrimSpace(strings.SplitN(findURL(out.String(), "bv serve (dev)"), "API on ", 2)[1])
	base = strings.SplitN(base, ",", 2)[0]
	base = strings.TrimSuffix(strings.SplitN(base, " ", 2)[0], ".")

	resp, err := http.Get(base + "/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("dev server served a frontend: %d", resp.StatusCode)
	}
	resp, err = (&http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}).
		Get(base + "/auth?token=" + token)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("dev /auth: %d", resp.StatusCode)
	}
	select {
	case got := <-opened:
		t.Fatalf("dev mode must not open the API-only server in a browser, opened %q", got)
	default:
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestServeGUIPortHandling(t *testing.T) {
	// A port that is already taken is a clear error, not a silent fallback.
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot listen on loopback: %v", err)
	}
	defer taken.Close()
	port := taken.Addr().(*net.TCPAddr).Port
	err = serveGUI(context.Background(), serveOptions{Port: port, NoOpen: true, Stdout: io.Discard})
	if err == nil || !strings.Contains(err.Error(), "listen on 127.0.0.1:"+strconv.Itoa(port)) {
		t.Fatalf("busy port: %v", err)
	}

	if err := serveGUI(context.Background(), serveOptions{Port: 70000, Stdout: io.Discard}); err == nil {
		t.Fatal("out-of-range port must fail")
	}

	// An explicit port is honoured.
	free, _ := net.Listen("tcp", "127.0.0.1:0")
	want := free.Addr().(*net.TCPAddr).Port
	free.Close()
	out, cancel, done := startServeGUI(t, serveOptions{Port: want, NoOpen: true})
	signIn := waitForURL(t, out, "http://127.0.0.1:")
	if !strings.HasPrefix(signIn, "http://127.0.0.1:"+strconv.Itoa(want)+"/auth?token=") {
		t.Fatalf("explicit port ignored: %q", signIn)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

// TestServeSubcommandBinary drives the real binary the way a user does:
// `bv serve --serve-no-open` prints a sign-in URL that opens the placeholder.
func TestServeSubcommandBinary(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the bv binary")
	}
	exe := buildTestBinary(t)
	cmd := exec.Command(exe, "serve", "--serve-no-open")
	cmd.Dir = t.TempDir()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	lines := make(chan string, 16)
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()
	var signIn string
	deadline := time.After(20 * time.Second)
	for signIn == "" {
		select {
		case line, ok := <-lines:
			if !ok {
				t.Fatal("bv serve exited before printing the sign-in URL")
			}
			if strings.HasPrefix(line, "http://127.0.0.1:") {
				signIn = line
			}
		case <-deadline:
			t.Fatal("bv serve printed no sign-in URL")
		}
	}

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 5 * time.Second}
	resp, err := client.Get(signIn)
	if err != nil {
		t.Fatal(err)
	}
	page, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(strings.ToLower(string(page)), "<html") {
		t.Fatalf("placeholder page: %d %q", resp.StatusCode, page)
	}
	resp, err = client.Get(strings.SplitN(signIn, "/auth", 2)[0] + "/api/v1/health")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health: %d", resp.StatusCode)
	}
}
