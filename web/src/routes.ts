import type { RouteDef } from "./lib/router/router.svelte";
import NotFound from "./views/NotFound.svelte";
import Overview from "./views/Overview.svelte";
import Placeholder from "./views/Placeholder.svelte";

// First match wins. Real views replace the Placeholder entries in later beads.
export const routes: RouteDef[] = [
  { pattern: "/", title: "Overview", component: Overview },
  { pattern: "/issues", title: "Issues", component: Placeholder, props: { title: "Issues" } },
  { pattern: "/issues/:id", title: "Issue", component: Placeholder, props: { title: "Issue" } },
  { pattern: "/triage", title: "Triage", component: Placeholder, props: { title: "Triage" } },
  { pattern: "/insights", title: "Insights", component: Placeholder, props: { title: "Insights" } },
  { pattern: "*", title: "Not found", component: NotFound },
];