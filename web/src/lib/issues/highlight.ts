export interface Segment {
  t: string;
  /** True for the part that matched the query. */
  m: boolean;
}

const escapeRe = (s: string): string => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

/**
 * Splits `text` around the case-insensitive occurrences of the whole trimmed
 * query. Render the segments with {#each} and <mark>; never as HTML.
 */
export function highlight(text: string, q: string): Segment[] {
  const query = q.trim();
  if (query === "" || text === "") return [{ t: text, m: false }];
  const re = new RegExp(escapeRe(query), "giu");
  const out: Segment[] = [];
  let pos = 0;
  for (const m of text.matchAll(re)) {
    const at = m.index;
    if (at > pos) out.push({ t: text.slice(pos, at), m: false });
    out.push({ t: m[0], m: true });
    pos = at + m[0].length;
  }
  if (pos < text.length) out.push({ t: text.slice(pos), m: false });
  return out.length > 0 ? out : [{ t: text, m: false }];
}
