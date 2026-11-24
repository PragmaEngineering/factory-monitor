<script lang="ts">
  import {
    SvelteFlow,
    Background,
    Controls,
    BackgroundVariant,
    PanOnScrollMode,
  } from "@xyflow/svelte";
  import "@xyflow/svelte/dist/style.css";
  import type { Execution, StepDefinition } from "../lib/types";
  import type { Viewport } from "@xyflow/svelte";
  import { workBounds, workViewport } from "../lib/viewport";
  import type { WorkflowLayout } from "../lib/layout";
  import RoutedEdge from "./RoutedEdge.svelte";
  import { layoutWorkflow } from "../lib/layout";
  import { graph } from "../lib/graph";
  import StepNode from "./StepNode.svelte";
  import GraphNavigation from "./GraphNavigation.svelte";
  import StageLabel from "./StageLabel.svelte";
  let {
    execution,
    now,
    onselect,
    expanded = $bindable(false),
  }: {
    expanded?: boolean;
    execution: Execution;
    now: number;
    onselect: (step: StepDefinition) => void;
  } = $props();
  let canvas: HTMLDivElement;
  let canvasWidth = $state(0);
  // Width buckets avoid relayout for every pixel during window resizing.
  const layoutWidth = $derived(
    Math.max(360, Math.floor(canvasWidth / 100) * 100),
  );
  function graphKey(event: KeyboardEvent) {
    if (!expanded) return;
    if (event.key === "Escape") {
      expanded = false;
      return;
    }
    if (event.key !== "Tab") return;
    const container = canvas.closest<HTMLElement>(".workflow-layout") ?? canvas;
    const buttons = [
      ...container.querySelectorAll<HTMLElement>(
        'button:not(:disabled), [tabindex="0"]',
      ),
    ];
    const first = buttons[0],
      last = buttons.at(-1);
    if (!first) return;
    if (
      !container.contains(document.activeElement) ||
      (!event.shiftKey && document.activeElement === last)
    ) {
      event.preventDefault();
      first.focus();
    } else if (event.shiftKey && document.activeElement === first) {
      event.preventDefault();
      last?.focus();
    }
  }
  let geometry = $state<WorkflowLayout>({ nodes: [], routes: {} });
  let layoutError = $state("");
  const configurationKey = $derived(
    JSON.stringify(execution.configuration.workflow),
  );
  $effect(() => {
    const workflow = JSON.parse(configurationKey);
    if (!canvasWidth) return;
    let cancelled = false;
    layoutError = "";
    layoutWorkflow(workflow, layoutWidth)
      .then((nodes) => {
        if (!cancelled) geometry = nodes;
      })
      .catch((error) => {
        if (!cancelled) layoutError = error.message;
      });
    return () => {
      cancelled = true;
    };
  });
  const layout = $derived(graph(execution, now, geometry));
  const defaultNodes = $derived.by(() => {
    const active = layout.nodes.filter(
      (node) =>
        node.type === "step" &&
        ["running", "waiting_input", "waiting_peer", "blocked"].includes(
          (node.data.visit as { state?: string })?.state ?? "",
        ),
    );
    const focus = active.length
      ? active
      : layout.nodes.filter(
          (node) =>
            node.id === `step:${execution.configuration.workflow.start}`,
        );
    const stages = new Set(focus.map((node) => node.parentId).filter(Boolean));
    // Center the whole active stage row so neighboring steps stay in view.
    return layout.nodes
      .filter(
        (node) =>
          node.type === "step" &&
          (focus.includes(node) || stages.has(node.parentId)),
      )
      .map((node) => ({ id: node.id }));
  });
  let viewport = $state<Viewport>({ x: 0, y: 0, zoom: 1 });
  let positioned = $state(false);
  $effect(() => {
    if (positioned || !geometry.nodes.length || !canvas) return;
    viewport = workViewport(
      workBounds(
        geometry,
        defaultNodes.map((node) => node.id),
      ),
      canvas.clientWidth,
    );
    positioned = true;
  });
  const hasActive = $derived(
    layout.nodes.some(
      (node) =>
        node.type === "step" &&
        ["running", "waiting_input", "waiting_peer", "blocked"].includes(
          (node.data.visit as { state?: string })?.state ?? "",
        ),
    ),
  );
  const nodeTypes = { step: StepNode, stageLabel: StageLabel };
</script>

<svelte:window onkeydown={graphKey} />
<div
  bind:this={canvas}
  bind:clientWidth={canvasWidth}
  class="workflow-canvas"
  class:expanded
  aria-label="Workflow graph"
>
  {#if layoutError}<p class="error-banner" role="alert">
      Workflow layout failed: {layoutError}
    </p>
  {:else if geometry.nodes.length && positioned}
    <SvelteFlow
      nodes={layout.nodes}
      edges={layout.edges}
      {nodeTypes}
      edgeTypes={{ routed: RoutedEdge }}
      initialViewport={viewport}
      bind:viewport
      panOnScroll={true}
      panOnScrollMode={PanOnScrollMode.Vertical}
      panOnScrollSpeed={1}
      zoomOnScroll={false}
      minZoom={0.1}
      maxZoom={1.6}
      nodesDraggable={false}
      nodesConnectable={false}
      elementsSelectable={true}
      deleteKey={null}
      onnodeclick={({ node }) => {
        if (node.type === "step") {
          onselect(node.data.definition as StepDefinition);
        }
      }}
    >
      <Background
        variant={BackgroundVariant.Dots}
        gap={18}
        size={1}
        patternColor="#d7ddda"
      />
      <Controls showLock={false} />
      <GraphNavigation
        {hasActive}
        {defaultNodes}
        {expanded}
        ontoggle={() => (expanded = !expanded)}
      />
    </SvelteFlow>
  {:else}<div class="empty-state">Arranging workflow…</div>{/if}
</div>
