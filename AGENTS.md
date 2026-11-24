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

### Test run time (`tests/e2e`)

`tests/e2e` drives a once-built `bv` binary and is the slow package. On the Windows dev host the default run takes ~3 min (it took ~40 min while serial; the export tests also overlap their waits with `t.Parallel`). An `--export-pages` run takes ~1s there: the offline manifest hashes files from the bytes as they are written, because Defender scans a freshly written file on its first open (even a stat), which blocked every export for ~10s on `vendor/mermaid.min.js` (beads-c2o.40). Use `-count=1` for a fresh gate run: without it `go test` spent another ~75s after the tests finished writing its result cache.

```powershell
go test -count=1 -timeout 30m ./tests/e2e/...                              # default, ~3 min
$env:BV_E2E_SLOW = "1"; go test -count=1 -timeout 60m ./tests/e2e/...      # full set, ~8 min
```

- `BV_E2E_SLOW=1` (any value other than empty or `0`) adds what the default run skips or shortens: `TestRobotSearchJudgedRelevance` (600 searches over corpora of up to 10,000 issues, skipped by default) and the full 100 cases of each `rapid.Check` metamorphic test (10 by default, `defaultRapidChecks` in `tests/e2e/common_test.go`). An explicit `-rapid.checks=N` or `RAPID_CHECKS` wins over both. Run the full set before changing `--robot-search`, the robot scoping paths or export-pages.
- Independent tests call `t.Parallel()`. Tests that use `t.Setenv` cannot (it panics), so they stay serial; pass the environment to the child through `cmd.Env` instead when a test needs to be parallel. Don't share a fixed directory between parallel tests.
- Tests that assert a wall-clock budget (`TestCorrelationOnThisRepository_StrategyCounts`, `TestExportTopology_LargeScale`) stay serial: under the parallel load they exceed their budget.
- Viewer assets are staged next to the shared binary once (`stageViewerAssets`, `sync.Once`), not per test.
- A timeout around a `bv` export in a parallel test is a hang guard, not a performance assertion: keep it generous (the export tests share the host with ~20 others).

### GUI frontend (`web/`)

Svelte + Vite + TypeScript SPA (ADR `docs/adr/0001-gui-architecture.md`), embedded into `bv` by `web/embed.go` and served by `bv serve`. Needs Node 24 (`web/.nvmrc`) and npm.

- Build: `make web` (or `npm --prefix web ci` then `npm --prefix web run build`) writes `web/dist/app`; the next `go build` embeds it. `make build`, `make install`, `make release-gate` and `make release-package` run it first.
- Without it, `go build` still works with no Node installed, and `bv serve` shows the placeholder page (`web/dist/index.html`).
- Only `web/dist/index.html` is tracked. Never commit `web/dist/app` or `web/node_modules`.
- Checks: `npm --prefix web run check` (svelte-check, also compares the golden fixtures in `web/src/lib/api/__fixtures__` with `types.ts`) and `npm --prefix web test` (vitest, pure modules only).
- E2E: `npm --prefix web run e2e` builds the web bundle and `bv`, creates a disposable `bd` tracker in `%TEMP%` (never a repo's own `.beads`), runs two `bv serve` instances (a writable one and a read-only copy) and drives the installed Edge with Playwright (`web/e2e/*.e2e.ts`, about 1 minute). It needs `bd` (`BV_E2E_BD`, else `../beads/dist/windows-amd64/bd.exe`, else `PATH`), Go, git and Edge (`BV_E2E_CHANNEL=chrome` switches browser). If one is missing, every test is skipped with the reason. `BV_E2E_KEEP=1` keeps the temp dir, `BV_E2E_BV=<exe>` skips the `go build`. After a failure, open the trace: `npx --prefix web playwright show-trace web/test-results/<test>/trace.zip`. No browser download (`playwright install`) is needed. It is local-only: no CI job runs it, and `make test` and `release-gate` leave it out. Tests wait on conditions, never fixed sleeps; a test that mutates owns its own issue (ids come from the fixture manifest in `e2e/lib/tracker.ts`), and a console error fails the test unless it is allowed with `gate.allow(...)`. `npm --prefix web run check` also type-checks `e2e/`.
- CSP is `default-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'`: no inline `<script>`/`<style>`, no `style=` attributes or `style:` directives in Svelte markup, no `data:` assets (the build sets `assetsInlineLimit: 0`).
- Dev loop: run `go run ./cmd/bv serve --serve-dev --serve-port 7777 --serve-no-open` and `npm --prefix web run dev` (set `BV_API_PORT` if the port is not 7777), then open `http://localhost:5173/auth?token=...` with the token `bv` prints.

## Conventions enforced by tests

- Read `BV_*` / `BEADS_*` environment variables through `internal/env`, never raw `os.Getenv`.
- Unexported methods on `ui.Model` (`pkg/ui`) use `(m *Model)` pointer receivers.
- Tests must never open a browser: browser-opening code returns early when `BV_NO_BROWSER` or `BV_TEST_MODE` is set.
- Edit patched dependencies in `third_party/`, then `go mod vendor`; never edit `vendor/` directly.

## Using bv as an agent

Never run bare `bv` (it opens the TUI and blocks); use `--robot-*` flags, e.g. `bv --robot-triage`. This repository's tracker is `br`.
