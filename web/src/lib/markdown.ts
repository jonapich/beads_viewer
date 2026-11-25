// A small Markdown subset for the free-text fields of an issue (description, design,
// acceptance, notes, comment bodies). It parses to an AST that the components render
// with Svelte's own escaping: no HTML string is ever built, so no tracker text can reach
// the DOM as markup and no sanitiser is needed. Links are allow-listed to http(s); a local file
// path never becomes an href: it becomes a "file" node, which the components resolve through the
// server (GET /api/v1/file/stat) and show as a link or a thumbnail, or as text when it is not
// one of the allowed files.
//
// The grammar is the approved mock's md()/inline() (beads-c2o.16), plus [text](url):
//   blocks:  ``` fences, pipe tables with a separator row, "#" headings plus the ALL-CAPS
//            and "Q1." heading heuristics, -/*/•/1./1) lists with indented continuation
//            lines, 2-space-indented code blocks, paragraphs (lines joined by a break)
//   inline:  `code`, **strong**, [text](url), bare http(s) autolinks, ![alt](target) images,
//            local file paths: [text](path), file: URLs, bare Windows drive paths (C:\x\a.png),
//            bare paths with a separator and a viewable extension (docs/a.png, ./a.pdf), and a
//            `code span` that is wholly a path. Spaces end a bare path; one with spaces goes in
//            <angle brackets> or a code span.
// Not supported, on purpose: raw HTML, nested emphasis, blockquotes. Remote images are never
// loaded (the CSP stays as it is, and a tracking pixel in bead text must not fire): an http(s)
// image is an "img" node with remote=true, which the components draw as a link.

export type Inline =
  | { t: "text"; v: string }
  | { t: "code"; v: string }
  | { t: "strong"; v: string }
  | { t: "link"; href: string; v: string }
  /**
   * ![alt](target). remote: an http(s) target, src is its absolute URL (drawn as a link, never
   * loaded). Otherwise src is the local path as written, resolved by the server.
   */
  | { t: "img"; src: string; alt: string; remote: boolean }
  /**
   * A local file path. path is as written (the server resolves it, the client never builds file
   * system paths); v is the text to show. explicit: written as a link, a file: URL or a code span,
   * so it is worth a chip when it cannot be opened; a bare path that does not resolve stays plain
   * text. code: it came from a code span and keeps the code look.
   */
  | { t: "file"; path: string; v: string; explicit: boolean; code: boolean }
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

// File types the server serves (pkg/api/files.go fileTypes): a bare path is only taken for a path
// when it ends in one of these, so "example.com/a" and prose with slashes stay text.
const EXTENSIONS = ["png", "jpg", "jpeg", "gif", "webp", "avif", "bmp", "svg", "mp4", "webm", "pdf", "txt", "log", "md", "csv"];
// Each letter as a [xX] class: the case-insensitive flag would also change the other alternatives.
const EXT = EXTENSIONS.map((e) => e.replace(/[a-z]/g, (c) => `[${c}${c.toUpperCase()}]`)).join("|");
// A bare path starts where no word or path character precedes it, so it is scanned once per run
// of such characters (a long run of "b" must not be rescanned from every position).
const BEFORE = String.raw`(?<![\w.@~+%\\/:-])`;
const PATH_STOP = String.raw`[^\s<>"'|?*)]`;
const PATH_END = String.raw`[^\s<>"'|?*).,;:]`;

// One pass over a text run (code spans are already cut out). Leftmost match wins; at one
// position the alternatives are tried in order: ![alt](target), **strong**, [text](target), bare
// http(s) URL, bare file: URL, bare drive path, bare path with a separator and a viewable
// extension. A bare URL or path ends before whitespace, "<" or ")" and never on trailing
// punctuation. The link text and target are length-bounded, and the text cannot hold "[": a long
// run of "[" would otherwise rescan to the end of the line from every one of them. A link
// target is "<angle brackets>" (spaces allowed) or a run without spaces or ")".
const TOKEN = new RegExp(
  [
    String.raw`!\[([^\[\]\n]{0,500})\]\((<[^>\n]{1,2000}>|[^)\s]{1,2000})\)`, // 1 alt, 2 target
    String.raw`\*\*([^*\n]+)\*\*`, // 3
    String.raw`\[([^[\]\n]{1,500})\]\((<[^>\n]{1,2000}>|[^)\s]{1,2000})\)`, // 4 text, 5 target
    String.raw`https?:\/\/[^\s<)]+[^\s<).,;:]`,
    String.raw`file:\/\/${PATH_STOP}*${PATH_END}`,
    String.raw`${BEFORE}[A-Za-z]:[\\/]${PATH_STOP}*${PATH_END}`,
    String.raw`${BEFORE}(?:\.{1,2}[\\/]|[\\/])?(?:[\w.@~+%-]+[\\/])+[\w.@~+%-]+\.(?:${EXT})(?!\.?[\w-])`,
  ].join("|"),
  "g",
);

const VIEWABLE_EXT = new RegExp(String.raw`\.(?:${EXT})$`);
const DRIVE_PATH = /^[A-Za-z]:[\\/]/;
const FILE_URL = /^file:/i;
const SCHEME = /^[A-Za-z][A-Za-z0-9+.-]*:/;

function pushText(out: Inline[], v: string): void {
  if (v !== "") out.push({ t: "text", v });
}

function pushLink(out: Inline[], url: string, text: string, raw: string): void {
  const href = safeHref(url);
  if (href === null) pushText(out, raw);
  else out.push({ t: "link", href, v: text });
}

function stripAngle(target: string): string {
  return target.startsWith("<") && target.endsWith(">") ? target.slice(1, -1).trim() : target;
}

/** A link target that names a local file: a file: URL, a drive path, or a path with a separator and a viewable extension. */
function isFileTarget(t: string): boolean {
  if (FILE_URL.test(t) || DRIVE_PATH.test(t)) return true;
  return (/[\\/]/.test(t) || /^\.{1,2}$/.test(t)) && VIEWABLE_EXT.test(t.toLowerCase().replace(/[?#].*$/, ""));
}

function pushImage(out: Inline[], alt: string, target: string, raw: string): void {
  const t = stripAngle(target);
  if (/^https?:/i.test(t)) {
    const href = safeHref(t);
    if (href === null) pushText(out, raw);
    else out.push({ t: "img", src: href, alt, remote: true });
    return;
  }
  // Any other scheme (javascript:, data:, ftp:) is not a path: plain text. A drive path and file: are.
  if (SCHEME.test(t) && !DRIVE_PATH.test(t) && !FILE_URL.test(t)) {
    pushText(out, raw);
    return;
  }
  out.push({ t: "img", src: t, alt, remote: false });
}

function pushFileLink(out: Inline[], text: string, target: string, raw: string): void {
  const t = stripAngle(target);
  if (/^https?:/i.test(t)) pushLink(out, t, text, raw);
  else if (isFileTarget(t)) out.push({ t: "file", path: t, v: text, explicit: true, code: false });
  else pushText(out, raw);
}

function parseText(src: string, out: Inline[]): void {
  let last = 0;
  for (const m of src.matchAll(TOKEN)) {
    const at = m.index ?? 0;
    pushText(out, src.slice(last, at));
    last = at + m[0].length;
    if (m[1] !== undefined && m[2] !== undefined) pushImage(out, m[1], m[2], m[0]);
    else if (m[3] !== undefined) out.push({ t: "strong", v: m[3] });
    else if (m[4] !== undefined && m[5] !== undefined) pushFileLink(out, m[4], m[5], m[0]);
    else if (/^https?:/.test(m[0])) pushLink(out, m[0], m[0], m[0]);
    else out.push({ t: "file", path: m[0], v: m[0], explicit: FILE_URL.test(m[0]), code: false });
  }
  pushText(out, src.slice(last));
}

/** A code span that is wholly a path: a drive path or file: URL (spaces allowed), or a path with a separator and a viewable extension. */
function isWholePath(s: string): boolean {
  const t = s.trim();
  if (t.length === 0 || t.length > 500 || /[<>|?*"\n]/.test(t)) return false;
  if (FILE_URL.test(t) || DRIVE_PATH.test(t)) return true;
  return /^(?:\.{1,2}[\\/]|[\\/])?(?:[^\s\\/]+[\\/])+[^\s\\/]+$/.test(t) && VIEWABLE_EXT.test(t.toLowerCase());
}
/** One line of inline Markdown. */
export function parseInline(src: string): Inline[] {
  const out: Inline[] = [];
  const parts = String(src).split("`");
  for (let i = 0; i < parts.length; i++) {
    const part = parts[i] ?? "";
    // An odd part is code only when its closing backtick exists; a lone one stays text.
    if (i % 2 === 1 && i < parts.length - 1) {
      if (isWholePath(part)) out.push({ t: "file", path: part.trim(), v: part, explicit: true, code: true });
      else out.push({ t: "code", v: part });
    }
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
