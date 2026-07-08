import { fromBinary, toBinary } from "@bufbuild/protobuf";

import {
  type BotCommand,
  BotCommandSchema,
  type BotEvent,
  BotEventSchema,
} from "./gen/orchestrator/v1/bot_pb.js";

export type StreamFields = Record<string, string>;

export function commandStream(botId: string): string {
  return `mc:bot:${botId}:commands`;
}

export function eventStream(): string {
  return "mc:events";
}

export function encodeEventFields(event: BotEvent): StreamFields {
  return {
    bot_id: event.botId,
    message_id: event.messageId,
    correlation_id: event.correlationId,
    schema: "orchestrator.v1.BotEvent",
    payload_b64: Buffer.from(toBinary(BotEventSchema, event)).toString("base64"),
  };
}

export function decodeCommandFields(fields: StreamFields): BotCommand {
  if (fields.schema !== "orchestrator.v1.BotCommand") {
    throw new Error(`unexpected command schema ${fields.schema}`);
  }
  if (!fields.payload_b64) {
    throw new Error("missing payload_b64");
  }

  return fromBinary(BotCommandSchema, Buffer.from(fields.payload_b64, "base64"));
}
