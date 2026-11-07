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

`factory monitor start` resolves the installed `pragma-monitor` command on `PATH` and forwards its options. Both launch commands work from any directory without `factory.json`. Bare `factory monitor` prints help without starting anything. Use `factory monitor start -d` (or `--detach`) to run in the background; it prints the URL, PID, and private log path. Additional options, such as `--port 4175`, are forwarded. Startup waits for the listener to bind and reports early failures. `factory monitor stop` stops all Factory-managed detached monitors without stopping the control-plane daemon or requiring the UI executable. Foreground or independently launched instances are not managed by this command. `factory stop --all` also stops managed detached monitors, while `factory stop` leaves Monitor running.

`make install` performs dependency installation, build and link. `make run` starts the built app. `npm run dev` starts the same local bridge with Vite development middleware.

Start Controls explicitly with `factory start` before opening Monitor. Operational commands such as `factory ls` only connect and fail if the service is not running. Monitor never starts, stops, installs or restarts the controller. No Factory/Registry checkout is required by the running monitor.

```sh
factory monitor start --port 4175
factory monitor start --socket /path/to/factory.sock
factory monitor
```

Default socket: `~/.factory/runtime/factory.sock`. The server binds only to `127.0.0.1`. Use the printed numeric-loopback URL. This is a local operator interface, not a remotely deployable authenticated dashboard.

## Views

- **Executions:** search and context filtering, independent of the CLI’s selected-context preference. Selection is retained in the URL. New executions use the selected context; All contexts creates a new context, independently of the CLI preference.
- **Workflow:** stages, configured outcome routes, concurrent members, latest step state, visits, attempts, timings, reported usage, hook failures and retry. Click a step for details. The canvas opens on the active stage when one exists. Use Overview to fit the full graph, Current stage to return to active work, or pan and zoom manually. Expand fills the browser viewport; Restore or Escape returns to the monitor. Selecting a step restores the layout and opens its details. Connections describe configured routes; they are not proof that a transition occurred.
- **Messages:** observed sender/recipient traffic, all-message and participant filters, full structured payloads, and session-targeted feedback. Drafts remain per execution/session while navigating views; failed sends retain their text. Delivery state does not prove that an agent acted on a message.
- **Decisions:** exact configured outcomes, feedback and explicit submission. Declared evidence files open in a sandboxed HTML preview or selectable text; paths can be copied. The controller validates decisions and resolves concurrent operator actions.
- **New execution:** absolute configuration path, a multiline brief and optional existing input-file paths on the Controls host. Submission is explicit and never automatically retried. Inspect execution status before repeating an ambiguous failed submission.

**Connected** means the most recent API refresh succeeded (every two seconds). It does not mean an agent is running. Disconnections retain existing data and disable mutations until reconnection. Closing the browser/server leaves executions running. No logs view is included. Use `factory SESSION-ID logs -f` or `factory EXECUTION-ID stream -f` for full output.

## Architecture

```text
Svelte browser application
        │ same-origin /api requests
        ▼
Monitor Node.js server ── private Unix socket ── Controls HTTP API
        │
        └── built static assets (Vite)
```

`src/lib/types.ts` describes the consumed JSON contract; `api.ts` centralizes calls. Components own presentation and operator interaction, not workflow routing. `graph.ts` builds stage groups and route edges from the execution’s frozen configuration. `server/bridge.ts` permits only monitor operations; it does not forward arbitrary API routes or session launch/stop/credential operations. Controller API version `factory-control-v3` is required.

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
