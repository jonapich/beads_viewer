export type ThemePref = "system" | "light" | "dark";

const KEY = "bv.theme";

function readPref(): ThemePref {
  try {
    const v = localStorage.getItem(KEY);
    if (v === "light" || v === "dark") return v;
  } catch {
    // storage unavailable: follow the system
  }
  return "system";
}

export const theme = $state({ pref: "system" as ThemePref });

// public/theme-init.js applies the same rule before first paint.
function apply(): void {
  const dark =
    theme.pref === "dark" || (theme.pref === "system" && matchMedia("(prefers-color-scheme: dark)").matches);
  document.documentElement.dataset.theme = dark ? "dark" : "light";
}

export function setThemePref(p: ThemePref): void {
  theme.pref = p;
  try {
    if (p === "system") localStorage.removeItem(KEY);
    else localStorage.setItem(KEY, p);
  } catch {
    // not persisted; still applies for this session
  }
  apply();
}

/** Reads the saved preference and follows the OS setting while it is "system". */
export function startTheme(): () => void {
  theme.pref = readPref();
  apply();
  const mq = matchMedia("(prefers-color-scheme: dark)");
  const onChange = () => {
    if (theme.pref === "system") apply();
  };
  mq.addEventListener("change", onChange);
  return () => mq.removeEventListener("change", onChange);
}