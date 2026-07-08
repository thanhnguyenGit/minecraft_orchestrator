import { create } from "@bufbuild/protobuf";
import { randomUUID } from "node:crypto";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { createClient } from "redis";
import { config as loadDotenv } from "dotenv";

import { commandStream, decodeCommandFields, encodeEventFields, eventStream, type StreamFields } from "./bus.js";
import { dispatchCommand } from "./dispatcher.js";
import {
  BotEventSchema,
  ChatReceivedEventSchema,
  ErrorEventSchema,
  KickedEventSchema,
  SpawnedEventSchema,
  StatusChangedEventSchema,
  type BotEvent,
} from "./gen/orchestrator/v1/bot_pb.js";
import { MineflayerAdapter, type BotStatus } from "./mineflayer_adapter.js";

const rootDir = resolve(dirname(fileURLToPath(import.meta.url)), "../..");
loadDotenv({ path: resolve(rootDir, ".env") });

const botId = process.env.BOT_ID ?? "king_crimson";
const redisUrl = process.env.REDIS_URL ?? "redis://localhost:6379/0";

const redis = createClient({ url: redisUrl });
const adapter = new MineflayerAdapter();

function log(message: string): void {
  console.log(`[worker:${botId}] ${message}`);
}

function makeEvent(payload: BotEvent["payload"], correlationId = ""): BotEvent {
  return create(BotEventSchema, {
    botId,
    messageId: randomUUID(),
    correlationId,
    payload,
  });
}

async function publish(event: BotEvent): Promise<void> {
  await redis.xAdd(eventStream(), "*", encodeEventFields(event));
}

function wireAdapterEvents(): void {
  adapter.on("status", (state: BotStatus, detail: string) => {
    log(`status=${state} detail=${detail}`);
    void publish(makeEvent({
      case: "statusChanged",
      value: create(StatusChangedEventSchema, { state, detail }),
    }));
  });
  adapter.on("spawn", () => {
    log("spawned");
    void publish(makeEvent({ case: "spawned", value: create(SpawnedEventSchema, {}) }));
  });
  adapter.on("chat", (username: string, message: string) => {
    log(`chat <${username}> ${message}`);
    void publish(makeEvent({
      case: "chatReceived",
      value: create(ChatReceivedEventSchema, { username, message }),
    }));
  });
  adapter.on("kicked", (reason: string) => {
    log(`kicked reason=${reason}`);
    void publish(makeEvent({
      case: "kicked",
      value: create(KickedEventSchema, { reason }),
    }));
  });
  adapter.on("error", (message: string) => {
    log(`error ${message}`);
    void publish(makeEvent({
      case: "error",
      value: create(ErrorEventSchema, { message }),
    }));
  });
}

function normalizeFields(message: Record<string, unknown>): StreamFields {
  const fields: StreamFields = {};
  for (const [key, value] of Object.entries(message)) {
    fields[key] = String(value);
  }
  return fields;
}

async function processCommands(): Promise<void> {
  let lastId = "$";
  const stream = commandStream(botId);
  log(`listening on ${stream}`);

  for (;;) {
    const response = await redis.xRead({ key: stream, id: lastId }, { BLOCK: 5000, COUNT: 10 });
    if (!response) continue;

    for (const streamResponse of response as Array<{ messages: Array<{ id: string; message: Record<string, unknown> }> }>) {
      for (const message of streamResponse.messages) {
        lastId = message.id;
        const command = decodeCommandFields(normalizeFields(message.message));
        log(`command ${message.id} payload=${command.payload.case ?? "unknown"} correlation=${command.correlationId}`);
        try {
          await dispatchCommand(adapter, command);
          await publish(makeEvent({
            case: "statusChanged",
            value: create(StatusChangedEventSchema, { state: adapter.status(), detail: `handled ${command.payload.case}` }),
          }, command.correlationId));
        } catch (error) {
          const detail = error instanceof Error ? error.message : String(error);
          await publish(makeEvent({
            case: "error",
            value: create(ErrorEventSchema, { message: detail }),
          }, command.correlationId));
        }
      }
    }
  }
}

async function main(): Promise<void> {
  wireAdapterEvents();
  log(`starting redis=${redisUrl}`);
  await redis.connect();
  await publish(makeEvent({
    case: "statusChanged",
    value: create(StatusChangedEventSchema, { state: adapter.status(), detail: "worker started" }),
  }));
  log("started");
  await processCommands();
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
