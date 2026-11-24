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
  /** True when the write endpoints can reach the tracker (the first issue's origin is a complete route). */
  writes: boolean;
  /** True while the server watches the tracker and serves GET /api/v1/events. */
  events: boolean;
}

/** Data of a GET /api/v1/events "snapshot" message (pkg/api/dto.go SnapshotEvent). A signal: refetch what you show. */
export interface SnapshotEvent {
  generation: number;
  hash: string;
  /** "phase1" | "phase2" | "phase2_error" */
  phase: string;
  /** Set while the last reload failed; the served data is the last good load. */
  reload_error?: string;
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
  /** Store generation: 1 at startup, plus one per published reload. */
  generation: number;
  /** RFC 3339 instant the served snapshot was built. */
  loaded_at: string;
  /** Set while the last reload failed; the data is then the last good load. */
  reload_error?: string;
  /** RFC 3339 */
  reload_error_at?: string;
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

export interface TopPick {
  id: string;
  title: string;
  score: number;
  reasons: string[];
  /** How many items this unblocks. */
  unblocks: number;
}

/** Counts at a glance (pkg/analysis QuickRef). open_count and blocked_count are strict status counts. */
export interface QuickRef {
  open_count: number;
  actionable_count: number;
  blocked_count: number;
  in_progress_count: number;
  not_closed_count: number;
  not_actionable_count: number;
  readiness_blocked_count: number;
  deferred_or_parked_count: number;
  /** Top 3 recommended items. */
  top_picks: TopPick[];
}

/** The weighted score factors behind a recommendation (the risk signals are not sent). */
export interface ScoreBreakdown {
  pagerank: number;
  betweenness: number;
  blocker_ratio: number;
  staleness: number;
  priority_boost: number;
  time_to_impact: number;
  urgency: number;
  risk: number;
  pagerank_norm: number;
  betweenness_norm: number;
  blocker_ratio_norm: number;
  staleness_norm: number;
  priority_boost_norm: number;
  time_to_impact_norm: number;
  urgency_norm: number;
  risk_norm: number;
  time_to_impact_explanation?: string;
  urgency_explanation?: string;
  risk_explanation?: string;
}

/** pkg/analysis Recommendation without its actions (bd argv, working directories). */
export interface TriageRecommendation {
  id: string;
  title: string;
  type: string;
  status: string;
  assignee?: string;
  priority: number;
  labels: string[];
  defer_until?: string;
  score: number;
  breakdown: ScoreBreakdown;
  /** Suggested next action, human-readable. */
  action: string;
  reasons: string[];
  unblocks_ids: string[];
  blocked_by?: string[];
  /** The robot top-pick gate: open, unblocked, unassigned, not deferred, not a container. */
  claimable: boolean;
}

export interface QuickWin {
  id: string;
  title: string;
  status: string;
  score: number;
  reason: string;
  unblocks_ids?: string[];
}

export interface BlockerItem {
  id: string;
  title: string;
  unblocks_count: number;
  unblocks_ids: string[];
  /** Can this blocker be worked on now? */
  actionable: boolean;
  blocked_by?: string[];
}

export interface HealthCounts {
  total: number;
  open: number;
  closed: number;
  blocked: number;
  actionable: number;
  not_closed: number;
  dependency_blocked: number;
  by_status: Record<string, number>;
  by_type: Record<string, number>;
  /** Keys are the priorities as strings ("0".."4"). */
  by_priority: Record<string, number>;
}

export interface GraphHealth {
  node_count: number;
  edge_count: number;
  density: number;
  has_cycles: boolean;
  cycle_count?: number;
  phase2_ready: boolean;
}

export interface VelocityWeek {
  /** RFC 3339, UTC Monday */
  week_start: string;
  closed: number;
}

export interface Velocity {
  closed_last_7_days: number;
  closed_last_30_days: number;
  avg_days_to_close: number;
  /** Newest first. */
  weekly?: VelocityWeek[];
  /** True when computed from the current snapshot only. */
  estimated?: boolean;
}

export interface Staleness {
  stale_count: number;
  stalest_issue_id: string;
  stalest_issue_days: number;
  threshold_days: number;
}

export interface ProjectHealth {
  counts: HealthCounts;
  graph: GraphHealth;
  velocity?: Velocity;
  staleness?: Staleness;
}

export interface Alert {
  /** "stale" | "velocity_drop" | "cycle" | "duplicate" | ... */
  type: string;
  /** "info" | "warning" | "error" | "critical" */
  severity: string;
  message: string;
  issue_id?: string;
  issue_ids?: string[];
}

/**
 * GET /api/v1/triage (pkg/api/dto.go TriageResponse). 503 "analysis_pending"
 * (Retry-After: 2) until triage exists on the large and huge tiers; on the
 * small and medium tiers it is served at phase 1 with phase2_ready false and
 * the ETag changes when Phase 2 lands. 500 "analysis_failed" when Phase 2
 * failed and there is no triage. A tracker with nothing to triage answers 200
 * with empty lists once Phase 2 is done.
 */
export interface TriageResponse {
  phase2_ready: boolean;
  /** Phase 2 failed but triage is still served from phase 1; phase2_ready then stays false. */
  phase2_error?: string;
  issue_count: number;
  quick_ref: QuickRef;
  recommendations: TriageRecommendation[];
  quick_wins: QuickWin[];
  blockers_to_clear: BlockerItem[];
  project_health: ProjectHealth;
  alerts?: Alert[];
}

/** One row of an insight list. */
export interface InsightEntry {
  id: string;
  title?: string;
  value: number;
}

/**
 * GET /api/v1/insights?limit=N (pkg/api/dto.go InsightsResponse). Every list
 * holds at most N entries (default 25, 1..500). The lists may be empty or
 * partial until phase2_ready (the small and medium tiers fill some at load), and
 * stay empty when phase2_skipped (huge tier).
 */
export interface InsightsResponse {
  phase2_ready: boolean;
  phase2_skipped: boolean;
  phase2_error?: string;
  cluster_density: number;
  bottlenecks: InsightEntry[];
  keystones: InsightEntry[];
  influencers: InsightEntry[];
  hubs: InsightEntry[];
  authorities: InsightEntry[];
  cores: InsightEntry[];
  slack: InsightEntry[];
  articulation: string[];
  orphans: string[];
  /** Total orphans before the cap. */
  orphan_count: number;
  cycles: string[][];
  /** Total cycles before the cap. */
  cycle_count: number;
  /**
   * List name -> reason its metric was skipped or timed out (e.g. keystones and
   * slack when the graph has a cycle, bottlenecks on a big graph), so an empty
   * list can be told from "none found". Filled once phase2_ready. Project
   * velocity is in the triage response (project_health.velocity).
   */
  skipped: Record<string, string>;
  /**
   * Id -> title for every id in articulation, orphans and cycles (after the
   * cap). Ids not in the snapshot are left out. Always an object.
   */
  titles: Record<string, string>;
}

/** The plan's headline: the actionable issue that unblocks the most. */
export interface PlanSummary {
  /** Issue id; set even when unblocks_count is 0 (then it is an arbitrary pick). */
  highest_impact: string;
  impact_reason: string;
  unblocks_count: number;
}

/** One actionable issue in a plan track (pkg/api/dto.go PlanItem). */
export interface PlanItem {
  id: string;
  title: string;
  issue_type: string;
  status: string;
  priority: number;
  assignee?: string;
  /** Issues that become actionable when this one is done. */
  unblocks_ids: string[];
  /** Absent until triage exists (large tier before phase 2). */
  triage_score?: number;
}

/** A group of related actionable items; tracks come in the server's order. */
export interface PlanTrack {
  track_id: string;
  reason: string;
  items: PlanItem[];
}

/**
 * GET /api/v1/plan (pkg/api/dto.go PlanResponse): the TUI's execution plan.
 * Served at phase 1 and never 503. total_in_progress overlaps total_actionable
 * and total_blocked, so the four totals must not be added up.
 */
export interface PlanResponse {
  tracks: PlanTrack[];
  total_actionable: number;
  total_blocked: number;
  total_deferred_or_parked: number;
  total_in_progress: number;
  summary: PlanSummary;
}

/** Body of POST /api/v1/issues/{id}/signoff (pkg/api/dto.go SignoffRequest). */
export interface SignoffRequest {
  /** Required. */
  approve: boolean;
  /** Required for a dispute (what is wrong or missing); not allowed on an approve. At most 10000 bytes. */
  note?: string;
}

/** Body of POST /api/v1/issues/{id}/answer (pkg/api/dto.go AnswerRequest). At least one of the two. */
export interface AnswerRequest {
  /** The key of one of the decision's options (a, b, ...). */
  option?: string;
  /** Free text, at most 10000 bytes; sent with an option it is appended to it. */
  text?: string;
}

/** Body of PUT /api/v1/issues/{id}/priority (pkg/api/dto.go PriorityRequest). */
export interface PriorityRequest {
  /** 0 (P0) to 4 (P4). */
  priority: number;
}

/**
 * Body of PATCH /api/v1/issues/{id} (pkg/api/dto.go IssuePatchRequest): the
 * fields of the TUI's O editor. An absent field stays as it is; at least one is
 * required. The response is a WriteResponse.
 */
export interface IssuePatchRequest {
  /** Trimmed server-side; not blank, one line, at most 500 bytes (as are status, type and assignee). */
  title?: string;
  /** Free text the tracker validates (a bd status such as "open" or "in_progress"); not blank, one line. */
  status?: string;
  /** 0 (P0) to 4 (P4). */
  priority?: number;
  /** One line; "" clears the assignee. */
  assignee?: string;
  /** Free text the tracker validates (a bd type such as "bug" or "task"); not blank, one line. */
  type?: string;
  /**
   * Leading newlines and trailing whitespace are dropped server-side, as in the
   * O editor. At most 16000 bytes after that normalising.
   */
  description?: string;
  /**
   * Required: the issue's updated_at as the client last read it, echoed
   * verbatim (RFC 3339 with nanoseconds; never round-trip it through a JS Date,
   * which truncates to milliseconds). A mismatch is a 409 "conflict".
   */
  base_updated_at: string;
}

/**
 * 200 body of every write: the five quick actions, a no-op included
 * (pkg/api/dto.go WriteResponse). The data is already reloaded when it arrives,
 * so refetch what you show; there is no issue DTO.
 */
export interface WriteResponse {
  id: string;
  /** Store generation after the reload (meta.generation). */
  generation: number;
  /** Data hash after the reload (meta.data_hash). */
  data_hash: string;
  /** The follow-up bead a dispute filed. */
  created_id?: string;
  /** The write took effect, but refreshing the export or the reload failed: the list may lag. */
  warning?: string;
}
