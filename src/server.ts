import { createServer, type IncomingMessage, type ServerResponse } from "node:http";
import { readFile } from "node:fs/promises";
import { dirname, extname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { SkillRegistry } from "./skill-registry.ts";
import { PluginRegistry } from "./plugin-registry.ts";
import { OpenAIRuntime } from "./openai-runtime.ts";
import type { Plugin } from "./types.ts";

const root = dirname(dirname(fileURLToPath(import.meta.url)));
const skills = new SkillRegistry(join(root, "skills"));
const plugins = new PluginRegistry(join(root, "data", "plugins.json"));
const runtime = new OpenAIRuntime(process.env.OPENAI_API_KEY, process.env.OPENAI_MODEL ?? "gpt-5.6-sol");
const port = Number(process.env.PORT ?? 3000);

const mimeTypes: Record<string, string> = { ".html": "text/html; charset=utf-8", ".css": "text/css; charset=utf-8", ".js": "text/javascript; charset=utf-8", ".svg": "image/svg+xml" };

async function body<T>(request: IncomingMessage): Promise<T> {
  const chunks: Buffer[] = [];
  for await (const chunk of request) chunks.push(Buffer.from(chunk));
  if (!chunks.length) return {} as T;
  return JSON.parse(Buffer.concat(chunks).toString("utf8")) as T;
}

function json(response: ServerResponse, status: number, payload: unknown): void {
  response.writeHead(status, { "Content-Type": "application/json; charset=utf-8", "Cache-Control": "no-store" });
  response.end(JSON.stringify(payload));
}

function publicPlugin(plugin: Plugin): Omit<Plugin, "authorizationEnv"> & { connected: boolean; authorizationEnv?: string } {
  return { ...plugin, connected: runtime.isConnected(plugin), authorizationEnv: plugin.authorizationEnv };
}

async function api(request: IncomingMessage, response: ServerResponse, url: URL): Promise<boolean> {
  if (request.method === "GET" && url.pathname === "/api/state") {
    json(response, 200, { live: runtime.live, model: process.env.OPENAI_MODEL ?? "gpt-5.6-sol", skills: await skills.all(), plugins: (await plugins.all()).map(publicPlugin) });
    return true;
  }

  if (request.method === "POST" && url.pathname === "/api/chat") {
    const input = await body<{ sessionId?: string; message?: string }>(request);
    if (!input.message?.trim()) throw new Error("Message is required.");
    const skill = await skills.route(input.message);
    json(response, 200, await runtime.run(input.sessionId, input.message.trim(), skill, await plugins.enabled(skill.plugins)));
    return true;
  }

  if (request.method === "POST" && url.pathname === "/api/approvals") {
    const input = await body<{ sessionId?: string; approvalId?: string; approve?: boolean; skillId?: string }>(request);
    if (!input.sessionId || !input.approvalId || typeof input.approve !== "boolean" || !input.skillId) throw new Error("Incomplete approval response.");
    const skill = (await skills.all()).find((candidate) => candidate.id === input.skillId);
    if (!skill) throw new Error("The selected skill no longer exists.");
    json(response, 200, await runtime.approve(input.sessionId, input.approvalId, input.approve, skill, await plugins.enabled(skill.plugins)));
    return true;
  }

  if (request.method === "POST" && url.pathname === "/api/plugins") {
    const input = await body<Plugin>(request);
    const plugin: Plugin = { ...input, allowedTools: input.allowedTools ?? [], approval: input.approval ?? "always", enabled: true };
    json(response, 201, publicPlugin(await plugins.install(plugin)));
    return true;
  }

  const toggle = url.pathname.match(/^\/api\/plugins\/([a-z0-9-]+)\/toggle$/);
  if (request.method === "POST" && toggle) {
    const input = await body<{ enabled?: boolean }>(request);
    if (typeof input.enabled !== "boolean") throw new Error("Enabled must be a boolean.");
    json(response, 200, publicPlugin(await plugins.setEnabled(toggle[1], input.enabled)));
    return true;
  }

  const connect = url.pathname.match(/^\/api\/plugins\/([a-z0-9-]+)\/connect$/);
  if (request.method === "POST" && connect) {
    const input = await body<{ token?: string }>(request);
    runtime.connect(connect[1], input.token ?? "");
    json(response, 200, { connected: true, persisted: false });
    return true;
  }

  return false;
}

async function serve(request: IncomingMessage, response: ServerResponse): Promise<void> {
  try {
    const url = new URL(request.url ?? "/", `http://${request.headers.host ?? "localhost"}`);
    if (url.pathname.startsWith("/api/")) {
      if (!await api(request, response, url)) json(response, 404, { error: "API route not found." });
      return;
    }

    const relative = url.pathname === "/" ? "index.html" : url.pathname.slice(1);
    if (relative.includes("..")) throw new Error("Invalid path.");
    const file = await readFile(join(root, "public", relative));
    response.writeHead(200, { "Content-Type": mimeTypes[extname(relative)] ?? "application/octet-stream" });
    response.end(file);
  } catch (error) {
    const message = error instanceof Error ? error.message : "Unexpected error.";
    if ((error as NodeJS.ErrnoException).code === "ENOENT") return json(response, 404, { error: "Not found." });
    json(response, 400, { error: message });
  }
}

createServer(serve).listen(port, () => {
  console.log(`Hot Take is running at http://localhost:${port} (${runtime.live ? "live" : "simulation"} mode)`);
});
