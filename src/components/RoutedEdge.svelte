<script lang="ts">
  import { BaseEdge, type EdgeProps, type Edge } from "@xyflow/svelte";
  import type { RouteGeometry } from "../lib/layout";
  type RoutedData = RouteGeometry & Record<string, unknown>;
  let {
    id,
    data,
    style,
    label,
    labelStyle,
    markerEnd,
  }: EdgeProps<Edge<RoutedData>> = $props();
  const path = $derived(
    data?.points
      .map((point, index) => `${index ? "L" : "M"} ${point.x} ${point.y}`)
      .join(" ") ?? "",
  );
</script>

<BaseEdge
  {id}
  {path}
  {style}
  {label}
  {labelStyle}
  {markerEnd}
  labelX={data?.label.x}
  labelY={data?.label.y}
/>
