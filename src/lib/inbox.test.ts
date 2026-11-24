import { test } from "node:test";
import assert from "node:assert/strict";
import { addressedToOperator, operatorConversation } from "./inbox";
import type { Message } from "./types";
const message = (sender: string, recipients: string[], kind = "message") =>
  ({
    sender_name: sender,
    recipient_names: recipients,
    payload: { kind },
  }) as Message;
test("operator unread notifications include the human participant but exclude peers and lifecycle broadcasts", () => {
  assert.equal(addressedToOperator(message("architect", ["human"])), true);
  assert.equal(addressedToOperator(message("architect", ["operator"])), true);
  assert.equal(addressedToOperator(message("architect", ["reviewer"])), false);
  assert.equal(addressedToOperator(message("human", ["architect"])), false);
  assert.equal(
    addressedToOperator(message("architect", ["human"], "runtime_status")),
    false,
  );
  assert.equal(operatorConversation(message("human", ["architect"])), true);
});
