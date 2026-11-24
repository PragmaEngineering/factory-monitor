import { test } from "node:test";
import assert from "node:assert/strict";
import { evidence, failureDuration, type ReportDocument } from "./reports";
test("historical counts stay distinct from captured failures and unknown context", () => {
  const legacy = {
    id: "old",
    snapshot: {
      as_of: "now",
      selected: { id: "run", resources: { tool_failures: 7 } },
    },
    html: "",
  } as ReportDocument;
  const summary = evidence(legacy);
  assert.equal(summary.recorded, 7);
  assert.equal(summary.missing, 7);
  assert.equal(summary.peak, undefined);
  assert.equal(summary.failures.length, 0);
  assert.equal(
    evidence({ id: "unknown", snapshot: { as_of: "now" }, html: "" }).recorded,
    undefined,
  );
});
test("context peaks use each response's own window; native turns are independent", () => {
  const report = {
    id: "r",
    snapshot: {
      as_of: "now",
      selected: {
        id: "run",
        agents: [
          {
            name: "coder",
            session_id: "s",
            step_id: "step",
            diagnostics: {
              responses: [
                {
                  number: 1,
                  tokens: {
                    input: 100,
                    cache_read: 600,
                    cache_write: 0,
                    output: 100,
                  },
                  context_window: 1000,
                },
                {
                  number: 2,
                  tokens: {
                    input: 10000,
                    cache_read: 0,
                    cache_write: 0,
                    output: 100,
                  },
                },
              ],
              boundaries: [{ kind: "turn_finished", at: "now" }],
            },
          },
        ],
      },
    },
    html: "",
  } as ReportDocument;
  const summary = evidence(report);
  assert.equal(summary.peak, 80);
  assert.equal(summary.responses.length, 2);
  assert.equal(summary.turns, 1);
  assert.equal(
    failureDuration({
      tool_id: "c",
      started_at: "0001-01-01T00:00:00Z",
      finished_at: "now",
    }),
    "Unknown",
  );
  assert.equal(
    failureDuration({
      tool_id: "c",
      started_at: "2026-11-24T12:00:00Z",
      finished_at: "2026-11-24T12:00:02Z",
    }),
    "2.0 s",
  );
});
