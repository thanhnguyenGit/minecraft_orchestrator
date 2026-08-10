import { readFile } from "node:fs/promises";
import assert from "node:assert/strict";
import { describe, it, test } from "node:test";

describe("Mineflayer host package contract", () => {
  it("exposes only the socket host entrypoint", async () => {
    const manifest = JSON.parse(await readFile(new URL("../package.json", import.meta.url), "utf8")) as { scripts: Record<string, string>; dependencies: Record<string, string> };

    assert.equal(manifest.scripts.host, "tsx src/host.ts");
    assert.equal(manifest.scripts.worker, undefined);
    assert.equal(manifest.dependencies.redis, undefined);
  });
});

test("host dispatches control only through ControllerState", async () => {
  const source = await readFile(
    new URL("../src/host.ts", import.meta.url),
    "utf8",
  );

  assert.doesNotMatch(
    source,
    /case "(?:command|attackCommand|breakBlock|equip|craft|placeBlock)":\s*\{\s*const/,
  );
});
