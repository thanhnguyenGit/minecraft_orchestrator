import assert from "node:assert/strict";
import { EventEmitter } from "node:events";
import { createRequire } from "node:module";
import { describe, it } from "node:test";

import { MineflayerAdapter } from "../src/mineflayer_adapter.js";

describe("telemetry state", () => {
  it("coalesces inventory changes and limits motion deltas", async () => {
    const telemetry = await import("../src/telemetry.js");
    assert.deepEqual(telemetry.coalesceInventoryUpdates([
      { slot: 38, item: { id: 1, name: "stone", metadata: 0, count: 1 } },
      { slot: 10 },
      { slot: 38, item: { id: 1, name: "stone", metadata: 0, count: 32 } },
    ]), [{ slot: 10 }, { slot: 38, item: { id: 1, name: "stone", metadata: 0, count: 32 } }]);
    const previous = { dimension: "minecraft:overworld", x: 1, y: 64, z: 1, yaw: 0, pitch: 0, velocityX: 0, velocityY: 0, velocityZ: 0 };
    assert.equal(telemetry.shouldEmitMotion(previous, { ...previous, x: 2 }, 1000, 1100), false);
    assert.equal(telemetry.shouldEmitMotion(previous, { ...previous, x: 2 }, 1000, 1250), true);
  });
});

describe("MineflayerAdapter inventory readiness", () => {
  it("waits for inject_allowed before subscribing to inventory updates", async () => {
    const bot = new EventEmitter() as EventEmitter & { inventory?: EventEmitter & { slots: unknown[] }; quit(): void };
    bot.quit = () => {};
    let created = 0;
    const adapter = new MineflayerAdapter(() => {
      created++;
      return bot as never;
    });

    await adapter.connect({ host: "127.0.0.1", port: 25565, username: "test_bot", auth: "offline", version: "1.21.11" });
    assert.equal(created, 1);
    assert.equal(bot.inventory, undefined);

    const inventory = new EventEmitter() as EventEmitter & { slots: unknown[] };
    inventory.slots = [];
    bot.inventory = inventory;
    bot.emit("inject_allowed");

    assert.equal(inventory.listenerCount("updateSlot"), 1);
  });
});

describe("MineflayerAdapter controller sessions", () => {
  it("discards a stale dig completion after reconnect while a reused sequence executes normally", async () => {
    const registry = createRequire(import.meta.url)("prismarine-registry")("1.21.1");
    let resolveOldDig: (() => void) | undefined;
    let secondDigCalls = 0;
    const makeBot = (dig: () => Promise<void>) => Object.assign(new EventEmitter(), {
      health: 20, food: 20, foodSaturation: 5, oxygenLevel: 20, quickBarSlot: 0,
      physicsEnabled: true, registry, game: { dimension: "minecraft:overworld" },
      entity: { id: 42, position: { x: 0, y: 64, z: 0, distanceTo: () => 1 }, yaw: 0, pitch: 0, velocity: { x: 0, y: 0, z: 0 }, effects: {} },
      inventory: { slots: [], items: () => [] }, entities: {}, pathfinder: { setGoal() {}, setMovements() {} },
      blockAt: () => ({ name: "oak_log", position: { x: 1, y: 64, z: 0 } }),
      canSeeBlock: () => true, canDigBlock: () => true, dig,
      loadPlugin() {}, quit() {},
    });
    const firstBot = makeBot(() => new Promise<void>((resolve) => { resolveOldDig = resolve; }));
    const secondBot = makeBot(async () => { secondDigCalls++; });
    const bots = [firstBot, secondBot];
    const adapter = new MineflayerAdapter(() => bots.shift() as never);
    const outcomes: Array<{ kind: string; controllerSequence: bigint; succeeded: boolean; detail: string }> = [];
    adapter.on("actionOutcome", (outcome) => outcomes.push(outcome));
    const options = { host: "127.0.0.1", port: 25565, username: "test_bot", auth: "offline", version: "1.21.11" };

    await adapter.connect(options);
    adapter.applyState({ break: { x: 1, y: 64, z: 0 } }, 9n);
    firstBot.emit("spawn");
    await new Promise((resolve) => setTimeout(resolve, 120));
    assert.ok(resolveOldDig);
    firstBot.emit("end");

    await adapter.connect(options);
    adapter.applyState({ break: { x: 1, y: 64, z: 0 } }, 9n);
    secondBot.emit("spawn");
    resolveOldDig();
    await new Promise((resolve) => setTimeout(resolve, 120));
    secondBot.emit("end");

    assert.equal(secondDigCalls, 1);
    assert.deepEqual(outcomes, [{ kind: "break", controllerSequence: 9n, succeeded: true, detail: "dig_completed" }]);
  });

  it("accepts a reused kind and sequence after reconnect without replaying stale state", async () => {
    const registry = createRequire(import.meta.url)("prismarine-registry")("1.21.1");
    const makeBot = () => Object.assign(new EventEmitter(), {
      health: 20,
      food: 20,
      foodSaturation: 5,
      oxygenLevel: 20,
      quickBarSlot: 0,
      physicsEnabled: true,
      registry,
      game: { dimension: "minecraft:overworld" },
      entity: {
        id: 42,
        position: { x: 0, y: 64, z: 0, distanceTo: () => 0 },
        yaw: 0,
        pitch: 0,
        velocity: { x: 0, y: 0, z: 0 },
        effects: {},
      },
      inventory: { slots: [], items: () => [] },
      entities: {},
      pathfinder: { setGoal() {}, setMovements() {} },
      recipesFor: () => [{}],
      craft: async () => {},
      loadPlugin() {},
      quit() {},
    });
    const firstBot = makeBot();
    const secondBot = makeBot();
    const bots = [firstBot, secondBot];
    const adapter = new MineflayerAdapter(() => bots.shift() as never);
    const outcomes: Array<{ kind: string; controllerSequence: bigint }> = [];
    adapter.on("actionOutcome", (outcome) => outcomes.push(outcome));
    const options = { host: "127.0.0.1", port: 25565, username: "test_bot", auth: "offline", version: "1.21.11" };

    await adapter.connect(options);
    adapter.applyState({ goto: { x: 1, y: 64, z: 1 }, craft: { itemName: "oak_planks", count: 1 } }, 7n);
    firstBot.emit("spawn");
    await new Promise((resolve) => setTimeout(resolve, 120));
    firstBot.emit("end");

    await adapter.connect(options);
    adapter.applyState({ craft: { itemName: "oak_planks", count: 1 } }, 7n);
    secondBot.emit("spawn");
    await new Promise((resolve) => setTimeout(resolve, 120));
    secondBot.emit("end");

    assert.deepEqual(outcomes.filter((outcome) => outcome.kind === "craft").map((outcome) => outcome.controllerSequence), [7n, 7n]);
    assert.equal(outcomes.filter((outcome) => outcome.kind === "goto").length, 1);
  });
});

describe("MineflayerAdapter physics diagnostics", () => {
  it("reports the local player velocity packet only when diagnostics are enabled", async () => {
    const client = new EventEmitter();
    const bot = Object.assign(new EventEmitter(), {
      _client: client,
      quit() {},
      health: 20,
      food: 20,
      foodSaturation: 5,
      oxygenLevel: 20,
      quickBarSlot: 0,
      physicsEnabled: true,
      registry: { effects: {} },
      game: { dimension: "minecraft:overworld" },
      entity: { id: 42, position: { x: 0, y: 64, z: 0 }, yaw: 0, pitch: 0, velocity: { x: 0, y: 0, z: 0 }, effects: {} },
      blockAt: () => ({ name: "stone" }),
      waitForChunksToLoad: async () => {},
    });
    const diagnostics: Array<{ kind: string; velocity?: { x: number; y: number; z: number } }> = [];
    const adapter = new MineflayerAdapter(() => bot as never, { physicsDebug: true });
    adapter.on("physicsDiagnostic", (event) => diagnostics.push(event));

    await adapter.connect({ host: "127.0.0.1", port: 25565, username: "test_bot", auth: "offline", version: "1.21.11" });
    client.emit("entity_velocity", { entityId: 42, velocity: { x: 1, y: 2, z: 3 } });
    await new Promise<void>((resolve) => queueMicrotask(resolve));

    assert.deepEqual(diagnostics.find((event) => event.kind === "entity_velocity")?.velocity, { x: 1, y: 2, z: 3 });
  });

  it("does not emit diagnostics by default", async () => {
    const client = new EventEmitter();
    const bot = Object.assign(new EventEmitter(), {
      _client: client, quit() {}, health: 20, food: 20, foodSaturation: 5, oxygenLevel: 20, quickBarSlot: 0,
      physicsEnabled: true, registry: { effects: {} }, game: { dimension: "minecraft:overworld" },
      entity: { id: 42, position: { x: 0, y: 64, z: 0 }, yaw: 0, pitch: 0, velocity: { x: 0, y: 0, z: 0 }, effects: {} },
      blockAt: () => ({ name: "stone" }), waitForChunksToLoad: async () => {},
    });
    const adapter = new MineflayerAdapter(() => bot as never);
    const diagnostics: unknown[] = [];
    adapter.on("physicsDiagnostic", (event) => diagnostics.push(event));

    await adapter.connect({ host: "127.0.0.1", port: 25565, username: "test_bot", auth: "offline", version: "1.21.11" });
    client.emit("entity_velocity", { entityId: 42, velocity: { x: 1, y: 2, z: 3 } });
    await new Promise<void>((resolve) => queueMicrotask(resolve));

    assert.deepEqual(diagnostics, []);
  });
});
