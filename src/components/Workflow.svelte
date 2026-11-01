<script lang="ts">
  import {
    SvelteFlow,
    Background,
    Controls,
    BackgroundVariant,
  } from "@xyflow/svelte";
  import "@xyflow/svelte/dist/style.css";
  import type { Execution, StepDefinition } from "../lib/types";
  import { graph } from "../lib/graph";
  import StepNode from "./StepNode.svelte";
  import GraphNavigation from "./GraphNavigation.svelte";
  import StageLabel from "./StageLabel.svelte";
  let {
    execution,
    now,
    onselect,
  }: {
    execution: Execution;
    now: number;
    onselect: (step: StepDefinition) => void;
  } = $props();
  let expanded = $state(false);
  let canvas: HTMLDivElement;
  function graphKey(event: KeyboardEvent) {
    if (!expanded) return;
    if (event.key === "Escape") {
      expanded = false;
      return;
    }
    if (event.key !== "Tab") return;
    const buttons = [
      ...canvas.querySelectorAll<HTMLElement>(
        'button:not(:disabled), [tabindex="0"]',
      ),
    ];
    const first = buttons[0],
      last = buttons.at(-1);
    if (!first) return;
    if (
      !canvas.contains(document.activeElement) ||
      (!event.shiftKey && document.activeElement === last)
    ) {
      event.preventDefault();
      first.focus();
    } else if (event.shiftKey && document.activeElement === first) {
      event.preventDefault();
      last?.focus();
    }
  }
  const layout = $derived(graph(execution, now));
  const focusNodes = $derived.by(() => {
    const active = layout.nodes.filter(
      (node) =>
        node.type === "step" &&
        ["running", "waiting_input", "waiting_peer", "blocked"].includes(
          (node.data.visit as { state?: string })?.state ?? "",
        ),
    );
    const parents = new Set(active.map((node) => node.parentId));
    return layout.nodes
      .filter((node) => parents.has(node.id))
      .map((node) => ({ id: node.id }));
  });
  const nodeTypes = { step: StepNode, stageLabel: StageLabel };
</script>

<svelte:window onkeydown={graphKey} />
<div
  bind:this={canvas}
  class="workflow-canvas"
  class:expanded
  aria-label="Workflow graph"
>
  <SvelteFlow
    nodes={layout.nodes}
    edges={layout.edges}
    {nodeTypes}
    fitView
    fitViewOptions={{
      nodes: focusNodes.length ? focusNodes : undefined,
      maxZoom: 1,
      padding: 0.12,
    }}
    minZoom={0.3}
    maxZoom={1.6}
    nodesDraggable={false}
    nodesConnectable={false}
    elementsSelectable={true}
    deleteKey={null}
    onnodeclick={({ node }) => {
      if (node.type === "step") {
        expanded = false;
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
      {focusNodes}
      {expanded}
      ontoggle={() => (expanded = !expanded)}
    />
  </SvelteFlow>
</div>
