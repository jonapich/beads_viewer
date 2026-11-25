import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { FileStat } from "./api/types";
import { STAT_BATCH, UNAVAILABLE, fileStats, noteGeneration, refreshFileStats, requestStat, resetFileStats, setStatFetcher } from "./files.svelte";

const ok = (path: string): FileStat => ({ path, status: "ok", abs: `/root/${path}`, kind: "image", url: `/api/v1/file?path=${path}` });
const tick = () => new Promise<void>((r) => setTimeout(r, 0));

let fetcher: ReturnType<typeof vi.fn<(paths: string[]) => Promise<FileStat[]>>>;

beforeEach(() => {
  resetFileStats();
  fetcher = vi.fn(async (paths: string[]) => paths.map(ok));
  setStatFetcher(fetcher);
});

afterEach(() => {
  setStatFetcher();
  resetFileStats();
});

describe("file stat lookups", () => {
  it("batches the paths asked for in one tick into one call, in order", async () => {
    requestStat("a.png");
    requestStat("b/c.png");
    requestStat("a.png"); // asked twice: looked up once
    await tick();
    expect(fetcher).toHaveBeenCalledTimes(1);
    expect(fetcher.mock.calls[0]?.[0]).toEqual(["a.png", "b/c.png"]);
    expect(fileStats.get("a.png")?.status).toBe("ok");
    expect(fileStats.get("b/c.png")?.abs).toBe("/root/b/c.png");
  });

  it("remembers answers and does not ask again", async () => {
    requestStat("a.png");
    await tick();
    requestStat("a.png");
    await tick();
    expect(fetcher).toHaveBeenCalledTimes(1);
  });

  it("does not ask for a path already in flight", async () => {
    let release: (s: FileStat[]) => void = () => {};
    fetcher.mockImplementationOnce(() => new Promise<FileStat[]>((r) => (release = r)));
    requestStat("a.png");
    await tick();
    requestStat("a.png"); // the first call has not answered yet
    await tick();
    expect(fetcher).toHaveBeenCalledTimes(1);
    release([ok("a.png")]);
    await tick();
    expect(fileStats.get("a.png")?.status).toBe("ok");
  });

  it("splits more than 64 paths into calls of at most 64", async () => {
    for (let i = 0; i < STAT_BATCH + 6; i++) requestStat(`p${i}.png`);
    await tick();
    expect(fetcher).toHaveBeenCalledTimes(2);
    expect(fetcher.mock.calls.map((c) => c[0].length).sort((a, b) => b - a)).toEqual([STAT_BATCH, 6]);
    expect(fileStats.size).toBe(STAT_BATCH + 6);
  });

  it("marks a path unavailable when the call fails, and retries it on refresh", async () => {
    fetcher.mockRejectedValueOnce(new Error("offline"));
    requestStat("a.png");
    await tick();
    expect(fileStats.get("a.png")?.status).toBe(UNAVAILABLE);
    await refreshFileStats();
    expect(fileStats.get("a.png")?.status).toBe("ok");
  });

  it("marks a path unavailable when the answer is short", async () => {
    fetcher.mockResolvedValueOnce([ok("a.png")]);
    requestStat("a.png");
    requestStat("b.png");
    await tick();
    expect(fileStats.get("b.png")?.status).toBe(UNAVAILABLE);
  });

  it("refreshes every known path in place, without clearing first", async () => {
    requestStat("a.png");
    await tick();
    let during: string | undefined;
    fetcher.mockImplementationOnce(async (paths) => {
      during = fileStats.get("a.png")?.status; // still the old answer while the new one is on its way
      return paths.map((p) => ({ path: p, status: "missing" }));
    });
    await refreshFileStats();
    expect(during).toBe("ok");
    expect(fileStats.get("a.png")?.status).toBe("missing");
  });

  it("notes a generation change, not the first generation", () => {
    expect(noteGeneration(3)).toBe(false); // the baseline
    expect(noteGeneration(3)).toBe(false);
    expect(noteGeneration(4)).toBe(true);
    expect(noteGeneration(4)).toBe(false);
  });
});