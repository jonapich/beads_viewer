import type { RouteDef } from "./lib/router/router.svelte";
import Desk from "./views/Desk.svelte";
import IssueDetail from "./views/IssueDetail.svelte";
import Insights from "./views/Insights.svelte";
import Issues from "./views/Issues.svelte";
import NotFound from "./views/NotFound.svelte";
import Overview from "./views/Overview.svelte";
import Plan from "./views/Plan.svelte";
import Triage from "./views/Triage.svelte";

// First match wins.
export const routes: RouteDef[] = [
  { pattern: "/", title: "Overview", component: Overview },
  { pattern: "/issues", title: "Issues", component: Issues, fill: true },
  { pattern: "/issues/:id", title: "Issue", component: IssueDetail },
  { pattern: "/desk", title: "Agent desk", component: Desk, fill: true },
  { pattern: "/triage", title: "Triage", component: Triage, fill: true },
  { pattern: "/plan", title: "Plan", component: Plan, fill: true },
  { pattern: "/insights", title: "Insights", component: Insights, fill: true },
  { pattern: "*", title: "Not found", component: NotFound },
];