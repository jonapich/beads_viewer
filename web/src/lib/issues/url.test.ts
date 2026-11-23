import { describe, expect, it } from "vitest";
import { defaultFilters } from "./filter";
import { fromQuery, onlyQueryDiffers, toQuery, type UrlState } from "./url";

const base = (): UrlState => ({ query: "", filters: defaultFilters(), sortPick: null, modePick: null, peek: null });

describe("toQuery", () => {
  it("omits defaults", () => {
    expect(toQuery(base()).toString()).toBe("");
    expect(toQuery({ ...base(), query: "   " }).toString()).toBe("");
  });

  it("writes repeated params, with an empty assignee for unassigned", () => {
    const p = toQuery({
      query: "sign off",
      filters: { state: "ready", priority: [1, 0], type: ["bug"], label: ["a,b", "c"], assignee: ["alice", ""] },
      sortPick: "triage",
      modePick: "flat",
      peek: "bv-1",
    });
    expect(p.getAll("p")).toEqual(["0", "1"]);
    expect(p.getAll("label")).toEqual(["a,b", "c"]);
    expect(p.getAll("assignee")).toEqual(["alice", ""]);
    expect(p.toString()).toBe(
      "q=sign+off&state=ready&p=0&p=1&type=bug&label=a%2Cb&label=c&assignee=alice&assignee=&sort=triage&mode=flat&peek=bv-1",
    );
  });
});

describe("fromQuery", () => {
  it("round-trips", () => {
    const cases: UrlState[] = [
      base(),
      { ...base(), query: "hello world", peek: "x-1" },
      { ...base(), filters: { state: "closed", priority: [0, 4], type: ["bug", "feature"], label: ["a&b", "x=y"], assignee: ["", "ada"] } },
      { ...base(), sortPick: "relevance", modePick: "tree" },
      { ...base(), filters: { ...defaultFilters(), assignee: [""] } },
    ];
    for (const c of cases) expect(fromQuery(new URLSearchParams(toQuery(c).toString()))).toEqual(c);
  });

  it("drops unknown or invalid values silently", () => {
    const s = fromQuery(
      new URLSearchParams("state=nope&p=9&p=x&p=2&p=2&type=&label=&sort=zzz&mode=sideways&peek=&unknown=1&assignee=&assignee="),
    );
    expect(s).toEqual({
      query: "",
      filters: { state: "active", priority: [2], type: [], label: [], assignee: [""] },
      sortPick: null,
      modePick: null,
      peek: null,
    });
  });

  it("reads state=active as the default", () => {
    expect(fromQuery(new URLSearchParams("state=active")).filters.state).toBe("active");
  });
});

describe("onlyQueryDiffers", () => {
  const p = (s: string) => new URLSearchParams(s);
  it("is true when only q differs", () => {
    expect(onlyQueryDiffers(p("q=a&state=all"), p("q=ab&state=all"))).toBe(true);
    expect(onlyQueryDiffers(p("state=all"), p("q=x&state=all"))).toBe(true);
  });
  it("is false when anything else differs", () => {
    expect(onlyQueryDiffers(p("q=a"), p("q=a&peek=x"))).toBe(false);
    expect(onlyQueryDiffers(p("state=all"), p("state=closed"))).toBe(false);
    expect(onlyQueryDiffers(p("sort=triage&q=a"), p("q=b"))).toBe(false);
  });
});
