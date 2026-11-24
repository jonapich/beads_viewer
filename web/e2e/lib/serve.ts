import { spawn, spawnSync } from "node:child_process";
import { createWriteStream } from "node:fs";

export interface Served {
  pid: number;
  /** http://127.0.0.1:<port> */
  base: string;
  /** The sign-in URL bv printed (its token is reusable). */
  signIn: string;
}

// The trailing newline matters: without it a chunk that ends mid-token would match.
const SIGN_IN = /^(http:\/\/127\.0\.0\.1:\d+)\/auth\?token=\S+\r?\n/m;
const START_TIMEOUT_MS = 60_000;

/**
 * Starts `bv --serve` in `cwd` on a kernel-chosen port and resolves once it printed the sign-in URL
 * (bv loads the tracker before binding, so that line means it is ready). Output goes to `logPath`.
 */
export function startServe(bvExe: string, cwd: string, env: NodeJS.ProcessEnv, logPath: string): Promise<Served> {
  return new Promise((resolve, reject) => {
    const log = createWriteStream(logPath);
    const child = spawn(bvExe, ["--serve", "--serve-port", "0", "--serve-no-open"], {
      cwd,
      env,
      windowsHide: true,
      stdio: ["ignore", "pipe", "pipe"],
    });
    let out = "";
    let settled = false;
    const done = (fn: () => void): void => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      fn();
    };
    const timer = setTimeout(() => {
      done(() => {
        if (child.pid !== undefined) killTree(child.pid);
        reject(new Error(`bv --serve did not print its sign-in URL within ${START_TIMEOUT_MS / 1000}s; see ${logPath}\n${out}`));
      });
    }, START_TIMEOUT_MS);

    child.stdout.on("data", (chunk: Buffer) => {
      log.write(chunk);
      out += chunk.toString("utf8");
      const m = SIGN_IN.exec(out);
      if (m && child.pid !== undefined) {
        const [signIn, base] = [m[0].trim(), m[1]];
        if (base) done(() => resolve({ pid: child.pid as number, base, signIn }));
      }
    });
    child.stderr.on("data", (chunk: Buffer) => log.write(chunk));
    child.on("error", (e) => done(() => reject(e)));
    child.on("exit", (code, signal) => {
      log.end();
      done(() => reject(new Error(`bv --serve exited early (code ${code}, signal ${signal}); see ${logPath}\n${out}`)));
    });
    // Do not keep the runner alive on a stray child; teardown kills it explicitly.
    child.unref();
  });
}

/** Kills a process and its children. bv runs bd as a child, which child.kill() would not reach on Windows. */
export function killTree(pid: number): void {
  if (process.platform === "win32") {
    spawnSync("taskkill", ["/pid", String(pid), "/T", "/F"], { stdio: "ignore", windowsHide: true });
    return;
  }
  try {
    process.kill(pid, "SIGKILL");
  } catch {
    // already gone
  }
}
