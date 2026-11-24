import type { MetaResponse } from "../api/types";

/** Local HH:MM:SS of an RFC 3339 instant; "" when it does not parse. */
export function clockTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  const p = (n: number) => String(n).padStart(2, "0");
  return `${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
}

/** The banner text for a failed reload, or null while the last reload succeeded. */
export function reloadNotice(meta: Pick<MetaResponse, "reload_error" | "loaded_at"> | null): string | null {
  if (!meta?.reload_error) return null;
  const at = clockTime(meta.loaded_at);
  return `Couldn't reload the tracker: ${meta.reload_error}. Showing the data loaded${at ? ` at ${at}` : ""}.`;
}
