// Compile-time drift check: every golden fixture must be assignable to its
// TypeScript type. Covered by tsconfig.app.json and run by `npm run check`;
// imported by nothing, so it never ships.
import type {
  ApiErrorBody,
  HealthResponse,
  InboxResponse,
  IssueDetail,
  InsightsResponse,
  IssuePatchRequest,
  IssuesResponse,
  MetaResponse,
  PipelineResponse,
  PlanResponse,
  SearchResponse,
  SignoffResponse,
  SliceMetricsResponse,
  SnapshotEvent,
  TriageResponse,
  WriteResponse,
} from "./types";
import health from "./__fixtures__/health.json";
import unauthorized from "./__fixtures__/error-unauthorized.json";
import meta from "./__fixtures__/meta.json";
import metaPhase1 from "./__fixtures__/meta-phase1.json";
import eventSnapshot from "./__fixtures__/event-snapshot.json";
import issuesTree from "./__fixtures__/issues-tree.json";
import issuesFlat from "./__fixtures__/issues-flat.json";
import searchResult from "./__fixtures__/search.json";
import issueDetail from "./__fixtures__/issue-detail.json";
import issueDetailDecision from "./__fixtures__/issue-detail-decision.json";
import issueDetailSignoff from "./__fixtures__/issue-detail-signoff.json";
import signoff from "./__fixtures__/signoff.json";
import inbox from "./__fixtures__/inbox.json";
import inboxEmpty from "./__fixtures__/inbox-empty.json";
import pipeline from "./__fixtures__/pipeline.json";
import pipelineEmpty from "./__fixtures__/pipeline-empty.json";
import sliceMetrics from "./__fixtures__/slice-metrics.json";
import sliceMetricsEmpty from "./__fixtures__/slice-metrics-empty.json";
import notFound from "./__fixtures__/error-not-found.json";
import triage from "./__fixtures__/triage.json";
import insights from "./__fixtures__/insights.json";
import plan from "./__fixtures__/plan.json";
import writeResult from "./__fixtures__/write-result.json";
import writeResultDispute from "./__fixtures__/write-result-dispute.json";
import writeResultWarning from "./__fixtures__/write-result-warning.json";
import issuePatchRequest from "./__fixtures__/issue-patch-request.json";
import readOnly from "./__fixtures__/error-read-only.json";
import conflict from "./__fixtures__/error-conflict.json";
import staleBase from "./__fixtures__/error-stale-base.json";

export const healthFixture: HealthResponse = health;
export const unauthorizedFixture: ApiErrorBody = unauthorized;
export const metaFixture: MetaResponse = meta;
export const metaPhase1Fixture: MetaResponse = metaPhase1;
export const eventSnapshotFixture: SnapshotEvent = eventSnapshot;
export const issuesTreeFixture: IssuesResponse = issuesTree;
export const issuesFlatFixture: IssuesResponse = issuesFlat;
export const searchFixture: SearchResponse = searchResult;
export const issueDetailFixture: IssueDetail = issueDetail;
export const issueDetailDecisionFixture: IssueDetail = issueDetailDecision;
export const issueDetailSignoffFixture: IssueDetail = issueDetailSignoff;
export const signoffFixture: SignoffResponse = signoff;
export const inboxFixture: InboxResponse = inbox;
export const inboxEmptyFixture: InboxResponse = inboxEmpty;
export const pipelineFixture: PipelineResponse = pipeline;
export const pipelineEmptyFixture: PipelineResponse = pipelineEmpty;
export const sliceMetricsFixture: SliceMetricsResponse = sliceMetrics;
export const sliceMetricsEmptyFixture: SliceMetricsResponse = sliceMetricsEmpty;
export const notFoundFixture: ApiErrorBody = notFound;
export const triageFixture: TriageResponse = triage;
export const insightsFixture: InsightsResponse = insights;
export const planFixture: PlanResponse = plan;
export const writeResultFixture: WriteResponse = writeResult;
export const writeResultDisputeFixture: WriteResponse = writeResultDispute;
export const writeResultWarningFixture: WriteResponse = writeResultWarning;
export const issuePatchRequestFixture: IssuePatchRequest = issuePatchRequest;
export const readOnlyFixture: ApiErrorBody = readOnly;
export const conflictFixture: ApiErrorBody = conflict;
export const staleBaseFixture: ApiErrorBody = staleBase;
