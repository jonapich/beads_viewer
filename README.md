# Beads Viewer (bv) — jonapich fork

A terminal UI for browsing [Beads](https://github.com/steveyegge/beads) issue trackers: list/detail split view, kanban board, dependency graph and graph-metric insights.

This is a personal fork of [Dicklesworthstone/beads_viewer](https://github.com/Dicklesworthstone/beads_viewer). For full documentation, see upstream.

## Build

```bash
go build -o bv ./cmd/bv
```

## Use

Run `bv` inside a repository that has a `.beads/` directory.

## Differences from upstream

- No automatic update check at startup; `bv upgrade` targets this fork's releases.
- TOON / alternate output formats removed.
- No `--agents-*` AGENTS.md blurb management.
- Documentation trimmed to this file.
