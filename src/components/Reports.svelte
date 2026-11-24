<script lang="ts">
  import { api, type ReportDocument } from "../lib/api";
  import {
    evidence,
    footprint,
    utilization,
    failureDuration,
  } from "../lib/reports";
  import {
    FileText,
    Download,
    RefreshCw,
    AlertTriangle,
    Search,
    ArrowUpRight,
    Activity,
    Check,
    Copy,
  } from "@lucide/svelte";
  import { short, number } from "../lib/format";
  let { executionID }: { executionID: string } = $props();
  let document = $state<ReportDocument | null>(null);
  let loading = $state(false),
    error = $state(""),
    revision = $state(0);
  let tab = $state<"failures" | "context" | "full">("failures");
  let query = $state(""),
    agentID = $state(""),
    active = $state(0),
    copied = $state(false);
  const selectedID = $derived(executionID);
  const data = $derived(document ? evidence(document) : null);
  const filtered = $derived(
    (data?.failures ?? []).filter(
      ({ agent, call }) =>
        (!agentID || agent.session_id === agentID) &&
        `${agent.name} ${agent.step_id} ${call.tool_id} ${call.name ?? ""} ${call.command ?? ""} ${call.error ?? ""} ${call.output ?? ""}`
          .toLowerCase()
          .includes(query.toLowerCase()),
    ),
  );
  const chosen = $derived(filtered[active] ?? filtered[0]);
  const agents = $derived(
    (data?.agents ?? []).filter((a) => !agentID || a.session_id === agentID),
  );
  const preview = $derived(
    document?.html.replace(
      /<head>/i,
      `<head><meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; img-src data:;">`,
    ) ?? "",
  );
  function time(value?: string) {
    return !value || value.startsWith("0001-")
      ? "Unknown"
      : new Date(value).toLocaleString();
  }
  $effect(() => {
    const id = selectedID;
    revision;
    const controller = new AbortController();
    loading = true;
    error = "";
    document = null;
    active = 0;
    query = "";
    agentID = "";
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
  async function copyDiagnosis() {
    if (!chosen || !document) return;
    try {
      await navigator.clipboard.writeText(
        JSON.stringify(
          {
            report_id: document.id,
            execution_id: executionID,
            session_id: chosen.agent.session_id,
            step_id: chosen.agent.step_id,
            agent: chosen.agent.name,
            ...chosen.call,
          },
          null,
          2,
        ),
      );
      copied = true;
    } catch {
      error = "Could not copy diagnosis. Download the JSON report instead.";
    }
  }
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
      <div class="eyebrow">EXECUTION EVIDENCE</div>
      <h1>Reports</h1>
      <p>Inspect failed calls, context usage and recorded outcomes.</p>
    </div>
    <div class="report-actions">
      <button disabled={loading} onclick={() => revision++}
        ><RefreshCw size={14} />{loading
          ? "Generating…"
          : "Refresh report"}</button
      >
      <button disabled={!document} onclick={() => download("html")}
        ><Download size={14} />HTML</button
      >
      <button disabled={!document} onclick={() => download("json")}
        ><Download size={14} />JSON</button
      >
    </div>
  </header>
  {#if error}<div class="report-error" role="alert">
      <AlertTriangle size={18} />
      <div>
        <strong>Report unavailable</strong>
        <p>
          {error === "Unknown monitor operation"
            ? "This Monitor server does not support reports. Rebuild and restart Monitor to load its report endpoint."
            : error}
        </p>
      </div>
    </div>{/if}
  {#if loading}<div class="report-empty" role="status">
      <RefreshCw size={24} />
      <h2>Preparing execution evidence</h2>
      <p>Reading a consistent snapshot of this execution.</p>
    </div>{/if}
  {#if document && data}
    <div class="report-summary">
      <button
        class="summary-card failure-metric"
        onclick={() => (tab = "failures")}
        ><span><AlertTriangle size={16} />Recorded tool failures</span><strong
          >{data.recorded === undefined ? "—" : number(data.recorded)}</strong
        ><small
          >{data.failures.length} calls with diagnostic evidence<ArrowUpRight
            size={13}
          /></small
        ></button
      >
      <button class="summary-card" onclick={() => (tab = "context")}
        ><span><Activity size={16} />Peak context utilization</span><strong
          >{data.peak === undefined ? "—" : `${data.peak.toFixed(1)}%`}</strong
        ><small
          >{data.responses.length} response {data.responses.length === 1
            ? "record"
            : "records"}<ArrowUpRight size={13} /></small
        ></button
      >
      <button class="summary-card" onclick={() => (tab = "context")}
        ><span><FileText size={16} />Completed native turns</span><strong
          >{data.boundaries.some((b) => b.kind.startsWith("turn_"))
            ? number(data.turns)
            : "—"}</strong
        ><small
          >{document.snapshot.selected?.resources?.samples ?? "—"} reported model
          responses<ArrowUpRight size={13} /></small
        ></button
      >
    </div>
    <nav class="report-tabs" aria-label="Report views">
      <button
        class:active={tab === "failures"}
        aria-pressed={tab === "failures"}
        onclick={() => (tab = "failures")}
        >Failed tool calls<span class="failure-count"
          >{data.recorded ?? "—"}</span
        ></button
      >
      <button
        class:active={tab === "context"}
        aria-pressed={tab === "context"}
        onclick={() => (tab = "context")}>Context &amp; turns</button
      >
      <button
        class:active={tab === "full"}
        aria-pressed={tab === "full"}
        onclick={() => (tab = "full")}>Full report</button
      >
    </nav>
    {#if tab !== "full"}
      <div class="report-filters">
        {#if tab === "failures"}<label class="filter-search"
            ><Search size={15} /><input
              aria-label="Search failed tool calls"
              placeholder="Search tool, command or error…"
              bind:value={query}
              oninput={() => {
                active = 0;
                copied = false;
              }}
            /></label
          >{/if}
        <select
          aria-label="Filter report by agent"
          bind:value={agentID}
          onchange={() => {
            active = 0;
            copied = false;
          }}
          ><option value="">All agent sessions</option
          >{#each data.agents as agent}<option value={agent.session_id}
              >{agent.name} · {short(agent.session_id)}</option
            >{/each}</select
        >
      </div>
    {/if}
    {#if tab === "failures"}
      {#if data.missing || (data.recorded === undefined && !data.failures.length)}<div
          class="coverage-note"
        >
          <AlertTriangle size={18} />
          <div>
            <strong
              >{data.recorded === undefined
                ? "Call-level evidence unavailable"
                : `${data.missing} recorded ${data.missing === 1 ? "failure has" : "failures have"} no diagnostic evidence`}</strong
            >
            <p>
              {#if document.snapshot.version !== undefined && document.snapshot.version < 2}Controls
                returned an older report format. Update Controls to capture call
                diagnostics for new sessions. Existing sessions may still have
                counts only.{:else}Only captured calls are listed below. Missing
                details may still be available in retained session output.{/if}
            </p>
          </div>
        </div>{/if}
      {#if filtered.length}
        <div class="failure-workspace">
          <section class="failure-list" aria-label="Failed tool calls">
            <div class="list-caption">{filtered.length} captured calls</div>
            {#each filtered as item, i (item.agent.session_id + item.call.tool_id)}<button
                class:selected={chosen === item}
                aria-pressed={chosen === item}
                onclick={() => {
                  active = i;
                  copied = false;
                }}
                ><span class="failure-row-title"
                  ><strong>{item.call.name || "Tool"}</strong><span
                    class="exit-code"
                    >{item.call.exit_code === undefined
                      ? "Failed"
                      : `Exit ${item.call.exit_code}`}</span
                  ></span
                ><code>{item.call.command || item.call.tool_id}</code><small
                  >{item.agent.name} · {time(item.call.finished_at)}</small
                ></button
              >{/each}
          </section>
          {#if chosen}<article
              class="failure-detail"
              aria-label="Failed call details"
            >
              <header>
                <div>
                  <div class="eyebrow">FAILED TOOL CALL</div>
                  <h2>{chosen.call.name || "Tool execution"}</h2>
                  <p>{chosen.agent.name} · {chosen.agent.step_id}</p>
                </div>
                <button onclick={copyDiagnosis}
                  >{#if copied}<Check size={14} />Copied{:else}<Copy
                      size={14}
                    />Copy diagnosis{/if}</button
                >
              </header>
              <dl class="call-metadata">
                <div>
                  <dt>Call ID</dt>
                  <dd><code>{chosen.call.tool_id}</code></dd>
                </div>
                <div>
                  <dt>Session</dt>
                  <dd><code>{chosen.agent.session_id}</code></dd>
                </div>
                <div>
                  <dt>Started</dt>
                  <dd>{time(chosen.call.started_at)}</dd>
                </div>
                <div>
                  <dt>Duration / exit</dt>
                  <dd>
                    {failureDuration(chosen.call)} · {chosen.call.exit_code ??
                      "Unknown exit code"}
                  </dd>
                </div>
                <div>
                  <dt>Finished</dt>
                  <dd>{time(chosen.call.finished_at)}</dd>
                </div>
                <div>
                  <dt>Native turn</dt>
                  <dd>{chosen.call.turn ?? "Unknown"}</dd>
                </div>
              </dl>
              {#if chosen.call.truncated}<p class="coverage-note">
                  Evidence was truncated during capture. Retained session output
                  may contain more detail.
                </p>{/if}
              <h3>Command</h3>
              <pre>{chosen.call.command || "Not captured"}</pre>
              <h3>Arguments</h3>
              <pre>{chosen.call.arguments || "Not captured"}</pre>
              <h3>Returned output</h3>
              <pre class="error-output">{chosen.call.output ||
                  "No output captured"}</pre>
              <h3>Error / structured details</h3>
              <pre>{chosen.call.error || "No additional details captured"}</pre>
            </article>{/if}
        </div>
      {:else}<div class="report-empty">
          <AlertTriangle size={26} />
          <h2>
            {data.failures.length
              ? "No calls match your filters"
              : data.recorded === 0
                ? "No tool failures recorded"
                : "No failed-call details available"}
          </h2>
          <p>
            {data.failures.length
              ? "Try another agent or search term."
              : "Any captured failures will appear here with their command, arguments and returned diagnostics."}
          </p>
        </div>{/if}
    {:else if tab === "context"}
      <p class="context-method">
        Context footprint is measured per completed response, including cache
        and output tokens. It is separate from cumulative spend. Missing
        capacities and native turn boundaries remain unknown.
      </p>
      {#each agents as agent}<section class="context-session">
          <header>
            <div>
              <h2>{agent.name}</h2>
              <p>
                Session <code>{agent.session_id}</code> · Step
                <code>{agent.step_id}</code>
              </p>
            </div>
            <span
              >{agent.diagnostics?.responses?.length ?? 0} response records</span
            >
          </header>
          <div class="report-table">
            <table>
              <thead
                ><tr
                  ><th>Response / turn</th><th>Model</th><th>Input</th><th
                    >Cache read / write</th
                  ><th>Output</th><th>Footprint / capacity</th><th
                    >Utilization</th
                  ><th>Stop reason</th></tr
                ></thead
              ><tbody
                >{#each agent.diagnostics?.responses ?? [] as response}<tr
                    ><td
                      >#{response.number} / {response.turn ?? "—"}<small
                        >{time(response.at)}</small
                      ></td
                    ><td
                      >{response.provider ?? "—"}<small
                        >{response.model ?? "—"}</small
                      ></td
                    ><td>{number(response.tokens.input)}</td><td
                      >{number(response.tokens.cache_read)} / {number(
                        response.tokens.cache_write,
                      )}</td
                    ><td>{number(response.tokens.output)}</td><td
                      >{number(footprint(response))} / {response.context_window
                        ? number(response.context_window)
                        : "Unknown"}</td
                    ><td
                      >{#if utilization(response) !== undefined}<span
                          class="context-percent"
                          >{utilization(response)!.toFixed(1)}%</span
                        ><meter
                          min="0"
                          max="100"
                          value={Math.min(100, utilization(response)!)}
                          aria-label={`Response ${response.number} context utilization`}
                        ></meter>{:else}Unknown{/if}</td
                    ><td
                      >{response.stop_reason || "—"}{#if response.error}<small
                          >{response.error}</small
                        >{/if}</td
                    ></tr
                  >{:else}<tr
                    ><td colspan="8"
                      >Per-response context evidence was not captured for this
                      session.</td
                    ></tr
                  >{/each}</tbody
              >
            </table>
          </div>
          <details class="boundary-details">
            <summary
              >Turn &amp; compaction timeline<span
                >{agent.diagnostics?.boundaries?.length ?? 0} events</span
              ></summary
            >
            <ol>
              {#each agent.diagnostics?.boundaries ?? [] as boundary}<li>
                  <time>{time(boundary.at)}</time><strong
                    >{boundary.kind.replaceAll("_", " ")}{boundary.turn
                      ? ` · turn ${boundary.turn}`
                      : ""}</strong
                  ><span
                    >{boundary.reason ?? ""}{boundary.aborted
                      ? " · aborted"
                      : ""}{boundary.tokens_before === undefined
                      ? ""
                      : ` · ${number(boundary.tokens_before)} tokens before`}{boundary.estimated_tokens_after ===
                    undefined
                      ? ""
                      : ` · ~${number(boundary.estimated_tokens_after)} after (estimated)`}{boundary.error
                      ? ` · ${boundary.error}`
                      : ""}</span
                  >
                </li>{:else}<li>No native boundary events captured.</li>{/each}
            </ol>
          </details>
        </section>{:else}<div class="report-empty">
          <h2>No agent sessions in this snapshot</h2>
          <p>
            Context evidence is shown when it is recorded by an agent runtime.
          </p>
        </div>{/each}
    {:else}<iframe
        title="Execution report"
        sandbox=""
        referrerpolicy="no-referrer"
        srcdoc={preview}
      ></iframe>{/if}
    <footer class="snapshot-footer">
      <span>Snapshot captured {time(document.snapshot.as_of)}</span>
      <details>
        <summary>Snapshot details</summary><code>{document.id}</code>
        <p>Saved reports stay unchanged. Refresh creates a new snapshot.</p>
      </details>
    </footer>
  {/if}
</section>

<style>
  .reports-panel {
    min-width: 0;
    max-width: 1600px;
    width: 100%;
    padding: 28px 32px;
    margin: 0 auto;
  }
  .report-toolbar,
  .report-actions {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
  }
  .report-toolbar {
    justify-content: space-between;
    gap: 16px;
    margin: 4px 0 24px;
  }
  h1 {
    margin: 6px 0;
    font-size: 27px;
  }
  p {
    font-size: 13px;
    color: #65736a;
    margin: 0;
  }
  .report-actions button {
    font-size: 12px;
  }
  .report-summary {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 14px;
    margin-bottom: 25px;
  }
  .summary-card {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    align-items: center;
    text-align: left;
    gap: 10px 12px;
    padding: 16px 18px;
    border-radius: 10px;
  }
  .summary-card > span {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 12px;
    color: #647267;
  }
  .summary-card strong {
    grid-column: 2;
    grid-row: 1 / 3;
    font-size: 28px;
    font-weight: 600;
    line-height: 1;
    letter-spacing: -1px;
  }
  .summary-card small {
    display: flex;
    align-items: center;
    justify-content: space-between;
    color: #65736a;
  }
  .failure-metric strong {
    color: #a44835;
  }
  .report-tabs {
    display: flex;
    gap: 22px;
    border-bottom: 1px solid var(--line);
    margin-bottom: 20px;
    overflow-x: auto;
  }
  .report-tabs button {
    border: 0;
    border-bottom: 2px solid transparent;
    border-radius: 0;
    background: transparent;
    padding: 0 0 13px;
    color: #6c7a70;
    white-space: nowrap;
  }
  .report-tabs button.active {
    color: #254e36;
    border-bottom-color: #326f52;
    font-weight: 600;
  }
  .failure-count {
    padding: 2px 7px;
    background: #faf0eb;
    color: #a44835;
    border-radius: 5px;
    font-size: 11px;
  }
  .report-filters {
    display: flex;
    gap: 12px;
    margin: 0 0 16px;
  }
  .filter-search {
    position: relative;
    flex: 1;
    max-width: 440px;
  }
  .filter-search :global(svg) {
    position: absolute;
    top: 12px;
    left: 12px;
    color: #859087;
  }
  .filter-search input {
    padding: 10px 12px 10px 36px;
    font-size: 12px;
  }
  .report-filters select {
    width: auto;
    max-width: 330px;
    font-size: 12px;
    padding: 9px 30px 9px 12px;
  }
  .coverage-note,
  .report-error {
    display: flex;
    align-items: flex-start;
    gap: 11px;
    border: 1px solid #efdfbb;
    background: #fffaf0;
    border-radius: 8px;
    padding: 14px 16px;
    margin: 0 0 18px;
    font-size: 12px;
    color: #805b23;
  }
  .coverage-note :global(svg),
  .report-error :global(svg) {
    flex-shrink: 0;
  }
  .coverage-note p,
  .report-error p {
    margin-top: 4px;
    font-size: 12px;
    color: inherit;
    max-width: 90ch;
  }
  .report-error {
    border-color: #edcfc6;
    background: #fff6f2;
    color: #984332;
  }
  .failure-workspace {
    display: grid;
    grid-template-columns: minmax(240px, 29%) minmax(0, 1fr);
    border: 1px solid var(--line);
    border-radius: 10px;
    overflow: hidden;
    align-items: start;
  }
  .failure-list {
    min-width: 0;
    max-height: 780px;
    overflow-y: auto;
    background: #fafbf9;
    border-right: 1px solid var(--line);
  }
  .list-caption {
    padding: 13px 16px;
    color: #7a857c;
    font-size: 11px;
    border-bottom: 1px solid var(--line);
  }
  .failure-list button {
    display: flex;
    flex-direction: column;
    gap: 9px;
    align-items: stretch;
    text-align: left;
    border: 0;
    border-radius: 0;
    border-bottom: 1px solid var(--line);
    background: transparent;
    padding: 17px 16px;
    width: 100%;
  }
  .failure-list button.selected {
    background: #edf4ee;
    box-shadow: inset 3px 0 #326f52;
  }
  .failure-row-title {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 10px;
  }
  .failure-row-title strong {
    font-size: 13px;
  }
  .exit-code {
    font-size: 10px;
    color: #a44835;
    background: #f9ece7;
    border-radius: 4px;
    padding: 3px 6px;
  }
  .failure-list code {
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
    line-height: 1.6;
  }
  .failure-list small {
    font-size: 10px;
    color: #77847a;
  }
  .failure-detail {
    padding: 24px 26px;
    min-width: 0;
  }
  .failure-detail header {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    gap: 12px;
    margin-bottom: 21px;
  }
  .failure-detail h2 {
    font-size: 20px;
    margin: 5px 0;
  }
  .failure-detail header button {
    font-size: 11px;
    white-space: nowrap;
  }
  .call-metadata {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 15px 22px;
    padding: 17px 0;
    margin: 0 0 22px;
    border-block: 1px solid var(--line);
  }
  dt {
    font-size: 10px;
    color: #819086;
    text-transform: uppercase;
    letter-spacing: 0.6px;
    margin-bottom: 5px;
  }
  dd {
    margin: 0;
    font-size: 12px;
    overflow-wrap: anywhere;
  }
  .failure-detail h3 {
    font-size: 11px;
    margin: 21px 0 9px;
    color: #65756a;
    font-weight: 600;
  }
  pre {
    margin: 0;
    padding: 13px 15px;
    background: #f7f9f6;
    border: 1px solid #e7ece5;
    border-radius: 6px;
    font-size: 11px;
    max-height: 360px;
    overflow: auto;
  }
  .error-output {
    background: #fdf9f6;
    border-color: #eee3db;
  }
  .report-empty {
    display: flex;
    flex-direction: column;
    align-items: center;
    text-align: center;
    justify-content: center;
    padding: 65px 24px;
    border: 1px dashed #dce4db;
    border-radius: 10px;
    gap: 10px;
    color: #8a978c;
    background: #fcfdfb;
  }
  .report-empty h2 {
    margin: 4px 0 0;
    color: #3d5042;
    font-size: 17px;
  }
  .report-empty p {
    max-width: 60ch;
  }
  .context-method {
    margin-bottom: 20px;
    max-width: 95ch;
    font-size: 12px;
  }
  .context-session {
    border: 1px solid var(--line);
    border-radius: 10px;
    margin-bottom: 18px;
    overflow: hidden;
  }
  .context-session > header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 12px;
    padding: 20px;
  }
  .context-session h2 {
    font-size: 16px;
    margin: 0 0 6px;
  }
  .context-session header p,
  .context-session header > span {
    font-size: 11px;
    color: #7b897f;
  }
  .report-table {
    overflow-x: auto;
  }
  table {
    width: 100%;
    border-collapse: collapse;
    text-align: left;
    font-size: 11px;
    white-space: nowrap;
  }
  th {
    font-weight: 500;
    color: #7a887f;
    background: #f8faf6;
    border-block: 1px solid var(--line);
  }
  th,
  td {
    padding: 13px 16px;
    border-bottom: 1px solid var(--line);
    vertical-align: top;
  }
  td small {
    display: block;
    font-size: 10px;
    margin-top: 4px;
    max-width: 200px;
    white-space: normal;
  }
  .context-percent {
    display: inline-block;
    margin-right: 8px;
  }
  meter {
    width: 65px;
    height: 7px;
    vertical-align: middle;
  }
  .boundary-details {
    padding: 15px 20px;
    font-size: 12px;
  }
  .boundary-details summary {
    display: flex;
    justify-content: space-between;
    cursor: pointer;
    color: #516459;
  }
  .boundary-details summary span {
    color: #89968b;
    font-size: 11px;
  }
  .boundary-details ol {
    list-style: none;
    padding: 0;
    margin: 15px 0 0;
  }
  .boundary-details li {
    display: flex;
    gap: 12px;
    flex-wrap: wrap;
    font-size: 11px;
    border-top: 1px solid var(--line);
    padding: 12px 0;
  }
  .boundary-details time {
    color: #839087;
  }
  .boundary-details strong {
    font-weight: 500;
  }
  .boundary-details li span {
    color: #6c7a70;
  }
  iframe {
    display: block;
    width: 100%;
    min-height: 78vh;
    border: 1px solid var(--line);
    border-radius: 8px;
    background: white;
  }
  .snapshot-footer {
    display: flex;
    justify-content: space-between;
    gap: 15px;
    font-size: 10px;
    color: #8a978c;
    padding: 18px 0 8px;
    margin-top: 12px;
    border-top: 1px solid var(--line);
  }
  .snapshot-footer details {
    max-width: 55%;
    text-align: right;
  }
  .snapshot-footer summary {
    cursor: pointer;
  }
  .snapshot-footer code {
    display: block;
    margin: 8px 0;
    font-size: 10px;
  }
  .snapshot-footer p {
    font-size: 10px;
  }
  @media (max-width: 900px) {
    .failure-workspace {
      grid-template-columns: 1fr;
    }
    .failure-list {
      max-height: 270px;
      border-right: 0;
      border-bottom: 1px solid var(--line);
    }
    .failure-detail {
      padding: 20px;
    }
    .summary-card {
      padding: 16px;
    }
  }
  @media (max-width: 600px) {
    .reports-panel {
      padding: 20px 16px;
    }
    .report-summary {
      grid-template-columns: repeat(3, minmax(0, 1fr));
      gap: 8px;
    }
    .summary-card {
      display: flex;
      flex-direction: column;
      align-items: stretch;
      gap: 9px;
      padding: 12px;
    }
    .summary-card > span {
      font-size: 10px;
      align-items: flex-start;
      line-height: 1.4;
    }
    .summary-card > span :global(svg) {
      display: none;
    }
    .summary-card strong {
      font-size: 24px;
    }
    .summary-card small {
      font-size: 9px;
      line-height: 1.4;
    }
    .summary-card small :global(svg) {
      display: none;
    }
    .report-toolbar {
      align-items: flex-start;
    }
    .report-actions {
      width: 100%;
      align-items: stretch;
    }
    .report-actions button {
      flex: 1;
      padding: 8px;
      font-size: 11px;
    }
    .report-actions button:first-child {
      flex: 1.4;
    }
    .report-tabs {
      gap: 19px;
    }
    .report-tabs button {
      font-size: 12px;
    }
    .report-filters {
      flex-direction: column;
    }
    .report-filters select {
      max-width: none;
      width: 100%;
    }
    .failure-detail header {
      flex-direction: column;
    }
    .call-metadata {
      grid-template-columns: 1fr;
    }
    .snapshot-footer {
      flex-direction: column;
    }
    .snapshot-footer details {
      max-width: 100%;
      text-align: left;
    }
  }
</style>
