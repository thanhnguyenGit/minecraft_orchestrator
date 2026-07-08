import { EventEmitter } from "node:events";
import mineflayer from "mineflayer";

export type ConnectOptions = {
  host: string;
  port: number;
  username: string;
  auth: string;
  version: string;
};

export type BotStatus = "idle" | "connecting" | "connected" | "disconnected" | "error";

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
};

export class MineflayerAdapter extends EventEmitter implements BotAdapter {
  #bot: ReturnType<typeof mineflayer.createBot> | undefined;
  #status: BotStatus = "idle";

  async connect(options: ConnectOptions): Promise<void> {
    if (this.#bot) return;

    this.#status = "connecting";
    this.emit("status", this.#status, "connecting");

    this.#bot = mineflayer.createBot({
      host: options.host,
      port: options.port,
      username: options.username,
      auth: options.auth as "offline" | "microsoft",
      version: options.version,
    });

    this.#bot.once("spawn", () => {
      this.#status = "connected";
      this.emit("status", this.#status, "spawned");
      this.emit("spawn");
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
}
