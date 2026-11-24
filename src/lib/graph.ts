import type { MarkerType, Node, Edge } from "@xyflow/svelte";
import type { Execution, StepDefinition, StepVisit } from "./types";
import type { WorkflowLayout } from "./layout";
import { stepState } from "./format";

export interface StepData extends Record<string, unknown> {
  definition: StepDefinition;
  visit?: StepVisit;
  state: string;
  now: number;
}

/** Apply fresh execution observations to stable, configuration-derived geometry. */
export function graph(
  execution: Execution,
  now: number,
  geometry: WorkflowLayout,
): { nodes: Node[]; edges: Edge[] } {
  const latest = new Map<string, StepVisit>();
  for (const visit of execution.steps ?? []) latest.set(visit.key, visit);
  const nodes = geometry.nodes.map((node) => {
    if (node.type !== "step") return node;
    const definition = node.data.definition as StepDefinition;
    const visit = latest.get(definition.id);
    return {
      ...node,
      data: {
        definition,
        visit,
        state: stepState(execution.state, visit),
        now,
      },
    };
  });
  const byID = new Map(nodes.map((node) => [node.id, node]));
  const edges: Edge[] = [];
  for (const step of execution.configuration.workflow.steps) {
    for (const [outcome, target] of Object.entries(step.routes)) {
      const sourceNode = byID.get(`step:${step.id}`),
        targetNode = byID.get(`step:${target}`);
      if (!sourceNode || !targetNode) continue;
      const routed = geometry.routes[`${step.id}:${outcome}`];
      if (!routed) continue;
      const unvisited =
        sourceNode.data.state === "not_visited" ||
        targetNode.data.state === "not_visited";
      edges.push({
        id: `${step.id}:${outcome}:${target}`,
        source: sourceNode.id,
        target: targetNode.id,
        sourceHandle: `out-${routed.sourceSide}`,
        targetHandle: `in-${routed.targetSide}`,
        label: outcome,
        type: "routed",
        markerEnd: {
          type: "arrowclosed" as MarkerType,
          color: unvisited ? "#d4dbd6" : "#819e8d",
          width: 16,
          height: 16,
        },
        data: { ...routed },
        style: `stroke:${unvisited ? "#d4dbd6" : "#a8b9ae"};stroke-width:1.4${unvisited ? ";stroke-dasharray:4 4" : ""}`,
        labelStyle: `font-size:11px;fill:${unvisited ? "#9da69f" : "#626a70"}`,
      });
    }
  }
  return { nodes, edges };
}
