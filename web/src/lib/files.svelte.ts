import { SvelteMap } from "svelte/reactivity";
import { getFileStats } from "./api/client";
import type { FileStat } from "./api/types";
import { live, onLive } from "./live.svelte";

// Local file paths in bead text (beads-c2o.64). The client never builds file system paths: it
// hands the text as written to GET /api/v1/file/stat, which resolves it against the project
// directory and the allowed roots, and shows what came back. Lookups from every component on the
// page are batched into one request per tick (64 paths at most each) and remembered.

/** A client-side status: the stat call itself failed, so nothing is known about the path. */
export const UNAVAILABLE = "unavailable" as const;

/** The server accepts this many paths per call (pkg/api maxStatPaths). */
export const STAT_BATCH = 64;

export type StatFetcher = (paths: string[]) => Promise<FileStat[]>;

const defaultFetcher: StatFetcher = async (paths) => (await getFileStats(paths)).files;
let fetcher: StatFetcher = defaultFetcher;

/** Replaces the stat call (tests); no argument restores it. */
export function setStatFetcher(f: StatFetcher = defaultFetcher): void {
  fetcher = f;
}

/** What is known about each path, by the text as written. Reactive: a component reads it and updates when its path resolves. */
export const fileStats = new SvelteMap<string, FileStat>();

const queued = new Set<string>();
const inflight = new Set<string>();
let scheduled = false;

/** Asks for a path to be resolved, once: known, queued and in-flight paths are skipped. */
export function requestStat(raw: string): void {
  if (fileStats.has(raw) || queued.has(raw) || inflight.has(raw)) return;
  queued.add(raw);
  if (!scheduled) {
    scheduled = true;
    queueMicrotask(() => {
      scheduled = false;
      const paths = [...queued];
      queued.clear();
      void runBatches(paths);
    });
  }
}

function chunks<T>(list: readonly T[], size: number): T[][] {
  const out: T[][] = [];
  for (let i = 0; i < list.length; i += size) out.push(list.slice(i, i + size));
  return out;
}

async function runBatches(paths: string[]): Promise<void> {
  await Promise.all(chunks(paths, STAT_BATCH).map((chunk) => lookup(chunk)));
}

async function lookup(chunk: string[]): Promise<void> {
  for (const p of chunk) inflight.add(p);
  try {
    const stats = await fetcher(chunk);
    chunk.forEach((p, i) => fileStats.set(p, stats[i] ?? { path: p, status: UNAVAILABLE }));
  } catch {
    for (const p of chunk) fileStats.set(p, { path: p, status: UNAVAILABLE });
  } finally {
    for (const p of chunk) inflight.delete(p);
  }
}

/**
 * Looks every known path up again and replaces the answers in place (nothing is cleared first, so
 * a thumbnail does not blink out). A capture saved after the page loaded shows up, and a path that
 * could not be checked is retried.
 */
export async function refreshFileStats(): Promise<void> {
  const known = [...fileStats.keys()];
  await runBatches(known.filter((p) => !inflight.has(p)));
}

/** Forgets everything (tests). */
export function resetFileStats(): void {
  fileStats.clear();
  queued.clear();
  inflight.clear();
  scheduled = false;
  lastGeneration = null;
}

let lastGeneration: number | null = null;

/**
 * Notes the store generation of a live snapshot event; true when it moved on from the last one
 * seen (the first one only sets the baseline). A reload is when bead text, and the files it
 * names, can have changed.
 */
export function noteGeneration(generation: number): boolean {
  const moved = lastGeneration !== null && generation !== lastGeneration;
  lastGeneration = generation;
  return moved;
}

/** Starts following the live stream: paths are looked up again when the generation changes. Returns the cleanup. Call from App's onMount. */
export function startFileStats(): () => void {
  return onLive(() => {
    if (live.last && noteGeneration(live.last.generation)) void refreshFileStats();
  });
}
