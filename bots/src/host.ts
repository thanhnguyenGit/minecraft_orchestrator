import {
  create,
  fromBinary,
  toBinary,
  type MessageInitShape,
} from "@bufbuild/protobuf";
import { randomUUID } from "node:crypto";
import net from "node:net";
import {
  BotConnectionState,
  BotObservationSchema,
  BotSpawnedSchema,
  BotStatusChangedSchema,
  HostConfigureSchema,
  HostEffectsChangedSchema,
  HostEnvelopeSchema,
  HostHelloSchema,
  HostInventoryChangedSchema,
  HostInventorySlotSchema,
  HostInventorySchema,
  HostItemStackSchema,
  HostPositionChangedSchema,
  HostPositionSchema,
  HostPotionEffectSchema,
  HostStateSnapshotSchema,
  HostVitalsChangedSchema,
  HostVitalsSchema,
  HostBotStateSchema,
  type BotConfiguration,
  type BotObservation,
  type HostEnvelope,
} from "./gen/orchestrator/v1/host_pb.js";
import { MineflayerAdapter, type BotStatus } from "./mineflayer_adapter.js";
import { FrameDecoder, encodeFrame } from "./socket_frame.js";
import type {
  BotState,
  Effect,
  InventorySlot,
  Position,
  Vitals,
} from "./telemetry.js";

const option = (name: string): string => {
  const index = process.argv.indexOf(name);
  return index >= 0 ? (process.argv[index + 1] ?? "") : "";
};
const socketPath = option("--socket");
const token = option("--token");
if (!socketPath || !token) throw new Error("--socket and --token are required");
const key = (value: Uint8Array): string => Buffer.from(value).toString("hex");

type Send = (observation: BotObservation) => void;

class Controller {
  #adapter = new MineflayerAdapter(undefined, {
    physicsDebug: process.env.MINEFLAYER_PHYSICS_DEBUG === "true",
  });
  #session = "";
  #sequence = 0n;
  #retry = 1000;
  #retryScheduled = false;
  #stopped = false;
  constructor(
    private readonly config: BotConfiguration,
    private readonly send: Send,
  ) {
    this.#adapter.on("status", (status: BotStatus, detail: string) => {
      this.status(status, detail);
      if (status === "connected") {
        this.#retry = 1000;
        this.#retryScheduled = false;
      }
      if ((status === "disconnected" || status === "error") && !this.#stopped)
        this.schedule();
    });
    this.#adapter.on("kicked", (detail: string) =>
      this.observe({
        case: "statusChanged",
        value: create(BotStatusChangedSchema, {
          state: BotConnectionState.KICKED,
          detail,
        }),
      }),
    );
    // EventEmitter treats "error" specially; retain a listener while the
    // following status event remains the authoritative lifecycle observation.
    this.#adapter.on("error", () => {});
    this.#adapter.on("spawn", () =>
      this.observe({ case: "spawned", value: create(BotSpawnedSchema, {}) }),
    );
    this.#adapter.on("telemetrySnapshot", (state: BotState) =>
      this.observe({
        case: "stateSnapshot",
        value: create(HostStateSnapshotSchema, { state: hostState(state) }),
      }),
    );
    this.#adapter.on("vitalsChanged", (value: Vitals) =>
      this.observe({
        case: "vitalsChanged",
        value: create(HostVitalsChangedSchema, { vitals: hostVitals(value) }),
      }),
    );
    this.#adapter.on("effectsChanged", (values: Effect[]) =>
      this.observe({
        case: "effectsChanged",
        value: create(HostEffectsChangedSchema, {
          effects: values.map(hostEffect),
        }),
      }),
    );
    this.#adapter.on("positionChanged", (value: Position) =>
      this.observe({
        case: "positionChanged",
        value: create(HostPositionChangedSchema, {
          position: hostPosition(value),
        }),
      }),
    );
    this.#adapter.on(
      "inventoryChanged",
      (
        slots: InventorySlot[],
        selectedHotbarSlot: number,
        selectedHotbarSlotChanged: boolean,
      ) =>
        this.observe({
          case: "inventoryChanged",
          value: create(HostInventoryChangedSchema, {
            slots: slots.map(hostSlot),
            selectedHotbarSlot,
            selectedHotbarSlotChanged,
          }),
        }),
    );
    this.#adapter.on("physicsDiagnostic", (event) =>
      console.log(
        JSON.stringify({
          msg: "mineflayer.physics",
          profileId: key(this.config.profileId),
          username: this.config.username,
          sessionId: this.#session,
          ...event,
        }),
      ),
    );
  }
  start(): void {
    this.connect();
  }
  stop(): void {
    this.#stopped = true;
    void this.#adapter.disconnect();
  }
  private connect(): void {
    if (this.#stopped) return;
    this.#session = randomUUID();
    this.#sequence = 0n;
    this.#retryScheduled = false;
    void this.#adapter.connect({
      host: this.config.host,
      port: this.config.port,
      username: this.config.username,
      auth: this.config.auth,
      version: this.config.version,
    });
  }
  private schedule(): void {
    if (this.#retryScheduled) return;
    this.#retryScheduled = true;
    const delay = this.#retry;
    this.#retry = Math.min(this.#retry * 2, 30_000);
    setTimeout(() => this.connect(), delay).unref();
  }
  private status(status: BotStatus, detail: string): void {
    const state = (
      {
        idle: BotConnectionState.UNSPECIFIED,
        connecting: BotConnectionState.CONNECTING,
        connected: BotConnectionState.CONNECTED,
        disconnected: BotConnectionState.DISCONNECTED,
        error: BotConnectionState.ERROR,
      } as const
    )[status];
    this.observe({
      case: "statusChanged",
      value: create(BotStatusChangedSchema, { state, detail }),
    });
  }
  private observe(
    payload: MessageInitShape<typeof BotObservationSchema>["payload"],
  ): void {
    this.send(
      create(BotObservationSchema, {
        profileId: this.config.profileId,
        sessionId: this.#session,
        sequence: ++this.#sequence,
        observedAtUnixMs: BigInt(Date.now()),
        payload,
      }),
    );
  }
}

function hostVitals(value: Vitals) {
  return create(HostVitalsSchema, value);
}
function hostEffect(value: Effect) {
  return create(HostPotionEffectSchema, value);
}
function hostPosition(value: Position) {
  return create(HostPositionSchema, value);
}
function hostSlot(value: InventorySlot) {
  return create(HostInventorySlotSchema, {
    slot: value.slot,
    ...(value.item ? { item: create(HostItemStackSchema, value.item) } : {}),
  });
}
function hostState(value: BotState) {
  return create(HostBotStateSchema, {
    vitals: hostVitals(value.vitals),
    effects: value.effects.map(hostEffect),
    position: hostPosition(value.position),
    inventory: create(HostInventorySchema, {
      selectedHotbarSlot: value.inventory.selectedHotbarSlot,
      slots: value.inventory.slots.map(hostSlot),
    }),
    gameMode: 0,
  });
}

const socket = net.createConnection(socketPath);
const decoder = new FrameDecoder();
const controllers = new Map<string, Controller>();

function send(payload: HostEnvelope["payload"]): void {
  socket.write(
    encodeFrame(
      toBinary(HostEnvelopeSchema, create(HostEnvelopeSchema, { payload })),
    ),
  );
}

socket.on("connect", () =>
  send({
    case: "hello",
    value: create(HostHelloSchema, { token, protocolVersion: 1 }),
  }),
);

socket.on("data", (chunk: Buffer) => {
  for (const frame of decoder.push(chunk)) {
    const envelope = fromBinary(HostEnvelopeSchema, frame);
    switch (envelope.payload.case) {
      case "configure": {
        const config = create(HostConfigureSchema, envelope.payload.value);
        for (const bot of config.bots) {
          if (controllers.has(key(bot.profileId))) continue;
          const controller = new Controller(bot, (observation) =>
            send({ case: "observation", value: observation }),
          );
          controllers.set(key(bot.profileId), controller);
          setTimeout(() => controller.start(), controllers.size * 250).unref();
        }
        break;
      }
      case "shutdown":
        for (const controller of controllers.values()) controller.stop();
        socket.end();
        break;
      default:
        break;
    }
  }
});
socket.on("error", (error) => {
  console.error(error);
  process.exitCode = 1;
});
socket.on("close", () => {
  for (const controller of controllers.values()) controller.stop();
  process.exit(1);
});
