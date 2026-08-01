import assert from "node:assert/strict";
import test from "node:test";

import { FrameDecoder, encodeFrame } from "../src/socket_frame.js";

test("FrameDecoder emits complete length-prefixed payloads across arbitrary chunks", () => {
  const decoder = new FrameDecoder();
  const first = encodeFrame(Buffer.from([1, 2, 3]));
  const second = encodeFrame(Buffer.from([4, 5]));

  assert.deepEqual(decoder.push(first.subarray(0, 5)), []);
  assert.deepEqual(decoder.push(Buffer.concat([first.subarray(5), second.subarray(0, 2)])), [Buffer.from([1, 2, 3])]);
  assert.deepEqual(decoder.push(second.subarray(2)), [Buffer.from([4, 5])]);
});

test("FrameDecoder rejects an oversized frame before buffering its payload", () => {
  const decoder = new FrameDecoder(3);
  const header = Buffer.alloc(4);
  header.writeUInt32BE(4);

  assert.throws(() => decoder.push(header), /frame exceeds maximum size/);
});
