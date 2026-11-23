// Hand-maintained mirrors of the Go API response types (ADR 0001 section 6).
// Each type is checked against a golden fixture in ./__fixtures__ by
// fixtures.typecheck.ts, so `npm run check` fails when they drift.
//
// Enum-like fields are typed `string`, not a literal union: resolveJsonModule
// widens JSON string values to `string`, so a literal type would make the
// fixture check fail.

/** Error envelope for every non-2xx /api response (pkg/api/errors.go). */
export interface ApiErrorBody {
  error: { code: string; message: string };
}

/** GET /api/v1/health (pkg/api/api.go). */
export interface HealthResponse {
  status: string;
  version: string;
}