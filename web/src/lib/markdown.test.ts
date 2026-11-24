import { describe, expect, it } from "vitest";
import { parseInline, parseMarkdown, safeHref, type Block, type Inline } from "./markdown";

const t = (v: string): Inline => ({ t: "text", v });
const br: Inline = { t: "br" };

describe("safeHref", () => {
  it("accepts absolute http and https URLs", () => {
    expect(safeHref("https://example.com/a?b=1#c")).toBe("https://example.com/a?b=1#c");
    expect(safeHref("http://localhost:7777/x")).toBe("http://localhost:7777/x");
    expect(safeHref("  https://example.com/x  ")).toBe("https://example.com/x");
  });

  it("refuses every other scheme, relative and malformed URLs", () => {
    for (const u of [
      "javascript:alert(1)",
      "JaVaScRiPt:alert(1)",
      "java\nscript:alert(1)",
      "data:text/html,<b>x</b>",
      "vbscript:x",
      "file:///etc/passwd",
      "mailto:a@b.c",
      "//evil.example/x",
      "/issues/x",
      "example.com",
      "",
      "https://",
    ]) {
      expect(safeHref(u), u).toBeNull();
    }
  });
});

describe("parseInline", () => {
  it("returns plain text as one run and nothing for empty input", () => {
    expect(parseInline("hello world")).toEqual([t("hello world")]);
    expect(parseInline("")).toEqual([]);
  });

  it("keeps markup characters as text: nothing is HTML", () => {
    expect(parseInline('<b onclick="x">hi</b> & <script>alert(1)</script>')).toEqual([
      t('<b onclick="x">hi</b> & <script>alert(1)</script>'),
    ]);
  });

  it("parses code spans and keeps their content verbatim", () => {
    expect(parseInline("run `go test ./...` now")).toEqual([t("run "), { t: "code", v: "go test ./..." }, t(" now")]);
    expect(parseInline("`**not bold** https://x.test`")).toEqual([{ t: "code", v: "**not bold** https://x.test" }]);
  });

  it("treats an unclosed backtick as text", () => {
    expect(parseInline("a ` b")).toEqual([t("a "), t("` b")]);
    expect(parseInline("`a` b ` c")).toEqual([{ t: "code", v: "a" }, t(" b "), t("` c")]);
  });

  it("parses **strong** on one line only", () => {
    expect(parseInline("a **b** c")).toEqual([t("a "), { t: "strong", v: "b" }, t(" c")]);
    expect(parseInline("**a** and **b**")).toEqual([{ t: "strong", v: "a" }, t(" and "), { t: "strong", v: "b" }]);
    expect(parseInline("**a b")).toEqual([t("**a b")]);
    expect(parseInline("****")).toEqual([t("****")]);
  });

  it("autolinks bare http(s) URLs and leaves trailing punctuation outside", () => {
    expect(parseInline("see https://example.com/a.")).toEqual([t("see "), { t: "link", href: "https://example.com/a", v: "https://example.com/a" }, t(".")]);
    expect(parseInline("(http://example.com/x)")).toEqual([t("("), { t: "link", href: "http://example.com/x", v: "http://example.com/x" }, t(")")]);
    expect(parseInline("a http://x.test/p?q=1&r=2, b")).toEqual([
      t("a "),
      { t: "link", href: "http://x.test/p?q=1&r=2", v: "http://x.test/p?q=1&r=2" },
      t(", b"),
    ]);
  });

  it("does not autolink other schemes", () => {
    expect(parseInline("javascript:alert(1) ftp://x.test mailto:a@b.c")).toEqual([t("javascript:alert(1) ftp://x.test mailto:a@b.c")]);
  });

  it("parses [text](url) through safeHref", () => {
    expect(parseInline("go [here](https://example.com/p) now")).toEqual([
      t("go "),
      { t: "link", href: "https://example.com/p", v: "here" },
      t(" now"),
    ]);
  });

  it("keeps a [text](url) with an unsafe URL as plain text", () => {
    expect(parseInline("[x](javascript:alert(1))")).toEqual([t("[x](javascript:alert(1)"), t(")")]);
    expect(parseInline("[x](/relative)")).toEqual([t("[x](/relative)")]);
    expect(parseInline("[x](data:text/html,hi)")).toEqual([t("[x](data:text/html,hi)")]);
  });

  it("never produces a link whose href is not http or https", () => {
    const inputs = [
      "[a](javascript:alert(1))",
      "[a](JAVASCRIPT:alert(1))",
      "[a](data:text/html;base64,PHNjcmlwdD4=)",
      "[a](vbscript:x) https://ok.test [b](//x.test)",
      "[a](https://ok.test/x) [b](ftp://x.test)",
    ];
    for (const s of inputs) {
      for (const n of parseInline(s)) {
        if (n.t === "link") expect(n.href).toMatch(/^https?:\/\//);
      }
    }
  });

  it("mixes everything in one line", () => {
    expect(parseInline("**Q** `x` https://a.test")).toEqual([
      { t: "strong", v: "Q" },
      t(" "),
      { t: "code", v: "x" },
      t(" "),
      { t: "link", href: "https://a.test/", v: "https://a.test" },
    ]);
  });
});

describe("parseMarkdown", () => {
  const p = (...c: Inline[]): Block => ({ t: "p", c });

  it("returns no blocks for blank input", () => {
    expect(parseMarkdown("")).toEqual([]);
    expect(parseMarkdown("  \n\n \r\n")).toEqual([]);
  });

  it("makes a paragraph per blank-line-separated run and joins its lines with a break", () => {
    expect(parseMarkdown("one\ntwo\n\nthree")).toEqual([p(t("one"), br, t("two")), p(t("three"))]);
  });

  it("accepts CRLF line endings", () => {
    expect(parseMarkdown("a\r\nb")).toEqual([p(t("a"), br, t("b"))]);
  });

  it("parses fenced code verbatim, with or without a closing fence", () => {
    expect(parseMarkdown("```go\nfunc <x>() {}\n\n  indented\n```\nafter")).toEqual([
      { t: "pre", v: "func <x>() {}\n\n  indented" },
      p(t("after")),
    ]);
    expect(parseMarkdown("```\nopen ended")).toEqual([{ t: "pre", v: "open ended" }]);
  });

  it("parses # headings and strips the marks", () => {
    expect(parseMarkdown("## Why **now**\ntext")).toEqual([{ t: "h", c: [t("Why "), { t: "strong", v: "now" }] }, p(t("text"))]);
    expect(parseMarkdown("#nospace")).toEqual([p(t("#nospace"))]);
  });

  it("treats an ALL-CAPS line and a Q1. line as headings", () => {
    expect(parseMarkdown("RECOMMENDATIONS\nbody")).toEqual([{ t: "h", c: [t("RECOMMENDATIONS")] }, p(t("body"))]);
    expect(parseMarkdown("Q2. Markdown\nbody")).toEqual([{ t: "h", c: [t("Q2. Markdown")] }, p(t("body"))]);
    expect(parseMarkdown("OK")).toEqual([p(t("OK"))]); // too few capitals to be a heading
  });

  it("parses bullet and numbered lists", () => {
    expect(parseMarkdown("- a\n* b\n• c")).toEqual([{ t: "ul", items: [[t("a")], [t("b")], [t("c")]] }]);
    expect(parseMarkdown("1. a\n2) b")).toEqual([{ t: "ol", items: [[t("a")], [t("b")]] }]);
  });

  it("folds an indented continuation line into the item above, after a break", () => {
    expect(parseMarkdown("- a\n  more **b**\n- c")).toEqual([
      { t: "ul", items: [[t("a"), br, t("more "), { t: "strong", v: "b" }], [t("c")]] },
    ]);
  });

  it("flattens a nested bullet into the list", () => {
    expect(parseMarkdown("- a\n  - b")).toEqual([{ t: "ul", items: [[t("a")], [t("b")]] }]);
  });

  it("ends a list at the first other line", () => {
    expect(parseMarkdown("- a\ntext")).toEqual([{ t: "ul", items: [[t("a")] ] }, p(t("text"))]);
  });

  it("parses a 2-space-indented run as code, dropping two spaces", () => {
    expect(parseMarkdown("intro\n\n  a\n    b\n\n  c\nout")).toEqual([p(t("intro")), { t: "pre", v: "a\n  b\n\nc" }, p(t("out"))]);
  });

  it("parses a pipe table with a separator row", () => {
    expect(parseMarkdown("| A | B |\n|---|:-:|\n| 1 | `x` |\n| 2 | **y** |\nafter")).toEqual([
      {
        t: "table",
        head: [[t("A")], [t("B")]],
        rows: [
          [[t("1")], [{ t: "code", v: "x" }]],
          [[t("2")], [{ t: "strong", v: "y" }]],
        ],
      },
      p(t("after")),
    ]);
  });

  it("does not make a table without a separator row", () => {
    expect(parseMarkdown("| A | B |\n| 1 | 2 |")).toEqual([p(t("| A | B |"), br, t("| 1 | 2 |"))]);
  });

  it("starts a new block when a special line follows a paragraph line", () => {
    expect(parseMarkdown("text\n- item\nmore")).toEqual([p(t("text")), { t: "ul", items: [[t("item")]] }, p(t("more"))]);
    expect(parseMarkdown("text\n```\nc\n```")).toEqual([p(t("text")), { t: "pre", v: "c" }]);
    expect(parseMarkdown("text\n## H")).toEqual([p(t("text")), { t: "h", c: [t("H")] }]);
  });

  it("carries html as text and unsafe links as text, never as markup or hrefs", () => {
    const blocks = parseMarkdown('<img src=x onerror="alert(1)">\n\n[click](javascript:alert(1))\n\n```\n<script>alert(1)</script>\n```');
    expect(blocks[0]).toEqual(p(t('<img src=x onerror="alert(1)">')));
    expect(JSON.stringify(blocks)).not.toContain('"href":"javascript');
    expect(blocks[2]).toEqual({ t: "pre", v: "<script>alert(1)</script>" });
  });

  it("terminates on every kind of odd input", () => {
    for (const s of ["```", "|", "| a |\n|", "-", "- ", "  ", "#", "# ", "\n\n\n", "1.", "Q1.", "  \n  x", "|a|\n|-|\n"]) {
      expect(() => parseMarkdown(s), JSON.stringify(s)).not.toThrow();
    }
  });
});
