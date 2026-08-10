import assert from "node:assert/strict";
import { EventEmitter } from "node:events";
import { readFile } from "node:fs/promises";
import { createRequire } from "node:module";
import { test } from "node:test";
import {
  ControllerDiagnosticDetailMaxLength,
  MineflayerAdapter,
  mergeControllerState,
} from "../src/mineflayer_adapter.js";

test("ControllerState deltas preserve prior targets and apply explicit clears", () => {
  const afterGoto = mergeControllerState(
    {},
    { goto: { x: 2, y: 64, z: -3 } },
  );
  const afterCraft = mergeControllerState(afterGoto, {
    craft: { itemName: "oak_planks", count: 4 },
  });
  const cleared = mergeControllerState(afterCraft, { goto: null });

  assert.deepEqual(afterCraft, {
    goto: { x: 2, y: 64, z: -3 },
    craft: { itemName: "oak_planks", count: 4 },
  });
  assert.deepEqual(cleared, {
    goto: null,
    craft: { itemName: "oak_planks", count: 4 },
  });
});

test("ControllerState has one tick executor and clears one-shot targets", async () => {
  const source = await readFile(
    new URL("../src/mineflayer_adapter.ts", import.meta.url),
    "utf8",
  );

  assert.match(source, /setInterval\(\(\) => this\.#tick\(\), 100\)/);
  for (const action of ["craft", "equip", "consume", "place"]) {
    assert.match(
      source,
      new RegExp(`this\\.#state = \\{ \\...this\\.#state, ${action}: null \\}`),
    );
  }
});

test("controller waits for break range without failure, then digs when the target is reachable", async () => {
	const registry = createRequire(import.meta.url)("prismarine-registry")("1.21.1");
  let distance = 5;
  let digCalls = 0;
  let block: { name: string; position: { x: number; y: number; z: number } } | undefined;
  const bot = Object.assign(new EventEmitter(), {
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
      position: { x: 0, y: 64, z: 0, distanceTo: () => distance },
      yaw: 0,
      pitch: 0,
      velocity: { x: 0, y: 0, z: 0 },
      effects: {},
    },
    inventory: { slots: [], items: () => [] },
    entities: {},
    pathfinder: { setGoal() {}, setMovements() {} },
    blockAt: () => block,
    canSeeBlock: () => true,
    canDigBlock: () => true,
    dig: async () => { digCalls++; },
    loadPlugin() {},
    quit() {},
  });
  const adapter = new MineflayerAdapter(() => bot as never, {
    diagnosticProfileID: "profile-42",
    diagnosticUsername: "test_bot",
  });
  const messages: Array<Record<string, unknown>> = [];
  const outcomes: Array<{ controllerSequence: bigint; kind: string; succeeded: boolean; detail: string }> = [];
  const originalInfo = console.info;
  adapter.on("actionOutcome", (outcome) => outcomes.push(outcome));
  console.info = (message: unknown) => messages.push(JSON.parse(String(message)) as Record<string, unknown>);

  try {
    await adapter.connect({ host: "127.0.0.1", port: 25565, username: "test_bot", auth: "offline", version: "1.21.11" });
    adapter.applyState({ goto: { x: 4, y: 64, z: 0 }, break: { x: 0, y: 64, z: -1 } }, 42n);
    bot.emit("spawn");
    await new Promise((resolve) => setTimeout(resolve, 220));
    assert.equal(digCalls, 0);
    assert.deepEqual(outcomes.filter((outcome) => outcome.kind === "break"), []);
    assert.ok(messages.filter((entry) => entry.event === "controller.break_disposition").length >= 2);

    block = { name: "oak_log", position: { x: 0, y: 64, z: -1 } };
    distance = 4.5;
    await new Promise((resolve) => setTimeout(resolve, 320));
  } finally {
    bot.emit("end");
    console.info = originalInfo;
  }

  const event = (name: string) => messages.find((entry) => entry.event === name);
  const stateApplied = event("controller.state_applied");
  const tick = event("controller.tick");
  const waiting = event("controller.break_disposition");
  const breakOutcomes = messages.filter((entry) => entry.event === "controller.action_outcome" && entry.kind === "break");
  const reality = messages.filter((entry) => entry.event === "controller.reality").at(-1);
  assert.ok(stateApplied && tick && waiting && reality, `diagnostics = ${JSON.stringify(messages)}`);
  assert.equal(stateApplied.sequence, "42");
  assert.deepEqual(stateApplied.action_sequences, { goto: "42", break: "42" });
  assert.deepEqual(stateApplied.controller_state, {
    goto: { x: 4, y: 64, z: 0 }, break: { x: 0, y: 64, z: -1 }, attack: null,
    craft: null, equip: null, consume: null, place: null,
  });
  assert.equal(waiting.disposition, "waiting_for_range");
  assert.equal(digCalls, 1);
  assert.deepEqual(outcomes.filter((outcome) => outcome.kind === "break"), [
    { controllerSequence: 42n, kind: "break", succeeded: true, detail: "dig_completed" },
  ]);
  assert.equal(breakOutcomes.length, 1);
  assert.equal(reality.goto_distance, 4.5);
  for (const message of messages) {
    assert.equal(message.profile_id, "profile-42");
    assert.equal(message.username, "test_bot");
  }
});

test("controller waits for attack range without failure, then attacks when the target is reachable", async () => {
  const registry = createRequire(import.meta.url)("prismarine-registry")("1.21.1");
  let distance = 50;
  let attackCalls = 0;
  const bot = Object.assign(new EventEmitter(), {
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
      position: { x: 0, y: 64, z: 0, distanceTo: () => distance },
      yaw: 0,
      pitch: 0,
      velocity: { x: 0, y: 0, z: 0 },
      effects: {},
    },
    inventory: { slots: [], items: () => [] },
    entities: {
      99: { id: 99, position: { x: 50, y: 64, z: 0, offset: () => ({ x: 50, y: 65, z: 0 }) }, height: 1 },
    },
    pathfinder: { setGoal() {}, setMovements() {} },
    lookAt() {},
    attack() { attackCalls++; },
    loadPlugin() {},
    quit() {},
  });
  const adapter = new MineflayerAdapter(() => bot as never);
  const outcomes: Array<{ controllerSequence: bigint; kind: string; succeeded: boolean; detail: string }> = [];
  const diagnostics: Array<Record<string, unknown>> = [];
  const originalInfo = console.info;
  adapter.on("actionOutcome", (outcome) => outcomes.push(outcome));
  console.info = (message: unknown) => diagnostics.push(JSON.parse(String(message)) as Record<string, unknown>);

  try {
    await adapter.connect({ host: "127.0.0.1", port: 25565, username: "test_bot", auth: "offline", version: "1.21.11" });
    adapter.applyState({ attack: 99 }, 44n);
    bot.emit("spawn");
    await new Promise((resolve) => setTimeout(resolve, 220));
    assert.equal(attackCalls, 0);
    assert.deepEqual(outcomes.filter((outcome) => outcome.kind === "attack"), []);
    assert.ok(diagnostics.filter((entry) => entry.event === "controller.attack_disposition").length >= 2);

    distance = 3.5;
    await new Promise((resolve) => setTimeout(resolve, 320));
  } finally {
    bot.emit("end");
    console.info = originalInfo;
  }

  assert.equal(attackCalls, 1);
  assert.deepEqual(outcomes.filter((outcome) => outcome.kind === "attack"), [
    { controllerSequence: 44n, kind: "attack", succeeded: true, detail: "attack_sent" },
  ]);
  const waiting = diagnostics.filter((entry) => entry.event === "controller.attack_disposition");
  assert.ok(waiting.length >= 2, `diagnostics = ${JSON.stringify(diagnostics)}`);
  assert.ok(waiting.every((entry) => entry.disposition === "waiting_for_range"));
});

test("controller reports genuine break and attack failures exactly once", async () => {
  const registry = createRequire(import.meta.url)("prismarine-registry")("1.21.1");
  let mode = "missing_block";
  let digCalls = 0;
  let attackCalls = 0;
  const block = { name: "oak_log", position: { x: 1, y: 64, z: 0 } };
  const entity = { id: 99, position: { x: 1, y: 64, z: 0, offset: () => ({ x: 1, y: 65, z: 0 }) }, height: 1 };
  const bot = Object.assign(new EventEmitter(), {
    health: 20, food: 20, foodSaturation: 5, oxygenLevel: 20, quickBarSlot: 0,
    physicsEnabled: true, registry, game: { dimension: "minecraft:overworld" },
    entity: { id: 42, position: { x: 0, y: 64, z: 0, distanceTo: () => 1 }, yaw: 0, pitch: 0, velocity: { x: 0, y: 0, z: 0 }, effects: {} },
    inventory: { slots: [], items: () => [] },
    entities: new Proxy({}, { get: (_target, property) => mode === "missing_entity" || property !== "99" ? undefined : entity }),
    pathfinder: { setGoal() {}, setMovements() {} },
    blockAt: () => mode === "missing_block" ? undefined : block,
    canSeeBlock: () => true,
    canDigBlock: () => mode !== "non_diggable",
    dig: async () => { digCalls++; if (mode === "dig_rejects") throw new Error("dig failed"); },
    lookAt() {},
    attack() { attackCalls++; if (mode === "attack_throws") throw new Error("attack failed"); },
    loadPlugin() {}, quit() {},
  });
  const adapter = new MineflayerAdapter(() => bot as never);
  const outcomes: Array<{ controllerSequence: bigint; kind: string; succeeded: boolean; detail: string }> = [];
  adapter.on("actionOutcome", (outcome) => outcomes.push(outcome));

  try {
    await adapter.connect({ host: "127.0.0.1", port: 25565, username: "test_bot", auth: "offline", version: "1.21.11" });
    bot.emit("spawn");
    for (const [nextMode, state, sequence] of [
      ["missing_block", { break: { x: 1, y: 64, z: 0 } }, 51n],
      ["non_diggable", { break: { x: 1, y: 64, z: 0 } }, 52n],
      ["dig_rejects", { break: { x: 1, y: 64, z: 0 } }, 53n],
      ["missing_entity", { attack: 99 }, 54n],
      ["attack_throws", { attack: 99 }, 55n],
    ] as const) {
      mode = nextMode;
      adapter.applyState(state, sequence);
      await new Promise((resolve) => setTimeout(resolve, 220));
    }
  } finally {
    bot.emit("end");
  }

  assert.equal(digCalls, 1);
  assert.equal(attackCalls, 1);
  assert.deepEqual(outcomes, [
    { controllerSequence: 51n, kind: "break", succeeded: false, detail: "target_block_unavailable" },
    { controllerSequence: 52n, kind: "break", succeeded: false, detail: "target_not_diggable" },
    { controllerSequence: 53n, kind: "break", succeeded: false, detail: "Error: dig failed" },
    { controllerSequence: 54n, kind: "attack", succeeded: false, detail: "target_entity_unavailable" },
    { controllerSequence: 55n, kind: "attack", succeeded: false, detail: "Error: attack failed" },
  ]);
});

test("controller diagnostics retain the newest applied sequence after an explicit clear", () => {
  const messages: Array<Record<string, unknown>> = [];
  const originalInfo = console.info;
  console.info = (message: unknown) => messages.push(JSON.parse(String(message)) as Record<string, unknown>);

  try {
    const adapter = new MineflayerAdapter(undefined, {
      diagnosticProfileID: "profile-clear",
      diagnosticUsername: "clear_bot",
    });
    adapter.applyState({ goto: { x: 4, y: 64, z: 0 } }, 41n);
    adapter.applyState({ goto: null }, 42n);
  } finally {
    console.info = originalInfo;
  }

  const clear = messages.at(-1);
  assert.ok(clear);
  assert.equal(clear.event, "controller.state_applied");
  assert.equal(clear.sequence, "42");
  assert.deepEqual(clear.action_sequences, {});
  assert.equal(clear.profile_id, "profile-clear");
  assert.equal(clear.username, "clear_bot");
});

test("controller diagnostics bound asynchronous outcome errors deterministically", async () => {
  const registry = createRequire(import.meta.url)("prismarine-registry")("1.21.1");
  const longMessage = "x".repeat(ControllerDiagnosticDetailMaxLength + 40);
  const bot = Object.assign(new EventEmitter(), {
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
    craft: async () => { throw new Error(longMessage); },
    loadPlugin() {},
    quit() {},
  });
  const adapter = new MineflayerAdapter(() => bot as never);
  const emitted: Array<{ detail: string; kind: string; controllerSequence: bigint }> = [];
  adapter.on("actionOutcome", (outcome) => emitted.push(outcome));
  const diagnostics: Array<Record<string, unknown>> = [];
  const originalInfo = console.info;
  console.info = (message: unknown) => diagnostics.push(JSON.parse(String(message)) as Record<string, unknown>);

  try {
    await adapter.connect({ host: "127.0.0.1", port: 25565, username: "test_bot", auth: "offline", version: "1.21.11" });
    adapter.applyState({ craft: { itemName: "oak_planks", count: 1 } }, 43n);
    bot.emit("spawn");
    await new Promise((resolve) => setTimeout(resolve, 120));
  } finally {
    bot.emit("end");
    console.info = originalInfo;
  }

  const diagnostic = diagnostics.find((entry) => entry.event === "controller.action_outcome" && entry.kind === "craft");
  const outcome = emitted.find((entry) => entry.kind === "craft");
  assert.ok(diagnostic && outcome, `diagnostics=${JSON.stringify(diagnostics)} outcomes=${emitted.map((entry) => `${entry.kind}:${entry.controllerSequence}:${entry.detail}`).join(",")}`);
  const expected = `Error: ${longMessage.slice(0, ControllerDiagnosticDetailMaxLength - 3 - "Error: ".length)}...`;
  assert.equal(outcome.controllerSequence, 43n);
  assert.equal(outcome.detail, expected);
  assert.equal(outcome.detail.length, ControllerDiagnosticDetailMaxLength);
  assert.equal(diagnostic.detail, expected);
  assert.equal(diagnostic.sequence, "43");
  assert.deepEqual(diagnostic.action_sequences, { craft: "43" });
});
