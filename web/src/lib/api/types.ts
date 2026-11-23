// Hand-maintained mirrors of the Go API response types (ADR 0001 section 6).
// Each type is checked against a golden fixture in ./__fixtures__ by
// fixtures.typecheck.ts, so `npm run check` fails when they drift.
//
// Enum-like fields are typed `string`, not a literal union: resolveJsonModule
// widens JSON string values to `string`, so a literal type would make the
// fixture check fail.

/** Error envelope for every non-2xx /api response (pkg/api/errors.go). */
export interface ApiErrorBody {
  error: { code: string; message: string };
}

/** GET /api/v1/health (pkg/api/api.go). */
export interface HealthResponse {
  status: string;
  version: string;
}

/** Where the served data came from (pkg/api/dto.go MetaSource). */
export interface MetaSource {
  path: string;
  /** "jsonl" | "sqlite" */
  kind: string;
  /** "bd" | "br" | "" when unknown */
  tracker: string;
  project_dir: string;
}

/** The snapshot's status counts (pkg/api/dto.go MetaCounts). */
export interface MetaCounts {
  open: number;
  ready: number;
  blocked: number;
  closed: number;
}

/** What the server can do (pkg/api/dto.go MetaCaps). */
export interface MetaCaps {
  /** False until the write endpoints land (beads-c2o.19). */
  writes: boolean;
}

/** GET /api/v1/meta (pkg/api/dto.go MetaResponse). */
export interface MetaResponse {
  version: string;
  data_hash: string;
  /**
   * True once the store swapped in the Phase 2 result. The ETag changes at that
   * moment while data_hash does not, so poll with If-None-Match until true.
   */
  phase2_ready: boolean;
  /** True on the huge tier, where graph metrics are not computed. */
  phase2_skipped: boolean;
  phase2_error?: string;
  /** "small" | "medium" | "large" | "huge" | "unknown" */
  tier: string;
  issue_count: number;
  view_count: number;
  source_issue_count: number;
  loaded_open_only: boolean;
  truncated_count: number;
  large_dataset_warning?: string;
  load_warning_count: number;
  authority_complete: boolean;
  /** RFC 3339 instant readiness and scoring were computed at. */
  reference_time: string;
  source: MetaSource;
  recipe?: string;
  counts: MetaCounts;
  capabilities: MetaCaps;
}