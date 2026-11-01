#!/usr/bin/env node
import http from "node:http";
import { readFile } from "node:fs/promises";
import path from "node:path";
import { homedir } from "node:os";
import { fileURLToPath } from "node:url";
import { bridge, json } from "./bridge.js";

const args = process.argv.slice(2);
if (args.includes("--help")) {
  console.log(
    "pragma-monitor [--port 4173] [--socket PATH] [--dev]\n\nLocal web monitor for Controls. Does not start or stop the controller.",
  );
  process.exit(0);
}
function option(name: string, fallback: string) {
  const index = args.indexOf(name);
  if (index === -1) return fallback;
  if (!args[index + 1] || args[index + 1].startsWith("--"))
    throw new Error(`${name} requires a value`);
  return args[index + 1];
}
const valid = new Set(["--port", "--socket", "--dev"]);
for (let i = 0; i < args.length; i++) {
  if (!valid.has(args[i])) throw new Error(`Unknown option: ${args[i]}`);
  if (args[i] !== "--dev") i++;
}
const port = Number(option("--port", "4173"));
if (!Number.isInteger(port) || port < 1 || port > 65535)
  throw new Error("Port must be between 1 and 65535");
const socket = option(
  "--socket",
  path.join(homedir(), ".factory/runtime/factory.sock"),
);
const origin = `http://127.0.0.1:${port}`;
const development = args.includes("--dev");
const root = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  development ? ".." : "../..",
);
const vite = development
  ? await (
      await import("vite")
    ).createServer({ root, server: { middlewareMode: true }, appType: "spa" })
  : undefined;
const types: Record<string, string> = {
  ".html": "text/html",
  ".js": "text/javascript",
  ".css": "text/css",
  ".svg": "image/svg+xml",
  ".woff2": "font/woff2",
  ".png": "image/png",
};
const server = http.createServer(async (req, res) => {
  try {
    if (req.headers.host !== new URL(origin).host) {
      json(res, 403, { error: "Invalid host" });
      return;
    }
    if (await bridge(req, res, socket, origin)) return;
    if (vite) {
      vite.middlewares(req, res);
      return;
    }
    if (req.method !== "GET" && req.method !== "HEAD") {
      res.writeHead(405);
      res.end();
      return;
    }
    const pathname = decodeURIComponent(
      new URL(req.url ?? "/", origin).pathname,
    );
    const file = path.resolve(root, "dist", "." + pathname);
    const dist = path.join(root, "dist");
    if (file !== dist && !file.startsWith(dist + path.sep)) {
      res.writeHead(403);
      res.end();
      return;
    }
    const target = path.extname(file) ? file : path.join(dist, "index.html");
    const content = await readFile(target);
    res.writeHead(200, {
      "Content-Type": types[path.extname(target)] ?? "application/octet-stream",
      "X-Content-Type-Options": "nosniff",
    });
    res.end(req.method === "HEAD" ? undefined : content);
  } catch {
    res.writeHead(404);
    res.end("Not found");
  }
});
server.listen(port, "127.0.0.1", () =>
  console.log(`Pragma Monitor: ${origin}\nControls socket: ${socket}`),
);
server.on("error", (error) => {
  console.error(error.message);
  process.exitCode = 1;
});
for (const signal of ["SIGINT", "SIGTERM"] as const)
  process.on(signal, () => {
    server.close();
    server.closeAllConnections();
    void vite?.close();
  });
