# Project configuration

An agent project separates configuration from executable tools and provider
adapters. Hot Take uses strict JSON manifests so unknown fields fail fast and
the core remains dependency-free.

## Layout

```text
agent-project/
├── agent.json
├── instructions.md
├── skills/
│   └── general/
│       ├── skill.json
│       └── SKILL.md
└── plugins/
    └── github/
        └── plugin.json
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
  "plugins_directory": "plugins",
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
- `plugins_directory` is optional and contains external provider manifests.
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

## Plugin manifest

```json
{
  "name": "github",
  "transport": "mcp_streamable_http",
  "endpoint": "https://mcp.example.com/mcp",
  "token_environment": "GITHUB_MCP_TOKEN",
  "approval": "ask",
  "capabilities": [
    {
      "name": "repository.search",
      "tools": ["repositories.search"]
    }
  ]
}
```

The directory and `name` must match. `transport` currently accepts only
`mcp_streamable_http`. Remote endpoints must use HTTPS; plain HTTP is accepted
only for loopback development servers. `token_environment` is optional and
names an environment variable containing a bearer token. A secret value in the
manifest is rejected as an unknown field.

`approval` must be `allow`, `deny`, or `ask`. The CLI prompts on standard input
for `ask` calls and defaults to rejection. Applications embedding the framework
can replace the terminal approver.

Capabilities form an explicit allowlist. Hot Take verifies each named tool
against MCP discovery and exposes only configured tools. A remote tool such as
`repositories.search` from plugin `github` is presented to model providers as
`github__repositories_search`; normalisation collisions fail registration.

See [MCP support](mcp.md) for the implemented protocol boundary.

## Current provider support

The CLI composes the `openai` provider, the built-in `clock.read` capability,
and configured MCP plugins. The framework contracts allow applications to
compose different providers and tools without changing `agent.Runtime`.
