export const short = (id: string) => id.slice(0, 8);
export const number = (value: number) =>
  new Intl.NumberFormat("en-US", {
    notation: value >= 10000 ? "compact" : "standard",
    maximumFractionDigits: 1,
  }).format(value);
export function elapsed(
  start?: string,
  end?: string,
  now = Date.now(),
): string {
  if (!start) return "—";
  const seconds = Math.max(
    0,
    Math.floor(((end ? Date.parse(end) : now) - Date.parse(start)) / 1000),
  );
  if (!Number.isFinite(seconds)) return "—";
  return seconds >= 3600
    ? `${Math.floor(seconds / 3600)}h ${Math.floor((seconds % 3600) / 60)}m`
    : `${Math.floor(seconds / 60)}m ${seconds % 60}s`;
}
export const time = (value: string) =>
  new Date(value).toLocaleTimeString([], {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
export const pending = (state: string) =>
  ["pending", "open", "waiting"].includes(state);
export const active = (state: string) =>
  ["running", "waiting_input", "waiting_peer", "queued"].includes(state);

/** An unvisited alternative is not pending work after an execution terminates. */
export function stepState(
  executionState: string,
  visit?: { state: string },
): string {
  return (
    visit?.state ??
    (["completed", "failed", "cancelled"].includes(executionState)
      ? "not_visited"
      : "pending")
  );
}
