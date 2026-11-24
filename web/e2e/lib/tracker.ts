import { execFileSync } from "node:child_process";
import { copyFileSync, mkdirSync } from "node:fs";
import { join } from "node:path";
import { childEnv } from "./env";

/** The roles of the fixture tracker, each mapped to the id bd gave it. A test that mutates owns one. */
export interface Manifest {
  /** "Checkout redesign": epic with three children. */
  epic: string;
  /** "Cart model": child of the epic; blocks payform. */
  cart: string;
  /** "Payment form": feature P1, assignee Alice, two comments, blocked by cart. */
  payform: string;
  /** "Order summary": child of the epic. */
  summary: string;
  /** "Tax rounding off by a cent": standalone bug P2; the priority target. */
  bug: string;
  /** "Old closed task": closed; hidden by the default state filter. */
  oldtask: string;
  /** "Ship the cart": closed and awaiting sign-off; the approve target. */
  signoff: string;
  /** "Pick a payment provider": needs-decision with options A to C; the answer target. */
  decision: string;
  /** "Edit target": the inline-edit target. */
  edit: string;
  /** "Conflict target": the 409 target. */
  conflict: string;
  /** "Live target": changed from outside while the page is open. */
  live: string;
}

export const TITLES: Record<keyof Manifest, string> = {
  epic: "Checkout redesign",
  cart: "Cart model",
  payform: "Payment form",
  summary: "Order summary",
  bug: "Tax rounding off by a cent",
  oldtask: "Old closed task",
  signoff: "Ship the cart",
  decision: "Pick a payment provider",
  edit: "Edit target",
  conflict: "Conflict target",
  live: "Live target",
};

/** Runs bd in `dir` and returns stdout. Throws (with bd's output in the message) on a non-zero exit. */
export function bd(bdExe: string, dir: string, ...args: string[]): string {
  try {
    return execFileSync(bdExe, args, {
      cwd: dir,
      env: childEnv(bdExe),
      encoding: "utf8",
      windowsHide: true,
      timeout: 120_000,
      stdio: ["ignore", "pipe", "pipe"],
    });
  } catch (e) {
    const err = e as { stdout?: string; stderr?: string; message: string };
    throw new Error(`bd ${args.join(" ")} failed in ${dir}: ${err.message}\n${err.stdout ?? ""}\n${err.stderr ?? ""}`);
  }
}

/** bd's JSON output, skipping any log line before the first brace or bracket. */
export function parseJson<T>(out: string): T {
  const start = out.search(/[[{]/);
  if (start < 0) throw new Error(`no JSON in bd output: ${out}`);
  return JSON.parse(out.slice(start)) as T;
}

export function bdJson<T>(bdExe: string, dir: string, ...args: string[]): T {
  return parseJson<T>(bd(bdExe, dir, ...args));
}

export interface BdIssue {
  id: string;
  title: string;
  status: string;
  priority: number;
  labels?: string[];
}

/** `bd show <id> --json` (an array of one). */
export function bdShow(bdExe: string, dir: string, id: string): BdIssue {
  const list = bdJson<BdIssue[]>(bdExe, dir, "show", id, "--json");
  const issue = list[0];
  if (!issue) throw new Error(`bd show ${id} returned nothing`);
  return issue;
}

/** Writes the tracker's JSONL (the file bv watches). bd's own auto-export is off in the fixture. */
export function exportJsonl(bdExe: string, dir: string): string {
  const out = join(dir, ".beads", "issues.jsonl");
  bd(bdExe, dir, "export", "-o", out);
  return out;
}

const SIGNOFF_COMMENT = JSON.stringify({
  what_changed: ["Cart now persists across sessions."],
  calls_made: [{ call: "Keep carts 30 days", alternative: "7 days", why: "Matches the marketing emails" }],
  removed_or_renamed: ["Nothing."],
  where_to_see: ["The cart page"],
});

const DECISION_DESCRIPTION = [
  "We must choose a provider before the payment form can ship.",
  "",
  "Question: Which provider should we integrate first?",
  "",
  "A (recommended): Stripe: best docs, saved cards",
  "B: Adyen: lower fees at scale",
  "C: Braintree: already in the legacy stack",
].join("\n");

/**
 * Creates the fixture tracker in `dir` (a git repo, with an embedded-Dolt bd database) and exports
 * it. Returns the role -> id manifest. ~25 bd calls.
 */
export function buildFixture(bdExe: string, dir: string): Manifest {
  mkdirSync(dir, { recursive: true });
  execFileSync("git", ["init", "-q"], { cwd: dir, env: childEnv(bdExe), windowsHide: true, stdio: "ignore" });
  bd(bdExe, dir, "init", "--non-interactive", "--skip-hooks", "--prefix", "e2e");
  // bv watches only the JSONL file; the tests export it explicitly, never wait for bd's throttled auto-export.
  bd(bdExe, dir, "config", "set", "export.auto", "false");

  const create = (title: string, ...args: string[]): string => bdJson<{ id: string }>(bdExe, dir, "create", "--json", title, ...args).id;

  const epic = create(TITLES.epic, "--type", "epic", "--priority", "1", "--description", "Redesign the checkout so a returning customer pays in two clicks.");
  const cart = create(TITLES.cart, "--type", "task", "--parent", epic);
  const payform = create(TITLES.payform, "--type", "feature", "--priority", "1", "--parent", epic, "--assignee", "Alice");
  const summary = create(TITLES.summary, "--type", "task", "--parent", epic);
  const bug = create(TITLES.bug, "--type", "bug", "--priority", "2");
  const oldtask = create(TITLES.oldtask, "--type", "task");
  const signoff = create(TITLES.signoff, "--type", "task", "--label", "needs-signoff");
  const decision = create(TITLES.decision, "--type", "task", "--label", "needs-decision", "--description", DECISION_DESCRIPTION);
  const edit = create(TITLES.edit, "--type", "task");
  const conflict = create(TITLES.conflict, "--type", "task");
  const live = create(TITLES.live, "--type", "task");

  bd(bdExe, dir, "dep", "add", payform, cart);
  bd(bdExe, dir, "comments", "add", payform, "--kind", "brief", "Brief: the payment form must support saved cards.");
  bd(bdExe, dir, "comments", "add", payform, "Plain comment: pushed a fixup.");
  bd(bdExe, dir, "comments", "add", signoff, "--kind", "signoff", SIGNOFF_COMMENT);
  bd(bdExe, dir, "close", signoff, "--reason", "Shipped.");
  bd(bdExe, dir, "close", oldtask, "--reason", "Done.");

  exportJsonl(bdExe, dir);
  return { epic, cart, payform, summary, bug, oldtask, signoff, decision, edit, conflict, live };
}

/**
 * Copies only the exported JSONL to `roDir/.beads/issues.jsonl`. With no database next to it the
 * origin has no complete write route, so bv serves it read-only. Call before any test mutates.
 */
export function copyReadOnly(rwDir: string, roDir: string): void {
  mkdirSync(join(roDir, ".beads"), { recursive: true });
  copyFileSync(join(rwDir, ".beads", "issues.jsonl"), join(roDir, ".beads", "issues.jsonl"));
}
