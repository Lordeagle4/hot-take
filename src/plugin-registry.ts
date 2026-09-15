import { readFile, writeFile } from "node:fs/promises";
import type { Plugin } from "./types.ts";

const CONNECTOR_IDS = new Set([
  "connector_dropbox",
  "connector_gmail",
  "connector_googlecalendar",
  "connector_googledrive",
  "connector_microsoftteams",
  "connector_outlookcalendar",
  "connector_outlookemail",
  "connector_sharepoint",
]);

function validate(plugin: Plugin): void {
  if (!/^[a-z0-9][a-z0-9-]{1,39}$/.test(plugin.id)) throw new Error("Plugin ID must be 2–40 lowercase letters, numbers or hyphens.");
  if (!plugin.label.trim() || !plugin.description.trim()) throw new Error("Plugin label and description are required.");
  if (plugin.kind !== "remote_mcp" && plugin.kind !== "connector") throw new Error("Plugin kind must be remote_mcp or connector.");
  if (!Array.isArray(plugin.allowedTools) || !plugin.allowedTools.every((tool) => typeof tool === "string" && /^[A-Za-z0-9_.:-]+$/.test(tool))) throw new Error("Allowed tools must contain valid tool names.");
  if (plugin.authorizationEnv && !/^HOT_TAKE_PLUGIN_[A-Z0-9_]+$/.test(plugin.authorizationEnv)) throw new Error("Token variables must start with HOT_TAKE_PLUGIN_.");
  if (plugin.approval === "never" && !plugin.trusted) throw new Error("Only trusted plugins may bypass approval.");

  if (plugin.kind === "remote_mcp") {
    if (!plugin.serverUrl) throw new Error("Remote MCP plugins require a server URL.");
    const url = new URL(plugin.serverUrl);
    if (url.protocol !== "https:" && url.hostname !== "localhost") throw new Error("Remote MCP URLs must use HTTPS.");
  }

  if (plugin.kind === "connector" && (!plugin.connectorId || !CONNECTOR_IDS.has(plugin.connectorId))) {
    throw new Error("Unsupported OpenAI connector ID.");
  }
}

export class PluginRegistry {
  private readonly file: string;

  public constructor(file: string) {
    this.file = file;
  }

  public async all(): Promise<Plugin[]> {
    return JSON.parse(await readFile(this.file, "utf8")) as Plugin[];
  }

  public async enabled(ids: string[]): Promise<Plugin[]> {
    const wanted = new Set(ids);
    return (await this.all()).filter((plugin) => plugin.enabled && wanted.has(plugin.id));
  }

  public async install(plugin: Plugin): Promise<Plugin> {
    validate(plugin);
    const plugins = await this.all();
    if (plugins.some((candidate) => candidate.id === plugin.id)) throw new Error(`Plugin ${plugin.id} is already installed.`);
    plugins.push(plugin);
    await writeFile(this.file, `${JSON.stringify(plugins, null, 2)}\n`, "utf8");
    return plugin;
  }

  public async setEnabled(id: string, enabled: boolean): Promise<Plugin> {
    const plugins = await this.all();
    const plugin = plugins.find((candidate) => candidate.id === id);
    if (!plugin) throw new Error(`Plugin ${id} was not found.`);
    plugin.enabled = enabled;
    await writeFile(this.file, `${JSON.stringify(plugins, null, 2)}\n`, "utf8");
    return plugin;
  }
}

export { validate as validatePlugin };
