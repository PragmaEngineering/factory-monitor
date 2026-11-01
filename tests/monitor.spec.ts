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
    if (path === "/executions") return route.fulfill({ json: [execution] });
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
    page.getByRole("heading", { name: "golang", exact: true }),
  ).toBeVisible();
  await expect(page.locator(".step-card")).toHaveCount(6);
  await expect(
    page.getByRole("button", { name: "Logs", exact: true }),
  ).toHaveCount(0);
  await page.getByRole("button", { name: "Expand graph", exact: true }).click();
  await expect(page.locator(".workflow-canvas")).toHaveClass(/expanded/);
  const bounds = await page.locator(".workflow-canvas").boundingBox();
  expect(bounds).toMatchObject({ x: 0, y: 0, width: 1600, height: 1000 });
  await page.keyboard.press("Escape");
  await expect(page.locator(".workflow-canvas")).not.toHaveClass(/expanded/);
  await page.screenshot({
    path: "/tmp/pragma-monitor-workflow.png",
    fullPage: true,
  });
  await page
    .getByRole("button", { name: "Messages", exact: false })
    .first()
    .click();
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
    .getByRole("button", { name: "Decisions", exact: false })
    .first()
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
  await page.getByLabel("Factory configuration").fill("/project/factory.json");
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
