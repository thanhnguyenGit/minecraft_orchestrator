import { describe, it } from "node:test";
import assert from "node:assert/strict";

import {
  commandStream,
  decodeCommandFields,
  encodeEventFields,
  eventStream,
} from "../src/bus.js";
import {
  BotCommandSchema,
  BotEventSchema,
} from "../src/gen/orchestrator/v1/bot_pb.js";
import { create, toBinary } from "@bufbuild/protobuf";

describe("Redis stream helpers", () => {
  it("derives stream names from bot ids", () => {
    assert.equal(commandStream("king_crimson"), "mc:bot:king_crimson:commands");
    assert.equal(eventStream(), "mc:events");
  });

  it("round trips protobuf payload fields", () => {
    const event = create(BotEventSchema, {
      botId: "king_crimson",
      messageId: "msg-1",
      correlationId: "corr-1",
      payload: {
        case: "statusChanged",
        value: { state: "connected", detail: "spawned" },
      },
    });

    const fields = encodeEventFields(event);

    assert.equal(fields.bot_id, "king_crimson");
    assert.equal(fields.schema, "orchestrator.v1.BotEvent");
    assert.ok(fields.payload_b64.length > 0);
  });

  it("decodes command fields into a protobuf command", () => {
    const command = create(BotCommandSchema, {
      botId: "king_crimson",
      messageId: "msg-2",
      correlationId: "corr-2",
      payload: {
        case: "sendChat",
        value: { message: "hello" },
      },
    });

    const payload = Buffer.from(toBinary(BotCommandSchema, command)).toString("base64");
    const decoded = decodeCommandFields({
      bot_id: "king_crimson",
      message_id: "msg-2",
      correlation_id: "corr-2",
      schema: "orchestrator.v1.BotCommand",
      payload_b64: payload,
    });

    assert.equal(decoded.payload.case, "sendChat");
    assert.equal(decoded.payload.value.message, "hello");
  });
});
