import { EventEmitter } from "node:events";
import { createRequire } from "node:module";
import type { Vec3 } from "vec3";
import mineflayer from "mineflayer";
import pf from "mineflayer-pathfinder";

const require = createRequire(import.meta.url);
const vec3 = require("vec3") as (x: number, y: number, z: number) => Vec3;
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
  diagnosticProfileID?: string;
  diagnosticUsername?: string;
};

export type ControllerState = {
  goto?: { x: number; y: number; z: number } | null;
  break?: { x: number; y: number; z: number } | null;
  attack?: number | null;
  craft?: { itemName: string; count: number } | null;
  equip?: string | null;
  consume?: string | null;
  place?: { x: number; y: number; z: number; fx: number; fy: number; fz: number } | null;
};

// ControllerState carries deltas: an omitted field keeps its previous target,
// while null explicitly clears that target.
export function mergeControllerState(
  state: ControllerState,
  delta: Partial<ControllerState>,
): ControllerState {
  return { ...state, ...delta };
}

export type RealityState = {
  arrivalDistance?: number;
  diggingBlock?: { x: number; y: number; z: number };
  attackingEntity?: number;
  equippedItem?: string;
  gotoBlock?: { x: number; y: number; z: number };
};

export type ControllerActionOutcome = {
  controllerSequence: bigint;
  kind: "goto" | "break" | "attack" | "craft" | "equip" | "consume" | "place";
  succeeded: boolean;
  detail: string;
};

// ControllerDiagnosticDetailMaxLength bounds error-derived controller details
// before they reach either local diagnostics or host reality emissions.
export const ControllerDiagnosticDetailMaxLength = 256;
const controllerDetailTruncationSuffix = "...";

export interface BotAdapter {
  connect(options: ConnectOptions): Promise<void>;
  disconnect(): Promise<void>;
  status(): BotStatus;
  sendChat(message: string): Promise<void>;
  applyState(delta: Partial<ControllerState>): void;
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
  entitiesChanged: [
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
  ];
  reality: [state: RealityState];
  actionOutcome: [outcome: ControllerActionOutcome];
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
  #pendingEntities: {
    added: Map<
      number,
      {
        entityId: number;
        name: string;
        x: number;
        y: number;
        z: number;
        yaw: number;
        pitch: number;
      }
    >;
    removed: Set<number>;
    moved: Map<
      number,
      {
        entityId: number;
        name: string;
        x: number;
        y: number;
        z: number;
        yaw: number;
        pitch: number;
      }
    >;
  } = { added: new Map(), removed: new Set(), moved: new Map() };
  #entityFlushTimer: ReturnType<typeof setTimeout> | undefined;
  #tickTimer: ReturnType<typeof setInterval> | undefined;
  #state: ControllerState = {};
  #actionSequences: Partial<Record<ControllerActionOutcome["kind"], bigint>> = {};
  #lastAppliedControllerSequence = 0n;
  #controllerSessionEpoch = 0;
  #reportedOutcomes = new Set<string>();
  #startedActions = new Set<string>();
  #terminalActions = new Set<string>();
  #diagnosticProfileID: string;
  #diagnosticUsername: string;

  constructor(
    botFactory: MineflayerBotFactory = mineflayer.createBot,
    options: MineflayerAdapterOptions = {},
  ) {
    super();
    this.#botFactory = botFactory;
    this.#physicsDebug = options.physicsDebug ?? false;
    this.#diagnosticProfileID = options.diagnosticProfileID ?? "";
    this.#diagnosticUsername = options.diagnosticUsername ?? "";
  }

  async connect(options: ConnectOptions): Promise<void> {
    if (this.#bot) return;

	this.#resetControllerSession();
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
    this.#wireEntityTelemetry(this.#bot);
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

      this.#bot!.loadPlugin(pf.pathfinder);
      const moves = new pf.Movements(this.#bot!);
      moves.canDig = false;
      moves.allowFreeMotion = false;
      (this.#bot! as any).pathfinder.setMovements(moves);

      this.#startTick();
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
      this.#stopTick();
      this.#bot = undefined;
    });
  }

	// Controller sequences and action targets are scoped to one host session.
	// A reconnect may reuse sequence numbers, so neither stale targets nor
	// prior outcome de-duplication may survive into the new bot instance.
	#resetControllerSession(): void {
		this.#controllerSessionEpoch++;
		this.#state = {};
		this.#actionSequences = {};
		this.#lastAppliedControllerSequence = 0n;
		this.#reportedOutcomes.clear();
		this.#startedActions.clear();
		this.#terminalActions.clear();
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

  applyState(delta: Partial<ControllerState>, controllerSequence = 0n): void {
    this.#lastAppliedControllerSequence = controllerSequence;
    this.#state = mergeControllerState(this.#state, delta);
    this.#recordActionSequences(delta, controllerSequence);
    if (delta.goto !== undefined) {
      if (delta.goto === null) (this.#bot as any)?.pathfinder?.setGoal(null);
    }
    if (delta.break !== undefined) {
      if (delta.break === null && this.#bot?.targetDigBlock) this.#bot.stopDigging();
    }
    this.#diagnostic("controller.state_applied");
  }

  #recordActionSequences(delta: Partial<ControllerState>, sequence: bigint): void {
    const fields: [keyof ControllerState, ControllerActionOutcome["kind"]][] = [
      ["goto", "goto"],
      ["break", "break"],
      ["attack", "attack"],
      ["craft", "craft"],
      ["equip", "equip"],
      ["consume", "consume"],
      ["place", "place"],
    ];
    for (const [field, kind] of fields) {
      if (delta[field] === undefined) continue;
      if (delta[field] === null) {
        delete this.#actionSequences[kind];
        continue;
      }
      this.#actionSequences[kind] = sequence;
    }
  }

  #startTick(): void {
    if (this.#tickTimer) return;
    this.#tickTimer = setInterval(() => this.#tick(), 100);
  }

  #stopTick(): void {
    if (this.#tickTimer) {
      clearInterval(this.#tickTimer);
      this.#tickTimer = undefined;
    }
  }

  #tick(): void {
    const bot = this.#bot!;
    if (!bot) return;

    const state = this.#state;
    const gotoDistance = state.goto
      ? bot.entity!.position.distanceTo(vec3(state.goto.x, state.goto.y, state.goto.z))
      : null;
    this.#diagnostic("controller.tick", {
      goto_distance: gotoDistance,
      digging: Boolean(bot.targetDigBlock),
      digging_block: bot.targetDigBlock ? this.#blockPosition(bot.targetDigBlock.position) : null,
    });

    if (state.goto) {
      const GoalNear = (pf.goals as any).GoalNear;
      const existingGoal = (bot as any).pathfinder.goal;
      const goalX = state.goto.x | 0;
      const goalY = state.goto.y | 0;
      const goalZ = state.goto.z | 0;
      if (
        !existingGoal ||
        existingGoal.x !== goalX ||
        existingGoal.y !== goalY ||
        existingGoal.z !== goalZ
      ) {
        (bot as any).pathfinder.setGoal(new GoalNear(goalX, goalY, goalZ, 1));
        this.#reportOutcome("goto", true, "navigation_started");
      }
    }

    if (state.break) {
      this.#handleBreak(bot, state.break);
    }

    if (state.attack !== undefined && state.attack !== null) {
      this.#handleAttack(bot, state.attack);
    }

    if (state.craft) {
      this.#handleCraft(bot, state.craft);
      this.#state = { ...this.#state, craft: null };
    }

    if (state.equip) {
      this.#handleEquip(bot, state.equip);
      this.#state = { ...this.#state, equip: null };
    }

    if (state.consume) {
      this.#handleConsume(bot, state.consume);
      this.#state = { ...this.#state, consume: null };
    }

    if (state.place) {
      this.#handlePlace(bot, state.place);
      this.#state = { ...this.#state, place: null };
    }

    const reality = this.#buildReality(bot);
    this.#diagnostic("controller.reality", {
      goto_distance: reality.arrivalDistance ?? null,
      digging: Boolean(reality.diggingBlock),
      digging_block: reality.diggingBlock ?? null,
      attacking_entity: reality.attackingEntity ?? null,
    });
    this.emit("reality", reality);
  }

  #handleBreak(bot: MineflayerBot, target: NonNullable<ControllerState["break"]>): void {
    const targetPosition = vec3(target.x, target.y, target.z);
    const controllerSequence = this.#actionSequences.break ?? 0n;
    const actionKey = this.#actionKey("break", controllerSequence, target);
    if (this.#terminalActions.has(actionKey)) return;

    const dist = bot.entity!.position.distanceTo(targetPosition);
    if (dist > 4.5) {
      this.#diagnostic("controller.break_disposition", {
        disposition: "waiting_for_range",
        target: this.#blockPosition(targetPosition),
        distance: dist,
      });
      return;
    }

    const block = bot.blockAt(targetPosition);
    if (!block || block.name === "air") {
      this.#reportTerminalOutcome("break", actionKey, controllerSequence, false, "target_block_unavailable");
      return;
    }

    if (!bot.canSeeBlock(block) || !bot.canDigBlock(block)) {
      this.#reportTerminalOutcome("break", actionKey, controllerSequence, false, "target_not_diggable");
      return;
    }

    if (!bot.targetDigBlock && !this.#startedActions.has(actionKey)) {
      const sessionEpoch = this.#controllerSessionEpoch;
      this.#startedActions.add(actionKey);
      bot.dig(block, false, "raycast").then(
        () => {
          if (this.#controllerSessionEpoch !== sessionEpoch) return;
          this.#reportTerminalOutcome("break", actionKey, controllerSequence, true, "dig_completed");
        },
        (error: unknown) => {
          if (this.#controllerSessionEpoch !== sessionEpoch) return;
          this.#reportTerminalOutcome("break", actionKey, controllerSequence, false, String(error));
        },
      );
    }
  }

  #handleAttack(bot: MineflayerBot, targetEntityId: number): void {
    const controllerSequence = this.#actionSequences.attack ?? 0n;
    const actionKey = this.#actionKey("attack", controllerSequence, targetEntityId);
    if (this.#terminalActions.has(actionKey)) return;

    const entity = bot.entities[targetEntityId];
    if (!entity) {
      this.#reportTerminalOutcome("attack", actionKey, controllerSequence, false, "target_entity_unavailable");
      return;
    }

    const dist = bot.entity!.position.distanceTo(entity.position);
    if (dist > 3.5) {
      this.#diagnostic("controller.attack_disposition", {
        disposition: "waiting_for_range",
        target_entity_id: targetEntityId,
        distance: dist,
      });
      return;
    }

    try {
      bot.lookAt(entity.position.offset(0, entity.height ?? 1, 0));
      bot.attack(entity);
      this.#reportTerminalOutcome("attack", actionKey, controllerSequence, true, "attack_sent");
    } catch (error) {
      this.#reportTerminalOutcome("attack", actionKey, controllerSequence, false, String(error));
    }
  }

  #actionKey(kind: ControllerActionOutcome["kind"], controllerSequence: bigint, target: unknown): string {
    return `${kind}:${controllerSequence}:${JSON.stringify(target)}`;
  }

  #reportTerminalOutcome(
    kind: ControllerActionOutcome["kind"],
    actionKey: string,
    controllerSequence: bigint,
    succeeded: boolean,
    detail: string,
  ): void {
    this.#startedActions.delete(actionKey);
    if (this.#terminalActions.has(actionKey)) return;
    this.#terminalActions.add(actionKey);
    this.#reportOutcome(kind, succeeded, detail, controllerSequence, actionKey);
  }

  #handleCraft(bot: MineflayerBot, target: NonNullable<ControllerState["craft"]>): void {
    const itemID = bot.registry.itemsByName[target.itemName]?.id;
    if (itemID === undefined) {
      this.#reportOutcome("craft", false, "unknown_item");
      return;
    }
    const recipes = bot.recipesFor(itemID, null, null, null);
    if (!recipes || recipes.length === 0) {
      this.#reportOutcome("craft", false, "recipe_unavailable");
      return;
    }
    bot.craft(recipes[0], target.count).then(
      () => this.#reportOutcome("craft", true, "craft_completed"),
      (error: unknown) => this.#reportOutcome("craft", false, String(error)),
    );
  }

  #handleEquip(bot: MineflayerBot, itemName: string): void {
    const item = bot.inventory.items().find((entry) => entry.name === itemName);
    if (!item) {
      this.#reportOutcome("equip", false, "item_unavailable");
      return;
    }
    if (bot.heldItem?.name === itemName) {
      this.#reportOutcome("equip", true, "already_equipped");
      return;
    }
    bot.equip(item, "hand").then(
      () => this.#reportOutcome("equip", true, "equip_completed"),
      (error: unknown) => this.#reportOutcome("equip", false, String(error)),
    );
  }

  #handleConsume(bot: MineflayerBot, itemName: string): void {
    const item = bot.inventory.items().find((entry) => entry.name === itemName);
    if (!item) {
      this.#reportOutcome("consume", false, "item_unavailable");
      return;
    }
    bot.equip(item, "hand")
      .then(() => bot.consume())
      .then(
        () => this.#reportOutcome("consume", true, "consume_completed"),
        (error: unknown) => this.#reportOutcome("consume", false, String(error)),
      );
  }

  #handlePlace(bot: MineflayerBot, target: NonNullable<ControllerState["place"]>): void {
    const refBlock = bot.blockAt(vec3(target.x, target.y, target.z));
    if (!refBlock) {
      this.#reportOutcome("place", false, "reference_block_unavailable");
      return;
    }
    const faceVec = vec3(target.fx, target.fy, target.fz);
    const targetItem = bot.inventory.items().find((i) => i.name === refBlock.name);
    if (!targetItem) {
      this.#reportOutcome("place", false, "placement_item_unavailable");
      return;
    }
    bot.equip(targetItem, "hand")
      .then(() => bot.placeBlock(refBlock, faceVec))
      .then(
        () => this.#reportOutcome("place", true, "place_completed"),
        (error: unknown) => this.#reportOutcome("place", false, String(error)),
      );
  }

  #reportOutcome(
    kind: ControllerActionOutcome["kind"],
    succeeded: boolean,
    detail: string,
    controllerSequence = this.#actionSequences[kind] ?? 0n,
    deduplicationKey = `${kind}:${controllerSequence}`,
  ): void {
    if (controllerSequence === 0n) return;
    if (this.#reportedOutcomes.has(deduplicationKey)) return;
    this.#reportedOutcomes.add(deduplicationKey);
    detail = boundedControllerDetail(detail);
    this.#diagnostic("controller.action_outcome", { kind, success: succeeded, detail });
    this.emit("actionOutcome", { controllerSequence, kind, succeeded, detail });
  }

  #diagnostic(event: string, details: Record<string, unknown> = {}): void {
    console.info(JSON.stringify({
      event,
      profile_id: this.#diagnosticProfileID,
      username: this.#diagnosticUsername,
      sequence: this.#lastAppliedControllerSequence.toString(),
      controller_state: this.#controllerStateSummary(),
      action_sequences: this.#actionSequenceSummary(),
      ...details,
    }));
  }

  #controllerStateSummary(): ControllerState {
    return {
      goto: this.#state.goto ?? null,
      break: this.#state.break ?? null,
      attack: this.#state.attack ?? null,
      craft: this.#state.craft ?? null,
      equip: this.#state.equip ?? null,
      consume: this.#state.consume ?? null,
      place: this.#state.place ?? null,
    };
  }

  #actionSequenceSummary(): Record<string, string> {
    const sequences: Record<string, string> = {};
    for (const [kind, sequence] of Object.entries(this.#actionSequences)) {
      if (sequence !== undefined) sequences[kind] = sequence.toString();
    }
    return sequences;
  }

  #blockPosition(position: { x: number; y: number; z: number }): { x: number; y: number; z: number } {
    return { x: position.x | 0, y: position.y | 0, z: position.z | 0 };
  }

  #buildReality(bot: MineflayerBot): RealityState {
    const reality: RealityState = {};

    if (this.#state.goto) {
      const gt = this.#state.goto;
      reality.arrivalDistance = bot.entity!.position.distanceTo(
        vec3(gt.x, gt.y, gt.z),
      );
      reality.gotoBlock = { x: gt.x | 0, y: gt.y | 0, z: gt.z | 0 };
    }

    if (bot.targetDigBlock) {
      const block = bot.targetDigBlock.position;
      reality.diggingBlock = { x: block.x | 0, y: block.y | 0, z: block.z | 0 };
    }

    if (this.#state.attack !== undefined && this.#state.attack !== null) {
      reality.attackingEntity = this.#state.attack;
    }

    if (bot.heldItem) {
      reality.equippedItem = bot.heldItem.name;
    }

    return reality;
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

        const buffer = packet.chunkData || packet.data;

        if (!buffer) {
          console.warn(
            `Missing chunk buffer for chunk at ${packet.x}, ${packet.z}`,
          );
          return;
        }

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

  #wireEntityTelemetry(bot: MineflayerBot): void {
    const entityData = (e: any) => ({
      entityId: e.id as number,
      name: e.name as string,
      x: (e.position as any).x as number,
      y: (e.position as any).y as number,
      z: (e.position as any).z as number,
      yaw: e.yaw as number,
      pitch: e.pitch as number,
    });

    bot.on("entitySpawn", (e: any) => {
      if (bot !== this.#bot) return;
      const d = entityData(e);
      this.#pendingEntities.added.set(d.entityId, d);
      this.#pendingEntities.removed.delete(d.entityId);
      this.#scheduleEntityFlush();
    });

    bot.on("entityGone", (e: any) => {
      if (bot !== this.#bot) return;
      const id = e.id ?? e;
      this.#pendingEntities.added.delete(id);
      this.#pendingEntities.moved.delete(id);
      this.#pendingEntities.removed.add(id);
      this.#scheduleEntityFlush();
    });

    bot.on("entityMoved", (e: any) => {
      if (bot !== this.#bot) return;
      const d = entityData(e);
      if (!this.#pendingEntities.added.has(d.entityId)) {
        this.#pendingEntities.moved.set(d.entityId, d);
      }
      this.#scheduleEntityFlush();
    });
  }

  #scheduleEntityFlush(): void {
    if (this.#entityFlushTimer) return;
    this.#entityFlushTimer = setTimeout(() => {
      this.#entityFlushTimer = undefined;
      const added = Array.from(this.#pendingEntities.added.values());
      const removed = Array.from(this.#pendingEntities.removed);
      const moved = Array.from(this.#pendingEntities.moved.values());
      this.#pendingEntities.added.clear();
      this.#pendingEntities.removed.clear();
      this.#pendingEntities.moved.clear();
      if (added.length === 0 && removed.length === 0 && moved.length === 0)
        return;
      this.emit("entitiesChanged", added, removed, moved);
    }, 200);
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
        _data: any,
        _meta: {
          name: string;
          state: string;
        },
      ) => {
        // packet debug logging omitted for production
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

function boundedControllerDetail(detail: string): string {
  if (detail.length <= ControllerDiagnosticDetailMaxLength) return detail;
  return `${detail.slice(0, ControllerDiagnosticDetailMaxLength - controllerDetailTruncationSuffix.length)}${controllerDetailTruncationSuffix}`;
}
