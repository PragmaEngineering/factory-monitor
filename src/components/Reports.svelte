<script lang="ts">
  import { api, type ReportDocument } from "../lib/api";
  import { FileText, Download, RefreshCw } from "@lucide/svelte";
  let { executionID }: { executionID: string } = $props();
  let document = $state<ReportDocument | null>(null);
  let loading = $state(false),
    error = $state(""),
    revision = $state(0);
  const selectedID = $derived(executionID);
  $effect(() => {
    const id = selectedID;
    revision;
    const controller = new AbortController();
    loading = true;
    error = "";
    document = null;
    api
      .report(id, controller.signal)
      .then((value) => {
        if (!controller.signal.aborted) document = value;
      })
      .catch((cause) => {
        if (!controller.signal.aborted)
          error =
            cause instanceof Error ? cause.message : "Report generation failed";
      })
      .finally(() => {
        if (!controller.signal.aborted) loading = false;
      });
    return () => controller.abort();
  });
  function download(format: "html" | "json") {
    if (!document) return;
    const content =
      format === "html" ? document.html : JSON.stringify(document, null, 2);
    const url = URL.createObjectURL(
      new Blob([content], {
        type: format === "html" ? "text/html" : "application/json",
      }),
    );
    const link = window.document.createElement("a");
    link.href = url;
    link.download = `execution-report-${document.id}.${format}`;
    link.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  }
</script>

<section class="reports-panel" aria-label="Execution reports">
  <header class="report-toolbar">
    <div>
      <h1><FileText size={21} />Reports</h1>
      <p>Execution evidence and factory history · immutable snapshot</p>
    </div>
    <div class="report-actions">
      <button disabled={loading} onclick={() => revision++}
        ><RefreshCw size={15} />{loading
          ? "Generating…"
          : "Refresh report"}</button
      >
      <button disabled={!document} onclick={() => download("html")}
        ><Download size={15} />HTML</button
      >
      <button disabled={!document} onclick={() => download("json")}
        ><Download size={15} />JSON</button
      >
    </div>
  </header>
  {#if error}<div class="error-banner" role="alert">{error}</div>{/if}
  {#if loading}<p role="status">Generating execution report…</p>{/if}
  {#if document}<p class="snapshot-identity">
      Snapshot <code>{document.id}</code> · {document.snapshot.as_of}. Refresh
      creates a new snapshot; saved reports stay unchanged.
    </p>
    <iframe
      title="Execution report"
      sandbox=""
      referrerpolicy="no-referrer"
      srcdoc={`<meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; img-src data:;">${document.html}`}
    ></iframe>
  {/if}
</section>

<style>
  .reports-panel {
    min-width: 0;
  }
  .report-toolbar,
  .report-actions {
    display: flex;
    align-items: center;
    gap: 10px;
    flex-wrap: wrap;
  }
  .report-toolbar {
    justify-content: space-between;
    margin-bottom: 16px;
  }
  h1 {
    display: flex;
    align-items: center;
    gap: 8px;
    margin: 0;
  }
  p {
    color: var(--muted, #64748b);
    font-size: 13px;
  }
  .snapshot-identity {
    overflow-wrap: anywhere;
  }
  iframe {
    display: block;
    width: 100%;
    min-height: 75vh;
    border: 1px solid var(--line, #d5dce5);
    border-radius: 8px;
    background: white;
  }
</style>
