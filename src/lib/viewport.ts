import type { Viewport, Rect } from "@xyflow/svelte";
import type { WorkflowLayout } from "./layout";

/** Place readable work near the top, leaving a vertical continuation below. */
export function workViewport(bounds: Rect, width: number): Viewport {
  const zoom = Math.min(
    1,
    Math.max(0.85, (width - 96) / Math.max(1, bounds.width)),
  );
  return {
    x: width / 2 - (bounds.x + bounds.width / 2) * zoom,
    y: 76 - bounds.y * zoom,
    zoom,
  };
}
export function workBounds(layout: WorkflowLayout, ids: string[]): Rect {
  const nodes = new Map(layout.nodes.map((node) => [node.id, node]));
  const rectangles = ids.flatMap((id) => {
    const node = nodes.get(id);
    if (!node) return [];
    const parent = node.parentId ? nodes.get(node.parentId) : undefined;
    return [
      {
        x: node.position.x + (parent?.position.x ?? 0),
        y: node.position.y + (parent?.position.y ?? 0),
        width: node.width ?? 280,
        height: node.height ?? 150,
      },
    ];
  });
  if (!rectangles.length) return { x: 0, y: 0, width: 280, height: 150 };
  const x = Math.min(...rectangles.map((rect) => rect.x)),
    y = Math.min(...rectangles.map((rect) => rect.y));
  return {
    x,
    y,
    width: Math.max(...rectangles.map((rect) => rect.x + rect.width)) - x,
    height: Math.max(...rectangles.map((rect) => rect.y + rect.height)) - y,
  };
}
