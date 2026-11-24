import { test } from "node:test";
import assert from "node:assert/strict";
import { layoutWorkflow } from "./layout";
import { graph } from "./graph";
import type { Execution } from "./types";
test("graph uses pinned routes and retains latest visits and concurrent members", async () => {
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
  const result = graph(
    execution,
    0,
    await layoutWorkflow(execution.configuration.workflow),
  );
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

test("standalone entry precedes stages and user names cannot collide with graph containers", async () => {
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
  const result = graph(
    execution,
    0,
    await layoutWorkflow(execution.configuration.workflow),
  );
  assert.equal(
    new Set(result.nodes.map((node) => node.id)).size,
    result.nodes.length,
  );
  const entry = result.nodes.find((node) => node.id === "step:stage:0")!;
  const worker = result.nodes.find((node) => node.id === "step:worker")!;
  assert.ok(
    entry.position.y <
      result.nodes.find((node) => node.id === worker.parentId)!.position.y,
  );
});

test("alternate admission review precedes the joined stage regardless of declaration order", async () => {
  const execution = {
    state: "completed",
    steps: [{ key: "readiness", state: "completed", outcome: "admitted" }],
    configuration: {
      workflow: {
        name: "branching",
        start: "readiness",
        stages: [
          {
            id: "design",
            steps: ["plan", "review"],
            loop: { mode: "sequential", limit: 3 },
          },
        ],
        steps: [
          { id: "plan", kind: "run", routes: { completed: "review" } },
          {
            id: "review",
            kind: "run",
            routes: { changes_requested: "plan", completed: "$complete" },
          },
          {
            id: "admission",
            kind: "decision",
            routes: { approve: "plan", reject: "$fail" },
          },
          {
            id: "readiness",
            kind: "run",
            routes: { admitted: "plan", review_required: "admission" },
          },
        ],
      },
    },
  } as unknown as Execution;
  for (const reverse of [false, true]) {
    if (reverse) execution.configuration.workflow.steps.reverse();
    const result = graph(
      execution,
      0,
      await layoutWorkflow(execution.configuration.workflow),
    );
    const readiness = result.nodes.find((n) => n.id === "step:readiness")!;
    const admission = result.nodes.find((n) => n.id === "step:admission")!;
    const design = result.nodes.find((n) => n.type === "group")!;
    assert.equal(
      admission.parentId,
      undefined,
      "standalone decisions are not stages",
    );
    assert.ok(
      readiness.position.y + 150 < admission.position.y,
      "gate comes before its alternate decision",
    );
    assert.ok(
      admission.position.y + 150 < design.position.y,
      "alternate path must join before design",
    );
    const direct = result.edges.find(
      (edge) => edge.source === "step:readiness" && edge.target === "step:plan",
    )!;
    const points = (
      direct.data as unknown as { points: { x: number; y: number }[] }
    ).points;
    for (let index = 1; index < points.length; index++) {
      const a = points[index - 1],
        b = points[index];
      const left = admission.position.x,
        right = left + 280,
        top = admission.position.y,
        bottom = top + 150;
      const throughHorizontal =
        a.y === b.y &&
        a.y > top &&
        a.y < bottom &&
        Math.max(a.x, b.x) > left &&
        Math.min(a.x, b.x) < right;
      const throughVertical =
        a.x === b.x &&
        a.x > left &&
        a.x < right &&
        Math.max(a.y, b.y) > top &&
        Math.min(a.y, b.y) < bottom;
      assert.equal(
        throughHorizontal || throughVertical,
        false,
        "admitted connection must avoid the alternate decision card",
      );
    }
    assert.equal(admission.data.state, "not_visited");
    assert.equal(readiness.data.state, "completed");
    assert.ok(
      result.edges.some(
        (e) => e.source === "step:readiness" && e.target === "step:admission",
      ),
    );
    assert.ok(
      result.edges.some(
        (e) => e.source === "step:admission" && e.target === "step:plan",
      ),
    );
  }
});

test("concurrent members stay side by side with the human boundary below them", async () => {
  const execution = {
    state: "running",
    configuration: {
      workflow: {
        name: "collaborate",
        start: "author",
        stages: [
          {
            id: "work",
            steps: ["author", "critic"],
            loop: { mode: "concurrent", limit: 3 },
            human: { required: true, step: "decision" },
          },
        ],
        steps: [
          { id: "author", kind: "run", routes: { ready: "critic" } },
          {
            id: "critic",
            kind: "run",
            routes: { revise: "author", complete: "decision" },
          },
          {
            id: "decision",
            kind: "decision",
            routes: { approve: "$complete", revise: "author" },
          },
        ],
      },
    },
  } as unknown as Execution;
  const layout = await layoutWorkflow(execution.configuration.workflow);
  const author = layout.nodes.find((n) => n.id === "step:author")!,
    critic = layout.nodes.find((n) => n.id === "step:critic")!,
    decision = layout.nodes.find((n) => n.id === "step:decision")!;
  assert.equal(author.position.y, critic.position.y);
  assert.ok(
    author.position.x < critic.position.x,
    "declared member order remains recognizable through review cycles",
  );
  assert.ok(decision.position.y > author.position.y + 150);
  assert.ok(decision.position.y > critic.position.y + 150);
  const graphData = graph(execution, 0, layout);
  assert.ok(
    graphData.edges.every((edge) => edge.markerEnd),
    "route direction is explicit",
  );
});

test("sequential stages use wide rows and stack when the available width is narrow", async () => {
  const workflow = {
    name: "responsive",
    start: "a",
    stages: [
      { id: "first", steps: ["a", "b"] },
      { id: "second", steps: ["c"] },
    ],
    steps: [
      { id: "a", kind: "run", routes: { done: "b" } },
      { id: "b", kind: "run", routes: { done: "c" } },
      { id: "c", kind: "run", routes: { done: "$complete" } },
    ],
  } as Execution["configuration"]["workflow"];
  const wide = await layoutWorkflow(workflow, 1600);
  const narrow = await layoutWorkflow(workflow, 600);
  const find = (layout: typeof wide, id: string) =>
    layout.nodes.find((n) => n.id === id)!;
  assert.equal(
    find(wide, "step:a").position.y,
    find(wide, "step:b").position.y,
  );
  assert.ok(find(wide, "step:a").position.x < find(wide, "step:b").position.x);
  assert.ok(
    find(narrow, "step:a").position.y < find(narrow, "step:b").position.y,
  );
  assert.ok(
    find(wide, "stage:0").position.y < find(wide, "stage:1").position.y,
  );
});
