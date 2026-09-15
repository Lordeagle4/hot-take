import { readdir, readFile } from "node:fs/promises";
import { join } from "node:path";
import type { Skill } from "./types.ts";

function parseList(value: string | undefined): string[] {
  if (!value) return [];
  const trimmed = value.trim();
  if (!trimmed.startsWith("[") || !trimmed.endsWith("]")) return [];
  return trimmed.slice(1, -1).split(",").map((item) => item.trim()).filter(Boolean);
}

export function parseSkill(id: string, source: string): Skill {
  const match = source.match(/^---\n([\s\S]*?)\n---\n([\s\S]*)$/);
  if (!match) throw new Error(`Skill ${id} has invalid front matter.`);

  const metadata = Object.fromEntries(match[1].split("\n").map((line) => {
    const separator = line.indexOf(":");
    if (separator < 0) return [line.trim(), ""];
    return [line.slice(0, separator).trim(), line.slice(separator + 1).trim()];
  }));

  return {
    id,
    name: metadata.name ?? id,
    description: metadata.description ?? "",
    triggers: parseList(metadata.triggers).map((trigger) => trigger.toLowerCase()),
    plugins: parseList(metadata.plugins),
    instructions: match[2].trim(),
  };
}

export class SkillRegistry {
  private readonly directory: string;

  public constructor(directory: string) {
    this.directory = directory;
  }

  public async all(): Promise<Skill[]> {
    const entries = await readdir(this.directory, { withFileTypes: true });
    const skills = await Promise.all(entries.filter((entry) => entry.isDirectory()).map(async (entry) => {
      const source = await readFile(join(this.directory, entry.name, "SKILL.md"), "utf8");
      return parseSkill(entry.name, source);
    }));
    return skills.sort((left, right) => left.id === "general" ? 1 : right.id === "general" ? -1 : left.name.localeCompare(right.name));
  }

  public async route(message: string): Promise<Skill> {
    const skills = await this.all();
    const normalised = message.toLowerCase();
    const scored = skills.map((skill) => ({
      skill,
      score: skill.triggers.reduce((score, trigger) => score + (normalised.includes(trigger) ? Math.max(2, trigger.length) : 0), 0),
    })).sort((left, right) => right.score - left.score);

    return scored[0].score > 0 ? scored[0].skill : skills.find((skill) => skill.id === "general") ?? skills[0].skill;
  }
}
