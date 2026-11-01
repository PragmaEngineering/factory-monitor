<script lang="ts">
  import { Handle, Position, type NodeProps, type Node } from "@xyflow/svelte";
  import { Bot, GitBranch, AlertCircle, Clock3 } from "@lucide/svelte";
  import type { StepData } from "../lib/graph";
  import { elapsed } from "../lib/format";
  import Status from "./Status.svelte";
  let { data, selected }: NodeProps<Node<StepData>> = $props();
  const visit = $derived(data.visit);
</script>

<Handle type="target" position={Position.Left} />
<div
  class="step-card"
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
    <Status value={visit?.state || "pending"} /><span
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
        : "Not started"}<span>{data.definition.id}</span>
    </div>{/if}
</div>
<Handle type="source" position={Position.Right} />
