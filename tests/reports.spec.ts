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
            selected: { id: execution.id },
          },
          html: '<!doctype html><html><body><h1>Context windows, turns and failed tool calls</h1><p>Failed call: go test; exit 2</p><script>parent.document.body.dataset.unsafe="yes"</script></body></html>',
        },
      });
    }
    return route.fulfill({ json: [] });
  });
  await page.goto("/");
  await page.getByRole("button", { name: "Reports", exact: true }).click();
  const frame = page.frameLocator('iframe[title="Execution report"]');
  await expect(frame.getByRole("heading")).toHaveText(
    "Context windows, turns and failed tool calls",
  );
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
  await expect(page.getByText("snapshot-2", { exact: true })).toBeVisible();
  fail = true;
  await page.getByRole("button", { name: "Refresh report" }).click();
  await expect(
    page.getByRole("alert").filter({ hasText: "Report snapshot unavailable" }),
  ).toBeVisible();
  await expect(page.locator('iframe[title="Execution report"]')).toHaveCount(0);
});
