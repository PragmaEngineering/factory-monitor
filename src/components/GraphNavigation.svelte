<script lang="ts">
  import { Panel, useSvelteFlow } from "@xyflow/svelte";
  import { tick } from "svelte";
  import { Scan, Focus, Maximize2, Minimize2 } from "@lucide/svelte";
  let {
    focusNodes,
    expanded,
    ontoggle,
  }: { focusNodes: { id: string }[]; expanded: boolean; ontoggle: () => void } =
    $props();
  async function toggleViewport() {
    ontoggle();
    await tick();
    // Let the canvas ResizeObserver receive its new viewport before fitting.
    requestAnimationFrame(() =>
      requestAnimationFrame(() => {
        void fitView({ padding: 0.12, maxZoom: 1, duration: 200 });
      }),
    );
  }
  const { fitView } = useSvelteFlow();
</script>

<Panel position="top-right">
  <div class="graph-navigation">
    <button onclick={() => fitView({ padding: 0.12, duration: 200 })}
      ><Scan size={13} />Overview</button
    >
    <button
      disabled={!focusNodes.length}
      onclick={() =>
        fitView({
          nodes: focusNodes,
          padding: 0.15,
          maxZoom: 1,
          duration: 200,
        })}><Focus size={13} />Current stage</button
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
