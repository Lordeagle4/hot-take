# Project configuration

An agent project separates configuration from executable tools and provider
adapters. Hot Take uses strict JSON manifests so unknown fields fail fast and
the core remains dependency-free.

## Layout

```text
agent-project/
├── agent.json
├── instructions.md
└── skills/
    └── general/
        ├── skill.json
        └── SKILL.md
```

Configured paths must remain inside the project or skill directory. Absolute
paths, traversal, and symlinks that escape their owning directory are rejected.
Individual project files are limited to 1 MiB.

## Agent manifest

```json
{
  "name": "Base Agent",
  "instructions": "instructions.md",
  "default_skill": "general",
  "skills_directory": "skills",
  "max_steps": 8,
  "provider": {
    "name": "openai",
    "model": "gpt-6-astra"
  }
}
```

- `name` is the human-readable agent name.
- `instructions` points to the base Markdown instructions.
- `default_skill` names the fallback skill directory.
- `skills_directory` contains one directory per skill.
- `max_steps` bounds model turns and prevents infinite tool loops.
- `provider` selects an installed provider adapter and model.

Credentials never belong in this file. The OpenAI adapter reads
`OPENAI_API_KEY` from the environment through the CLI composition root.

## Skill manifest

```json
{
  "name": "clock",
  "description": "Answer questions about the current date and time.",
  "instructions": "SKILL.md",
  "examples": ["time", "date", "day"],
  "required_capabilities": ["clock.read"]
}
```

The directory name and `name` must match. `examples` are routing phrases, not
prompts. `required_capabilities` names semantic operations; it does not identify
vendors or transports.

## Current provider support

The CLI currently composes the `openai` provider and the built-in `clock.read`
capability. The framework contracts allow applications to compose different
providers and tools without changing `agent.Runtime`.
