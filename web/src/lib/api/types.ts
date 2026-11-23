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

/** A URL attached to a bead (pkg/agentdesk Link). */
export interface Link {
  /** "mock" | "artifact" | "capture" | "commit" | "doc" | "url" | other typed kinds */
  kind: string;
  url: string;
  note?: string;
  /** RFC 3339 */
  at?: string;
  by?: string;
  /** "typed" (metadata link.<kind>) | "comment" (a URL found in a comment) */
  source: string;
}

/** One answer an awaiting bead offers (pkg/agentdesk Option). */
export interface DecisionOption {
  /** Lower-case letter: a, b, c... */
  key: string;
  text: string;
  recommended?: boolean;
}

/** One "call made on your behalf": the call, what it was chosen over, and why. */
export interface SignoffCall {
  call: string;
  alternative?: string;
  why?: string;
}

/**
 * A closed bead awaiting sign-off (pkg/api SignoffDTO: pkg/agentdesk
 * SignoffItem plus `calls`). The parts are lists of lines; `calls` is the
 * structured form of calls_made, which flattens objects into sorted
 * "key: value" lines.
 */
export interface SignoffItem {
  id: string;
  title: string;
  epic?: string;
  priority: number;
  closed_at?: string;
  /** "pending" | "approved" | "disputed" | "waived" */
  state: string;
  has_signoff_comment: boolean;
  /** Structured calls for the sign-off card rows; [] when none. */
  calls: SignoffCall[];
  calls_made?: string[];
  what_changed?: string[];
  removed_or_renamed?: string[];
  where_to_see?: string[];
  commits?: string[];
  captures?: string[];
  links?: Link[];
  /** The comment body when it fits none of the parts. */
  raw?: string;
}

export interface SignoffGroup {
  /** Epic id; "" for beads with no epic. */
  epic: string;
  epic_title?: string;
  items: SignoffItem[];
}

/** GET /api/v1/signoff: closed beads pending sign-off, grouped by epic. */
export interface SignoffResponse {
  total: number;
  groups: SignoffGroup[];
}

/**
 * Another issue, with just enough to render a row. When `missing` is true the
 * id is not loaded and everything else is absent. `priority` can be 0.
 */
export interface IssueRef {
  id: string;
  title?: string;
  status?: string;
  priority?: number;
  issue_type?: string;
  missing?: boolean;
}

/**
 * An IssueRef in a blocking relation. For blocked_by, `active` is true while
 * the blocker still blocks; for blocks, while the dependent is still blocked.
 */
export interface DepRef extends IssueRef {
  active: boolean;
}

/** A non-blocking, non-parent link. direction "out": this issue declared it. */
export interface RelatedRef extends IssueRef {
  /** "related" | "discovered-from" | ... */
  type: string;
  /** "out" | "in" */
  direction: string;
}

export interface BlockerCounts {
  active: number;
  resolved: number;
}

export interface TriageReasons {
  primary: string;
  all: string[];
  action_hint: string;
}

/** Absent from IssueDetail when triage was not computed. */
export interface DetailTriage {
  score: number;
  reasons?: TriageReasons;
  quick_win: boolean;
  is_blocker: boolean;
  unblocks_ids: string[];
}

/** Present only once Phase 2 is done, and never on the huge tier. */
export interface DetailGraph {
  pagerank: number;
  betweenness: number;
  eigenvector: number;
  hub: number;
  authority: number;
  /** Analysis critical-path score; higher means more work is chained behind this issue. */
  critical_path_score: number;
}

/** The "Needs your decision" card data (open bead labelled needs-decision/needs-design). */
export interface DecisionInfo {
  /** The awaiting label an answer removes. */
  label: string;
  /** A mock or artifact link is attached. */
  ready_to_judge: boolean;
  /** For needs-design: the artifact still missing. */
  waiting_on?: string;
  /** The description's "Question: ...?" sentence; absent when it has none (fall back to the title). */
  question?: string;
  /** Empty when the description offers none: an answer then needs free text. */
  options: DecisionOption[];
}

/**
 * A comment with its bd kind header split off. `text` is the body after the
 * header. Collapsible comments render as a one-line `summary`; the client keeps
 * signoff, user, answer and plain comments expanded. `data` is the JSON object
 * body, when there is one.
 */
export interface IssueComment {
  id: string;
  author: string;
  created_at: string;
  kind?: string;
  text: string;
  summary?: string;
  data?: Record<string, unknown>;
  collapsible: boolean;
}

/** GET /api/v1/issues/{id} (pkg/api/dto.go IssueDetail). */
export interface IssueDetail {
  phase2_ready: boolean;

  id: string;
  title: string;
  description: string;
  design?: string;
  acceptance_criteria?: string;
  notes?: string;
  status: string;
  priority: number;
  issue_type: string;
  assignee?: string;
  estimated_minutes?: number;
  created_at: string;
  created_by?: string;
  updated_at: string;
  due_date?: string;
  defer_until?: string;
  closed_at?: string;
  close_reason?: string;
  external_ref?: string;
  started_at?: string;
  lease_expires_at?: string;
  heartbeat_at?: string;
  labels: string[];
  source_repo?: string;

  /**
   * The decoded metadata object: stage, stage_at.<stage> and signoff_at
   * timestamps, link.<kind> and claim data. The History timeline is built from
   * it plus the dates above.
   */
  metadata?: Record<string, unknown>;

  parent?: IssueRef;
  /** Issues that declare this one as their parent, in snapshot order. */
  children: IssueRef[];
  /** Every distinct blocking dependency, active ones first. */
  blocked_by: DepRef[];
  /** Open issues with a blocking dependency on this one. */
  blocks: DepRef[];
  related: RelatedRef[];

  blocker_counts: BlockerCounts;
  active_dependents: number;
  ready: boolean;
  blocked: boolean;

  triage?: DetailTriage;
  graph?: DetailGraph;

  /** Present when the issue is in the sign-off flow, in any state. */
  signoff?: SignoffItem;
  decision?: DecisionInfo;
  links: Link[];
  /** Oldest first, as bd lists them. */
  comments: IssueComment[];

  /** Why bv cannot write to this issue's tracker; absent when it can. */
  read_only_reason?: string;
}
