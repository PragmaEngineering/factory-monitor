import { test, expect } from "@playwright/test";
import execution from "./execution.json" with { type: "json" };

test("execution graph, messages, decision errors, input spaces and stale state", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  const mutations: { path: string; body: Record<string, unknown> }[] = [];
  let online = true;
  await page.route("**/api/**", async (route) => {
    const request = route.request(),
      url = new URL(request.url()),
      path = url.pathname.slice(4);
    if (!online)
      return route.fulfill({
        status: 502,
        json: { error: "Controls service is unavailable" },
      });
    if (request.method() === "POST") {
      mutations.push({ path, body: request.postDataJSON() });
      if (path.includes("/answer"))
        return route.fulfill({
          status: 409,
          json: { error: "Decision already answered by another operator" },
        });
      if (path === "/executions")
        return route.fulfill({
          status: 400,
          json: {
            error:
              "Configuration file not found. " +
              "Detailed diagnostic. ".repeat(40),
          },
        });
      return route.fulfill({ json: { id: "sent" } });
    }
    if (path === "/version")
      return route.fulfill({ json: { api: "factory-control-v3" } });
    if (path === "/contexts")
      return route.fulfill({
        json: [{ id: execution.context_id, description: "Payments API" }],
      });
    if (path === "/executions") {
      expect(url.searchParams.get("context")).toBe("*");
      return route.fulfill({ json: [execution] });
    }
    if (path.startsWith("/resources/"))
      return route.fulfill({ json: { type: "execution", execution } });
    if (path === "/sessions")
      return route.fulfill({
        json: [
          {
            id: "agent-1",
            name: "architect",
            state: "running",
            runner: "pi",
            input_mode: "rpc",
            pid: 1201,
            started_at: execution.created_at,
          },
          {
            id: "agent-2",
            name: "architecture_reviewer",
            state: "running",
            runner: "pi",
            input_mode: "rpc",
            pid: 1202,
            started_at: execution.created_at,
          },
        ],
      });
    if (path.startsWith("/sessions/")) return route.fulfill({ json: [] });
    if (path === "/document")
      return route.fulfill({
        json: {
          name: "spec.html",
          path: "/files/spec.html",
          content:
            '<h1>Proposed API</h1><script>parent.postMessage("unsafe","*")</script>',
        },
      });
    return route.fulfill({
      json: [
        {
          id: "m1",
          thread_id: "thread",
          sequence: 1,
          sender: "a",
          sender_name: "architect",
          recipients: ["b"],
          recipient_names: ["architecture_reviewer"],
          at: execution.updated_at,
          payload: {
            kind: "message",
            version: 1,
            parts: [
              {
                kind: "text",
                text: "The revised API contract is ready for review.",
              },
            ],
          },
        },
        {
          id: "m2",
          thread_id: "thread",
          sequence: 2,
          sender: "b",
          sender_name: "architecture_reviewer",
          recipients: ["a"],
          recipient_names: ["architect"],
          at: execution.updated_at,
          payload: {
            kind: "message",
            version: 1,
            parts: [
              {
                kind: "text",
                text: "Verified replay behavior. No blocking findings.",
              },
            ],
          },
        },
      ],
    });
  });
  await page.setViewportSize({ width: 1600, height: 1000 });
  await page.goto("/");
  await expect(
    page.getByRole("combobox", { name: "Filter context" }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("heading", { name: "golang", exact: true }),
  ).toBeVisible();
  await expect(page.locator(".step-card")).toHaveCount(6);
  await expect(page.getByText("Operator inbox", { exact: true })).toHaveCount(
    0,
  );
  await expect(page.locator(".inspector")).toHaveCount(0);
  await expect
    .poll(async () => {
      const sizes = await page
        .locator(".step-card")
        .evaluateAll((cards) =>
          cards.map((card) => card.getBoundingClientRect().width),
        );
      return Math.min(...sizes);
    })
    .toBeGreaterThanOrEqual(260);
  const graphWidth = await page
    .locator(".workflow-panel")
    .evaluate((node) => node.getBoundingClientRect().width);
  expect(graphWidth).toBe(1600);
  const transform = () =>
    page.locator(".svelte-flow__viewport").evaluate((node) => {
      const matrix = new DOMMatrixReadOnly(getComputedStyle(node).transform);
      return { zoom: matrix.a, y: matrix.f };
    });
  const beforeScroll = await transform();
  await page.locator(".workflow-canvas").hover();
  await page.mouse.wheel(0, 360);
  await expect
    .poll(async () => (await transform()).y)
    .toBeLessThan(beforeScroll.y);
  const afterScroll = await transform();
  expect(afterScroll.zoom).toBeCloseTo(beforeScroll.zoom);
  await page.waitForResponse((response) =>
    response.url().includes("/api/resources/"),
  );
  expect((await transform()).y).toBeCloseTo(afterScroll.y);
  await page.getByRole("button", { name: "Active work", exact: true }).click();

  await expect(
    page.getByRole("button", { name: "Logs", exact: true }),
  ).toHaveCount(0);
  await page.getByRole("button", { name: "Expand graph", exact: true }).click();
  await expect(page.locator(".workflow-canvas")).toHaveClass(/expanded/);
  const bounds = await page.locator(".workflow-layout").boundingBox();
  expect(bounds).toMatchObject({ x: 0, y: 0, width: 1600, height: 1000 });
  const activeCard = page
    .locator(".step-card")
    .filter({ hasText: "architect" })
    .first();
  // Expand navigation animates for 200ms; record the settled camera.
  await page.waitForTimeout(250);
  const expandedCanvas = await page.locator(".workflow-canvas").boundingBox();
  const expandedTransform = await page
    .locator(".svelte-flow__viewport")
    .getAttribute("style");
  const expandedCard = await activeCard.boundingBox();
  await activeCard.click();
  await expect(page.locator(".workflow-layout")).toHaveClass(/expanded/);
  await expect(page.locator(".inspector")).toBeVisible();
  await expect(page.locator(".inspector h3")).toHaveText(/.+/);
  expect(await page.locator(".workflow-canvas").boundingBox()).toEqual(
    expandedCanvas,
  );
  expect(
    await page.locator(".svelte-flow__viewport").getAttribute("style"),
  ).toEqual(expandedTransform);
  expect(await activeCard.boundingBox()).toEqual(expandedCard);
  await page
    .getByRole("button", { name: "Close step details", exact: true })
    .click();
  await expect(page.locator(".inspector")).toHaveCount(0);
  expect(await page.locator(".workflow-canvas").boundingBox()).toEqual(
    expandedCanvas,
  );
  expect(await activeCard.boundingBox()).toEqual(expandedCard);
  await expect(page.locator(".workflow-layout")).toHaveClass(/expanded/);
  await page.keyboard.press("Escape");
  await expect(page.locator(".workflow-canvas")).not.toHaveClass(/expanded/);
  await page.screenshot({
    path: "/tmp/pragma-monitor-workflow.png",
    fullPage: true,
  });
  await page
    .getByRole("button", { name: /^Inbox/ })
    .first()
    .click();
  await page
    .getByRole("button", { name: "All participants", exact: true })
    .click();
  await expect(
    page.getByRole("img", { name: "Participant communication graph" }),
  ).toBeVisible();
  const communications = await page
    .locator(".communications-panel")
    .boundingBox();
  const inbox = await page.locator(".messages-page").boundingBox();
  expect(communications!.width).toBeCloseTo(inbox!.width / 3, 0);
  expect(communications!.x + communications!.width).toBeCloseTo(
    inbox!.x + inbox!.width,
    0,
  );
  expect(communications!.height).toBeCloseTo(inbox!.height, 0);
  await expect(page.locator(".communication-link")).toHaveCount(2);
  await expect(page.locator(".communication-node")).toHaveCount(2);
  await page.screenshot({
    path: "/tmp/pragma-monitor-communications.png",
    fullPage: true,
  });

  await expect(
    page.getByText("The revised API contract is ready for review."),
  ).toBeVisible();
  await page
    .locator(".participant-index button")
    .filter({ hasText: "architect" })
    .filter({ hasNotText: "architecture_reviewer" })
    .click();
  await page
    .getByLabel("Message architect")
    .fill("Please verify the response contract.");
  await page.getByRole("button", { name: "Send message", exact: true }).click();
  await expect
    .poll(() =>
      mutations.some(
        (m) =>
          m.path === "/sessions/agent-1/messages" &&
          m.body.body === "Please verify the response contract.",
      ),
    )
    .toBe(true);
  await page
    .getByRole("navigation", { name: "Inbox views" })
    .getByRole("button", { name: /^Requests/ })
    .click();
  await page.getByRole("button", { name: "Review and respond" }).click();
  await page.getByLabel("Outcome", { exact: true }).selectOption("approve");
  await page
    .getByLabel("Reason / feedback")
    .fill("Reviewed contract and evidence.");
  await page.getByRole("button", { name: "Submit decision" }).click();
  await expect(page.getByRole("dialog").getByRole("alert")).toHaveText(
    "Decision already answered by another operator",
  );
  await page.getByRole("button", { name: "Close decision" }).click();
  await page.getByRole("button", { name: "spec.html", exact: true }).click();
  await expect(page.locator("iframe")).toHaveAttribute("sandbox", "");
  await page.getByRole("button", { name: "Close document" }).click();
  await page
    .getByRole("button", { name: "New execution", exact: true })
    .first()
    .click();
  await page.getByLabel("Factory configuration").fill("/project/factory.yaml");
  await page
    .getByLabel("Context / brief")
    .fill("Hello world in Go with spaces");
  await page
    .getByRole("button", { name: "Start execution", exact: true })
    .click();
  await expect(page.getByRole("dialog").getByRole("alert")).toContainText(
    "Detailed diagnostic.",
  );
  expect(mutations.at(-1)?.body.context_id).toBe("*");
  expect(mutations.at(-1)?.body.brief).toBe("Hello world in Go with spaces");
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await page.getByRole("button", { name: "Home", exact: true }).click();
  online = false;
  await expect(page.getByText("Disconnected", { exact: true })).toBeVisible({
    timeout: 8000,
  });
  await expect(
    page.getByRole("heading", { name: "golang", exact: true }),
  ).toBeVisible();
  expect(errors).toEqual([]);
});

test("single-agent workflow retries the selected visit and cancels only its execution", async ({
  page,
}) => {
  const record = structuredClone(execution);
  record.configuration.workflow.stages = [];
  record.configuration.workflow.steps = [
    {
      id: "worker",
      title: "Worker",
      kind: "run",
      run: { package: "standalone" },
      routes: { completed: "$complete" },
    },
  ] as typeof record.configuration.workflow.steps;
  record.configuration.workflow.start = "worker";
  record.steps = [
    {
      id: "visit-worker",
      key: "worker",
      state: "blocked",
      visit: 1,
      try: 1,
      started_at: record.created_at,
      error: "Provider unavailable",
    },
  ] as typeof record.steps;
  record.requests = [];
  record.state = "blocked";
  const actions: { path: string; body: Record<string, unknown> }[] = [];
  await page.route("**/api/**", async (route) => {
    const req = route.request(),
      path = new URL(req.url()).pathname.slice(4);
    if (req.method() === "POST") {
      actions.push({ path, body: req.postDataJSON() });
      return route.fulfill({ json: record });
    }
    if (path === "/version")
      return route.fulfill({ json: { api: "factory-control-v3" } });
    if (path === "/executions") return route.fulfill({ json: [record] });
    if (path.startsWith("/resources/"))
      return route.fulfill({ json: { execution: record } });
    return route.fulfill({ json: [] });
  });
  await page.goto("/");
  await page.locator(".step-card").click();
  await page.getByRole("button", { name: "Retry step", exact: true }).click();
  await page.getByLabel("Reason", { exact: true }).fill("Provider recovered");
  await page
    .getByRole("button", { name: "Confirm retry", exact: true })
    .click();
  await expect.poll(() => actions.length).toBe(1);
  expect(actions[0]).toEqual({
    path: "/steps/visit-worker/retry",
    body: { reason: "Provider recovered", actor: "operator" },
  });
  await page
    .getByRole("button", { name: "Cancel execution", exact: true })
    .click();
  expect(actions).toHaveLength(1); // Opening confirmation has no side effect.
  await page
    .getByRole("button", { name: "Confirm cancel", exact: true })
    .click();
  await expect.poll(() => actions.length).toBe(2);
  expect(actions[1].path).toBe("/executions/" + record.id + "/cancel");
  await page.setViewportSize({ width: 820, height: 900 });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
});

test("top-bar execution search sorts newest first and operator unread state is browser-scoped", async ({
  page,
}) => {
  const older = {
    ...execution,
    id: "older-run",
    created_at: "2026-10-01T12:00:00Z",
    brief: "Earlier run",
  };
  const correspondence = (id: string) => ({
    id,
    thread_id: "thread",
    sequence: 1,
    sender: "architect",
    sender_name: "architect",
    recipients: ["human-id"],
    recipient_names: ["human"],
    at: execution.updated_at,
    payload: {
      kind: "message",
      version: 1,
      parts: [{ kind: "text", text: "Operator message " + id }],
    },
  });
  const messages = [correspondence("first")];
  let mutations = 0;
  await page.route("**/api/**", async (route) => {
    const req = route.request(),
      path = new URL(req.url()).pathname.slice(4);
    if (req.method() !== "GET") {
      mutations++;
      return route.fulfill({ json: {} });
    }
    if (path === "/version")
      return route.fulfill({ json: { api: "factory-control-v3" } });
    if (path === "/executions")
      return route.fulfill({ json: [older, execution] });
    if (path === "/contexts") return route.fulfill({ json: [] });
    if (path.startsWith("/resources/"))
      return route.fulfill({
        json: { execution: path.endsWith(older.id) ? older : execution },
      });
    if (path.endsWith("/messages")) return route.fulfill({ json: messages });
    return route.fulfill({ json: [] });
  });
  await page.goto("/");
  await expect(page.locator(".sidebar")).toHaveCount(0);
  await expect(
    page.getByLabel("1 unread operator messages", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Select execution" }),
  ).toContainText(execution.id.slice(0, 8));
  await page.getByRole("button", { name: "Select execution" }).click();
  await expect(page.locator(".execution-option").first()).toContainText(
    execution.id.slice(0, 8),
  );
  await page
    .getByLabel("Search executions", { exact: true })
    .fill("Earlier run");
  await expect(page.locator(".execution-option")).toHaveCount(1);
  await page.keyboard.press("ArrowDown");
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL(/execution=older-run/);
  await page.getByRole("button", { name: "Select execution" }).click();
  await page
    .getByLabel("Search executions", { exact: true })
    .fill(execution.id);
  await page.locator(".execution-option").click();
  await expect(page).toHaveURL(/execution=demo-execution-001/);
  await page.getByRole("button", { name: /^Inbox/ }).click();
  await expect(
    page.getByText("Operator message first", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByLabel("1 unread operator messages", { exact: true }),
  ).toHaveCount(0);
  await page.getByRole("button", { name: "Home", exact: true }).click();
  await page.reload();
  await expect(
    page.getByRole("heading", { name: "golang", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByLabel("1 unread operator messages", { exact: true }),
  ).toHaveCount(0);
  messages.push(correspondence("second"));
  await expect(
    page.getByLabel("1 unread operator messages", { exact: true }),
  ).toBeVisible({ timeout: 8000 });
  expect(mutations).toBe(0);
});
