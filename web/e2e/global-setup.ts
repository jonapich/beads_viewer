import { execFileSync } from "node:child_process";
import { mkdirSync, mkdtempSync, realpathSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, relative, isAbsolute } from "node:path";
import { childEnv } from "./lib/env";
import { startServe } from "./lib/serve";
import { cleanup, type State } from "./lib/state";
import { browserChannel, browserOk, findBd, hasGit, hasGo, repoDir } from "./lib/tools";
import { buildFixture, copyReadOnly } from "./lib/tracker";

function skip(reason: string): void {
  process.env["BV_E2E_SKIP"] = reason;
  console.log(`bv e2e: skipping every test: ${reason}`);
}

/**
 * Resolves the tools (skipping every test when one is missing), builds bv once, creates the
 * fixture tracker in a temp dir and starts two bv serve instances: the writable tracker and a
 * read-only copy. Env vars set here reach the workers.
 */
export default async function globalSetup(): Promise<void> {
  const bd = findBd();
  if (!bd) return skip("bd not found (set BV_E2E_BD, build ../beads, or put bd on PATH)");
  if (!hasGo() && !process.env["BV_E2E_BV"]) return skip("go not found");
  if (!hasGit()) return skip("git not found");
  if (!(await browserOk())) return skip(`no ${browserChannel()} browser (set BV_E2E_CHANNEL)`);

  const root = realpathSync(mkdtempSync(join(tmpdir(), "bv-gui-e2e-")));
  const rel = relative(repoDir, root);
  if (rel !== "" && !rel.startsWith("..") && !isAbsolute(rel)) throw new Error(`the e2e root ${root} is inside the repo`);

  const pids: number[] = [];
  // Last resort for a runner that dies without its teardown (Ctrl+C): never leave a bv behind.
  process.on("exit", () => {
    if (process.env["BV_E2E_DONE"] !== "1") cleanup(undefined, pids);
  });
  try {
    const bv = process.env["BV_E2E_BV"] ?? join(root, "bv.exe");
    if (!process.env["BV_E2E_BV"]) {
      // A build error is a failure, not a skip: the suite is what proves bv builds with the web bundle.
      execFileSync("go", ["build", "-o", bv, "./cmd/bv"], { cwd: repoDir, env: childEnv(bd), stdio: "inherit", windowsHide: true });
    }

    const rwDir = join(root, "rw");
    const roDir = join(root, "ro");
    const ids = buildFixture(bd, rwDir);
    copyReadOnly(rwDir, roDir);

    const env = childEnv(bd);
    const rw = await startServe(bv, rwDir, env, join(root, "bv-rw.log"));
    pids.push(rw.pid);
    const ro = await startServe(bv, roDir, env, join(root, "bv-ro.log"));
    pids.push(ro.pid);

    const state: State = {
      root,
      bd,
      rw: { dir: rwDir, jsonl: join(rwDir, ".beads", "issues.jsonl"), ...rw },
      ro: { dir: roDir, jsonl: join(roDir, ".beads", "issues.jsonl"), ...ro },
      ids,
    };
    mkdirSync(root, { recursive: true });
    const statePath = join(root, "state.json");
    writeFileSync(statePath, JSON.stringify(state, null, 2));
    process.env["BV_E2E_STATE"] = statePath;
  } catch (e) {
    cleanup(root, pids);
    throw e;
  }
}
