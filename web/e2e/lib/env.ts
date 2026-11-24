import { delimiter, dirname } from "node:path";

/** What must never reach a child bd or bv: it would point the fixture at a real tracker (mirrors pkg/loader). */
const STRIPPED = ["BEADS_DIR", "BEADS_DB", "BD_DB", "BEADS_JSONL"];

/**
 * The environment of every child process: the current one without the tracker-selecting
 * variables, with bd's directory first on PATH (bv finds bd only through PATH; without it
 * the served tracker is silently read-only), and no browser opening.
 */
export function childEnv(bdExe: string): NodeJS.ProcessEnv {
  const env: NodeJS.ProcessEnv = {};
  for (const [k, v] of Object.entries(process.env)) {
    if (STRIPPED.includes(k.toUpperCase())) continue;
    env[k] = v;
  }
  // A plain copy of process.env is case-sensitive, Windows is not: reuse the existing PATH key.
  const pathKey = Object.keys(env).find((k) => k.toUpperCase() === "PATH") ?? "PATH";
  env[pathKey] = dirname(bdExe) + delimiter + (env[pathKey] ?? "");
  env["BV_NO_BROWSER"] = "1";
  return env;
}
