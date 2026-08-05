import { EventEmitter } from "node:events";
import mineflayer from "mineflayer";
import {
  buildStateSnapshot,
  coalesceInventoryUpdates,
  shouldEmitMotion,
  type BotState,
  type Effect,
  type InventorySlot,
  type Position,
  type Vitals,
} from "./telemetry.js";

export type ConnectOptions = {
  host: string;
  port: number;
  username: string;
  auth: string;
  version: string;
};

const ignorePackets = new Set([
  "keep_alive",
  "update_time",
  "entity_velocity",
  "entity_look",
]);

export type BotStatus =
  "idle" | "connecting" | "connected" | "disconnected" | "error";
type MineflayerBot = ReturnType<typeof mineflayer.createBot>;
type MineflayerBotFactory = typeof mineflayer.createBot;

export type PhysicsDiagnostic = {
  kind:
    | "spawn"
    | "chunks_ready"
    | "chunks_unavailable"
    | "entity_velocity"
    | "physics_tick"
    | "move"
    | "forced_move";
  position: Position;
  velocity: { x: number; y: number; z: number };
  physicsEnabled: boolean;
  physicsTicks: number;
  blockLoaded: boolean;
};

export type MineflayerAdapterOptions = {
  physicsDebug?: boolean;
};

export interface BotAdapter {
  connect(options: ConnectOptions): Promise<void>;
  disconnect(): Promise<void>;
  status(): BotStatus;
  sendChat(message: string): Promise<void>;
}

export type MineflayerAdapterEvents = {
  status: [state: BotStatus, detail: string];
  spawn: [];
  chat: [username: string, message: string];
  kicked: [reason: string];
  error: [message: string];
  telemetrySnapshot: [state: BotState];
  vitalsChanged: [vitals: Vitals];
  effectsChanged: [effects: Effect[]];
  positionChanged: [position: Position];
  inventoryChanged: [
    slots: InventorySlot[],
    selectedHotbarSlot: number,
    selectedHotbarSlotChanged: boolean,
  ];
  physicsDiagnostic: [event: PhysicsDiagnostic];
  chunkLoaded: [
    chunkX: number,
    chunkZ: number,
    data: Uint8Array,
    minY: number,
    height: number,
  ];
  chunkUnloaded: [chunkX: number, chunkZ: number];
  blockUpdated: [x: number, y: number, z: number, stateId: number];
  multiBlocksUpdated: [
    records: { x: number; y: number; z: number; stateId: number }[],
  ];
};

export class MineflayerAdapter extends EventEmitter implements BotAdapter {
  #bot: MineflayerBot | undefined;
  #botFactory: MineflayerBotFactory;
  #physicsDebug: boolean;
  #status: BotStatus = "idle";
  #lastMotion: Position | undefined;
  #lastMotionEmittedAtMs = 0;
  #pendingInventoryUpdates: InventorySlot[] = [];
  #inventoryFlushQueued = false;
  #selectedHotbarSlotChanged = false;

  constructor(
    botFactory: MineflayerBotFactory = mineflayer.createBot,
    options: MineflayerAdapterOptions = {},
  ) {
    super();
    this.#botFactory = botFactory;
    this.#physicsDebug = options.physicsDebug ?? false;
  }

  async connect(options: ConnectOptions): Promise<void> {
    if (this.#bot) return;

    this.#status = "connecting";
    this.emit("status", this.#status, "connecting");

    this.#pendingInventoryUpdates = [];
    this.#inventoryFlushQueued = false;
    this.#selectedHotbarSlotChanged = false;
    this.#bot = this.#botFactory({
      host: options.host,
      port: options.port,
      username: options.username,
      auth: options.auth as "offline" | "microsoft",
      version: options.version,
    });

    this.#wireTelemetry(this.#bot);
    this.#wirePhysicsDiagnostics(this.#bot);
    this.#debugAllPackets(this.#bot);

    this.#bot.once("spawn", () => {
      this.#status = "connected";
      this.emit("status", this.#status, "spawned");
      this.emit("spawn");
      this.#lastMotion = undefined;
      this.#lastMotionEmittedAtMs = 0;
      this.emit(
        "telemetrySnapshot",
        buildStateSnapshot(this.#readState(this.#bot!)),
      );
    });
    this.#bot.on("chat", (username: string, message: string) => {
      this.emit("chat", username, message);
    });
    this.#bot.on("kicked", (reason: unknown) => {
      this.#status = "disconnected";
      this.emit("kicked", String(reason));
      this.emit("status", this.#status, "kicked");
      this.#bot = undefined;
    });
    this.#bot.on("error", (error: Error) => {
      this.#status = "error";
      this.emit("error", error.message);
      this.emit("status", this.#status, error.message);
    });
    this.#bot.on("end", () => {
      this.#status = "disconnected";
      this.emit("status", this.#status, "ended");
      this.#bot = undefined;
    });
  }

  async disconnect(): Promise<void> {
    if (!this.#bot) return;
    this.#bot.quit();
  }

  status(): BotStatus {
    return this.#status;
  }

  async sendChat(message: string): Promise<void> {
    if (!this.#bot) {
      throw new Error("bot is not connected");
    }
    this.#bot.chat(message);
  }

  #wireTelemetry(bot: MineflayerBot): void {
    bot.on("health", () => {
      this.emit("vitalsChanged", this.#readVitals(bot));
    });
    bot.on("breath", () => {
      this.emit("vitalsChanged", this.#readVitals(bot));
    });
    const emitEffects = (entity: { id: number }) => {
      if (bot.entity && entity.id === bot.entity.id)
        this.emit("effectsChanged", this.#readEffects(bot));
    };
    bot.on("entityEffect", emitEffects);
    bot.on("entityEffectEnd", emitEffects);
    bot.on("move", () => this.#emitMotionIfChanged(bot));
    bot.on("forcedMove", () => this.#emitMotionIfChanged(bot, true));
    bot.on("physicsTick", () => this.#emitMotionIfChanged(bot));
    bot.once("inject_allowed", () => this.#wireInventoryTelemetry(bot));
    this.#wireChunkEvents(bot);
  }

  #wireChunkEvents(bot: MineflayerBot): void {
    if (!bot._client) return;

    bot._client.on(
      "map_chunk",
      (packet: { x: number; z: number; data?: Buffer; chunkData?: Buffer }) => {
        if (bot !== this.#bot) return;

        // Get the length of the Buffer in bytes
        const buffer = packet.chunkData || packet.data;

        if (!buffer) {
          console.warn(
            `Missing chunk buffer for chunk at ${packet.x}, ${packet.z}`,
          );
          return;
        }

        // const sizeInBytes = buffer.length;
        // console.log(
        //   `Chunk at ${packet.x}, ${packet.z} is ${sizeInBytes} bytes`,
        // );

        this.emit(
          "chunkLoaded",
          packet.x,
          packet.z,
          buffer,
          (bot.game as unknown as Record<string, number>).minY,
          (bot.game as unknown as Record<string, number>).height,
        );
      },
    );

    bot._client.on(
      "unload_chunk",
      (packet: {
        x?: number;
        z?: number;
        chunkX?: number;
        chunkZ?: number;
      }) => {
        if (bot !== this.#bot) return;

        const cx = packet.x ?? packet.chunkX;
        const cz = packet.z ?? packet.chunkZ;

        if (cx === undefined || cz === undefined) return;

        this.emit("chunkUnloaded", cx, cz);
      },
    );

    bot._client.on(
      "block_change",
      (packet: {
        location: { x: number; y: number; z: number };
        type: number;
      }) => {
        if (bot !== this.#bot) return;

        // console.log("BLOCK_CHANGE received!");

        this.emit(
          "blockUpdated",
          packet.location.x,
          packet.location.y,
          packet.location.z,
          packet.type,
        );
      },
    );

    bot._client.on(
      "multi_block_change",
      (packet: {
        chunkCoordinates: { x: number; y: number; z: number };
        notTrustEdges?: boolean;
        records: number[]; 
      }) => {
        if (bot !== this.#bot) return;

        const sectionWorldX = packet.chunkCoordinates.x * 16;
        const sectionWorldY = packet.chunkCoordinates.y * 16;
        const sectionWorldZ = packet.chunkCoordinates.z * 16;

        const records = packet.records.map((record) => ({
          x: sectionWorldX + ((record >> 8) & 0x0f),
          y: sectionWorldY + (record & 0x0f),
          z: sectionWorldZ + ((record >> 4) & 0x0f),
          stateId: record >>> 12,
        }));
        this.emit("multiBlocksUpdated", records);
      },
    );
  }

  #wirePhysicsDiagnostics(bot: MineflayerBot): void {
    if (!this.#physicsDebug) return;

    let physicsTicks = 0;
    const emit = (
      kind: PhysicsDiagnostic["kind"],
      velocity?: { x: number; y: number; z: number },
    ): void => {
      if (bot !== this.#bot) return;
      const position = this.#readPosition(bot);
      this.emit("physicsDiagnostic", {
        kind,
        position,
        velocity: velocity ?? {
          x: position.velocityX,
          y: position.velocityY,
          z: position.velocityZ,
        },
        physicsEnabled: bot.physicsEnabled,
        physicsTicks,
        blockLoaded: Boolean(
          bot.entity?.position && bot.blockAt(bot.entity.position),
        ),
      } satisfies PhysicsDiagnostic);
    };
    const onSpawn = (): void => {
      emit("spawn");
      void bot
        .waitForChunksToLoad()
        .then(() => emit("chunks_ready"))
        .catch(() => emit("chunks_unavailable"));
    };
    const onVelocity = (packet: {
      entityId: number;
      velocity: { x: number; y: number; z: number };
    }): void => {
      if (packet.entityId !== bot.entity?.id) return;
      queueMicrotask(() => emit("entity_velocity", packet.velocity));
    };
    const onPhysicsTick = (): void => {
      physicsTicks++;
      if (physicsTicks % 20 === 0) emit("physics_tick");
    };
    const onMove = (): void => emit("move");
    const onForcedMove = (): void => emit("forced_move");
    const cleanup = (): void => {
      bot.removeListener("spawn", onSpawn);
      bot.removeListener("physicsTick", onPhysicsTick);
      bot.removeListener("move", onMove);
      bot.removeListener("forcedMove", onForcedMove);
      bot._client.removeListener("entity_velocity", onVelocity);
    };

    bot.on("spawn", onSpawn);
    bot.on("physicsTick", onPhysicsTick);
    bot.on("move", onMove);
    bot.on("forcedMove", onForcedMove);
    bot._client.on("entity_velocity", onVelocity);
    bot.once("end", cleanup);
  }

  #wireInventoryTelemetry(bot: MineflayerBot): void {
    if (bot !== this.#bot || !bot.inventory) return;
    bot.inventory.on("updateSlot", (slot, _oldItem, newItem) => {
      this.#pendingInventoryUpdates.push({
        slot,
        ...(newItem ? { item: this.#itemStack(newItem) } : {}),
      });
      this.#queueInventoryFlush(bot);
    });
    bot.on("heldItemChanged", () => {
      this.#selectedHotbarSlotChanged = true;
      this.#queueInventoryFlush(bot);
    });
  }

  #queueInventoryFlush(bot: MineflayerBot): void {
    if (this.#inventoryFlushQueued) return;
    this.#inventoryFlushQueued = true;
    queueMicrotask(() => {
      this.#inventoryFlushQueued = false;
      const slots = coalesceInventoryUpdates(
        this.#pendingInventoryUpdates.splice(0),
      );
      if (slots.length === 0 && !this.#selectedHotbarSlotChanged) return;
      const changed = this.#selectedHotbarSlotChanged;
      this.#selectedHotbarSlotChanged = false;
      this.emit("inventoryChanged", slots, bot.quickBarSlot, changed);
    });
  }

  #emitMotionIfChanged(bot: MineflayerBot, force = false): void {
    if (!bot.entity) return;
    const position = this.#readPosition(bot);
    const now = Date.now();
    if (
      !force &&
      !shouldEmitMotion(
        this.#lastMotion,
        position,
        this.#lastMotionEmittedAtMs,
        now,
      )
    )
      return;
    this.#lastMotion = position;
    this.#lastMotionEmittedAtMs = now;
    this.emit("positionChanged", position);
  }

  #readState(bot: MineflayerBot): BotState {
    const inventory = bot.inventory;
    return {
      vitals: this.#readVitals(bot),
      effects: this.#readEffects(bot),
      position: this.#readPosition(bot),
      inventory: {
        selectedHotbarSlot: bot.quickBarSlot,
        slots: inventory
          ? inventory.slots.map((item, slot) => ({
              slot,
              ...(item ? { item: this.#itemStack(item) } : {}),
            }))
          : [],
      },
    };
  }
  #readVitals(bot: MineflayerBot): Vitals {
    return {
      health: bot.health,
      food: bot.food,
      saturation: bot.foodSaturation,
      oxygen: bot.oxygenLevel,
    };
  }
  #readEffects(bot: MineflayerBot): Effect[] {
    if (!bot.entity) return [];
    const names = (
      bot.registry as { effects?: Record<number, { name?: string }> }
    ).effects;
    return Object.values(bot.entity.effects)
      .map((effect) => ({
        id: effect.id,
        name: names?.[effect.id]?.name ?? `effect_${effect.id}`,
        amplifier: effect.amplifier,
        durationTicks: effect.duration,
      }))
      .sort((left, right) => left.id - right.id);
  }
  #readPosition(bot: MineflayerBot): Position {
    const entity = bot.entity;
    if (!entity)
      return {
        dimension: bot.game.dimension,
        x: 0,
        y: 0,
        z: 0,
        yaw: 0,
        pitch: 0,
        velocityX: 0,
        velocityY: 0,
        velocityZ: 0,
      };
    return {
      dimension: bot.game.dimension,
      x: entity.position.x,
      y: entity.position.y,
      z: entity.position.z,
      yaw: entity.yaw,
      pitch: entity.pitch,
      velocityX: entity.velocity.x,
      velocityY: entity.velocity.y,
      velocityZ: entity.velocity.z,
    };
  }

  #debugAllPackets(bot: MineflayerBot): void {
    if (!bot._client) return;

    bot._client.on(
      "packet",
      (
        data: any,
        meta: {
          name: string;
          state: string;
        },
      ) => {
        // console.log(`\n[PACKET RECEIVED] Name: ${meta.name}`);
        // console.dir(data, { depth: 4, colors: true });
      },
    );
  }

  #itemStack(item: {
    type: number;
    name: string;
    metadata: number;
    count: number;
  }): NonNullable<InventorySlot["item"]> {
    return {
      id: item.type,
      name: item.name,
      metadata: item.metadata,
      count: item.count,
    };
  }
}
