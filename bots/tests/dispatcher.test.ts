import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { create } from "@bufbuild/protobuf";

import { dispatchCommand } from "../src/dispatcher.js";
import { BotCommandSchema } from "../src/gen/orchestrator/v1/bot_pb.js";
import type { BotAdapter } from "../src/mineflayer_adapter.js";

describe("command dispatcher", () => {
  it("dispatches send_chat to the bot adapter", async () => {
    const sent: string[] = [];
    const adapter: BotAdapter = {
      connect: async () => {},
      disconnect: async () => {},
      status: () => "connected",
      sendChat: async (message) => {
        sent.push(message);
      },
    };

    const command = create(BotCommandSchema, {
      botId: "king_crimson",
      messageId: "msg-1",
      correlationId: "corr-1",
      payload: {
        case: "sendChat",
        value: { message: "hello from go" },
      },
    });

    await dispatchCommand(adapter, command);

    assert.deepEqual(sent, ["hello from go"]);
  });
});
