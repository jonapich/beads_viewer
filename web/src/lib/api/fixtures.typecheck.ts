// Compile-time drift check: every golden fixture must be assignable to its
// TypeScript type. Covered by tsconfig.app.json and run by `npm run check`;
// imported by nothing, so it never ships.
import type { ApiErrorBody, HealthResponse, MetaResponse } from "./types";
import health from "./__fixtures__/health.json";
import unauthorized from "./__fixtures__/error-unauthorized.json";
import meta from "./__fixtures__/meta.json";
import metaPhase1 from "./__fixtures__/meta-phase1.json";

export const healthFixture: HealthResponse = health;
export const unauthorizedFixture: ApiErrorBody = unauthorized;
export const metaFixture: MetaResponse = meta;
export const metaPhase1Fixture: MetaResponse = metaPhase1;
