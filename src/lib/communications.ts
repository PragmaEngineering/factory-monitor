import type { Message } from "./types";

export interface CommunicationNode {
  name: string;
  x: number;
  y: number;
  sent: number;
  received: number;
}
export interface CommunicationLink {
  from: string;
  to: string;
  count: number;
  path: string;
  label: { x: number; y: number };
}

const nameOf = (name: string) =>
  ["operator", "human"].includes(name.toLowerCase()) ? "Operator" : name;

/** Aggregate explicit sender/recipient pairs by participant name. This is a
 * persisted communication map, not inferred runtime/session connectivity. */
export function communicationGraph(messages: Message[]) {
  const names = new Map<string, string>();
  for (const message of messages) {
    if (message.sender_name)
      names.set(message.sender, nameOf(message.sender_name));
    message.recipients.forEach((id, index) => {
      const name = message.recipient_names?.[index];
      if (name) names.set(id, nameOf(name));
    });
  }
  const counts = new Map<string, { from: string; to: string; count: number }>();
  const participants = new Set<string>();
  for (const message of messages) {
    const from = nameOf(
      message.sender_name || names.get(message.sender) || message.sender,
    );
    participants.add(from);
    const recipients = message.recipient_names?.length
      ? message.recipient_names.map(nameOf)
      : message.recipients.map((id) => names.get(id) || nameOf(id));
    for (const to of new Set(recipients)) {
      participants.add(to);
      const key = JSON.stringify([from, to]);
      const link = counts.get(key) ?? { from, to, count: 0 };
      link.count++;
      counts.set(key, link);
    }
  }
  const ordered = [...participants].sort((a, b) => a.localeCompare(b));
  const radius = ordered.length <= 1 ? 0 : Math.max(135, ordered.length * 21);
  const size = radius * 2 + 200;
  const nodes: CommunicationNode[] = ordered.map((name, index) => {
    const angle = -Math.PI / 2 + (index * 2 * Math.PI) / ordered.length;
    return {
      name,
      x: size / 2 + Math.cos(angle) * radius,
      y: size / 2 + Math.sin(angle) * radius,
      sent: 0,
      received: 0,
    };
  });
  const byName = new Map(nodes.map((node) => [node.name, node]));
  const links: CommunicationLink[] = [...counts.values()]
    .sort((a, b) => a.from.localeCompare(b.from) || a.to.localeCompare(b.to))
    .map((link) => {
      const from = byName.get(link.from)!,
        to = byName.get(link.to)!;
      from.sent += link.count;
      to.received += link.count;
      if (from === to)
        return {
          ...link,
          path: `M ${from.x - 20} ${from.y - 15} C ${from.x - 80} ${from.y - 100}, ${from.x + 80} ${from.y - 100}, ${from.x + 20} ${from.y - 15}`,
          label: { x: from.x, y: from.y - 77 },
        };
      const dx = to.x - from.x,
        dy = to.y - from.y,
        distance = Math.hypot(dx, dy);
      const ux = dx / distance,
        uy = dy / distance;
      const start = { x: from.x + ux * 29, y: from.y + uy * 29 };
      const end = { x: to.x - ux * 33, y: to.y - uy * 33 };
      // The perpendicular offset separates reciprocal directions into distinct arcs.
      const control = {
        x: (from.x + to.x) / 2 - uy * distance * 0.22,
        y: (from.y + to.y) / 2 + ux * distance * 0.22,
      };
      return {
        ...link,
        path: `M ${start.x} ${start.y} Q ${control.x} ${control.y} ${end.x} ${end.y}`,
        label: {
          x: (start.x + 2 * control.x + end.x) / 4,
          y: (start.y + 2 * control.y + end.y) / 4,
        },
      };
    });
  return { nodes, links, size };
}

export function participantLabel(name: string): string[] {
  const words = name.replaceAll("_", " ").split(/\s+/);
  const lines = [""];
  for (const word of words) {
    const last = lines.length - 1;
    if (lines[last] && (lines[last] + " " + word).length > 20) lines.push(word);
    else lines[last] += (lines[last] ? " " : "") + word;
  }
  return lines
    .slice(0, 2)
    .map((line) => (line.length > 22 ? line.slice(0, 21) + "…" : line));
}
