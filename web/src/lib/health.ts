import { ApiError, getHealth } from "./api/client";
import { session } from "./session.svelte";

/**
 * Asks bv serve for /api/v1/health. The API client records the outcome in
 * session.status; this keeps the last good body for the UI.
 */
export async function refreshHealth(): Promise<void> {
  try {
    session.health = await getHealth();
  } catch (err) {
    if (err instanceof ApiError) return; // status already recorded by the client
    if (err instanceof DOMException && err.name === "AbortError") return;
    throw err;
  }
}