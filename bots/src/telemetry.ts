export type Vitals = { health: number; food: number; saturation: number; oxygen: number };
export type Effect = { id: number; name: string; amplifier: number; durationTicks: number };
export type Position = { dimension: string; x: number; y: number; z: number; yaw: number; pitch: number; velocityX: number; velocityY: number; velocityZ: number };
export type ItemStack = { id: number; name: string; metadata: number; count: number };
export type InventorySlot = { slot: number; item?: ItemStack };
export type Inventory = { selectedHotbarSlot: number; slots: InventorySlot[] };
export type BotState = { vitals: Vitals; effects: Effect[]; position: Position; inventory: Inventory };

export const MOTION_DELTA_INTERVAL_MS = 250;

export function buildStateSnapshot(state: BotState): BotState {
  return { vitals: { ...state.vitals }, effects: state.effects.map((effect) => ({ ...effect })), position: { ...state.position }, inventory: { selectedHotbarSlot: state.inventory.selectedHotbarSlot, slots: state.inventory.slots.map(({ slot, item }) => ({ slot, ...(item ? { item: { ...item } } : {}) })) } };
}

export function coalesceInventoryUpdates(updates: InventorySlot[]): InventorySlot[] {
  const bySlot = new Map<number, InventorySlot>();
  for (const { slot, item } of updates) bySlot.set(slot, { slot, ...(item ? { item: { ...item } } : {}) });
  return [...bySlot.values()].sort((left, right) => left.slot - right.slot);
}

export function shouldEmitMotion(previous: Position | undefined, current: Position, lastEmittedAtMs: number, nowMs: number): boolean {
  if (!previous || previous.dimension !== current.dimension) return true;
  if (previous.x === current.x && previous.y === current.y && previous.z === current.z && previous.yaw === current.yaw && previous.pitch === current.pitch && previous.velocityX === current.velocityX && previous.velocityY === current.velocityY && previous.velocityZ === current.velocityZ) return false;
  return nowMs - lastEmittedAtMs >= MOTION_DELTA_INTERVAL_MS;
}
