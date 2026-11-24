import { test } from "node:test";
import assert from "node:assert/strict";
import { workBounds, workViewport } from "./viewport";
import type { WorkflowLayout } from "./layout";
test("work opens at readable scale near the top rather than fitting a tall workflow", () => {
  const layout = {
    nodes: [
      { id: "stage", position: { x: 50, y: 2000 }, data: {} },
      {
        id: "step",
        parentId: "stage",
        position: { x: 100, y: 150 },
        width: 280,
        height: 150,
        data: {},
      },
    ],
    routes: {},
  } as WorkflowLayout;
  const bounds = workBounds(layout, ["step"]);
  const viewport = workViewport(bounds, 1200);
  assert.equal(viewport.zoom, 1);
  assert.equal(viewport.y + bounds.y * viewport.zoom, 76);
  assert.equal(viewport.x + (bounds.x + bounds.width / 2) * viewport.zoom, 600);
});
