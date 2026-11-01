import type { Node, Edge } from "@xyflow/svelte";
import type { Execution, StepDefinition, StepVisit } from "./types";
export interface StepData extends Record<string, unknown> {
  definition: StepDefinition;
  visit?: StepVisit;
  now: number;
}
export function graph(
  execution: Execution,
  now: number,
): { nodes: Node[]; edges: Edge[] } {
  const workflow = execution.configuration.workflow;
  const latest = new Map<string, StepVisit>();
  for (const visit of execution.steps ?? []) latest.set(visit.key, visit);
  const definitions = new Map(workflow.steps.map((step) => [step.id, step]));
  const used = new Set<string>();
  const nodes: Node[] = [];
  let x = 0;
  const groups = [
    ...(workflow.stages ?? []).map((stage) => ({
      ...stage,
      steps: [...stage.steps, ...(stage.human?.step ? [stage.human.step] : [])],
    })),
  ];
  for (const step of workflow.steps)
    if (!groups.some((group) => group.steps.includes(step.id)))
      groups.push({ id: step.stage || step.id, steps: [step.id] });
  // Put entry/admission groups first even when they are declared after stages.
  // Cycles are visited once; route edges remain the source of graph semantics.
  const rank = new Map<string, number>();
  const queue = [workflow.start];
  while (queue.length) {
    const id = queue.shift()!;
    if (rank.has(id)) continue;
    const definition = definitions.get(id);
    if (!definition) continue;
    rank.set(id, rank.size);
    queue.push(...Object.values(definition.routes));
  }
  const firstReachable = (steps: string[]) =>
    Math.min(...steps.map((id) => rank.get(id) ?? Infinity));
  groups.sort(
    (left, right) => firstReachable(left.steps) - firstReachable(right.steps),
  );
  for (const [index, group] of groups.entries()) {
    const ids = [...new Set(group.steps)].filter(
      (id) => definitions.has(id) && !used.has(id),
    );
    if (!ids.length) continue;
    const concurrent = group.loop?.mode === "concurrent";
    const members = ids.filter((id) => id !== group.human?.step);
    const width = concurrent ? Math.max(324, members.length * 300 + 24) : 324;
    const height = concurrent
      ? group.human?.step
        ? 470
        : 280
      : ids.length * 190 + 85;
    const parent = `stage:${index}`;
    nodes.push({
      id: parent,
      type: "group",
      position: { x, y: 0 },
      data: { label: group.id },
      style: `width:${width}px;height:${height}px`,
      selectable: false,
      draggable: false,
    });
    nodes.push({
      id: `label:${index}`,
      type: "stageLabel",
      parentId: parent,
      position: { x: 22, y: 18 },
      draggable: false,
      selectable: false,
      data: {
        label: group.id,
        policy: group.loop
          ? `${group.loop.mode || "sequential"} · limit ${group.loop.limit}`
          : "stage",
      },
    });
    ids.forEach((id, i) => {
      used.add(id);
      nodes.push({
        id: `step:${id}`,
        type: "step",
        parentId: parent,
        position: {
          x:
            22 +
            (concurrent
              ? id === group.human?.step
                ? (width - 324) / 2
                : members.indexOf(id) * 300
              : 0),
          y: 74 + (concurrent ? (id === group.human?.step ? 190 : 0) : i * 190),
        },
        data: { definition: definitions.get(id)!, visit: latest.get(id), now },
        draggable: false,
      });
    });
    x += width + 100;
  }
  const edges: Edge[] = [];
  for (const step of workflow.steps)
    for (const [outcome, target] of Object.entries(step.routes)) {
      if (!definitions.has(target)) continue;
      edges.push({
        id: `${step.id}:${outcome}:${target}`,
        source: `step:${step.id}`,
        target: `step:${target}`,
        label: outcome,
        type: "smoothstep",
        style: "stroke:#a8b9ae;stroke-width:1.4",
        labelStyle: "font-size:11px;fill:#626a70",
      });
    }
  return { nodes, edges };
}
