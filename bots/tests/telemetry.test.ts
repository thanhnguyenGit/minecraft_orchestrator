import assert from "node:assert/strict";
import { EventEmitter } from "node:events";
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
    bot.emit("spawn");
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
