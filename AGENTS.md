# AGENTS.md — beads_viewer (jonapich fork)

`bv` is a terminal UI for Beads issue trackers (`.beads/beads.db`, `.beads/issues.jsonl`, legacy `.beads/beads.jsonl`), with graph metrics and `--robot-*` JSON outputs. Entry point: `cmd/bv/`; libraries under `pkg/` and `internal/`.

## Build & check

Go modules only; dependencies are vendored (`vendor/`, patched modules in `third_party/`). After code changes:

```bash
go build ./...
go vet ./...
gofmt -l cmd internal pkg tests web/embed.go web/embed_test.go
go test ./...
```

List the two `web/*.go` files explicitly: `gofmt` ignores the `go.mod` `ignore` directive, so pointing it at `web` would walk `web/node_modules`.

### GUI frontend (`web/`)

Svelte + Vite + TypeScript SPA (ADR `docs/adr/0001-gui-architecture.md`), embedded into `bv` by `web/embed.go` and served by `bv serve`. Needs Node 24 (`web/.nvmrc`) and npm.

- Build: `make web` (or `npm --prefix web ci` then `npm --prefix web run build`) writes `web/dist/app`; the next `go build` embeds it. `make build`, `make install`, `make release-gate` and `make release-package` run it first.
- Without it, `go build` still works with no Node installed, and `bv serve` shows the placeholder page (`web/dist/index.html`).
- Only `web/dist/index.html` is tracked. Never commit `web/dist/app` or `web/node_modules`.
- Checks: `npm --prefix web run check` (svelte-check, also compares the golden fixtures in `web/src/lib/api/__fixtures__` with `types.ts`) and `npm --prefix web test` (vitest, pure modules only).
- CSP is `default-src 'self'`: no inline `<script>`/`<style>`, no `style=` attributes or `style:` directives in Svelte markup, no `data:` assets (the build sets `assetsInlineLimit: 0`).
- Dev loop: run `go run ./cmd/bv serve --serve-dev --serve-port 7777 --serve-no-open` and `npm --prefix web run dev` (set `BV_API_PORT` if the port is not 7777), then open `http://localhost:5173/auth?token=...` with the token `bv` prints.

## Conventions enforced by tests

- Read `BV_*` / `BEADS_*` environment variables through `internal/env`, never raw `os.Getenv`.
- Unexported methods on `ui.Model` (`pkg/ui`) use `(m *Model)` pointer receivers.
- Tests must never open a browser: browser-opening code returns early when `BV_NO_BROWSER` or `BV_TEST_MODE` is set.
- Edit patched dependencies in `third_party/`, then `go mod vendor`; never edit `vendor/` directly.

## Using bv as an agent

Never run bare `bv` (it opens the TUI and blocks); use `--robot-*` flags, e.g. `bv --robot-triage`. This repository's tracker is `br`.
