<script lang="ts">
  import { focusDialog } from "./lib/dialog";
  import { onMount } from "svelte";
  import {
    Workflow as WorkflowIcon,
    MessageSquare,
    House,
    FileText,
    Plus,
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
    StepDefinition,
  } from "./lib/types";
  import { elapsed, short, number, pending, stepState } from "./lib/format";
  import Status from "./components/Status.svelte";
  import Workflow from "./components/Workflow.svelte";
  import Messages from "./components/Messages.svelte";
  import ExecutionPicker from "./components/ExecutionPicker.svelte";
  import { addressedToOperator, loadSeen, saveSeen } from "./lib/inbox";
  import Reports from "./components/Reports.svelte";
  import Decisions from "./components/Decisions.svelte";
  let executions = $state<Execution[]>([]),
    execution = $state<Execution | null>(null),
    sessions = $state<Session[]>([]),
    messages = $state<Message[]>([]);
  let selected = $state(
      new URLSearchParams(location.search).get("execution") || "",
    ),
    view = $state<"home" | "inbox" | "reports">("home");
  let connected = $state(false),
    connectionError = $state(""),
    lastRefresh = $state(0),
    now = $state(Date.now()),
    loading = $state(true);
  let graphExpanded = $state(false);
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
  let pageVisible = $state(document.visibilityState === "visible");
  let messageRecipient = $state("operator");
  let inboxTab = $state<"messages" | "requests">("messages");
  let seenOperatorMessages = $state<Record<string, string[]>>(loadSeen());
  const incomingOperator = $derived(messages.filter(addressedToOperator));
  const unreadOperator = $derived(
    incomingOperator.filter(
      (message) => !(seenOperatorMessages[selected] ?? []).includes(message.id),
    ).length,
  );
  $effect(() => {
    if (
      !pageVisible ||
      view !== "inbox" ||
      inboxTab !== "messages" ||
      messageRecipient !== "operator" ||
      !selected
    )
      return;
    const seen = seenOperatorMessages[selected] ?? [];
    const unseen = incomingOperator.filter(
      (message) => !seen.includes(message.id),
    );
    if (!unseen.length) return;
    seenOperatorMessages = {
      ...seenOperatorMessages,
      [selected]: [...seen, ...unseen.map((message) => message.id)],
    };
    saveSeen(seenOperatorMessages);
  });
  let stopped = false,
    refreshing = false;
  let abort: AbortController | undefined;
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
      const list = await api.executions("*", signal);
      if (stopped) return;
      list.sort(
        (a, b) =>
          Date.parse(b.created_at) - Date.parse(a.created_at) ||
          b.id.localeCompare(a.id),
      );
      executions = list;
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
    messageRecipient = "operator";
    execution = null;
    sessions = [];
    messages = [];
    selectedStep = null;
    graphExpanded = false;
    const url = new URL(location.href);
    url.searchParams.set("execution", id);
    history.replaceState(null, "", url);
  }
  $effect(() => {
    selected;
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
        "*",
      );
      newExecution = false;
      select(record.id);
      view = "home";
      inboxTab = "messages";
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

<svelte:document
  onvisibilitychange={() =>
    (pageVisible = document.visibilityState === "visible")}
/>
<svelte:head
  ><title>{execution ? execution.workflow + " · " : ""}Pragma Monitor</title
  ></svelte:head
>
<div class="app-shell">
  <main>
    <header class="topbar">
      <a class="brand" href="/">Pragma<span class="brand-sub">MONITOR</span></a>
      <ExecutionPicker {executions} {selected} onselect={select} />
      <nav class="main-nav" aria-label="Main navigation">
        <button class:active={view === "home"} onclick={() => (view = "home")}
          ><House size={16} />Home</button
        >
        <button
          class:active={view === "inbox"}
          onclick={() => {
            view = "inbox";
            inboxTab = "messages";
            messageRecipient = "operator";
          }}
          ><MessageSquare size={16} />Inbox
          {#if unreadOperator}<span
              class="unread-badge"
              aria-label={`${unreadOperator} unread operator messages`}
              >{unreadOperator}</span
            >{/if}
          {#if decisions.length}<span
              class="request-dot"
              title={`${decisions.length} pending requests`}
              aria-label={`${decisions.length} pending requests`}
            ></span>{/if}
        </button>
        <button
          aria-label="Reports"
          aria-current={view === "reports" ? "page" : undefined}
          class:active={view === "reports"}
          onclick={() => (view = "reports")}
          ><FileText size={16} />Reports{#if usage?.tool_failures}<span
              class="report-nav-count"
              aria-hidden="true"
              title={`${usage.tool_failures} recorded tool failures`}
              >{number(usage.tool_failures)}</span
            >{/if}</button
        >
      </nav>
      <span
        class="nav-connection"
        title={connected
          ? `Refreshed ${Math.max(0, Math.floor((now - lastRefresh) / 1000))}s ago`
          : "Retained data may be stale"}
        ><span class="connection-dot" class:online={connected}></span>{connected
          ? "Connected"
          : "Disconnected"}</span
      >
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
    {#if execution}
      {#if view === "home"}<div class="execution-header">
          <div>
            <div class="title-line">
              <h1>{execution.workflow}</h1>
              <Status value={execution.state} />
            </div>
            <p title={execution.brief}>
              {execution.brief || "No brief provided"}
            </p>
            <div class="execution-location">
              <code>{execution.context_id}</code><span>{execution.project}</span
              >
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
                  >running / {sessions.filter((s) => s.runner !== "hook")
                    .length} total</em
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
        {#if usage?.tool_failures}<button
            class="failure-shortcut"
            onclick={() => (view = "reports")}
            ><Activity size={15} /><strong
              >{number(usage.tool_failures)} failed tool calls</strong
            ><span>Inspect execution report →</span></button
          >{/if}
        {#if execution.hooks?.length}<details
            class="workflow-hooks"
            open={execution.hooks.some((hook) =>
              ["failed", "blocked", "interrupted"].includes(hook.state),
            )}
          >
            <summary
              >Lifecycle hooks <span>{execution.hooks.length}</span></summary
            >
            <div class="workflow-hook-list">
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
                </div>{/each}
            </div>
          </details>{/if}
      {/if}
      {#if view === "inbox"}<nav class="inbox-tabs" aria-label="Inbox views">
          <div class="inbox-title">
            <h1>Inbox</h1>
            <small>{execution.workflow} · {short(execution.id)}</small>
          </div>
          <button
            class:active={inboxTab === "messages"}
            onclick={() => (inboxTab = "messages")}
            >Messages<span>{messages.length}</span></button
          >
          <button
            class:active={inboxTab === "requests"}
            onclick={() => (inboxTab = "requests")}
            >Requests{#if decisions.length}<span class="count-alert"
                >{decisions.length}</span
              >{/if}</button
          >
        </nav>{/if}
      {#if execution.error}<div class="error-banner" role="alert">
          {execution.error}
        </div>{/if}
      <div class="view-content">
        {#if view === "home"}<div
            class="workflow-layout"
            class:expanded={graphExpanded}
            class:has-inspector={!!selectedStep}
          >
            <section class="workflow-panel">
              <div class="section-heading">
                <span><WorkflowIcon size={17} />Execution graph</span><small
                  >Pinned configuration · select a step to inspect</small
                >
              </div>
              {#key execution.id}<Workflow
                  {execution}
                  {now}
                  bind:expanded={graphExpanded}
                  onselect={(step) => (selectedStep = step)}
                />{/key}
              <div class="graph-legend">
                <span><i class="running-dot"></i>Running</span><span
                  ><i class="completed-dot"></i>Completed</span
                ><span><i class="waiting-dot"></i>Input required</span><span
                  >Scroll vertically · drag to pan · arrows show configured
                  routes</span
                >
              </div>
            </section>
            {#if selectedStep}<aside class="inspector">
                <div class="section-heading">
                  <span>Step details</span><button
                    class="icon-button"
                    aria-label="Close step details"
                    onclick={() => (selectedStep = null)}
                    ><X size={16} /></button
                  >
                </div>
                <h3>{selectedStep.title || selectedStep.id}</h3>
                <Status value={stepState(execution.state, visit)} />
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
              </aside>{/if}
          </div>
        {:else if view === "reports"}{#key execution.id}<Reports
              executionID={selected}
            />{/key}
        {:else if inboxTab === "messages"}{#key execution.id}<Messages
              {messages}
              {sessions}
              bind:drafts={messageDrafts}
              bind:recipient={messageRecipient}
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
