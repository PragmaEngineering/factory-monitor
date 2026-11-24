import { test, expect } from "@playwright/test";
import execution from "./execution.json" with { type: "json" };

test("Reports loads a snapshot once, exports evidence, refreshes and exposes failures", async ({
  page,
}) => {
  let reports = 0,
    fail = false;
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname.slice(4);
    if (path === "/version")
      return route.fulfill({ json: { api: "factory-control-v3" } });
    if (path === "/executions") return route.fulfill({ json: [execution] });
    if (path.startsWith("/resources/"))
      return route.fulfill({ json: { execution } });
    if (path === "/reports") {
      reports++;
      expect(route.request().postDataJSON()).toEqual({
        execution_id: execution.id,
      });
      if (fail)
        return route.fulfill({
          status: 500,
          json: { error: "Report snapshot unavailable" },
        });
      return route.fulfill({
        json: {
          id: `snapshot-${reports}`,
          snapshot: {
            as_of: "2026-11-24T12:00:00Z",
            selected: {
              id: execution.id,
              resources: { samples: 2, tool_failures: 3 },
              agents: [
                {
                  name: "coder",
                  session_id: "s1",
                  step_id: "build-visit",
                  diagnostics: {
                    failures: [
                      {
                        tool_id: "call-bash",
                        name: "bash",
                        command: "go test ./...",
                        arguments: '{"command":"go test ./..."}',
                        started_at: "2026-11-24T12:00:00Z",
                        finished_at: "2026-11-24T12:00:02Z",
                        exit_code: 2,
                        output: "FAIL TestRetry: version changed",
                        error: "exit status 2",
                        turn: 1,
                      },
                      {
                        tool_id: "call-read",
                        name: "read",
                        command: "read missing.go",
                        finished_at: "2026-11-24T12:00:04Z",
                        output: "ENOENT: no such file",
                      },
                    ],
                    responses: [
                      {
                        number: 1,
                        turn: 1,
                        at: "2026-11-24T12:00:01Z",
                        tokens: {
                          input: 100,
                          cache_read: 600,
                          cache_write: 0,
                          output: 100,
                        },
                        context_window: 1000,
                        model: "test-model",
                        stop_reason: "toolUse",
                      },
                    ],
                    boundaries: [
                      {
                        kind: "turn_started",
                        at: "2026-11-24T12:00:00Z",
                        turn: 1,
                      },
                      {
                        kind: "turn_finished",
                        at: "2026-11-24T12:00:03Z",
                        turn: 1,
                      },
                    ],
                  },
                },
              ],
            },
          },
          html: '<!doctype html><html><body><h1>Context windows, turns and failed tool calls</h1><p>Failed call: go test; exit 2</p><script>parent.document.body.dataset.unsafe="yes"</script></body></html>',
        },
      });
    }
    return route.fulfill({ json: [] });
  });
  await page.goto("/");
  await page.getByRole("button", { name: "Reports", exact: true }).click();
  await expect(
    page.getByRole("button", { name: /Failed tool calls/ }),
  ).toBeVisible();
  await expect(
    page.getByText("1 recorded failure has no diagnostic evidence"),
  ).toBeVisible();
  await expect(page.locator(".failure-detail")).toContainText(
    "FAIL TestRetry: version changed",
  );
  await expect(page.locator(".failure-detail")).toContainText("2.0 s · 2");
  await page.getByLabel("Search failed tool calls").fill("ENOENT");
  await expect(page.locator(".failure-list button")).toHaveCount(1);
  await expect(page.locator(".failure-detail")).toContainText("call-read");
  await page.getByLabel("Search failed tool calls").fill("no-match");
  await expect(page.getByText("No calls match your filters")).toBeVisible();
  await page
    .getByRole("button", { name: "Context & turns", exact: true })
    .click();
  await expect(
    page.locator(".report-table").getByText("80.0%", { exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Full report", exact: true }).click();
  const frame = page.frameLocator('iframe[title="Execution report"]');
  await expect(frame.getByRole("heading")).toHaveText(
    "Context windows, turns and failed tool calls",
  );
  await page.getByText("Snapshot details", { exact: true }).click();
  await expect(page.getByText("snapshot-1", { exact: true })).toBeVisible();
  await expect(
    page.locator('iframe[title="Execution report"]'),
  ).toHaveAttribute("sandbox", "");
  expect(await page.locator("body").getAttribute("data-unsafe")).toBeNull();
  const download = page.waitForEvent("download");
  await page.getByRole("button", { name: "JSON", exact: true }).click();
  expect((await download).suggestedFilename()).toBe(
    "execution-report-snapshot-1.json",
  );
  await page.waitForTimeout(2200);
  expect(reports).toBe(1);
  await page.getByRole("button", { name: "Refresh report" }).click();
  await page.getByText("Snapshot details", { exact: true }).click();
  await expect(page.getByText("snapshot-2", { exact: true })).toBeVisible();
  await page.setViewportSize({ width: 390, height: 844 });
  const picker = await page.locator(".execution-picker").boundingBox();
  expect(picker?.width).toBeGreaterThan(300);
  expect(picker?.height).toBeLessThan(80);
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
  fail = true;
  await page.getByRole("button", { name: "Refresh report" }).click();
  await expect(
    page.getByRole("alert").filter({ hasText: "Report snapshot unavailable" }),
  ).toBeVisible();
  await expect(page.locator('iframe[title="Execution report"]')).toHaveCount(0);
});

test("old Monitor bridges get an actionable reports error", async ({
  page,
}) => {
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname.slice(4);
    if (path === "/version")
      return route.fulfill({ json: { api: "factory-control-v3" } });
    if (path === "/executions") return route.fulfill({ json: [execution] });
    if (path.startsWith("/resources/"))
      return route.fulfill({ json: { execution } });
    if (path === "/reports")
      return route.fulfill({
        status: 404,
        json: { error: "Unknown monitor operation" },
      });
    return route.fulfill({ json: [] });
  });
  await page.goto("/");
  await page.getByRole("button", { name: "Reports", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText(
    "Rebuild and restart Monitor",
  );
});
