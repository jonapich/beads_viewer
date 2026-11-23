package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	flag "github.com/spf13/pflag"

	"github.com/Dicklesworthstone/beads_viewer/pkg/api"
	"github.com/Dicklesworthstone/beads_viewer/pkg/export"
	"github.com/Dicklesworthstone/beads_viewer/pkg/version"
	"github.com/Dicklesworthstone/beads_viewer/web"
)

// viteDevURL is where `npm --prefix web run dev` serves the frontend; it is
// the origin --serve-dev lets talk to the API (see pkg/api).
const viteDevURL = "http://localhost:5173"

// serveOptions are the resolved `bv serve` settings.
type serveOptions struct {
	Port   int  // 0 = kernel-chosen
	NoOpen bool // never open a browser
	Dev    bool // API only; frontend comes from the Vite dev server

	// Stdout receives the startup banner; nil means os.Stdout.
	Stdout io.Writer
	// Open launches the browser; nil means export.OpenInBrowser, which honours
	// BV_NO_BROWSER and BV_TEST_MODE.
	Open func(url string) error
}

// serveModifierRules ties the --serve-* modifiers to --serve.
func serveModifierRules() []modifierFlagRule {
	return []modifierFlagRule{
		{modifier: "serve-port", requires: []string{"serve"}},
		{modifier: "serve-no-open", requires: []string{"serve"}},
		{modifier: "serve-dev", requires: []string{"serve"}},
	}
}

// serveExclusiveFlags are the other headless modes --serve cannot be combined
// with. The robot commands are covered by the primary command groups.
var serveExclusiveFlags = []string{
	"pages", "export-pages", "preview-pages", "export", "export-md", "export-graph",
	"hooks-info", "priority-brief", "agent-brief", "emit-script", "diff-since",
	"debug-render", "profile-startup", "init-drift", "save-baseline", "baseline-info",
	"check-drift", "feedback-accept", "feedback-ignore", "feedback-reset", "feedback-show",
}

// validateServeFlags rejects an out-of-range --serve-port and --serve combined
// with another headless mode.
func validateServeFlags(flags *flag.FlagSet) error {
	if !isFlagActive(flags, "serve") {
		return nil
	}
	if port, err := flags.GetInt("serve-port"); err == nil && (port < 0 || port > 65535) {
		return fmt.Errorf("--serve-port %d is out of range (0-65535)", port)
	}
	for _, name := range serveExclusiveFlags {
		if f := flags.Lookup(name); f != nil && f.Changed && isFlagActive(flags, name) {
			return fmt.Errorf("--serve cannot be combined with --%s", name)
		}
	}
	return nil
}

// runServe runs the GUI server until SIGINT/SIGTERM.
func runServe(opts serveOptions) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return serveGUI(ctx, opts)
}

// serveGUI binds 127.0.0.1, prints the sign-in URL (it carries the per-run
// token), optionally opens the browser, and serves until ctx is cancelled.
func serveGUI(ctx context.Context, opts serveOptions) error {
	out := opts.Stdout
	if out == nil {
		out = os.Stdout
	}
	open := opts.Open
	if open == nil {
		open = export.OpenInBrowser
	}
	if opts.Port < 0 || opts.Port > 65535 {
		return fmt.Errorf("--serve-port %d is out of range (0-65535)", opts.Port)
	}

	// Loopback only, never 0.0.0.0. Port 0 lets the kernel pick a free port;
	// FindAvailablePort's probe-then-listen would race.
	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(opts.Port)))
	if err != nil {
		return fmt.Errorf("listen on 127.0.0.1:%d: %w", opts.Port, err)
	}
	port := ln.Addr().(*net.TCPAddr).Port

	token, err := api.NewToken()
	if err != nil {
		_ = ln.Close()
		return err
	}
	cfg := api.Config{Port: port, Token: token, Version: version.Version, Dev: opts.Dev}
	if !opts.Dev {
		cfg.Assets = web.FS
	}
	handler, err := api.NewHandler(cfg)
	if err != nil {
		_ = ln.Close()
		return err
	}

	base := "http://127.0.0.1:" + strconv.Itoa(port)
	signIn := base + "/auth?token=" + token
	if opts.Dev {
		fmt.Fprintf(out, "bv serve (dev): API on %s, frontend not embedded.\n", base)
		fmt.Fprintln(out, "Run `npm --prefix web run dev`, then sign in through the Vite server")
		fmt.Fprintln(out, "(this URL contains the access token for this run; keep it private):")
		fmt.Fprintln(out, viteDevURL+"/auth?token="+token)
	} else {
		fmt.Fprintf(out, "bv serve: GUI on %s (127.0.0.1 only).\n", base)
		fmt.Fprintln(out, "Sign-in URL (contains the access token for this run; keep it private):")
		fmt.Fprintln(out, signIn)
	}
	fmt.Fprintln(out, "Press Ctrl+C to stop.")

	// In dev mode the page worth opening is Vite's, not this API-only server.
	if !opts.NoOpen && !opts.Dev {
		if err := open(signIn); err != nil {
			fmt.Fprintf(os.Stderr, "Could not open browser: %v\n", err)
		}
	}

	if err := api.Serve(ctx, ln, handler); err != nil {
		return err
	}
	fmt.Fprintln(out, "bv serve stopped.")
	return nil
}
