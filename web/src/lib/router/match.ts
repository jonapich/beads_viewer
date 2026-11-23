/**
 * Matches a URL path against a route pattern. Segments compare exactly; a
 * ":name" segment captures its decodeURIComponent'd value; "*" matches any
 * path. A trailing slash is ignored. Returns the captured params, or null.
 */
export function match(pattern: string, path: string): Record<string, string> | null {
  if (pattern === "*") return {};
  const want = segments(pattern);
  const got = segments(path);
  if (want.length !== got.length) return null;

  const params: Record<string, string> = {};
  for (let i = 0; i < want.length; i++) {
    const w = want[i] as string;
    const g = got[i] as string;
    if (w.startsWith(":")) {
      try {
        params[w.slice(1)] = decodeURIComponent(g);
      } catch {
        return null; // malformed percent-encoding
      }
    } else if (w !== g) {
      return null;
    }
  }
  return params;
}

function segments(p: string): string[] {
  return p.split("/").filter((s) => s !== "");
}