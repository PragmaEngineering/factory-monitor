<script lang="ts">
  import { Handle, Position, type NodeProps, type Node } from "@xyflow/svelte";
  import { Bot, GitBranch, AlertCircle, Clock3 } from "@lucide/svelte";
  import type { StepData } from "../lib/graph";
  import { elapsed } from "../lib/format";
  import Status from "./Status.svelte";
  let { data, selected }: NodeProps<Node<StepData>> = $props();
  const visit = $derived(data.visit);
</script>

{#each [Position.Left, Position.Right, Position.Top, Position.Bottom] as side}
  <Handle type="target" id={`in-${side}`} position={side} />
  <Handle type="source" id={`out-${side}`} position={side} />
{/each}
<div
  class="step-card"
  class:not-visited={data.state === "not_visited"}
  class:selected
  class:running={visit?.state === "running"}
  class:failed={!!visit?.error}
>
  <div class="step-heading">
    <span class="node-icon"
      >{#if data.definition.kind === "decision"}<GitBranch
          size={17}
        />{:else}<Bot size={17} />{/if}</span
    >
    <div>
      <strong>{data.definition.title || data.definition.id}</strong><small
        >{data.definition.run?.package ||
          data.definition.decision?.authority ||
          data.definition.kind}</small
      >
    </div>
  </div>
  <div class="step-meta">
    <Status value={data.state} /><span
      ><Clock3 size={12} />{elapsed(
        visit?.started_at,
        visit?.finished_at,
        data.now,
      )}</span
    >
  </div>
  {#if visit?.error}<div class="node-error">
      <AlertCircle size={13} />{visit.error}
    </div>{:else}<div class="node-bottom">
      {visit
        ? `Visit ${visit.visit} · Attempt ${visit.try || 1}`
        : data.state === "not_visited"
          ? "Not visited in this execution"
          : "Not started"}<span>{data.definition.id}</span>
    </div>{/if}
</div>
