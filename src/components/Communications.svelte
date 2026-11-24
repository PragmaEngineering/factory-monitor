<script lang="ts">
  import { Network } from "@lucide/svelte";
  import type { Message } from "../lib/types";
  import { communicationGraph, participantLabel } from "../lib/communications";
  let { messages, selected }: { messages: Message[]; selected?: string } =
    $props();
  const graph = $derived(communicationGraph(messages));
  const selectedName = $derived(
    selected && ["operator", "human"].includes(selected.toLowerCase())
      ? "Operator"
      : selected,
  );
  const id = $props.id();
</script>

<aside class="communications-panel" aria-label="Observed communication">
  <div class="section-heading">
    <span><Network size={16} />Observed communication</span>
  </div>
  <div class="communications-summary">
    <span><strong>{graph.nodes.length}</strong> participants</span>
    <span><strong>{messages.length}</strong> messages</span>
    <span><strong>{graph.links.length}</strong> directed links</span>
  </div>
  {#if graph.nodes.length}
    <div class="communications-network">
      <svg
        viewBox={`0 0 ${graph.size} ${graph.size}`}
        role="img"
        aria-label="Participant communication graph"
        style={`min-width:${Math.max(360, graph.size * 0.72)}px`}
      >
        <title>Observed communication between participants</title>
        <desc
          >Arrows point from sender to recipient. Labels show persisted message
          counts. Participants are grouped by name.</desc
        >
        <defs>
          <marker
            id={`${id}-arrow`}
            viewBox="0 0 10 10"
            refX="9"
            refY="5"
            markerWidth="5"
            markerHeight="5"
            orient="auto-start-reverse"
            ><path d="M 0 0 L 10 5 L 0 10 z" fill="context-stroke" /></marker
          >
        </defs>
        {#each graph.links as link}
          <g
            class="communication-link"
            class:highlighted={selectedName === link.from ||
              selectedName === link.to}
            class:muted-link={!!selectedName &&
              selectedName !== link.from &&
              selectedName !== link.to}
          >
            <title>{link.from} → {link.to}: {link.count} messages</title>
            <path
              d={link.path}
              fill="none"
              stroke-width={Math.min(5, 1.3 + Math.log2(link.count + 1) * 0.65)}
              marker-end={`url(#${id}-arrow)`}
            />
            <rect
              x={link.label.x - 15}
              y={link.label.y - 10}
              width="30"
              height="20"
              rx="7"
            />
            <text x={link.label.x} y={link.label.y + 4} text-anchor="middle"
              >{link.count}</text
            >
          </g>
        {/each}
        {#each graph.nodes as node}
          <g
            class="communication-node"
            class:selected={selectedName === node.name}
            class:operator={node.name === "Operator"}
            transform={`translate(${node.x},${node.y})`}
          >
            <title
              >{node.name}: {node.sent} sent · {node.received} received</title
            >
            <circle class="node-halo" r="33" /><circle
              class="node-core"
              r="25"
            />
            <text class="node-initial" y="5" text-anchor="middle"
              >{node.name.slice(0, 2).toUpperCase()}</text
            >
            {#each participantLabel(node.name) as line, index}<text
                class="participant-name"
                y={49 + index * 15}
                text-anchor="middle">{line}</text
              >{/each}
          </g>
        {/each}
      </svg>
    </div>
  {:else}
    <div class="empty-state">
      <Network size={30} />
      <h3>No communication recorded</h3>
      <p>The graph appears when participants send messages.</p>
    </div>
  {/if}
  <footer class="communications-legend">
    <span><i></i>Sender → recipient · count on each link</span>
    <p>
      Link width reflects message count. Grouped by participant name; explicit
      recipients only. Persisted traffic, not live activity.
    </p>
  </footer>
</aside>
