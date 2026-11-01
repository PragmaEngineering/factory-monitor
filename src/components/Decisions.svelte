<script lang="ts">
  import { focusDialog } from "../lib/dialog";
  import { FileText, CheckCircle2, Copy, X } from "@lucide/svelte";
  import type { Execution, Decision } from "../lib/types";
  import { api } from "../lib/api";
  import { pending, time } from "../lib/format";
  let {
    execution,
    disabled,
    refresh,
  }: { execution: Execution; disabled: boolean; refresh: () => void } =
    $props();
  let selected = $state<Decision | null>(null);
  let outcome = $state("");
  let reason = $state("");
  let busy = $state(false);
  let error = $state("");
  let document = $state<{ name: string; path: string; content: string } | null>(
    null,
  );
  let loading = $state(false);
  let notice = $state("");
  const requests = $derived(execution.requests ?? []);
  function documentNames(request: Decision): string[] {
    const visit = execution.steps?.find(
      (visit) => visit.id === request.step_id,
    );
    const definition = execution.configuration.workflow.steps.find(
      (step) => step.id === visit?.key,
    );
    const completed = new Set(
      execution.steps
        ?.filter((visit) => visit.state === "completed")
        .map((visit) => visit.key),
    );
    return [
      ...new Set([
        ...(definition?.decision?.artifacts ?? []),
        ...execution.configuration.workflow.steps
          .filter((step) => completed.has(step.id))
          .flatMap((step) => step.outputs ?? []),
      ]),
    ];
  }
  function choose(request: Decision) {
    selected = request;
    outcome = "";
    reason = "";
    error = "";
  }
  async function answer() {
    if (!selected || !outcome) return;
    busy = true;
    error = "";
    try {
      await api.answer(selected.id, outcome, reason);
      selected = null;
      refresh();
    } catch (e) {
      error = (e as Error).message;
    } finally {
      busy = false;
    }
  }
  async function open(name: string) {
    loading = true;
    error = "";
    try {
      document = await api.document(execution.id, name);
    } catch (e) {
      error = (e as Error).message;
    } finally {
      loading = false;
    }
  }
  async function copy(value: string) {
    try {
      await navigator.clipboard.writeText(value);
      notice = "Location copied";
    } catch {
      error = "Clipboard unavailable. Select and copy the location shown.";
    }
  }
</script>

<section class="decision-page">
  <div class="page-intro">
    <h2>Decisions</h2>
    <p>
      Requests from the workflow and its participants. Only the configured
      outcomes are available.
    </p>
  </div>
  {#if error}<div class="error-banner" role="alert">{error}</div>{/if}
  {#each requests as request (request.id)}<article class="decision-card">
      <div class="section-heading">
        <span
          ><CheckCircle2 size={18} />{request.kind === "step_input"
            ? "Agent input request"
            : "Workflow decision"}</span
        ><span class="status {request.state}">{request.state}</span>
      </div>
      <p class="decision-prompt">{request.prompt}</p>
      <small
        >{request.owner || request.step_id} · {time(request.created_at)}</small
      >
      <div class="document-links">
        {#each documentNames(request) as name}<button
            disabled={loading}
            onclick={() => open(name)}><FileText size={15} />{name}</button
          >{/each}
      </div>
      {#if pending(request.state)}<button
          class="primary"
          {disabled}
          onclick={() => choose(request)}>Review and respond</button
        >{:else}<p class="muted">
          {request.answer}{request.reason ? ` · ${request.reason}` : ""}
        </p>{/if}
    </article>{:else}<div class="empty-state">
      <CheckCircle2 size={30} />
      <h3>No decision requests</h3>
      <p>This workflow has not requested operator input.</p>
    </div>{/each}
</section>
{#if selected}<div class="modal-backdrop">
    <div
      class="modal"
      role="dialog"
      aria-modal="true"
      use:focusDialog={() => {
        if (!busy) selected = null;
      }}
      aria-labelledby="decision-title"
      tabindex="-1"
    >
      <header>
        <h2 id="decision-title">Respond to request</h2>
        <button
          class="icon-button"
          aria-label="Close decision"
          disabled={busy}
          onclick={() => (selected = null)}><X size={18} /></button
        >
      </header>
      <p class="decision-prompt">{selected.prompt}</p>
      <form
        onsubmit={(e) => {
          e.preventDefault();
          void answer();
        }}
      >
        <label for="outcome">Outcome</label><select
          id="outcome"
          bind:value={outcome}
          required
          ><option value="" disabled>Select an outcome</option
          >{#each selected.options as option}<option value={option}
              >{option}</option
            >{/each}</select
        ><label for="reason">Reason / feedback</label><textarea
          id="reason"
          bind:value={reason}
          rows="5"></textarea>{#if error}<p class="error-text" role="alert">
            {error}
          </p>{/if}
        <footer>
          <button
            type="button"
            disabled={busy}
            onclick={() => (selected = null)}>Cancel</button
          ><button class="primary" disabled={busy || disabled || !outcome}
            >{busy ? "Submitting…" : "Submit decision"}</button
          >
        </footer>
      </form>
    </div>
  </div>{/if}
{#if document}<div class="modal-backdrop">
    <div
      class="modal document-modal"
      role="dialog"
      aria-modal="true"
      use:focusDialog={() => (document = null)}
      aria-label="Execution document"
      tabindex="-1"
    >
      <header>
        <div>
          <h2>{document.name}</h2>
          <small>{document.path}</small>
        </div>
        <button
          class="icon-button"
          aria-label="Close document"
          onclick={() => (document = null)}><X size={18} /></button
        >
      </header>
      <div class="document-toolbar">
        <button onclick={() => copy(document!.path)}
          ><Copy size={14} />Copy location</button
        ><span>{notice}</span>
      </div>
      {#if document.name.endsWith(".html")}<iframe
          title={document.name}
          srcdoc={`<meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; img-src data:;">${document.content}`}
          sandbox=""
          referrerpolicy="no-referrer"
        ></iframe>{:else}<pre>{document.content}</pre>{/if}
    </div>
  </div>{/if}
