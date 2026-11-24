<script lang="ts">
  import { Panel, useSvelteFlow } from "@xyflow/svelte";
  import { workViewport } from "../lib/viewport";
  import { tick } from "svelte";
  import { Scan, Focus, Maximize2, Minimize2 } from "@lucide/svelte";
  let {
    hasActive,
    defaultNodes,
    expanded,
    ontoggle,
  }: {
    hasActive: boolean;
    defaultNodes: { id: string }[];
    expanded: boolean;
    ontoggle: () => void;
  } = $props();
  async function toggleViewport() {
    ontoggle();
    await tick();
    // Let the canvas ResizeObserver receive its new viewport before fitting.
    requestAnimationFrame(() =>
      requestAnimationFrame(() => {
        void showWork(200);
      }),
    );
  }
  const { fitView, getNodesBounds, setViewport } = useSvelteFlow();
  let toolbar: HTMLDivElement;
  function showWork(duration = 200) {
    const canvas = toolbar.closest<HTMLElement>(".workflow-canvas");
    if (!canvas) return;
    return setViewport(
      workViewport(
        getNodesBounds(defaultNodes.map((node) => node.id)),
        canvas.clientWidth,
      ),
      { duration },
    );
  }
</script>

<Panel position="top-right">
  <div class="graph-navigation" bind:this={toolbar}>
    <button onclick={() => fitView({ padding: 0.12, duration: 200 })}
      ><Scan size={13} />Overview</button
    >
    <button onclick={() => showWork()}
      ><Focus size={13} />{hasActive ? "Active work" : "Start"}</button
    >
    <button
      onclick={toggleViewport}
      aria-label={expanded ? "Restore graph" : "Expand graph"}
    >
      {#if expanded}<Minimize2 size={13} />{:else}<Maximize2 size={13} />{/if}
      {expanded ? "Restore" : "Expand"}
    </button>
  </div>
</Panel>
