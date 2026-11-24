import { test } from "node:test";
import assert from "node:assert/strict";
import { communicationGraph } from "./communications";
import type { Message } from "./types";
const message = (
  sender: string,
  recipients: string[],
  sender_name = sender,
  recipient_names = recipients,
) => ({ sender, sender_name, recipients, recipient_names }) as Message;

test("communication graph retains directed counts and separates reciprocal paths", () => {
  const graph = communicationGraph([
    message("a", ["b"], "author", ["reviewer"]),
    message("a", ["b"], "author", ["reviewer"]),
    message("b", ["a"], "reviewer", ["author"]),
  ]);
  assert.equal(graph.nodes.length, 2);
  const forward = graph.links.find((link) => link.from === "author")!;
  const reverse = graph.links.find((link) => link.from === "reviewer")!;
  assert.equal(forward.count, 2);
  assert.equal(reverse.count, 1);
  assert.notDeepEqual(forward.label, reverse.label);
  assert.equal(graph.nodes.find((node) => node.name === "author")!.received, 1);
  assert.equal(graph.nodes.find((node) => node.name === "author")!.sent, 2);
});
test("communication graph resolves known names, aliases humans, and does not infer broadcast recipients", () => {
  const unnamed = message("a", ["h"]);
  unnamed.sender_name = "";
  unnamed.recipient_names = undefined;
  const graph = communicationGraph([
    message("h", ["a"], "human", ["author"]),
    unnamed,
    message("a", [], "author"),
    message("a", ["a"], "author", ["author"]),
  ]);
  assert.deepEqual(
    graph.nodes.map((node) => node.name).sort(),
    ["Operator", "author"].sort(),
  );
  assert.equal(graph.links.length, 3);
  assert.ok(graph.links.every((link) => !link.path.includes("NaN")));
  assert.deepEqual(communicationGraph([]).nodes, []);
});
