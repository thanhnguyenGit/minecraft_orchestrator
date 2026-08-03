const DEFAULT_MAX_FRAME_SIZE = 1024 * 1024;
const LENGTH_PREFIX_BYTES = 4;

export function encodeFrame(payload: Uint8Array): Buffer {
  if (payload.length > DEFAULT_MAX_FRAME_SIZE) {
    throw new Error(`frame exceeds maximum size: ${payload.length}`);
  }
  const frame = Buffer.allocUnsafe(LENGTH_PREFIX_BYTES + payload.length);
  frame.writeUInt32BE(payload.length, 0);
  frame.set(payload, LENGTH_PREFIX_BYTES);
  return frame;
}

export class FrameDecoder {
  #buffer = Buffer.alloc(0);
  readonly #maximumFrameSize: number;

  constructor(maximumFrameSize = DEFAULT_MAX_FRAME_SIZE) {
    if (!Number.isSafeInteger(maximumFrameSize) || maximumFrameSize < 1) {
      throw new Error("maximum frame size must be a positive integer");
    }
    this.#maximumFrameSize = maximumFrameSize;
  }

  push(chunk: Uint8Array): Buffer[] {
    this.#buffer = Buffer.concat([this.#buffer, chunk]);
    const frames: Buffer[] = [];
    while (this.#buffer.length >= 4) {
      const length = this.#buffer.readUInt32BE(0);
      if (length > this.#maximumFrameSize) {
        throw new Error(`frame exceeds maximum size: ${length}`);
      }
      if (this.#buffer.length < 4 + length) break;
      frames.push(this.#buffer.subarray(4, 4 + length));
      this.#buffer = this.#buffer.subarray(4 + length);
    }
    return frames;
  }
}
