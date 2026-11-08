<script lang="ts">
  import { focusDialog } from "./lib/dialog";
  import { onMount } from "svelte";
  import {
    Workflow as WorkflowIcon,
    MessageSquare,
    CheckCircle2,
    Plus,
    Search,
    ArrowUpRight,
    RefreshCw,
    ChevronRight,
    Box,
    Bot,
    Clock3,
    Coins,
    Activity,
    X,
    Square,
    RotateCcw,
  } from "@lucide/svelte";
  import { api } from "./lib/api";
  import type {
    Execution,
    Session,
    Message,
    Context,
    StepDefinition,
  } from "./lib/types";
  import { elapsed, short, number, pending } from "./lib/format";
  import Status from "./components/Status.svelte";
  import Workflow from "./components/Workflow.svelte";
  import Messages from "./components/Messages.svelte";
  import Decisions from "./components/Decisions.svelte";
  let executions = $state<Execution[]>([]),
    contexts = $state<Context[]>([]),
    execution = $state<Execution | null>(null),
    sessions = $state<Session[]>([]),
    messages = $state<Message[]>([]);
  let selected = $state(
      new URLSearchParams(location.search).get("execution") || "",
    ),
    context = $state("*"),
    search = $state(""),
    view = $state<"workflow" | "messages" | "decisions">("workflow");
  let connected = $state(false),
    connectionError = $state(""),
    lastRefresh = $state(0),
    now = $state(Date.now()),
    loading = $state(true);
  let selectedStep = $state<StepDefinition | null>(null),
    newExecution = $state(false),
    configPath = $state(""),
    brief = $state(""),
    inputFiles = $state("");
  let action = $state<{
      kind: "retry" | "cancel";
      id: string;
      title: string;
    } | null>(null),
    reason = $state(""),
    busy = $state(false),
    error = $state("");
  let messageDrafts = $state<Record<string, string>>({});
  let messageRecipient = $state("all");
  let stopped = false,
    refreshing = false;
  let abort: AbortController | undefined;
  const visible = $derived(
    executions.filter((e) =>
      `${e.workflow} ${e.brief} ${e.id} ${e.context_id}`
        .toLowerCase()
        .includes(search.toLowerCase()),
    ),
  );
  const decisions = $derived(
    (execution?.requests ?? []).filter((r) => pending(r.state)),
  );
  const visit = $derived(
    (execution?.steps ?? []).filter((s) => s.key === selectedStep?.id).at(-1),
  );
  const usage = $derived(execution?.resources);
  const tokens = $derived(
    usage
      ? usage.tokens.input +
          usage.tokens.output +
          usage.tokens.cache_read +
          usage.tokens.cache_write
      : 0,
  );
  async function refresh() {
    if (refreshing || stopped) return;
    refreshing = true;
    abort = new AbortController();
    const signal = abort.signal;
    try {
      const version = await api.version(signal);
      if (version.api !== "factory-control-v3")
        throw new Error(`Unsupported controller API: ${version.api}`);
      const [list, available] = await Promise.all([
        api.executions(context, signal),
        api.contexts(signal),
      ]);
      if (stopped) return;
      executions = list;
      contexts = available;
      if (!selected && list.length) select(list[0].id);
      const id = selected;
      if (id) {
        const [record, agents, conversation] = await Promise.all([
          api.execution(id, signal),
          api.sessions(id, signal),
          api.messages(id, signal),
        ]);
        if (selected === id) {
          execution = record;
          sessions = agents;
          messages = conversation;
        }
      } else {
        execution = null;
        sessions = [];
        messages = [];
      }
      connected = true;
      connectionError = "";
      lastRefresh = Date.now();
    } catch (e) {
      if (!signal.aborted) {
        connected = false;
        connectionError = (e as Error).message;
      }
    } finally {
      refreshing = false;
      loading = false;
    }
  }
  function select(id: string) {
    selected = id;
    messageRecipient = "all";
    execution = null;
    sessions = [];
    messages = [];
    selectedStep = null;
    const url = new URL(location.href);
    url.searchParams.set("execution", id);
    history.replaceState(null, "", url);
  }
  $effect(() => {
    selected;
    context;
    void refresh();
  });
  onMount(() => {
    stopped = false;
    void refresh();
    const timer = setInterval(() => {
      now = Date.now();
      void refresh();
    }, 2000);
    return () => {
      stopped = true;
      clearInterval(timer);
      abort?.abort();
    };
  });
  async function submit() {
    busy = true;
    error = "";
    try {
      const record = await api.exec(
        configPath.trim(),
        brief,
        inputFiles
          .split("\n")
          .map((x) => x.trim())
          .filter(Boolean),
        context,
      );
      newExecution = false;
      select(record.id);
      void refresh();
    } catch (e) {
      error = (e as Error).message;
    } finally {
      busy = false;
    }
  }
  async function perform() {
    if (!action) return;
    busy = true;
    error = "";
    try {
      if (action.kind === "cancel") await api.cancel(action.id);
      else await api.retry(action.id, reason);
      action = null;
      void refresh();
    } catch (e) {
      error = (e as Error).message;
    } finally {
      busy = false;
    }
  }
  function requestAction(kind: "retry" | "cancel", id: string, title: string) {
    action = { kind, id, title };
    reason = "";
    error = "";
  }
</script>

<svelte:head
  ><title>{execution ? execution.workflow + " · " : ""}Pragma Monitor</title
  ></svelte:head
>
<div class="app-shell">
  <aside class="sidebar">
    <a class="brand" href="/"
      >Pragma<span class="brand-sub"
        >MONITOR</span
      ></a
    >
    <div class="sidebar-section">
      <span class="eyebrow">CONTROL PLANE</span>
      <div class="connection">
        <span class:online={connected} class="connection-dot"></span><strong
          >{connected ? "Connected" : "Disconnected"}</strong
        ><small
          >{connected
            ? `Refreshed ${Math.max(0, Math.floor((now - lastRefresh) / 1000))}s ago`
            : "Retained data may be stale"}</small
        >
      </div>
    </div>
    <div class="sidebar-section execution-index">
      <div class="sidebar-heading">
        <span class="eyebrow">EXECUTIONS</span><button
          class="icon-button"
          aria-label="New execution"
          disabled={!connected}
          onclick={() => {
            newExecution = true;
            error = "";
          }}><Plus size={17} /></button
        >
      </div>
      <label class="search"
        ><Search size={15} /><input
          aria-label="Search executions"
          placeholder="Search executions…"
          bind:value={search}
        /></label
      >
      <select
        aria-label="Filter context"
        bind:value={context}
        onchange={() => select("")}
        ><option value="*">All contexts</option>{#each contexts as c}<option
            value={c.id}>{c.id} · {c.description}</option
          >{/each}</select
      >
      <div class="execution-list">
        {#each visible as item (item.id)}<button
            class="execution-item"
            class:chosen={selected === item.id}
            onclick={() => select(item.id)}
            ><div>
              <strong>{item.workflow}</strong><ChevronRight size={14} />
            </div>
            <p>{item.brief || item.project}</p>
            <footer>
              <Status value={item.state} /><code>{short(item.id)}</code>
            </footer></button
          >{:else}<p class="muted sidebar-empty">
            {loading ? "Connecting…" : "No matching executions"}
          </p>{/each}
      </div>
    </div>
    <div class="sidebar-footer">
      <Box size={15} /><span>Local execution service</span><button
        class="icon-button"
        aria-label="Refresh"
        onclick={() => refresh()}><RefreshCw size={14} /></button
      >
    </div>
  </aside>
  <main>
    <header class="topbar">
      <div class="breadcrumb">
        Executions<ChevronRight size={14} /><strong
          >{execution?.workflow || "Overview"}</strong
        >{#if execution}<code>{short(execution.id)}</code>{/if}
      </div>
      <button
        class="primary"
        disabled={!connected}
        onclick={() => {
          newExecution = true;
          error = "";
        }}><Plus size={15} />New execution</button
      >
    </header>
    {#if connectionError}<div class="error-banner" role="alert">
        <Activity size={16} /><span>{connectionError}</span><button
          onclick={() => refresh()}>Reconnect</button
        >
      </div>{/if}
    {#if execution}<div class="execution-header">
        <div>
          <div class="title-line">
            <h1>{execution.workflow}</h1>
            <Status value={execution.state} />
          </div>
          <p title={execution.brief}>
            {execution.brief || "No brief provided"}
          </p>
          <div class="execution-location">
            <code>{execution.context_id}</code><span>{execution.project}</span>
          </div>
        </div>
        <button
          class="danger-quiet"
          disabled={!connected ||
            ["completed", "cancelled", "failed"].includes(execution.state)}
          onclick={() =>
            requestAction("cancel", execution!.id, "Cancel execution")}
          ><Square size={13} />Cancel execution</button
        >
      </div>
      <div class="metrics">
        <article>
          <span class="metric-icon"><Clock3 size={19} /></span>
          <div>
            <small>Elapsed</small><strong
              >{elapsed(
                execution.created_at,
                execution.finished_at,
                now,
              )}</strong
            >
          </div>
        </article>
        <article>
          <span class="metric-icon"><Bot size={19} /></span>
          <div>
            <small>Agent sessions</small><strong
              >{sessions.filter(
                (s) => s.runner !== "hook" && s.state === "running",
              ).length}<em
                >running / {sessions.filter((s) => s.runner !== "hook").length} total</em
              ></strong
            >
          </div>
        </article>
        <article>
          <span class="metric-icon"><Activity size={19} /></span>
          <div>
            <small>Reported tokens</small><strong
              >{usage?.samples ? number(tokens) : "—"}<em
                >{usage?.unreported_sessions
                  ? "partial coverage"
                  : usage?.samples
                    ? "includes cache tokens"
                    : "not reported"}</em
              ></strong
            >
          </div>
        </article>
        <article>
          <span class="metric-icon"><Coins size={19} /></span>
          <div>
            <small>Reported cost</small><strong
              >{usage &&
              usage.samples > 0 &&
              usage.cost_samples === usage.samples &&
              !usage.unreported_sessions
                ? "$" + usage.cost_usd.toFixed(4)
                : "—"}<em>USD</em></strong
            >
          </div>
        </article>
      </div>
      <nav class="view-tabs" aria-label="Execution views">
        <button
          class:active={view === "workflow"}
          onclick={() => (view = "workflow")}
          ><WorkflowIcon size={17} />Workflow</button
        ><button
          class:active={view === "messages"}
          onclick={() => (view = "messages")}
          ><MessageSquare size={17} />Messages<span>{messages.length}</span
          ></button
        ><button
          class:active={view === "decisions"}
          onclick={() => (view = "decisions")}
          ><CheckCircle2 size={17} />Decisions{#if decisions.length}<span
              class="count-alert">{decisions.length}</span
            >{/if}</button
        >
        <div class="view-spacer"></div>
        <small
          ><span class="connection-dot" class:online={connected}
          ></span>{connected ? "Live refresh · 2s" : "Offline"}</small
        >
      </nav>
      {#if execution.error}<div class="error-banner" role="alert">
          {execution.error}
        </div>{/if}
      <div class="view-content">
        {#if view === "workflow"}<div class="workflow-layout">
            <section class="workflow-panel">
              <div class="section-heading">
                <span><WorkflowIcon size={17} />Execution graph</span><small
                  >Pinned configuration · select a step to inspect</small
                >
              </div>
              {#key execution.id}<Workflow
                  {execution}
                  {now}
                  onselect={(step) => (selectedStep = step)}
                />{/key}
              <div class="graph-legend">
                <span><i class="running-dot"></i>Running</span><span
                  ><i class="completed-dot"></i>Completed</span
                ><span><i class="waiting-dot"></i>Input required</span><span
                  >Connections show configured outcomes</span
                >
              </div>
            </section>
            <aside class="inspector">
              {#if selectedStep}<div class="section-heading">
                  <span>Step details</span><button
                    class="icon-button"
                    aria-label="Close step details"
                    onclick={() => (selectedStep = null)}
                    ><X size={16} /></button
                  >
                </div>
                <h3>{selectedStep.title || selectedStep.id}</h3>
                <Status value={visit?.state || "pending"} />
                <dl>
                  <dt>Step</dt>
                  <dd>{selectedStep.id}</dd>
                  <dt>Package / authority</dt>
                  <dd>
                    {selectedStep.run?.package ||
                      selectedStep.decision?.authority ||
                      "—"}
                  </dd>
                  <dt>Visit / attempt</dt>
                  <dd>{visit ? `${visit.visit} / ${visit.try || 1}` : "—"}</dd>
                  <dt>Elapsed</dt>
                  <dd>{elapsed(visit?.started_at, visit?.finished_at, now)}</dd>
                  <dt>Cumulative tool failures</dt>
                  <dd>{visit?.resources?.tool_failures ?? "—"}</dd>
                </dl>
                {#if visit?.error}<p class="error-text">
                    {visit.error}
                  </p>{/if}{#if visit?.result?.summary}<p class="summary-text">
                    {visit.result.summary}
                  </p>{/if}{#if visit && ["failed", "blocked", "interrupted"].includes(visit.state)}<button
                    disabled={!connected}
                    onclick={() =>
                      requestAction("retry", visit!.id, "Retry step")}
                    ><RotateCcw size={14} />Retry step</button
                  >{/if}
                <h4>Visits</h4>
                {#each execution.steps?.filter((s) => s.key === selectedStep?.id) ?? [] as v}<div
                    class="visit-row"
                  >
                    <code>{short(v.id)}</code><span
                      >#{v.visit} / {v.try || 1}</span
                    ><Status value={v.state} />
                  </div>{/each}
              {:else}<div class="section-heading">
                  <span>Operator inbox</span><small
                    >{decisions.length} pending</small
                  >
                </div>
                {#each decisions as d}<button
                    class="inbox-request"
                    onclick={() => (view = "decisions")}
                    ><CheckCircle2 size={17} /><span
                      >{d.prompt}<small
                        >Review request<ArrowUpRight size={12} /></small
                      ></span
                    ></button
                  >{:else}<div class="quiet-empty">
                    <CheckCircle2 size={24} />
                    <p>No input required</p>
                    <small>Workflow requests will appear here.</small>
                  </div>{/each}
                <div class="section-heading agents-heading">
                  <span>Agent sessions</span>
                </div>
                {#each sessions.filter((s) => s.runner !== "hook") as agent}<button
                    class="agent-row"
                    onclick={() => {
                      messageRecipient = agent.id;
                      view = "messages";
                    }}
                    ><Bot size={16} /><span
                      ><strong>{agent.name}</strong><small
                        >{short(agent.id)} · PID {agent.pid || "—"}</small
                      ></span
                    ><Status value={agent.state} /></button
                  >{:else}<p class="muted">
                    No agent sessions yet.
                  </p>{/each}{/if}
              {#if execution.hooks?.length}<h4>Lifecycle hooks</h4>
                {#each execution.hooks as hook}<div class="hook-row">
                    <code>{short(hook.id)}</code><Status
                      value={hook.state}
                    />{#if hook.error}<p class="error-text">
                        {hook.error}
                      </p>{/if}{#if ["failed", "blocked", "interrupted"].includes(hook.state)}<button
                        disabled={!connected}
                        onclick={() =>
                          requestAction("retry", hook.id, "Retry hook")}
                        >Retry hook</button
                      >{/if}
                  </div>{/each}{/if}
            </aside>
          </div>
        {:else if view === "messages"}{#key execution.id}<Messages
              {messages}
              {sessions}
              bind:drafts={messageDrafts}
              initialRecipient={messageRecipient}
              executionID={execution.id}
              disabled={!connected}
              {refresh}
            />{/key}
        {:else}{#key execution.id}<Decisions
              {execution}
              disabled={!connected}
              {refresh}
            />{/key}{/if}
      </div>
    {:else}<div class="empty-state welcome">
        <span class="welcome-icon"><WorkflowIcon size={36} /></span>
        <h1>{loading ? "Connecting to Controls" : "Execution monitor"}</h1>
        <p>Select an execution or start one from an existing factory.yaml.</p>
        <button
          class="primary"
          disabled={!connected}
          onclick={() => (newExecution = true)}
          ><Plus size={16} />New execution</button
        >
      </div>{/if}
  </main>
</div>
{#if newExecution}<div class="modal-backdrop">
    <div
      class="modal"
      role="dialog"
      aria-modal="true"
      use:focusDialog={() => {
        if (!busy) newExecution = false;
      }}
      aria-labelledby="new-title"
      tabindex="-1"
    >
      <header>
        <h2 id="new-title">New execution</h2>
        <button
          class="icon-button"
          aria-label="Close new execution"
          disabled={busy}
          onclick={() => (newExecution = false)}><X size={18} /></button
        >
      </header>
      <form
        onsubmit={(e) => {
          e.preventDefault();
          void submit();
        }}
      >
        <label for="config">Factory configuration</label><input
          id="config"
          bind:value={configPath}
          placeholder="/absolute/path/to/project/factory.yaml"
          required
          pattern="/.*"
        /><small
          >Absolute path on the Controls host. This selects the project.</small
        ><label for="brief">Context / brief</label><textarea
          id="brief"
          bind:value={brief}
          rows="6"
          placeholder="Describe the work to execute…"></textarea><label
          for="files">Input files (one absolute path per line)</label
        ><textarea
          id="files"
          bind:value={inputFiles}
          rows="3"
          placeholder="/path/to/request.md"></textarea><small
          >Existing files on the Controls host; copied and pinned by the
          controller.</small
        >{#if error}<p class="error-text" role="alert">{error}</p>{/if}
        <footer>
          <button
            type="button"
            disabled={busy}
            onclick={() => (newExecution = false)}>Cancel</button
          ><button
            class="primary"
            disabled={busy ||
              !connected ||
              !configPath.trim() ||
              (!brief.trim() && !inputFiles.trim())}
            >{busy ? "Submitting…" : "Start execution"}</button
          >
        </footer>
      </form>
    </div>
  </div>{/if}
{#if action}<div class="modal-backdrop">
    <div
      class="modal"
      role="dialog"
      aria-modal="true"
      use:focusDialog={() => {
        if (!busy) action = null;
      }}
      aria-labelledby="action-title"
      tabindex="-1"
    >
      <header>
        <h2 id="action-title">{action.title}</h2>
        <button
          class="icon-button"
          aria-label="Close action"
          disabled={busy}
          onclick={() => (action = null)}><X size={18} /></button
        >
      </header>
      <p><code>{action.id}</code></p>
      <form
        onsubmit={(e) => {
          e.preventDefault();
          void perform();
        }}
      >
        {#if action.kind === "retry"}<label for="action-reason">Reason</label
          ><textarea id="action-reason" bind:value={reason} rows="4"
          ></textarea>{:else}<p>
            This stops this execution's work. Other executions remain running.
          </p>{/if}{#if error}<p class="error-text" role="alert">
            {error}
          </p>{/if}
        <footer>
          <button type="button" disabled={busy} onclick={() => (action = null)}
            >Back</button
          ><button
            class:danger={action.kind === "cancel"}
            class="primary"
            disabled={busy || !connected}
            >{busy ? "Submitting…" : "Confirm " + action.kind}</button
          >
        </footer>
      </form>
    </div>
  </div>{/if}
