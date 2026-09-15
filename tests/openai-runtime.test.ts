import assert from "node:assert/strict";
import test from "node:test";
import { OpenAIRuntime } from "../src/openai-runtime.ts";
import type { Plugin, Skill } from "../src/types.ts";

const skill: Skill = { id: "tabletop", name: "Tabletop dice", description: "Roll dice.", triggers: ["dice"], plugins: ["dice-lab"], instructions: "Use the dice tool." };
const plugin: Plugin = { id: "dice-lab", label: "Dice Lab", description: "Roll dice.", kind: "remote_mcp", serverUrl: "https://example.com/mcp", allowedTools: ["roll"], approval: "always", enabled: true, trusted: true };

test("simulation visibly pauses for approval", async () => {
  const runtime = new OpenAIRuntime(undefined, "test-model");
  const reply = await runtime.run(undefined, "Roll 2d6+3", skill, [plugin]);
  assert.equal(reply.mode, "simulation");
  assert.equal(reply.approval?.tool, "roll");
  assert.equal(reply.text, null);
});

test("approved simulation completes the tool loop", async () => {
  const runtime = new OpenAIRuntime(undefined, "test-model");
  const pending = await runtime.run(undefined, "Roll 1d1+2", skill, [plugin]);
  const reply = await runtime.approve(pending.sessionId, pending.approval!.id, true, skill, [plugin]);
  assert.match(reply.text ?? "", /\*\*3\*\*/);
  assert.equal(reply.approval, null);
});
