import type { Usage } from "./types";
export interface ReportResponse {
  number: number;
  turn?: number;
  at: string;
  provider?: string;
  model?: string;
  tokens: Usage["tokens"];
  context_window?: number;
  stop_reason?: string;
  error?: string;
}
export interface ToolFailure {
  tool_id: string;
  name?: string;
  command?: string;
  arguments?: string;
  started_at?: string;
  finished_at: string;
  turn?: number;
  output?: string;
  error?: string;
  exit_code?: number;
  truncated?: boolean;
}
export interface ReportBoundary {
  kind: string;
  at: string;
  turn?: number;
  reason?: string;
  aborted?: boolean;
  tokens_before?: number;
  estimated_tokens_after?: number;
  error?: string;
}
export interface ReportAgent {
  name: string;
  session_id: string;
  step_id: string;
  runner?: string;
  resources?: Usage;
  diagnostics?: {
    responses?: ReportResponse[];
    failures?: ToolFailure[];
    boundaries?: ReportBoundary[];
  };
}
export interface ReportDocument {
  id: string;
  snapshot: {
    as_of: string;
    version?: number;
    selected?: {
      id: string;
      factory?: string;
      state?: string;
      resources?: Usage;
      agents?: ReportAgent[];
    };
    [key: string]: unknown;
  };
  html: string;
}
export function footprint(r: ReportResponse): number {
  return (
    r.tokens.input +
    r.tokens.cache_read +
    r.tokens.cache_write +
    r.tokens.output
  );
}
export function utilization(r: ReportResponse): number | undefined {
  return r.context_window && r.context_window > 0
    ? (100 * footprint(r)) / r.context_window
    : undefined;
}
export function evidence(doc: ReportDocument) {
  const agents = doc.snapshot.selected?.agents ?? [];
  const failures = agents.flatMap((agent) =>
    (agent.diagnostics?.failures ?? []).map((call) => ({ agent, call })),
  );
  const responses = agents.flatMap(
    (agent) => agent.diagnostics?.responses ?? [],
  );
  const boundaries = agents.flatMap(
    (agent) => agent.diagnostics?.boundaries ?? [],
  );
  const occupancies = responses
    .map(utilization)
    .filter((n): n is number => n !== undefined);
  const recorded = doc.snapshot.selected?.resources?.tool_failures;
  return {
    agents,
    failures,
    responses,
    boundaries,
    recorded,
    missing:
      recorded === undefined
        ? undefined
        : Math.max(0, recorded - failures.length),
    peak: occupancies.length ? Math.max(...occupancies) : undefined,
    turns: boundaries.filter((b) => b.kind === "turn_finished").length,
  };
}
export function failureDuration(call: ToolFailure): string {
  if (!call.started_at || call.started_at.startsWith("0001-")) return "Unknown";
  const ms = Date.parse(call.finished_at) - Date.parse(call.started_at);
  return Number.isFinite(ms) && ms >= 0
    ? ms < 1000
      ? `${ms} ms`
      : `${(ms / 1000).toFixed(1)} s`
    : "Unknown";
}
