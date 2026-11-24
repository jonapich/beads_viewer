/** Presentation helpers shared by the Triage, Plan and Insights views. */

/** Whole percent of `max` for a value bar (the .pct-N classes). Unlike issues/present percent() it reaches 100. */
export function barPct(v: number, max: number): number {
  if (!(max > 0) || !Number.isFinite(v)) return 0;
  return Math.min(100, Math.max(0, Math.round((v / max) * 100)));
}

export type MetricFormat = "int" | "dec";

/** An insight value: whole numbers, or two decimals (three below 0.01, "<0.001" under that). */
export function formatMetric(v: number, format: MetricFormat): string {
  if (format === "int") return String(Math.round(v));
  if (v === 0) return "0";
  if (v < 0.001) return "<0.001";
  return v < 0.01 ? v.toFixed(3) : v.toFixed(2);
}

export type Tone = "info" | "warning" | "danger";

/** The tone an alert severity is drawn in. */
export function severityTone(severity: string): Tone {
  if (severity === "critical" || severity === "error") return "danger";
  if (severity === "warning") return "warning";
  return "info";
}

/** "1 issue", "3 issues". */
export function plural(n: number, one: string, many: string): string {
  return `${n} ${n === 1 ? one : many}`;
}
