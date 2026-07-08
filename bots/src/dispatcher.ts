import type { BotCommand } from "./gen/orchestrator/v1/bot_pb.js";
import type { BotAdapter } from "./mineflayer_adapter.js";

export async function dispatchCommand(adapter: BotAdapter, command: BotCommand): Promise<void> {
  switch (command.payload.case) {
    case "connect":
      await adapter.connect(command.payload.value);
      return;
    case "disconnect":
      await adapter.disconnect();
      return;
    case "status":
      adapter.status();
      return;
    case "sendChat":
      await adapter.sendChat(command.payload.value.message);
      return;
    default:
      throw new Error("command payload is required");
  }
}
