import { test } from "node:test";
import assert from "node:assert/strict";
import { graph } from "./graph";
import type { Execution } from "./types";
test("graph uses pinned routes and retains latest visits and concurrent members", () => {
  const execution = {
    steps: [
      { key: "build", visit: 1, state: "completed" },
      { key: "build", visit: 2, state: "running" },
    ],
    configuration: {
      workflow: {
        name: "one",
        start: "build",
        stages: [
          {
            id: "work",
            steps: ["build", "review"],
            loop: { mode: "concurrent", limit: 2 },
          },
        ],
        steps: [
          { id: "build", kind: "run", routes: { completed: "review" } },
          {
            id: "review",
            kind: "run",
            routes: { revise: "build", completed: "$complete" },
          },
        ],
      },
    },
  } as unknown as Execution;
  const result = graph(execution, 0);
  assert.equal(result.edges.length, 2);
  assert.equal(
    (
      result.nodes.find((n) => n.id === "step:build")!.data.visit as {
        visit: number;
      }
    ).visit,
    2,
  );
  assert.equal(
    result.nodes.find((n) => n.id === "step:build")!.position.y,
    result.nodes.find((n) => n.id === "step:review")!.position.y,
  );
});

test("standalone entry precedes stages and user names cannot collide with graph containers", () => {
  const execution = {
    configuration: {
      workflow: {
        name: "arbitrary",
        start: "stage:0",
        stages: [{ id: "work", steps: ["worker"] }],
        steps: [
          { id: "stage:0", kind: "run", routes: { ready: "worker" } },
          { id: "worker", kind: "run", routes: { done: "$complete" } },
        ],
      },
    },
  } as unknown as Execution;
  const result = graph(execution, 0);
  assert.equal(
    new Set(result.nodes.map((node) => node.id)).size,
    result.nodes.length,
  );
  const entry = result.nodes.find((node) => node.id === "step:stage:0")!;
  const worker = result.nodes.find((node) => node.id === "step:worker")!;
  assert.ok(
    result.nodes.find((node) => node.id === entry.parentId)!.position.x <
      result.nodes.find((node) => node.id === worker.parentId)!.position.x,
  );
});
