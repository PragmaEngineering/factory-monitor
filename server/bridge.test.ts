import { test } from "node:test";
import assert from "node:assert/strict";
import http from "node:http";
import { once } from "node:events";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { allowedRoute, bridge } from "./bridge.js";

test("UI allowlist excludes raw logs, agent launch, credentials and global context mutation", () => {
  for (const [method, route] of [
    ["GET", "/resources/id/logs"],
    ["POST", "/sessions"],
    ["POST", "/sessions/id/stop"],
    ["PUT", "/context"],
    ["GET", "/sessions/id/inbox"],
  ])
    assert.equal(allowedRoute(method, route), false);
  assert.equal(allowedRoute("POST", "/sessions/id/messages"), true);
  assert.equal(allowedRoute("GET", "/executions/id/messages"), true);
});

test("bridge preserves controller errors and rejects cross-origin mutations", async () => {
  const directory = await mkdtemp(path.join(tmpdir(), "monitor-"));
  const socket = path.join(directory, "api.sock");
  let calls = 0;
  const control = http.createServer((_req, res) => {
    calls++;
    res.writeHead(409, { "Content-Type": "application/json" });
    res.end('{"error":"Decision already answered"}');
  });
  control.listen(socket);
  await once(control, "listening");
  let origin = "";
  const app = http.createServer((req, res) => {
    void bridge(req, res, socket, origin);
  });
  app.listen(0, "127.0.0.1");
  await once(app, "listening");
  origin = `http://127.0.0.1:${(app.address() as { port: number }).port}`;
  try {
    let result = await fetch(origin + "/api/requests/id/answer", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Origin: "https://other.example",
      },
      body: "{}",
    });
    assert.equal(result.status, 403);
    assert.equal(calls, 0);
    result = await fetch(origin + "/api/requests/id/answer", {
      method: "POST",
      headers: { "Content-Type": "application/json", Origin: origin },
      body: "{}",
    });
    assert.equal(result.status, 409);
    assert.deepEqual(await result.json(), {
      error: "Decision already answered",
    });
  } finally {
    app.closeAllConnections();
    app.close();
    control.close();
    await rm(directory, { recursive: true, force: true });
  }
});

test("document preview is restricted to declared execution files and rejects symlink escapes", async () => {
  const { writeFile, mkdir, symlink } = await import("node:fs/promises");
  const directory = await mkdtemp(path.join(tmpdir(), "monitor-files-"));
  const root = path.join(directory, "files");
  await mkdir(root);
  await writeFile(path.join(root, "spec.html"), "<h1>Contract</h1>");
  await writeFile(path.join(directory, "outside.txt"), "private");
  await symlink(
    path.join(directory, "outside.txt"),
    path.join(root, "escape.txt"),
  );
  const socket = path.join(directory, "api.sock");
  const control = http.createServer((_req, res) => {
    res.setHeader("Content-Type", "application/json");
    res.end(
      JSON.stringify({
        execution: {
          configuration: {
            files_dir: root,
            workflow: {
              steps: [
                { outputs: ["spec.html", "escape.txt", "../outside.txt"] },
              ],
            },
          },
        },
      }),
    );
  });
  control.listen(socket);
  await once(control, "listening");
  let origin = "";
  const app = http.createServer((req, res) => {
    void bridge(req, res, socket, origin);
  });
  app.listen(0, "127.0.0.1");
  await once(app, "listening");
  origin = `http://127.0.0.1:${(app.address() as { port: number }).port}`;
  try {
    const result = await fetch(
      origin + "/api/document?execution=run&name=spec.html",
    );
    assert.equal(result.status, 200);
    assert.equal((await result.json()).content, "<h1>Contract</h1>");
    for (const name of ["escape.txt", "../outside.txt", "undeclared.txt"]) {
      assert.equal(
        (
          await fetch(
            origin +
              "/api/document?execution=run&name=" +
              encodeURIComponent(name),
          )
        ).status,
        403,
      );
    }
  } finally {
    app.closeAllConnections();
    app.close();
    control.close();
    await rm(directory, { recursive: true, force: true });
  }
});
