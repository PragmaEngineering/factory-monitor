<script lang="ts">
  let { value }: { value: string } = $props();
</script>

<span class="status {value}"
  ><span class="status-dot"></span>{value.replaceAll("_", " ")}</span
>
