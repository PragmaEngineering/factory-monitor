import http, { type IncomingMessage, type ServerResponse } from "node:http";
import { readFile, realpath, stat } from "node:fs/promises";
import path from "node:path";

// Explicitly expose UI operations, never session launch/stop, credentials or logs.
export function allowedRoute(method: string, pathname: string): boolean {
  const id = "[a-zA-Z0-9_-]+";
  const routes: Record<string, RegExp[]> = {
    GET: [
      /^\/version$/,
      /^\/contexts$/,
      /^\/executions$/,
      /^\/inbox$/,
      new RegExp(`^/resources/${id}$`),
      new RegExp(`^/executions/${id}/messages$`),
      /^\/sessions$/,
      new RegExp(`^/sessions/${id}/messages$`),
    ],
    POST: [
      /^\/executions$/,
      new RegExp(`^/requests/${id}/answer$`),
      new RegExp(`^/steps/${id}/retry$`),
      new RegExp(`^/executions/${id}/cancel$`),
      new RegExp(`^/sessions/${id}/messages$`),
    ],
  };
  return (routes[method] ?? []).some((route) => route.test(pathname));
}

export function json(res: ServerResponse, status: number, value: unknown) {
  res.writeHead(status, {
    "Content-Type": "application/json",
    "Cache-Control": "no-store",
  });
  res.end(JSON.stringify(value));
}

export function upstream(
  socketPath: string,
  method: string,
  url: string,
  body?: Buffer,
): Promise<{ status: number; body: Buffer }> {
  return new Promise((resolve, reject) => {
    const request = http.request(
      {
        socketPath,
        path: url,
        method,
        headers: { "Content-Type": "application/json" },
        timeout: 45_000,
      },
      (response) => {
        const chunks: Buffer[] = [];
        let size = 0;
        response.on("data", (chunk: Buffer) => {
          size += chunk.length;
          if (size > 48 * 1024 * 1024) {
            response.destroy(new Error("Controller response exceeds 48 MiB"));
            return;
          }
          chunks.push(chunk);
        });
        response.on("end", () =>
          resolve({
            status: response.statusCode ?? 502,
            body: Buffer.concat(chunks),
          }),
        );
        response.on("error", reject);
      },
    );
    request.on("timeout", () =>
      request.destroy(new Error("Controller request timed out")),
    );
    request.on("error", reject);
    request.end(body);
  });
}

async function bodyOf(req: IncomingMessage): Promise<Buffer> {
  const chunks: Buffer[] = [];
  let size = 0;
  for await (const chunk of req) {
    size += chunk.length;
    if (size > 4 * 1024 * 1024) throw new Error("Request exceeds 4 MiB");
    chunks.push(chunk);
  }
  return Buffer.concat(chunks);
}

// Serve text from the selected execution's files directory only. Realpath checks
// reject symlinks escaping the directory. HTML is previewed in a sandboxed frame.
async function document(socket: string, url: URL, res: ServerResponse) {
  const id = url.searchParams.get("execution");
  const name = url.searchParams.get("name");
  if (!id || !/^[\w-]+$/.test(id) || !name || path.isAbsolute(name))
    return json(res, 400, {
      error: "Execution and relative document name are required",
    });
  const record = await upstream(socket, "GET", `/resources/${id}`);
  if (record.status !== 200) {
    res.writeHead(record.status, { "Content-Type": "application/json" });
    res.end(record.body);
    return;
  }
  const execution = JSON.parse(record.body.toString()).execution;
  const configured = execution?.configuration?.workflow?.steps ?? [];
  const names = new Set<string>(
    configured.flatMap(
      (s: { outputs?: string[]; decision?: { artifacts?: string[] } }) => [
        ...(s.outputs ?? []),
        ...(s.decision?.artifacts ?? []),
      ],
    ),
  );
  if (!names.has(name))
    return json(res, 403, {
      error: "Document is not declared by this execution",
    });
  try {
    const root = await realpath(execution.configuration.files_dir);
    const file = await realpath(path.resolve(root, name));
    if (!file.startsWith(root + path.sep))
      return json(res, 403, { error: "Document is outside execution files" });
    if ((await stat(file)).size > 4 * 1024 * 1024)
      return json(res, 413, { error: "Document exceeds 4 MiB" });
    json(res, 200, { name, path: file, content: await readFile(file, "utf8") });
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === "ENOENT") {
      json(res, 404, { error: "Execution document not found" });
      return;
    }
    throw error;
  }
}

export async function bridge(
  req: IncomingMessage,
  res: ServerResponse,
  socket: string,
  origin: string,
): Promise<boolean> {
  const url = new URL(req.url ?? "/", origin);
  if (!url.pathname.startsWith("/api/")) return false;
  // Same-origin browser access only. Host validation also prevents DNS rebinding.
  if (
    req.headers.host !== new URL(origin).host ||
    (req.headers.origin && req.headers.origin !== origin) ||
    req.headers["sec-fetch-site"] === "cross-site"
  ) {
    json(res, 403, { error: "Cross-origin request rejected" });
    return true;
  }
  try {
    if (req.method === "GET" && url.pathname === "/api/document") {
      await document(socket, url, res);
      return true;
    }
    const pathname = url.pathname.slice(4);
    if (!allowedRoute(req.method ?? "", pathname)) {
      json(res, 404, { error: "Unknown monitor operation" });
      return true;
    }
    if (
      req.method === "POST" &&
      !req.headers["content-type"]?.startsWith("application/json")
    ) {
      json(res, 415, { error: "Content-Type must be application/json" });
      return true;
    }
    const body = req.method === "POST" ? await bodyOf(req) : undefined;
    const response = await upstream(
      socket,
      req.method!,
      pathname + url.search,
      body,
    );
    res.writeHead(response.status, {
      "Content-Type": "application/json",
      "Cache-Control": "no-store",
    });
    res.end(response.body);
  } catch (error) {
    const code = (error as NodeJS.ErrnoException).code;
    const message =
      code === "ENOENT" || code === "ECONNREFUSED"
        ? "Controls service is unavailable. Start it with a Factory CLI command, then reconnect."
        : error instanceof Error
          ? error.message
          : "Controller request failed";
    json(res, 502, { error: message });
  }
  return true;
}
