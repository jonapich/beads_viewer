// A small Markdown subset for the free-text fields of an issue (description, design,
// acceptance, notes, comment bodies). It parses to an AST that the components render
// with Svelte's own escaping: no HTML string is ever built, so no tracker text can reach
// the DOM as markup and no sanitiser is needed. Links are allow-listed to http(s).
//
// The grammar is the approved mock's md()/inline() (beads-c2o.16), plus [text](url):
//   blocks:  ``` fences, pipe tables with a separator row, "#" headings plus the ALL-CAPS
//            and "Q1." heading heuristics, -/*/•/1./1) lists with indented continuation
//            lines, 2-space-indented code blocks, paragraphs (lines joined by a break)
//   inline:  `code`, **strong**, [text](url), bare http(s) autolinks
// Not supported, on purpose: raw HTML, images, nested emphasis, blockquotes.

export type Inline =
  | { t: "text"; v: string }
  | { t: "code"; v: string }
  | { t: "strong"; v: string }
  | { t: "link"; href: string; v: string }
  | { t: "br" };

export type Block =
  | { t: "p"; c: Inline[] }
  | { t: "h"; c: Inline[] }
  | { t: "ul" | "ol"; items: Inline[][] }
  | { t: "pre"; v: string }
  | { t: "table"; head: Inline[][]; rows: Inline[][][] };

/**
 * The URL as a browser will read it when it is an absolute http: or https: URL, else
 * null (relative, javascript:, data:, malformed). Callers render a null as plain text.
 */
export function safeHref(url: string): string | null {
  let u: URL;
  try {
    u = new URL(url.trim());
  } catch {
    return null;
  }
  return u.protocol === "http:" || u.protocol === "https:" ? u.href : null;
}

// One pass over a text run (code spans are already cut out). Leftmost match wins; at one
// position the alternatives are tried in order: **strong**, [text](url), bare URL.
// A bare URL ends before whitespace, "<" or ")" and never on trailing punctuation.
// The link text and URL are length-bounded, and the text cannot hold "[": a long run of "[" would
// otherwise rescan to the end of the line from every one of them.
const TOKEN = /\*\*([^*\n]+)\*\*|\[([^[\]\n]{1,500})\]\(([^)\s]{1,2000})\)|https?:\/\/[^\s<)]+[^\s<).,;:]/g;

function pushText(out: Inline[], v: string): void {
  if (v !== "") out.push({ t: "text", v });
}

function pushLink(out: Inline[], url: string, text: string, raw: string): void {
  const href = safeHref(url);
  if (href === null) pushText(out, raw);
  else out.push({ t: "link", href, v: text });
}

function parseText(src: string, out: Inline[]): void {
  let last = 0;
  for (const m of src.matchAll(TOKEN)) {
    const at = m.index ?? 0;
    pushText(out, src.slice(last, at));
    last = at + m[0].length;
    if (m[1] !== undefined) out.push({ t: "strong", v: m[1] });
    else if (m[2] !== undefined && m[3] !== undefined) pushLink(out, m[3], m[2], m[0]);
    else pushLink(out, m[0], m[0], m[0]);
  }
  pushText(out, src.slice(last));
}

/** One line of inline Markdown. */
export function parseInline(src: string): Inline[] {
  const out: Inline[] = [];
  const parts = String(src).split("`");
  for (let i = 0; i < parts.length; i++) {
    const part = parts[i] ?? "";
    // An odd part is code only when its closing backtick exists; a lone one stays text.
    if (i % 2 === 1 && i < parts.length - 1) out.push({ t: "code", v: part });
    else parseText(i % 2 === 1 ? "`" + part : part, out);
  }
  return out;
}

const BULLET = /^(\s*)([-*•]|\d+[.)])\s+(.*)$/;
const INDENTED = /^\s{2,}\S/;
const PIPE_ROW = /^\s*\|.*\|\s*$/;
const PIPE_SEP = /^\s*\|[\s:|-]+\|\s*$/;
const FENCE = /^```/;

function isCaps(l: string): boolean {
  // The length test comes first: the pattern backtracks on a long run of spaces.
  return l.length <= 70 && /^[A-Z][A-Z0-9 /&,'-]{3,}(\s*\(.*\))?:?$/.test(l) && l.replace(/[^A-Z]/g, "").length >= 4;
}

function isHeading(l: string): boolean {
  return /^#{1,6}\s/.test(l) || isCaps(l) || /^Q\d+\.\s/.test(l);
}

function isTableStart(l: string, next: string | undefined): boolean {
  return PIPE_ROW.test(l) && PIPE_SEP.test(next ?? "");
}

/** A line that ends a paragraph (when it is not the paragraph's first line). */
function isSpecial(l: string, next: string | undefined): boolean {
  return FENCE.test(l) || BULLET.test(l) || INDENTED.test(l) || isHeading(l) || isTableStart(l, next);
}

function cells(row: string): string[] {
  return row
    .trim()
    .replace(/^\||\|$/g, "")
    .split("|")
    .map((c) => c.trim());
}

/** Markdown text to blocks. Never throws; blank input gives []. */
export function parseMarkdown(src: string): Block[] {
  const L = String(src ?? "")
    .replace(/\r/g, "")
    .split("\n");
  const out: Block[] = [];
  let i = 0;
  while (i < L.length) {
    const l = L[i] ?? "";
    if (/^\s*$/.test(l)) {
      i++;
      continue;
    }

    if (FENCE.test(l)) {
      const body: string[] = [];
      i++;
      while (i < L.length && !FENCE.test(L[i] ?? "")) body.push(L[i++] ?? "");
      i++; // the closing fence (or the end of input)
      out.push({ t: "pre", v: body.join("\n") });
      continue;
    }

    if (isTableStart(l, L[i + 1])) {
      const head = cells(l).map(parseInline);
      i += 2;
      const rows: Inline[][][] = [];
      while (i < L.length && PIPE_ROW.test(L[i] ?? "")) rows.push(cells(L[i++] ?? "").map(parseInline));
      out.push({ t: "table", head, rows });
      continue;
    }

    if (isHeading(l)) {
      out.push({ t: "h", c: parseInline(l.replace(/^#{1,6}\s/, "")) });
      i++;
      continue;
    }

    const bm = BULLET.exec(l);
    if (bm) {
      const ordered = /\d/.test(bm[2] ?? "");
      const items: Inline[][] = [];
      while (i < L.length) {
        const line = L[i] ?? "";
        const m = BULLET.exec(line);
        if (m) {
          items.push(parseInline(m[3] ?? ""));
          i++;
        } else if (INDENTED.test(line) && items.length > 0) {
          items[items.length - 1]?.push({ t: "br" }, ...parseInline(line.trim()));
          i++;
        } else break;
      }
      out.push({ t: ordered ? "ol" : "ul", items });
      continue;
    }

    if (INDENTED.test(l)) {
      const body: string[] = [];
      while (i < L.length) {
        const line = L[i] ?? "";
        const continues = INDENTED.test(line) || (/^\s*$/.test(line) && INDENTED.test(L[i + 1] ?? ""));
        if (!continues) break;
        body.push(line.replace(/^ {2}/, ""));
        i++;
      }
      out.push({ t: "pre", v: body.join("\n") });
      continue;
    }

    const para: Inline[] = [];
    let lines = 0;
    while (i < L.length) {
      const line = L[i] ?? "";
      if (/^\s*$/.test(line) || (lines > 0 && isSpecial(line, L[i + 1]))) break;
      if (lines > 0) para.push({ t: "br" });
      para.push(...parseInline(line));
      lines++;
      i++;
    }
    out.push({ t: "p", c: para });
  }
  return out;
}
