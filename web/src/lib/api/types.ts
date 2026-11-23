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

/** One row of the issue list (pkg/api/dto.go ListItem). */
export interface ListItem {
  id: string;
  title: string;
  status: string;
  priority: number;
  issue_type: string;
  assignee?: string;
  labels: string[];
  /** RFC 3339 */
  created_at: string;
  updated_at: string;
  closed_at?: string;
  /** When a deferred issue becomes actionable; while in the future it is open, unblocked and not ready. */
  defer_until?: string;
  /** First parent-child dependency whose target is in the list. */
  parent_id?: string;
  /**
   * Number of list items whose parent_id is this id (parent-child only, for
   * progress or labels). Not the tree child count: the expander uses
   * tree.children, where an open blocked child sits under its blockers.
   */
  child_count: number;
  /** Active blocker ids, deduplicated, in dependency order; may name issues outside the list. */
  blocked_by: string[];
  /** Open issues with a blocking dependency on this one. */
  dependent_count: number;
  ready: boolean;
  /** Open issue with status blocked or an active blocker. */
  blocked: boolean;
  /** Absent until triage is computed (large tier before phase 2). */
  triage_score?: number;
  quick_win?: boolean;
  is_blocker?: boolean;
  /** "pending" | "approved" | "disputed" | "waived"; absent outside the sign-off flow. */
  signoff?: string;
}

/**
 * One child of a tree node. "child" is a parent-child dependency; "block" means
 * the node blocks the child.
 */
export interface TreeEdge {
  id: string;
  /** "child" | "block" */
  kind: string;
}

/**
 * The list tree as adjacency. An OPEN blocked issue appears under each of its
 * in-list active blockers (kind "block") instead of under its parent (a closed
 * issue is not blocked and stays under its parent), so the same id can
 * have several parents: render lazily (only expanded nodes) and stop at an id
 * already on the current path. Nodes without children are absent from `children`.
 */
export interface ListTree {
  roots: string[];
  children: Record<string, TreeEdge[]>;
}

/** GET /api/v1/issues?view=tree|flat (pkg/api/dto.go IssuesResponse). */
export interface IssuesResponse {
  phase2_ready: boolean;
  total: number;
  items: ListItem[];
  /** Present for view=tree only. */
  tree?: ListTree;
}

export interface SearchHit {
  id: string;
  /** Fuzzy score; higher is better. */
  score: number;
}

/** GET /api/v1/search?q= (pkg/api/dto.go SearchResponse). Hits are in rank order. */
export interface SearchResponse {
  query: string;
  total: number;
  hits: SearchHit[];
}
