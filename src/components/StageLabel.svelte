<script lang="ts">
  let { data }: { data: { label: string; policy: string } } = $props();
</script>

<div class="stage-label">
  <strong>{data.label}</strong><span>{data.policy}</span>
</div>
