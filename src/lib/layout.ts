import type { Node } from "@xyflow/svelte";
import type { ElkNode, ElkExtendedEdge, ElkPort, ELK } from "elkjs/lib/elk-api";
import type { Execution, StageDefinition } from "./types";

type Workflow = Execution["configuration"]["workflow"];
type Side = "left" | "right" | "top" | "bottom";
export interface Point {
  x: number;
  y: number;
}
export interface RouteGeometry {
  points: Point[];
  sourceSide: Side;
  targetSide: Side;
  label: Point;
}
export interface WorkflowLayout {
  nodes: Node[];
  routes: Record<string, RouteGeometry>;
}
const WIDTH = 280,
  HEIGHT = 150;
const routeID = (source: string, outcome: string) => `${source}:${outcome}`;
const portID = (step: string, direction: "in" | "out") =>
  `port:${direction}:${step}`;

function options(direction: "RIGHT" | "DOWN") {
  return {
    "elk.algorithm": "layered",
    "elk.direction": direction,
    "elk.edgeRouting": "ORTHOGONAL",
    "elk.spacing.nodeNode": "65",
    "elk.layered.spacing.nodeNodeBetweenLayers": "90",
    "elk.padding": "[top=0,left=0,bottom=0,right=0]",
  };
}
function port(
  id: string,
  side: Side,
  width: number,
  height: number,
  offset?: number,
): ElkPort {
  const horizontal = side === "left" || side === "right";
  const sides = { left: "WEST", right: "EAST", top: "NORTH", bottom: "SOUTH" };
  return {
    id,
    width: 0,
    height: 0,
    x: horizontal ? (side === "left" ? 0 : width) : (offset ?? width / 2),
    y: horizontal ? (offset ?? height / 2) : side === "top" ? 0 : height,
    layoutOptions: { "elk.port.side": sides[side] },
  };
}
function route(
  source: string,
  outcome: string,
  target: string,
): ElkExtendedEdge {
  return {
    id: routeID(source, outcome),
    sources: [portID(source, "out")],
    targets: [portID(target, "in")],
    labels: [
      {
        text: outcome,
        width: outcome.length * 7 + 16,
        height: 20,
        layoutOptions: { "elk.edgeLabels.placement": "HEAD" },
      },
    ],
  };
}
function geometry(
  edge: ElkExtendedEdge,
  offset: Point,
  sourceSide: Side,
  targetSide: Side,
): RouteGeometry {
  const section = edge.sections?.[0];
  if (!section) throw new Error(`No route geometry for ${edge.id}`);
  const translate = (point: Point) => ({
    x: point.x + offset.x,
    y: point.y + offset.y,
  });
  const points = [
    section.startPoint,
    ...(section.bendPoints ?? []),
    section.endPoint,
  ].map(translate);
  const label = edge.labels?.[0];
  return {
    points,
    sourceSide,
    targetSide,
    label:
      label?.x !== undefined && label.y !== undefined
        ? {
            x: offset.x + label.x + (label.width ?? 0) / 2,
            y: offset.y + label.y + (label.height ?? 0) / 2,
          }
        : points[Math.floor(points.length / 2)],
  };
}
function attachment(node: Node, parents: Map<string, Node>, side: Side): Point {
  const parent = node.parentId ? parents.get(node.parentId) : undefined;
  const x = node.position.x + (parent?.position.x ?? 0),
    y = node.position.y + (parent?.position.y ?? 0);
  return {
    x: x + (side === "left" ? 0 : side === "right" ? WIDTH : WIDTH / 2),
    y: y + (side === "top" ? 0 : side === "bottom" ? HEIGHT : HEIGHT / 2),
  };
}

const STAGE_PADDING = "[top=74,left=28,bottom=28,right=28]";

function sidesAt(child: ElkNode, edge: ElkExtendedEdge, source: boolean): Side {
  const section = edge.sections![0];
  const point = source ? section.startPoint : section.endPoint;
  if (Math.abs(point.y - child.y!) < 0.01) return "top";
  if (Math.abs(point.y - child.y! - HEIGHT) < 0.01) return "bottom";
  return Math.abs(point.x - child.x!) < 0.01 ? "left" : "right";
}

/** Concurrent members share a row; the human boundary remains below that row.
 * Layout both levels with ELK, then flatten coordinates for Svelte Flow. */
async function concurrentStage(
  elk: ELK,
  id: string,
  members: string[],
  gate: string,
  definitions: Map<
    string,
    Execution["configuration"]["workflow"]["steps"][number]
  >,
): Promise<ElkNode> {
  const peers = members.filter((member) => member !== gate);
  const peerEdges: ElkExtendedEdge[] = [],
    boundaryEdges: ElkExtendedEdge[] = [];
  for (const member of members)
    for (const [outcome, target] of Object.entries(
      definitions.get(member)!.routes,
    )) {
      if (!members.includes(target)) continue;
      (member !== gate && target !== gate ? peerEdges : boundaryEdges).push(
        route(member, outcome, target),
      );
    }
  const row: ElkNode = await elk.layout({
    id: `row:${id}`,
    layoutOptions: {
      ...options("RIGHT"),
      "elk.layered.cycleBreaking.strategy": "MODEL_ORDER",
    },
    children: peers.map((member) => ({
      id: `step:${member}`,
      width: WIDTH,
      height: HEIGHT,
      layoutOptions: { "elk.portConstraints": "FIXED_POS" },
      ports: [
        port(portID(member, "out"), "right", WIDTH, HEIGHT),
        port(portID(member, "in"), "left", WIDTH, HEIGHT),
      ],
    })),
    edges: peerEdges,
  });
  const outer: ElkNode = await elk.layout({
    id,
    layoutOptions: {
      ...options("DOWN"),
      "elk.padding": STAGE_PADDING,
      "elk.layered.cycleBreaking.strategy": "MODEL_ORDER",
    },
    children: [
      {
        id: row.id,
        width: row.width,
        height: row.height,
        layoutOptions: { "elk.portConstraints": "FIXED_POS" },
        ports: (row.children ?? []).flatMap((child) => {
          const member = child.id.slice("step:".length),
            x = child.x! + WIDTH / 2;
          return [
            port(portID(member, "out"), "bottom", row.width!, row.height!, x),
            port(portID(member, "in"), "top", row.width!, row.height!, x),
          ];
        }),
      },
      {
        id: `step:${gate}`,
        width: WIDTH,
        height: HEIGHT,
        layoutOptions: { "elk.portConstraints": "FIXED_POS" },
        ports: [
          port(portID(gate, "out"), "bottom", WIDTH, HEIGHT),
          port(portID(gate, "in"), "top", WIDTH, HEIGHT),
        ],
      },
    ],
    edges: boundaryEdges,
  });
  const rowPosition = outer.children!.find((child) => child.id === row.id)!;
  const children = [
    ...(row.children ?? []).map((child) => ({
      ...child,
      x: child.x! + rowPosition.x!,
      y: child.y! + rowPosition.y!,
    })),
    ...outer.children!.filter((child) => child.id !== row.id),
  ];
  const byID = new Map(children.map((child) => [child.id, child]));
  const translate = (point: Point) => ({
    x: point.x + rowPosition.x!,
    y: point.y + rowPosition.y!,
  });
  const edges: ElkExtendedEdge[] = (row.edges ?? []).map((edge) => ({
    ...edge,
    sections: edge.sections?.map((section) => ({
      ...section,
      startPoint: translate(section.startPoint),
      endPoint: translate(section.endPoint),
      bendPoints: section.bendPoints?.map(translate),
    })),
    labels: edge.labels?.map((label) => ({
      ...label,
      x: label.x! + rowPosition.x!,
      y: label.y! + rowPosition.y!,
    })),
  }));
  for (const edge of outer.edges ?? []) {
    const source = byID.get(
      `step:${edge.sources[0].slice("port:out:".length)}`,
    )!;
    const target = byID.get(
      `step:${edge.targets[0].slice("port:in:".length)}`,
    )!;
    const section = edge.sections![0];
    edges.push({
      ...edge,
      sections: [
        {
          ...section,
          startPoint: { x: source.x! + WIDTH / 2, y: source.y! + HEIGHT },
          bendPoints: [
            section.startPoint,
            ...(section.bendPoints ?? []),
            section.endPoint,
          ],
          endPoint: { x: target.x! + WIDTH / 2, y: target.y! },
        },
      ],
    });
  }
  return { ...outer, children, edges };
}

/** Both card positions and connection paths come from ELK. Geometry depends
 * only on the frozen definition; status polling never repositions the graph. */
export async function layoutWorkflow(
  workflow: Workflow,
  availableWidth = 1200,
): Promise<WorkflowLayout> {
  const { default: ELK } = await import("elkjs/lib/elk.bundled.js");
  const elk = new ELK();
  const definitions = new Map(workflow.steps.map((step) => [step.id, step]));
  const owner = new Map<string, string>();
  const stages: {
    stage: StageDefinition;
    layout: ElkNode;
  }[] = [];

  for (const [index, stage] of (workflow.stages ?? []).entries()) {
    const id = `stage:${index}`;
    const members = [
      ...new Set([
        ...stage.steps,
        ...(stage.human?.step ? [stage.human.step] : []),
      ]),
    ].filter((step) => definitions.has(step) && !owner.has(step));
    if (!members.length) continue;
    for (const member of members) owner.set(member, id);
    const concurrent = stage.loop?.mode === "concurrent";
    const edges: ElkExtendedEdge[] = [];
    for (const member of members)
      for (const [outcome, target] of Object.entries(
        definitions.get(member)!.routes,
      )) {
        if (members.includes(target))
          edges.push(route(member, outcome, target));
      }
    const arrange = (horizontal: boolean): Promise<ElkNode> =>
      elk.layout({
        id,
        layoutOptions: {
          ...options(horizontal ? "RIGHT" : "DOWN"),
          "elk.padding": STAGE_PADDING,
          "elk.layered.cycleBreaking.strategy": "MODEL_ORDER",
        },
        children: members.map((member) => ({
          id: `step:${member}`,
          width: WIDTH,
          height: HEIGHT,
          layoutOptions: { "elk.portConstraints": "FIXED_POS" },
          ports: [
            port(
              portID(member, "out"),
              horizontal ? "right" : "bottom",
              WIDTH,
              HEIGHT,
            ),
            port(
              portID(member, "in"),
              horizontal ? "left" : "top",
              WIDTH,
              HEIGHT,
            ),
          ],
        })),
        edges: structuredClone(edges),
      });
    let layout =
      concurrent && stage.human?.step && members.includes(stage.human.step)
        ? await concurrentStage(elk, id, members, stage.human.step, definitions)
        : await arrange(true);
    // Preserve readable cards rather than scaling an oversized row to fit.
    if (!concurrent && layout.width! > Math.max(360, availableWidth - 96))
      layout = await arrange(false);
    stages.push({ stage, layout });
  }

  const standalone = workflow.steps.filter((step) => !owner.has(step.id));
  for (const step of standalone) owner.set(step.id, `step:${step.id}`);
  // Cross-stage ports align with the actual member card. Routing against only
  // stage centers would allow lines to cut through other cards or branch nodes.
  const externalSides = new Map<string, { source: Side; target: Side }>();
  const rootChildren: ElkNode[] = stages.map(({ layout }) => {
    const top = Math.min(...layout.children!.map((child) => child.y!));
    const bottom = Math.max(
      ...layout.children!.map((child) => child.y! + HEIGHT),
    );
    return {
      id: layout.id,
      width: layout.width,
      height: layout.height,
      layoutOptions: { "elk.portConstraints": "FIXED_POS" },
      ports: layout.children!.flatMap((child) => {
        const step = child.id.slice("step:".length);
        const source: Side =
          Math.abs(child.y! + HEIGHT - bottom) < 0.01 ? "bottom" : "right";
        const target: Side = Math.abs(child.y! - top) < 0.01 ? "top" : "left";
        externalSides.set(step, { source, target });
        const offset = (side: Side) =>
          side === "top" || side === "bottom"
            ? child.x! + WIDTH / 2
            : child.y! + HEIGHT / 2;
        return [
          port(
            portID(step, "out"),
            source,
            layout.width!,
            layout.height!,
            offset(source),
          ),
          port(
            portID(step, "in"),
            target,
            layout.width!,
            layout.height!,
            offset(target),
          ),
        ];
      }),
    };
  });
  for (const step of standalone) {
    externalSides.set(step.id, { source: "bottom", target: "top" });
    rootChildren.push({
      id: `step:${step.id}`,
      width: WIDTH,
      height: HEIGHT,
      layoutOptions: { "elk.portConstraints": "FIXED_POS" },
      ports: [
        port(portID(step.id, "out"), "bottom", WIDTH, HEIGHT),
        port(portID(step.id, "in"), "top", WIDTH, HEIGHT),
      ],
    });
  }
  const externalEdges: ElkExtendedEdge[] = [];
  const endpoints = new Map<string, { source: string; target: string }>();
  for (const step of workflow.steps)
    for (const [outcome, target] of Object.entries(step.routes)) {
      if (!owner.has(target) || owner.get(step.id) === owner.get(target))
        continue;
      const edge = route(step.id, outcome, target);
      externalEdges.push(edge);
      endpoints.set(edge.id, { source: step.id, target });
    }
  const root = await elk.layout({
    id: "workflow",
    layoutOptions: {
      ...options("DOWN"),
      "elk.layered.nodePlacement.strategy": "SIMPLE",
    },
    children: rootChildren,
    edges: externalEdges,
  });
  const rootPositions = new Map(root.children?.map((node) => [node.id, node]));
  const nodes: Node[] = [],
    routes: Record<string, RouteGeometry> = {};
  for (const { stage, layout } of stages) {
    const position = rootPositions.get(layout.id)!;
    nodes.push({
      id: layout.id,
      type: "group",
      position: { x: position.x!, y: position.y! },
      data: { label: stage.id },
      style: `width:${layout.width}px;height:${layout.height}px`,
      draggable: false,
      selectable: false,
    });
    nodes.push({
      id: `label:${layout.id}`,
      type: "stageLabel",
      parentId: layout.id,
      position: { x: 22, y: 18 },
      draggable: false,
      selectable: false,
      data: {
        label: stage.id,
        policy: stage.loop
          ? `${stage.loop.mode || "sequential"} · limit ${stage.loop.limit}`
          : "stage",
      },
    });
    for (const child of layout.children ?? [])
      nodes.push({
        id: child.id,
        type: "step",
        parentId: layout.id,
        position: { x: child.x!, y: child.y! },
        width: WIDTH,
        height: HEIGHT,
        draggable: false,
        data: { definition: definitions.get(child.id.slice("step:".length))! },
      });
    const members = new Map(layout.children?.map((child) => [child.id, child]));
    for (const edge of layout.edges ?? []) {
      const source = members.get(
        `step:${edge.sources[0].slice("port:out:".length)}`,
      )!;
      const target = members.get(
        `step:${edge.targets[0].slice("port:in:".length)}`,
      )!;
      routes[edge.id] = geometry(
        edge,
        { x: position.x!, y: position.y! },
        sidesAt(source, edge, true),
        sidesAt(target, edge, false),
      );
    }
  }
  for (const step of standalone) {
    const position = rootPositions.get(`step:${step.id}`)!;
    nodes.push({
      id: position.id,
      type: "step",
      position: { x: position.x!, y: position.y! },
      width: WIDTH,
      height: HEIGHT,
      draggable: false,
      data: { definition: step },
    });
  }
  const byID = new Map(nodes.map((node) => [node.id, node]));
  for (const edge of root.edges ?? []) {
    const { source, target } = endpoints.get(edge.id)!;
    const sourceSide = externalSides.get(source)!.source,
      targetSide = externalSides.get(target)!.target;
    const routed = geometry(edge, { x: 0, y: 0 }, sourceSide, targetSide);
    routed.points.unshift(
      attachment(byID.get(`step:${source}`)!, byID, sourceSide),
    );
    routed.points.push(
      attachment(byID.get(`step:${target}`)!, byID, targetSide),
    );
    routes[edge.id] = routed;
  }
  return { nodes, routes };
}
