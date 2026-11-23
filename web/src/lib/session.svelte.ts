import type { HealthResponse } from "./api/types";

export type SessionStatus = "unknown" | "ok" | "offline" | "signed-out";

/**
 * What the shell knows about its connection to bv serve. The API client
 * updates `status` on every response; the last good /health body is kept in
 * `health`.
 */
export const session = $state({
  status: "unknown" as SessionStatus,
  health: null as HealthResponse | null,
});