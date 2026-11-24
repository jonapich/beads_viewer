import type { RouteDef } from "./lib/router/router.svelte";
import IssueDetail from "./views/IssueDetail.svelte";
import Issues from "./views/Issues.svelte";
import NotFound from "./views/NotFound.svelte";
import Overview from "./views/Overview.svelte";
import Placeholder from "./views/Placeholder.svelte";
import Triage from "./views/Triage.svelte";

// First match wins. Real views replace the Placeholder entries in later beads.
export const routes: RouteDef[] = [
  { pattern: "/", title: "Overview", component: Overview },
  { pattern: "/issues", title: "Issues", component: Issues, fill: true },
  { pattern: "/issues/:id", title: "Issue", component: IssueDetail },
  { pattern: "/triage", title: "Triage", component: Triage, fill: true },
  { pattern: "/insights", title: "Insights", component: Placeholder, props: { title: "Insights" } },
  { pattern: "*", title: "Not found", component: NotFound },
];