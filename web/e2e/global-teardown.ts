import { existsSync } from "node:fs";
import { cleanup, readState } from "./lib/state";

/** Stops both bv instances (and the bd children of bv) and removes the temp root. */
export default function globalTeardown(): void {
  const path = process.env["BV_E2E_STATE"];
  if (!path || !existsSync(path)) return; // skipped, or setup failed and cleaned up itself
  const state = readState();
  cleanup(state.root, [state.rw.pid, state.ro.pid]);
}
