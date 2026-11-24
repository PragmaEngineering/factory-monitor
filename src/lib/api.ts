import type { ReportDocument } from "./reports";
export type { ReportDocument } from "./reports";
import type { Execution, Session, Message, SessionMessage } from "./types";
export async function request<T>(
  route: string,
  body?: unknown,
  signal?: AbortSignal,
): Promise<T> {
  const response = await fetch("/api" + route, {
    method: body === undefined ? "GET" : "POST",
    signal,
    headers: body === undefined ? {} : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const data = await response.json();
  if (!response.ok)
    throw new Error(data.error || `Request failed (${response.status})`);
  return data;
}
export const api = {
  report: (executionID: string, signal?: AbortSignal) =>
    request<ReportDocument>("/reports", { execution_id: executionID }, signal),
  version: (signal?: AbortSignal) =>
    request<{ api: string }>("/version", undefined, signal),
  executions: (context: string, signal?: AbortSignal) =>
    request<Execution[]>(
      "/executions?context=" + encodeURIComponent(context),
      undefined,
      signal,
    ).then((x) => x ?? []),
  execution: (id: string, signal?: AbortSignal) =>
    request<{ execution: Execution }>(
      "/resources/" + encodeURIComponent(id),
      undefined,
      signal,
    ).then((x) => x.execution),
  sessions: (id: string, signal?: AbortSignal) =>
    request<Session[]>(
      "/sessions?execution_id=" + encodeURIComponent(id),
      undefined,
      signal,
    ).then((x) => x ?? []),
  messages: (id: string, signal?: AbortSignal) =>
    request<Message[]>(
      "/executions/" + encodeURIComponent(id) + "/messages",
      undefined,
      signal,
    ).then((x) => x ?? []),
  sessionMessages: (id: string, signal?: AbortSignal) =>
    request<SessionMessage[]>(
      "/sessions/" + encodeURIComponent(id) + "/messages",
      undefined,
      signal,
    ).then((x) => x ?? []),
  send: (id: string, body: string) =>
    request("/sessions/" + encodeURIComponent(id) + "/messages", { body }),
  answer: (id: string, outcome: string, reason: string) =>
    request("/requests/" + encodeURIComponent(id) + "/answer", {
      outcome,
      reason,
      actor: "operator",
    }),
  retry: (id: string, reason: string) =>
    request("/steps/" + encodeURIComponent(id) + "/retry", {
      reason,
      actor: "operator",
    }),
  cancel: (id: string) =>
    request("/executions/" + encodeURIComponent(id) + "/cancel", {}),
  exec: (
    config_path: string,
    brief: string,
    files: string[],
    context_id = "*",
  ) =>
    request<Execution>("/executions", {
      config_path,
      context_id,
      brief,
      files,
      actor: "operator",
    }),
  document: (id: string, name: string) =>
    request<{ name: string; path: string; content: string }>(
      `/document?execution=${encodeURIComponent(id)}&name=${encodeURIComponent(name)}`,
    ),
};
