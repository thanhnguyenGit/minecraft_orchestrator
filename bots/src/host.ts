import {
  create,
  fromBinary,
  toBinary,
  type MessageInitShape,
} from "@bufbuild/protobuf";
import { randomUUID } from "node:crypto";
import net from "node:net";
import {
  HostBlockUpdatedSchema,
  HostChunkLoadedSchema,
  HostChunkUnloadedSchema,
  HostMultiBlocksUpdatedSchema,
  HostEffectsChangedSchema,
  HostEntitiesChangedSchema,
  HostEntitySchema,
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
  BotConnectionState,
  BotObservationSchema,
  BotSpawnedSchema,
  BotStatusChangedSchema,
  ActionOutcomeSchema,
  ControllerActionKind,
  CommandStatus,
  HostConfigureSchema,
  ControllerStateSchema,
  ControllerField,
  RealityStateSchema,
  Vec3iSchema,
  type BotConfiguration,
  type BotObservation,
  type HostEnvelope,
} from "./gen/orchestrator/v1/host_pb.js";
import {
  MineflayerAdapter,
  type BotStatus,
  type ControllerActionOutcome,
  type RealityState,
} from "./mineflayer_adapter.js";
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
  #adapter: MineflayerAdapter;
  #session = "";
  #sequence = 0n;
  #retry = 1000;
  #retryScheduled = false;
  #stopped = false;
  #actionOutcomes: ControllerActionOutcome[] = [];
  constructor(
    private readonly config: BotConfiguration,
    private readonly send: Send,
  ) {
    this.#adapter = new MineflayerAdapter(undefined, {
      physicsDebug: process.env.MINEFLAYER_PHYSICS_DEBUG === "true",
      diagnosticProfileID: key(this.config.profileId),
      diagnosticUsername: this.config.username,
    });
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
    this.#adapter.on(
      "chunkLoaded",
      (
        chunkX: number,
        chunkZ: number,
        data: Uint8Array,
        minY: number,
        height: number,
      ) =>
        this.observe({
          case: "chunkLoaded",
          value: create(HostChunkLoadedSchema, {
            chunkX,
            chunkZ,
            data,
            minY,
            height,
          }),
        }),
    );
    this.#adapter.on("chunkUnloaded", (chunkX: number, chunkZ: number) =>
      this.observe({
        case: "chunkUnloaded",
        value: create(HostChunkUnloadedSchema, { chunkX, chunkZ }),
      }),
    );
    this.#adapter.on(
      "blockUpdated",
      (x: number, y: number, z: number, stateId: number) =>
        this.observe({
          case: "blockUpdated",
          value: create(HostBlockUpdatedSchema, { x, y, z, stateId }),
        }),
    );
    this.#adapter.on(
      "multiBlocksUpdated",
      (records: { x: number; y: number; z: number; stateId: number }[]) =>
        this.observe({
          case: "multiBlocksUpdated",
          value: create(HostMultiBlocksUpdatedSchema, {
            records: records.map((r) => ({
              x: r.x,
              y: r.y,
              z: r.z,
              stateId: r.stateId,
            })),
          }),
        }),
    );
    this.#adapter.on(
      "entitiesChanged",
      (
        added: {
          entityId: number;
          name: string;
          x: number;
          y: number;
          z: number;
          yaw: number;
          pitch: number;
        }[],
        removed: number[],
        moved: {
          entityId: number;
          name: string;
          x: number;
          y: number;
          z: number;
          yaw: number;
          pitch: number;
        }[],
      ) =>
        this.observe({
          case: "entitiesChanged",
          value: create(HostEntitiesChangedSchema, {
            added: added.map((e) =>
              create(HostEntitySchema, {
                entityId: e.entityId,
                name: e.name,
                x: e.x,
                y: e.y,
                z: e.z,
                yaw: e.yaw,
                pitch: e.pitch,
              }),
            ),
            removed,
            moved: moved.map((e) =>
              create(HostEntitySchema, {
                entityId: e.entityId,
                name: e.name,
                x: e.x,
                y: e.y,
                z: e.z,
                yaw: e.yaw,
                pitch: e.pitch,
              }),
            ),
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
    this.#adapter.on("reality", (state: RealityState) => {
      const rs = create(RealityStateSchema, {
        profileId: this.config.profileId,
        sequence: this.#sequence,
		sessionId: this.#session,
      });
      if (state.arrivalDistance !== undefined) {
        rs.arrivalDistance = state.arrivalDistance;
      }
      if (state.diggingBlock) {
        rs.diggingBlock = create(Vec3iSchema, state.diggingBlock);
      }
      if (state.attackingEntity !== undefined) {
        rs.attackingEntity = state.attackingEntity;
      }
      if (state.equippedItem !== undefined) {
        rs.equippedItem = state.equippedItem;
      }
      if (state.gotoBlock) {
        rs.gotoTarget = create(Vec3iSchema, state.gotoBlock);
      }
      rs.actionOutcomes = this.#actionOutcomes.splice(0).map((outcome) =>
        create(ActionOutcomeSchema, {
          controllerSequence: outcome.controllerSequence,
          kind: controllerActionKind(outcome.kind),
          status: outcome.succeeded
            ? CommandStatus.COMPLETED
            : CommandStatus.FAILED,
          detail: outcome.detail,
        }),
      );
      socket.write(
        encodeFrame(
          toBinary(
            HostEnvelopeSchema,
            create(HostEnvelopeSchema, {
              payload: { case: "realityState", value: rs },
            }),
          ),
        ),
      );
    });
    this.#adapter.on("actionOutcome", (outcome: ControllerActionOutcome) =>
      this.#actionOutcomes.push(outcome),
    );
  }
  start(): void {
    this.connect();
  }
  stop(): void {
    this.#stopped = true;
    void this.#adapter.disconnect();
  }
  applyState(delta: Partial<{ goto: { x: number; y: number; z: number } | null; break: { x: number; y: number; z: number } | null; attack: number | null; craft: { itemName: string; count: number } | null; equip: string | null; consume: string | null; place: { x: number; y: number; z: number; fx: number; fy: number; fz: number } | null }>, sequence: bigint): void {
    this.#adapter.applyState(delta, sequence);
  }
  private connect(): void {
    if (this.#stopped) return;
	this.#actionOutcomes = [];
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
    value: create(HostHelloSchema, { token, protocolVersion: 2 }),
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
      case "controllerState": {
        const cs = create(ControllerStateSchema, envelope.payload.value);
        const profileKey = key(cs.profileId);
        const ctl = controllers.get(profileKey);
        if (ctl) {
          const delta: Record<string, unknown> = {};
          for (const field of cs.clearFields) {
            switch (field) {
              case ControllerField.GOTO_TARGET: delta.goto = null; break;
              case ControllerField.BREAK_TARGET: delta.break = null; break;
              case ControllerField.ATTACK_TARGET: delta.attack = null; break;
              case ControllerField.CRAFT_TARGET: delta.craft = null; break;
              case ControllerField.EQUIP_TARGET: delta.equip = null; break;
              case ControllerField.PLACE_TARGET: delta.place = null; break;
              case ControllerField.CONSUME_TARGET: delta.consume = null; break;
            }
          }
          if (cs.goToTarget !== undefined) {
            delta.goto = cs.goToTarget ? { x: cs.goToTarget.x, y: cs.goToTarget.y, z: cs.goToTarget.z } : null;
          }
          if (cs.breakTarget !== undefined) {
            delta.break = cs.breakTarget ? { x: cs.breakTarget.x, y: cs.breakTarget.y, z: cs.breakTarget.z } : null;
          }
          if (cs.attackTarget !== undefined) {
            delta.attack = cs.attackTarget ?? null;
          }
          if (cs.craftTarget !== undefined) {
            delta.craft = cs.craftTarget ? { itemName: cs.craftTarget.itemName, count: cs.craftTarget.count } : null;
          }
          if (cs.equipTarget !== undefined) {
            delta.equip = cs.equipTarget ?? null;
          }
          if (cs.consumeTarget !== undefined) {
            delta.consume = cs.consumeTarget ?? null;
          }
          if (cs.placeTarget !== undefined) {
            delta.place = cs.placeTarget ? { x: cs.placeTarget.x, y: cs.placeTarget.y, z: cs.placeTarget.z, fx: cs.placeTarget.faceX, fy: cs.placeTarget.faceY, fz: cs.placeTarget.faceZ } : null;
          }
          ctl.applyState(delta, cs.sequence);
        }
        break;
      }
      default:
        break;
    }
  }
});

function controllerActionKind(kind: ControllerActionOutcome["kind"]): ControllerActionKind {
  switch (kind) {
    case "goto": return ControllerActionKind.GOTO;
    case "break": return ControllerActionKind.BREAK;
    case "craft": return ControllerActionKind.CRAFT;
    case "consume": return ControllerActionKind.CONSUME;
    case "place": return ControllerActionKind.PLACE;
    case "attack": return ControllerActionKind.ATTACK;
    case "equip": return ControllerActionKind.EQUIP;
  }
}
socket.on("error", (error) => {
  console.error(error);
  process.exitCode = 1;
});
socket.on("close", () => {
  for (const controller of controllers.values()) controller.stop();
  process.exit(1);
});
