import type { Message } from "./types";

const humanName = (name: string) =>
  ["human", "operator"].includes(name.toLowerCase());
/** Controls names the execution's human participant "human"; keep the older
 * operator alias readable too. Runtime lifecycle reports are not correspondence. */
export function addressedToOperator(message: Message): boolean {
  return (
    message.payload.kind !== "runtime_status" &&
    !humanName(message.sender_name) &&
    (message.recipient_names ?? []).some(humanName)
  );
}
export function operatorConversation(message: Message): boolean {
  return addressedToOperator(message) || humanName(message.sender_name);
}
const key = "pragma.monitor.operator-seen.v1";
export function loadSeen(): Record<string, string[]> {
  try {
    const value = JSON.parse(localStorage.getItem(key) ?? "{}");
    if (!value || typeof value !== "object" || Array.isArray(value)) return {};
    const seen: Record<string, string[]> = {};
    for (const [execution, ids] of Object.entries(value)) {
      if (Array.isArray(ids) && ids.every((id) => typeof id === "string"))
        seen[execution] = ids;
    }
    return seen;
  } catch {
    return {};
  }
}
export function saveSeen(value: Record<string, string[]>): void {
  try {
    localStorage.setItem(key, JSON.stringify(value));
  } catch {
    /* Session-only read state when browser storage is unavailable. */
  }
}
