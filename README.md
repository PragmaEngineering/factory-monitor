# Pragma Monitor

Local web UI for the [Controls execution service](../controls/README.md). Svelte 5, TypeScript and Svelte Flow. The controller owns workflow state and actions; this application presents and operates its API.

## Run

Requires Node.js 22.12+ and npm. From this checkout:

```sh
npm ci
npm run build
npm start
```

Open **http://127.0.0.1:4173**. To install the command on your npm binary path:

```sh
npm link
factory monitor start
```

`factory monitor start` resolves the installed `pragma-monitor` command on `PATH` and forwards its options. Both launch commands work from any directory without `factory.yaml`. Bare `factory monitor` prints help without starting anything. Use `factory monitor start -d` (or `--detach`) to run in the background; it prints the URL, PID, and private log path. Additional options, such as `--port 4175`, are forwarded. Startup waits for the listener to bind and reports early failures. `factory monitor stop` stops all Factory-managed detached monitors without stopping the control-plane daemon or requiring the UI executable. Foreground or independently launched instances are not managed by this command. `factory stop --all` also stops managed detached monitors, while `factory stop` leaves Monitor running.

`make install` performs dependency installation, build and link. `make run` starts the built app. `npm run dev` starts the same local bridge with Vite development middleware.

Start Controls explicitly with `factory start` before opening Monitor. Operational commands such as `factory ls` only connect and fail if the service is not running. Monitor never starts, stops, installs or restarts the controller. No Factory/Registry checkout is required by the running monitor.

```sh
factory monitor start --port 4175
factory monitor start --socket /path/to/factory.sock
factory monitor
```

Default socket: `~/.factory/runtime/factory.sock`. The server binds only to `127.0.0.1`. Use the printed numeric-loopback URL. This is a local operator interface, not a remotely deployable authenticated dashboard.

## Views

- **Execution selector:** searchable dropdown in the top navigation, sorted by creation time (newest first). Search name, context, execution ID or project. Arrow keys select results and Escape closes the dropdown. All execution contexts are included. Selection is retained in the URL; new executions create their own context.
- **Home:** top-level execution statistics and the workflow. stages, configured outcome routes, concurrent members, latest step state, visits, attempts, timings, reported usage, hook failures and retry. Click a step for details. The graph uses the full Home width until a step is selected for inspection. It opens at readable card scale on active work, or the entry step when nothing is active. Operator correspondence and requests live only in Inbox. Use Overview to fit the full graph, Active work (or Start) to return to the relevant stage, or pan and zoom manually. Expand fills the browser viewport; Restore or Escape returns to the monitor. In expanded mode, step details float over the graph without resizing or moving it. Closing the details preserves the camera; only Restore or Escape exits expanded mode. Connections describe configured routes; they are not proof that a transition occurred.
- **Inbox → Messages:** a directed participant graph in the right third of the page (stacked below on narrow screens), all-message and participant filters, full structured payloads, and session-targeted feedback. Drafts remain per execution/session while navigating views; failed sends retain their text. Graph arrows and counts aggregate persisted messages by participant name, with explicit recipients only. Link width reflects volume; the graph does not indicate live activity. Delivery state does not prove that an agent acted on a message.
- **Inbox → Requests:** exact configured outcomes, feedback and explicit submission. Declared evidence files open in a sandboxed HTML preview or selectable text; paths can be copied. The controller validates decisions and resolves concurrent operator actions.
- **Reports:** opens directly to failed tool calls, with recorded/captured counts, agent and text filters, a call list and a diagnostic detail pane. Copy a selected diagnosis as JSON. Context & turns shows per-response window utilization and native boundary timelines. Full report retains the sandboxed document with factory history; download offline HTML or the full JSON snapshot. Home and the Reports navigation surface recorded failure counts. Refresh explicitly creates another immutable snapshot; reports do not regenerate on the two-second status poll. Missing diagnostic evidence is shown separately from zero failures. Older telemetry cannot reconstruct missing context limits or failure details. An “Unknown monitor operation” error means the running Monitor bridge needs rebuilding/restarting, even if its static frontend already shows the tab.
- **New execution:** absolute configuration path, a multiline brief and optional existing input-file paths on the Controls host. Submission is explicit and never automatically retried. Inspect execution status before repeating an ambiguous failed submission.

**Connected** means the most recent API refresh succeeded (every two seconds). It does not mean an agent is running. Disconnections retain existing data and disable mutations until reconnection. Closing the browser/server leaves executions running. No logs view is included. Use `factory SESSION-ID logs -f` or `factory EXECUTION-ID stream -f` for full output.

The Inbox navigation badge counts new incoming operator messages for the selected execution. Controls’ `human` participant is shown as Operator. Opening the operator message view marks currently displayed correspondence as seen in this browser; the state persists in local storage. It does not acknowledge MCP deliveries or claim that another human/browser has read them. Lifecycle status broadcasts do not trigger the badge. An amber dot separately indicates pending decision/input requests.

## Architecture

```text
Svelte browser application
        │ same-origin /api requests
        ▼
Monitor Node.js server ── private Unix socket ── Controls HTTP API
        │
        └── built static assets (Vite)
```

`src/lib/types.ts` describes the consumed JSON contract; `api.ts` centralizes calls. Components own presentation and operator interaction, not workflow routing. `layout.ts` uses ELK layered layout to position stage internals and the dependency graph between stages and standalone steps, including orthogonal connection routing and label placement. Geometry is recalculated when the frozen workflow definition or canvas width bucket changes. `graph.ts` applies fresh visit state and route edges to those positions; live refresh does not reposition the diagram. Unvisited alternatives in terminated executions are labeled Not visited, not Pending. `server/bridge.ts` permits only monitor operations; it does not forward arbitrary API routes or session launch/stop/credential operations. Controller API version `factory-control-v3` is required.

Documents are limited to declared workflow outputs/decision artifacts under that execution’s `files_dir`. Real-path checks reject symlink escapes; files above 4 MiB are refused. Preview frames disable scripts, isolate their origin and block external resource loads. Monitor does not serve arbitrary host files.

No runtime Go imports, database access, scheduler SDK or shared filesystem configuration connects this repo to Controls. Polling is deliberate for this prototype; refresh requests do not overlap. There is no separate authoritative monitor state. Fonts are bundled locally.

## Verify

```sh
npm run check
npm test
npm run build
npx playwright test
```

Browser tests use installed Google Chrome and synthetic API fixtures, never live agents or workflow mutations. They cover graph rendering, messages, decision conflicts, multiline input, complete errors and stale-state display. Bridge tests cover route restrictions, origin checks and controller error preservation. `tests/execution.json` is fictional test data, not eval evidence.

The previous terminal implementation and dependency context are preserved in [`.tui_deprecated`](.tui_deprecated/README.md). It is an archive, not a supported second UI.

The workflow progresses from top to bottom. Stages arrange steps horizontally when the canvas has room, falling back to a vertical arrangement for longer sequential stages on narrow screens. Concurrent members appear side by side, with any human gate below them. Scroll vertically to follow the workflow; Overview fits the complete graph.
