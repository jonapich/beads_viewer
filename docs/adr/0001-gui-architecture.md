# ADR 0001: GUI architecture (`bv serve`)

- Status: proposed
- Date: 2026-11-22
- Tracking: epic `beads-c2o`, this decision `beads-c2o.1`

## Context

bv's only UI is the bubbletea TUI in `pkg/ui`. We want a GUI that reuses as much
Go as possible. Phase 1 is a localhost web server (`bv serve`) with an embedded
Svelte frontend. Phase 2 is an optional Wails desktop wrapper around the same
frontend.

Ground rules from the epic:

- **Parity only.** The GUI exposes what bv already does against our bd fork, and
  nothing new. That includes the existing writes: sign-off approve/dispute,
  agent-question answers, priority, the close-candidate triage toggle, and the
  `O` field editor.
- No graph view.
- `--export-pages` and `pkg/export/viewer_assets` stay untouched. They are
  reference only.

What the code gives us today:

| Concern | Where it lives now | UI-free? |
|---|---|---|
| Loading | `internal/datasource` (`LoadIssues`, `LoadFromSource`), `pkg/loader` | yes |
| Model | `pkg/model.Issue` (has json tags) | yes |
| Analysis | `pkg/analysis` (`NewAnalyzer`, `ComputeTriage*`, insights, plan, ...) | yes |
| Change watching | `pkg/watcher.Watcher` (fsnotify with polling fallback, `Changed()` chan) | yes |
| TUI data pipeline | `pkg/ui/snapshot.go`: `SnapshotBuilder` → `DataSnapshot` (sorted issues, ID map, counts open/ready/blocked/closed, analyzer + `GraphStats`, insights, triage scores/reasons, quick-win/blocker sets, unblocks map, parent/child tree, board columns, data hash, two-phase analysis). Built off the UI thread | mostly. It mixes in `bubbles/list` items and graph layout |
| Robot JSON | `cmd/bv/robot_registry.go` etc. Agent-facing. Adds agent-only behaviour (triage history, `--brief`, usage hints, pinned-clock stabilisation) on top of `pkg/analysis` | no (package main), and **not used by the GUI** |
| Writes | `pkg/ui/model.go` (`runEditorTracker`, `runTrackerOps`, `runIssueUpdateCmd`), `pkg/ui/agent_desk_model.go` (`signoffDeskBead`, `postDeskAnswer`) | **no** (returns `tea.Cmd`) |
| Tracker origin | `pkg/loader` (`IssueOrigin`, executable/db/workdir resolution, `RefreshBDExport`) | yes |
| Local HTTP plumbing | `pkg/export/preview.go` (127.0.0.1 listener, `previewRequestGuard` Host/Origin check, no-cache middleware), `livereload.go` (SSE hub), `github.go` (`prepareBrowserOpen`, honours `BV_NO_BROWSER`) | yes |
| CLI | cobra root plus pflag mode flags in `cmd/bv/main.go`. Bare words are rewritten to flags by `rewriteAgentIntentCommand`. Modes are mutually exclusive via the tables around `main.go:1971` | — |

How bv is actually used (tally of 2,149 agent transcripts from a real project
using this fork, 2026-11-22): agents **never** invoked `bv`. All tracker
activity was plain bd CRUD. In order of frequency: `show`, `update`,
`comments`, `create`, `close`, `export`, `label`, `list`, `search`, `dep`,
`ready`. The robot outputs serve no one in our workflow. The GUI is a
replacement for the human-facing TUI, so **the TUI, not the robot commands, is
the reference for both behaviour and code reuse.**

## Decisions

### 1. Package layout

```
cmd/bv/              serve mode wiring only (flag parsing, start server)
pkg/snapshot/        TUI data pipeline lifted out of pkg/ui (beads-c2o.3)
pkg/tracker/         lifted bd/br write path (beads-c2o.18)
pkg/api/             HTTP handlers, DTOs, SSE hub, auth/guard, snapshot store
web/                 Svelte + Vite + TS app (beads-c2o.7)
web/embed.go         package web: //go:embed all:dist  -> exported fs.FS
```

- **`pkg/snapshot`**: move the UI-free core of `SnapshotBuilder` /
  `DataSnapshot` out of `pkg/ui`. That core is: issues, ID map, counts,
  analyzer/stats, insights, triage scores and reasons, quick-win/blocker sets,
  unblocks map, the parent/child tree (`IssueTreeNode`), data hash, two-phase
  readiness, and load warnings. The TUI-only parts (`IssueItem` /
  `bubbles/list` items, list indexes, graph layout, board columns) stay in
  `pkg/ui`, which wraps or extends the core snapshot. Both the TUI and
  `pkg/api` then build from the same pipeline, which is what guarantees parity:
  the same triage options and the same tree. Other UI-free derivations found by
  the beads-c2o.3 audit (e.g. search ranking, detail-pane assembly) go here or
  next to it.
- **Robot builders are not lifted.** `cmd/bv`'s `--robot-*` code stays as is
  (beads-c2o.2 is descoped). The API calls `pkg/snapshot` and `pkg/analysis`
  directly with the TUI's options.
- **`pkg/tracker`**: a `Client` built from a `model.Issue` (its `Origin`, the
  tracker route the loader attached) and targeting that issue, with methods
  that mirror today's actions: `Signoff(ctx, approve bool, note string)`,
  `SetPriority(ctx, p)`, `SetLabel(ctx, label, on bool)`, `AddComment`,
  `AnswerQuestion`, `Update(ctx, FieldDiff)`, `RefreshExport(ctx)`. Because the
  client is already bound to the issue, the methods take no `id`. It keeps the bd
  `signoff` path and the br/old-bd fallback exactly as `pkg/ui` does now. After
  that, `pkg/ui` wraps these calls in `tea.Cmd`, and the API calls them directly.
- **`pkg/api`** depends on `pkg/snapshot`, `pkg/analysis`, `pkg/tracker`,
  `pkg/watcher`, `internal/datasource`. It never depends on `pkg/ui` or bubbletea. A test
  enforces this (same approach as the existing convention tests).
- **Embedding**: `go:embed` cannot reach parent directories, so the embed lives
  in `web/embed.go`. `web/dist/index.html` is committed as a placeholder page
  ("frontend not built, run `npm --prefix web run build`") so `go build ./...`
  works on a machine with no Node. The Vite build writes to `web/dist/app`
  (gitignored, so the placeholder is never overwritten); `web/embed.go` serves
  `dist/app` when it is built and the placeholder otherwise.
  Release builds run the frontend build first (new `make web` target, which the
  `build` and `release-*` targets depend on).

### 2. CLI surface

Follow the existing mode-flag pattern instead of introducing real cobra
subcommands:

- `--serve` is a new mode flag, added to the mutual-exclusion tables. The bare
  word `serve` is rewritten to `--serve` in `rewriteAgentIntentCommand`, so
  `bv serve` works.
- `--serve-port N`: default `0`, meaning a kernel-chosen random port.
- `--serve-no-open`: don't open the browser. `BV_NO_BROWSER` also suppresses it,
  via `prepareBrowserOpen`.
- `--serve-dev`: API only (no embedded assets), relaxed so the Vite origin is
  allowed. See §5.
- On startup it prints the URL (including the bootstrap token) to stdout, so a
  user with `--serve-no-open` can paste it.

### 3. API style

REST JSON under `/api/v1`, plus one SSE stream.

Read (beads-c2o.5):

| Endpoint | Source |
|---|---|
| `GET /api/v1/meta` | data hash, source path, tracker kind (bd/br), capabilities (e.g. bd `signoff` available) |
| `GET /api/v1/issues?view=tree\|flat` | snapshot issues + a tree adjacency (`roots`, `children` edges of kind child/block; an open blocked issue sits under each of its blockers), projected to a slim list DTO (id, title, status, priority, type, assignee, labels, counts, triage score, ready/blocked flags) |
| `GET /api/v1/issues/{id}` | full issue + deps/dependents + comments + unblocks + triage reasons, the same data as the TUI detail pane |
| `GET /api/v1/triage` | snapshot triage (TUI options, feedback-tuned weights as in the TUI), without `meta` timing, `commands` or per-recommendation `actions` (they carry local paths). 503 `analysis_pending` with `Retry-After: 2` until triage exists (large/huge tiers before phase 2); 500 `analysis_failed` when phase 2 failed without it. Small/medium tiers serve it at phase 1 with `phase2_ready: false` |
| `GET /api/v1/insights?limit=N` | snapshot insights (TUI options). `limit` defaults to 25, 1..500, else 400; every list is capped, `orphan_count`/`cycle_count` are the pre-cap totals, and `skipped` maps a list name to the reason its metric was skipped or timed out (as the TUI insights panels show; filled once phase 2 is done). Lists may be empty or partial until `phase2_ready` |
| `GET /api/v1/search?q=` | the TUI's search path (`pkg/search`) |
| `GET /api/v1/signoff` | the TUI agent-desk data (lifted with the snapshot if it lives in `pkg/ui`) |
| `GET /api/v1/events` | SSE (§4) |

Views beyond these (plan, labels, sprints, history) get endpoints when their
GUI bead is picked up, sourced the same way (TUI path, not robot).

Write (beads-c2o.19, beads-c2o.14), one endpoint per existing action:

| Endpoint | `pkg/tracker` call |
|---|---|
| `POST /api/v1/issues/{id}/signoff` `{approve, note}` | `Signoff` |
| `POST /api/v1/issues/{id}/answer` `{text}` | `AnswerQuestion` |
| `PUT /api/v1/issues/{id}/priority` `{priority}` | `SetPriority` |
| `PUT /api/v1/issues/{id}/labels/close-candidate` / `DELETE` | `SetLabel` |
| `PATCH /api/v1/issues/{id}` `{title?, status?, priority?, assignee?, type?, description?, base_updated_at}` | `Update` (only fields the `O` editor supports) |

Conventions:

- **DTOs are GUI-specific**, defined in `pkg/api`, and shaped for the screens
  rather than mirroring robot payloads. When a field already exists on
  `model.Issue` or an `analysis` type, the DTO uses the same json name.
- **Errors** are always `{"error": {"code": "...", "message": "..."}}` with a
  matching HTTP status. Tracker failures pass bd's stderr through in `message`.
- **Snapshot store**: the server holds one immutable `pkg/snapshot` snapshot
  behind an `atomic.Pointer`. It is built with the same two-phase approach as
  the TUI: phase 1 is served immediately, and phase 2 swaps in and fires an SSE
  event when ready.
  Snapshot-backed responses carry an `ETag` so the client can use `If-None-Match`:
  `"<data hash>-g<generation>-<phase>"`, where the phase is `p1` for phase 1,
  `p2` once phase 2 is swapped in and `p1e` when phase 2 failed. The data hash
  alone does not change at phase 2, so the suffix is what makes phase 2 visible
  (with `meta.phase2_ready`). The generation is 1 at startup and goes up once
  per published reload (a phase 2 swap keeps it); it keeps a moved reference
  time from reusing a tag when the data hash is unchanged. `/meta` adds a
  `~e<seq>` variant while the last reload failed (`seq` goes up per failure), so
  its tag changes without touching the data endpoints' tags. Endpoints whose
  body depends on the request add `~<variant>`: `view=`, `q=`, `limit=` and the
  issue id. Errors carry no ETag.
- **Writes are serialised** behind a mutex, the same model as the TUI (one bd
  process at a time). After a successful write the server runs `RefreshExport`,
  reloads, and swaps the snapshot before it responds. The response contains the
  new data hash, so the client knows its next GET is fresh.
- **Conflict check** on `PATCH`: if `base_updated_at` doesn't match the current
  snapshot, return `409`. This reproduces what the TUI editor does when it
  re-reads with `show --json` before an update.

### 4. Live updates (beads-c2o.6)

- A `pkg/watcher.Watcher` runs on the same source paths the TUI watches (the
  same debounce, so the polling fallback carries over for free). On change it
  reloads, swaps the snapshot, and broadcasts.
- `GET /api/v1/events` is SSE. Events are signals only, not data:
  `event: snapshot` / `data: {"generation": n, "hash": "...", "phase":
  "phase1"|"phase2"|"phase2_error", "reload_error"?: "..."}`. The client
  refetches whatever the current view needs. The stream starts with a
  `retry: 2000` hint and one `snapshot` event on connect, so a reconnecting
  client catches up on what it missed; after that there is one event per
  published reload, phase 2 swap or reload-error change (a failed reload is a
  `snapshot` event with `reload_error` set, not a separate event type). A burst
  coalesces: the payload is read from the store when it is written. A heartbeat
  comment every 25 s keeps proxies and Wails from dropping the stream.
- The hub is a small new type in `pkg/api`, modelled on
  `export.LiveReloadHub`'s fan-out (non-blocking send per client). We don't
  reuse it directly: it watches a static bundle directory and has no payload.

### 5. Security

Even read-only data is private. Writes run bd on the user's behalf.

- Bind **127.0.0.1 only** (never `0.0.0.0`). Random port by default.
- **Host/Origin guard**: lift `previewRequestGuard` / `previewAuthority` from
  `pkg/export/preview.go` into a shared helper (e.g. `internal/localhttp`) and
  use it in both places. It rejects foreign `Host` headers (DNS rebinding) and
  mismatched `Origin`. Unlike preview, `pkg/api` allows non-GET methods.
- **Per-run token**: 32 random bytes, generated at startup. The opened URL is
  `http://127.0.0.1:PORT/auth?token=...`. That endpoint sets an
  `HttpOnly; SameSite=Strict; Path=/` cookie and redirects to `/`, so the token
  leaves the address bar and history. Every `/api/*` request (including SSE,
  where `EventSource` can't set headers, which is why we use a cookie rather
  than a bearer header) needs the cookie. Missing or invalid → `401`.
- Mutating methods additionally require an exact `Origin` match. This is
  defence in depth on top of SameSite.
- Response headers come from preview: `nosniff`, COOP/CORP same-origin, plus a
  strict CSP (`default-src 'self'`).
- `--serve-dev` adds `http://localhost:5173` and `http://127.0.0.1:5173` to the
  allowed origins. Everything else stays the same.

### 6. TypeScript types

**Hand-maintained**, in `web/src/lib/api/types.ts`. Golden fixtures check them
for drift:

- API tests (beads-c2o.12) write golden JSON responses for a fixture tracker to
  `web/src/lib/api/__fixtures__/*.json`.
- `web/src/lib/api/fixtures.typecheck.ts` imports each fixture and assigns it to
  its declared type (`const x: TriageResponse = triageFixture`). `npm run check`
  (`svelte-check`/`tsc`) fails if a Go change breaks the shape the frontend
  relies on.

We rejected tygo (or similar generators) for three reasons:

- the DTOs are small and owned by `pkg/api`, so hand-writing them is cheap
- the few embedded `pkg/analysis` types (triage reasons, insights) are deep and
  cross-package, which generators handle poorly
- the repo is vendored, and adding a codegen tool to the build adds friction

We can revisit this if the hand-written types become a burden.

### 7. Dev loop

```powershell
# terminal 1: API only, fixed port, no browser
go -C beads_viewer run ./cmd/bv serve --serve-dev --serve-port 7777 --serve-no-open
# terminal 2: Vite with HMR; proxies /api and /auth to 127.0.0.1:7777
npm --prefix beads_viewer/web run dev
```

- `vite.config.ts` reads the API port from `BV_API_PORT` (default 7777) and
  proxies `/api` (with `ws`/SSE passthrough) and `/auth`.
- Open the Vite URL using the token path printed by bv:
  `http://localhost:5173/auth?token=...`. The proxy sets the cookie on the Vite
  origin.
- Production: `make web` (`npm --prefix web run build`) → `web/dist/app`, then `go build`
  embeds it. Hashed assets are served with `Cache-Control: immutable`, and
  `index.html` with `no-store` (borrowed from preview's no-cache middleware).
  Unknown non-`/api` paths fall back to `index.html` (SPA routing).
- Node toolchain: pin the Node major in `web/.nvmrc` and `engines`. Use npm
  with a committed lockfile.

### 8. What we borrow from `pkg/export`

| Take | How |
|---|---|
| `previewRequestGuard` / `previewAuthority` | lift to a shared helper (§5) |
| `prepareBrowserOpen` / `OpenInBrowser` | call as is (handles `BV_NO_BROWSER`, Windows `rundll32`) |
| no-cache middleware, server timeouts (`ReadHeaderTimeout` 5 s, etc.) | copy the pattern. The SSE route needs no `WriteTimeout` |
| graceful shutdown on SIGINT (`StartWithGracefulShutdown`) | copy the pattern |
| `LiveReloadHub` | pattern only (§4) |
| `FindAvailablePort` (9000–9100) | **not used**. We use `:0` |
| `viewer_assets` (Alpine, sql.js, graph code, `bv-graph-wasm`) | **not used**. There's no graph view. The viewer's SQL and filters are only a hint at which fields each screen needs |

## Consequences

- beads-c2o.3 (lift the snapshot core) and beads-c2o.18 (lift the write path)
  are refactors with no behaviour change: TUI behaviour and robot output stay
  identical. They land first, each as its own PR. The snapshot lift is the
  larger one, because `DataSnapshot` mixes list-widget caches into the core.
- The robot CLI is untouched, which keeps our diff against upstream small in
  `cmd/bv`.
- `pkg/api` can be tested entirely with `httptest` against a fixture tracker,
  with no browser (beads-c2o.12). Playwright runs against a real `bv serve`.
- Every release build now needs Node. A Go-only build still works but serves
  the placeholder page.
- Phase 2 (Wails, beads-c2o.13) can mount the same `web.FS` and either proxy to
  an in-process `pkg/api` handler or call it through Wails bindings. We'll
  decide that in a later ADR.

## Downstream bead updates

- beads-c2o.2 (lift robot builders): descoped, not needed.
- beads-c2o.3: becomes "lift the `DataSnapshot` core into `pkg/snapshot`" plus
  the audit, and now blocks beads-c2o.5.

- beads-c2o.4: add the `--serve*` flags (§2), the auth bootstrap (§5), and the
  placeholder `web/dist` (§1).
- beads-c2o.5 / .19 / .14: endpoint list as in §3.
- beads-c2o.7: hand-maintained types + fixture typecheck (§6), Vite proxy (§7).
- beads-c2o.6: snapshot-hash signal events (§4).
- beads-c2o.12: golden fixtures double as TS drift checks (§6).
