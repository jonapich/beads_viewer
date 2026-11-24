// Copied from pkg/ui/insights.go metricDescriptions; keep in sync.
// The Icon field is left out, and *emphasis* is plain text: lib/markdown.ts parseInline
// knows only **strong** and `code`.
import type { MetricFormat } from "./present";

/** The insight lists the nine cards show (the keys of InsightsResponse, and of its `skipped` map). */
export type MetricKey =
  | "bottlenecks"
  | "keystones"
  | "influencers"
  | "hubs"
  | "authorities"
  | "cores"
  | "articulation"
  | "slack"
  | "cycles";

export interface MetricDef {
  key: MetricKey;
  title: string;
  /** The algorithm, under the title. */
  short: string;
  whatIs: string;
  whyUseful: string;
  howToUse: string;
  formula: string;
  /** How a value reads. Cut points and cycles list ids, not values. */
  format: MetricFormat;
}

/** In the TUI's order (pkg/ui/insights.go renderInsightsPanels). */
export const METRICS: readonly MetricDef[] = [
  {
    key: "bottlenecks",
    title: "Bottlenecks",
    short: "Betweenness Centrality",
    whatIs: "Measures how often a bead lies on **shortest paths** between other beads in the dependency graph.",
    whyUseful: "High-scoring beads are critical junctions. Delays here ripple across the entire project.",
    howToUse: "**Prioritize** these to unblock parallel workstreams. Consider breaking them into smaller pieces.",
    formula: "`BW(v) = Σ (σst(v) / σst)` for all s≠v≠t",
    format: "dec",
  },
  {
    key: "keystones",
    title: "Keystones",
    short: "Impact Depth",
    whatIs: "Measures how **deep** in the dependency chain a bead sits (downstream chain length).",
    whyUseful: "Keystones are foundational. Everything above them depends on their completion.",
    howToUse: "**Complete these first.** Blocking a keystone blocks the entire chain above it.",
    formula: "`Impact(v) = 1 + max(Impact(u))` for all u depending on v",
    format: "int",
  },
  {
    key: "influencers",
    title: "Influencers",
    short: "Eigenvector Centrality",
    whatIs: "Scores beads by their connections to other **well-connected** beads.",
    whyUseful: "Influencers are connected to important beads. Changes here have wide-reaching effects.",
    howToUse: "**Review carefully** before changes. They're central to project structure.",
    formula: "`EV(v) = (1/λ) × Σ A[v,u] × EV(u)`",
    format: "dec",
  },
  {
    key: "hubs",
    title: "Hubs",
    short: "HITS Hub Score",
    whatIs: "Beads that **depend on** many important authorities (aggregators).",
    whyUseful: "Hubs collect dependencies. They often represent high-level features or epics.",
    howToUse: "**Track for milestones.** Their completion signals major project progress.",
    formula: "`Hub(v) = Σ Authority(u)` for all u where v→u",
    format: "dec",
  },
  {
    key: "authorities",
    title: "Authorities",
    short: "HITS Authority Score",
    whatIs: "Beads that are **depended upon** by many important hubs (providers).",
    whyUseful: "Authorities are foundational services/components that many features need.",
    howToUse: "**Stabilize early.** Breaking an authority breaks many dependent hubs.",
    formula: "`Auth(v) = Σ Hub(u)` for all u where u→v",
    format: "dec",
  },
  {
    key: "cores",
    title: "Cores",
    short: "k-core Cohesion",
    whatIs: "Nodes with highest **k-core numbers** (embedded in dense subgraphs).",
    whyUseful: "High-core nodes sit in tightly knit clusters—changes can ripple locally.",
    howToUse: "Use for **resilience checks**; prioritize when breaking apart tightly coupled areas.",
    formula: "Max `k` such that node remains in k-core after peeling",
    format: "int",
  },
  {
    key: "articulation",
    title: "Cut points",
    short: "Articulation Vertices",
    whatIs: "Nodes whose **removal disconnects** the undirected graph.",
    whyUseful: "Single points of failure. Instability here can isolate workstreams.",
    howToUse: "**Harden or split** these nodes; avoid piling more dependencies onto them.",
    formula: "Tarjan articulation detection on undirected view",
    format: "int",
  },
  {
    key: "slack",
    title: "Slack",
    short: "Longest-path slack",
    whatIs: "Distance from **critical chain** (`0` = critical path; higher = parallel-friendly).",
    whyUseful: "Zero-slack tasks are schedule-critical; high-slack can fill gaps without blocking.",
    howToUse: "**Schedule zero-slack early**; slot high-slack tasks when waiting on blockers.",
    formula: "`Slack(v) = max_path_len - dist_start(v) - dist_end(v)`",
    format: "int",
  },
  {
    key: "cycles",
    title: "Cycles",
    short: "Circular Dependencies",
    whatIs: "Groups of beads forming **dependency loops** (A→B→C→A).",
    whyUseful: "Cycles indicate structural problems. They can't be resolved in sequence.",
    howToUse: "**Break cycles** by removing or reversing a dependency. Refactor to decouple.",
    formula: "Detected via Tarjan's SCC algorithm",
    format: "int",
  },
];

/** The metrics whose list is ids only (no value to show). */
export const ID_LISTS: ReadonlySet<MetricKey> = new Set<MetricKey>(["articulation", "cycles"]);

/** A metric's cap on how many rows a card lists before "Show all". */
export const CARD_ROWS = 5;