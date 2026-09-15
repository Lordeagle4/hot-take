import assert from "node:assert/strict";
import test from "node:test";
import { join } from "node:path";
import { dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { parseSkill, SkillRegistry } from "../src/skill-registry.ts";

const root = dirname(dirname(fileURLToPath(import.meta.url)));

test("parses skill metadata and instructions", () => {
  const skill = parseSkill("weather", `---\nname: Weather\ndescription: Read forecasts.\ntriggers: [rain, forecast]\nplugins: [weather-mcp]\n---\n\n# Weather\n\nCheck the forecast.`);
  assert.equal(skill.name, "Weather");
  assert.deepEqual(skill.triggers, ["rain", "forecast"]);
  assert.deepEqual(skill.plugins, ["weather-mcp"]);
  assert.match(skill.instructions, /Check the forecast/);
});

test("routes a dice request to the tabletop skill", async () => {
  const registry = new SkillRegistry(join(root, "skills"));
  assert.equal((await registry.route("Please roll 2d20")).id, "tabletop");
});

test("falls back to general reasoning", async () => {
  const registry = new SkillRegistry(join(root, "skills"));
  assert.equal((await registry.route("What is consciousness?")).id, "general");
});
