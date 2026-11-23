import { isDefaultFilters, type ListFilters, type StateFilter } from "./filter";

export type FilterKey = "state" | "priority" | "type" | "label" | "assignee";

export const FILTER_KEYS: readonly FilterKey[] = ["state", "priority", "type", "label", "assignee"];

export const FILTER_NAMES: Record<FilterKey, string> = {
  state: "State",
  priority: "Priority",
  type: "Type",
  label: "Label",
  assignee: "Assignee",
};

export const STATE_LABELS: Record<StateFilter, string> = {
  active: "Active",
  open: "Open",
  ready: "Ready",
  closed: "Closed",
  all: "All",
};

export const UNASSIGNED = "Unassigned";

/** What one chip shows: its name, its value (empty = default) and whether it is non-default. */
export function chipText(key: FilterKey, f: ListFilters): { name: string; value: string; on: boolean } {
  const name = FILTER_NAMES[key];
  switch (key) {
    case "state":
      return { name, value: STATE_LABELS[f.state], on: f.state !== "active" };
    case "priority": {
      const v = [...f.priority].sort((a, b) => a - b).map((p) => `P${p}`);
      return { name, value: v.join(", "), on: v.length > 0 };
    }
    case "type":
      return { name, value: f.type.join(", "), on: f.type.length > 0 };
    case "label": {
      const [first, ...rest] = f.label;
      return { name, value: first === undefined ? "" : rest.length > 0 ? `${first} +${rest.length}` : first, on: f.label.length > 0 };
    }
    case "assignee":
      return { name, value: f.assignee.map((a) => (a === "" ? UNASSIGNED : a)).join(", "), on: f.assignee.length > 0 };
  }
}

export function showClearFilters(f: ListFilters): boolean {
  return !isDefaultFilters(f);
}

export function toggleValue<T>(values: readonly T[], v: T): T[] {
  return values.includes(v) ? values.filter((x) => x !== v) : [...values, v];
}

export interface FacetOption {
  value: string;
  label: string;
  count: number;
  selected: boolean;
}

/**
 * The checkbox list for Type, Label or Assignee: every facet value with its
 * count plus any selected value the data no longer has (a stale URL), filtered
 * by the popover's search box, most common first ("Unassigned" always leads).
 */
export function facetOptions(counts: ReadonlyMap<string, number>, selected: readonly string[], search: string): FacetOption[] {
  const labelOf = (v: string) => (v === "" ? UNASSIGNED : v);
  const values = new Set<string>([...counts.keys(), ...selected]);
  const q = search.trim().toLowerCase();
  const out: FacetOption[] = [];
  for (const value of values) {
    const label = labelOf(value);
    if (q !== "" && !label.toLowerCase().includes(q)) continue;
    out.push({ value, label, count: counts.get(value) ?? 0, selected: selected.includes(value) });
  }
  return out.sort(
    (a, b) => Number(b.value === "") - Number(a.value === "") || b.count - a.count || (a.label < b.label ? -1 : a.label > b.label ? 1 : 0),
  );
}
