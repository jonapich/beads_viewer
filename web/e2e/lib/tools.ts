import { execFileSync } from "node:child_process";
import { existsSync, statSync } from "node:fs";
import { delimiter, join, resolve } from "node:path";
import { chromium } from "@playwright/test";

/** beads_viewer/web, and the repo root above it (where `go build ./cmd/bv` runs). */
export const webDir = resolve(import.meta.dirname, "..", "..");
export const repoDir = resolve(webDir, "..");

function runs(exe: string, args: string[]): boolean {
  try {
    execFileSync(exe, args, { stdio: "ignore", timeout: 30_000, windowsHide: true });
    return true;
  } catch {
    return false;
  }
}

function isFile(p: string): boolean {
  try {
    return statSync(p).isFile();
  } catch {
    return false;
  }
}

function onPath(name: string): string | undefined {
  const exts = process.platform === "win32" ? [".exe", ".cmd", ""] : [""];
  for (const dir of (process.env["PATH"] ?? "").split(delimiter)) {
    if (!dir) continue;
    for (const ext of exts) {
      const p = join(dir, name + ext);
      if (isFile(p)) return p;
    }
  }
  return undefined;
}

/**
 * The bd executable: BV_E2E_BD, else the sibling checkout's build (../beads/dist/windows-amd64),
 * else bd on PATH. A candidate must answer `bd version`. Undefined when none does.
 */
export function findBd(): string | undefined {
  const candidates: (string | undefined)[] = [
    process.env["BV_E2E_BD"],
    resolve(repoDir, "..", "beads", "dist", "windows-amd64", "bd.exe"),
    onPath("bd"),
  ];
  for (const c of candidates) {
    if (c && existsSync(c) && runs(c, ["version"])) return resolve(c);
  }
  return undefined;
}

export const hasGo = (): boolean => runs("go", ["version"]);
export const hasGit = (): boolean => runs("git", ["--version"]);

export const browserChannel = (): string => process.env["BV_E2E_CHANNEL"] ?? "msedge";

/** True when the configured browser channel can be launched. */
export async function browserOk(): Promise<boolean> {
  try {
    const b = await chromium.launch({ channel: browserChannel() });
    await b.close();
    return true;
  } catch {
    return false;
  }
}
