import assert from "node:assert/strict";
import test from "node:test";
import { validatePlugin } from "../src/plugin-registry.ts";
import type { Plugin } from "../src/types.ts";

function plugin(overrides: Partial<Plugin> = {}): Plugin {
  return {
    id: "test-plugin",
    label: "Test plugin",
    description: "A test plugin.",
    kind: "remote_mcp",
    serverUrl: "https://example.com/mcp",
    allowedTools: [],
    approval: "always",
    enabled: true,
    trusted: false,
    ...overrides,
  };
}

test("accepts an HTTPS remote MCP plugin", () => {
  assert.doesNotThrow(() => validatePlugin(plugin()));
});

test("rejects an untrusted plugin that bypasses approval", () => {
  assert.throws(() => validatePlugin(plugin({ approval: "never" })), /trusted/i);
});

test("rejects insecure remote URLs", () => {
  assert.throws(() => validatePlugin(plugin({ serverUrl: "http://example.com/mcp" })), /HTTPS/i);
});

test("does not let plugin manifests read arbitrary environment variables", () => {
  assert.throws(() => validatePlugin(plugin({ authorizationEnv: "OPENAI_API_KEY" })), /HOT_TAKE_PLUGIN_/i);
});
