import { readFileSync, rmSync } from "node:fs";
import type { Manifest } from "./tracker";
import { killTree } from "./serve";

/** One bv serve instance as the tests see it. */
export interface Instance {
  /** The tracker directory bv runs in. */
  dir: string;
  /** The JSONL bv watches. */
  jsonl: string;
  base: string;
  signIn: string;
  pid: number;
}

/** Written by global-setup to <root>/state.json; its path is in BV_E2E_STATE. */
export interface State {
  root: string;
  bd: string;
  rw: Instance;
  ro: Instance;
  ids: Manifest;
}

export function readState(): State {
  const path = process.env["BV_E2E_STATE"];
  if (!path) throw new Error("BV_E2E_STATE is not set: global setup did not run");
  return JSON.parse(readFileSync(path, "utf8")) as State;
}

/** Stops the servers and removes the temp root (kept, and printed, when BV_E2E_KEEP=1). Never throws. */
export function cleanup(root: string | undefined, pids: number[]): void {
  for (const pid of pids) killTree(pid);
  // The exit hook of global-setup must not kill these pids a second time: a pid can be reused.
  process.env["BV_E2E_DONE"] = "1";
  if (!root) return;
  if (process.env["BV_E2E_KEEP"] === "1") {
    console.log(`bv e2e: kept ${root}`);
    return;
  }
  // Windows keeps a killed process's files locked for a moment: retry for up to ~15 s.
  const wait = new Int32Array(new SharedArrayBuffer(4));
  let last: unknown;
  for (let attempt = 0; attempt < 30; attempt++) {
    try {
      rmSync(root, { recursive: true, force: true });
      return;
    } catch (e) {
      last = e;
      Atomics.wait(wait, 0, 0, 500);
    }
  }
  console.warn(`bv e2e: could not remove ${root}: ${String(last)}`);
}
