# AGENTS.md — beads_viewer (jonapich fork)

`bv` is a terminal UI for Beads issue trackers (`.beads/beads.db`, `.beads/issues.jsonl`, legacy `.beads/beads.jsonl`), with graph metrics and `--robot-*` JSON outputs. Entry point: `cmd/bv/`; libraries under `pkg/` and `internal/`.

## Build & check

Go modules only; dependencies are vendored (`vendor/`, patched modules in `third_party/`). After code changes:

```bash
go build ./...
go vet ./...
gofmt -l cmd internal pkg tests
go test ./...
```

## Conventions enforced by tests

- Read `BV_*` / `BEADS_*` environment variables through `internal/env`, never raw `os.Getenv`.
- Unexported methods on `ui.Model` (`pkg/ui`) use `(m *Model)` pointer receivers.
- Tests must never open a browser: browser-opening code returns early when `BV_NO_BROWSER` or `BV_TEST_MODE` is set.
- Edit patched dependencies in `third_party/`, then `go mod vendor`; never edit `vendor/` directly.

## Using bv as an agent

Never run bare `bv` (it opens the TUI and blocks); use `--robot-*` flags, e.g. `bv --robot-triage`. This repository's tracker is `br`.
