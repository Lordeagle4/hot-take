# Hot Take

Hot Take is a deliberately transparent AI agent demo. It shows the complete path from a user's intent to a selected skill, scoped plugin, MCP tool discovery, human approval, tool result and final answer.

It is small enough to read in one sitting and real enough to connect to remote MCP servers through OpenAI's Responses API.

## What it demonstrates

- **Skills as procedural knowledge** — Markdown files define when a capability applies, which plugins it may use and how the agent should behave.
- **Plugins as installable access** — add a remote MCP server or an OpenAI connector without changing the runtime.
- **Tools discovered over MCP** — the Responses API imports the enabled server's tool schemas.
- **Approval gates** — MCP calls pause before data leaves the agent and continue only after the user approves.
- **Conversation memory** — `previous_response_id` preserves the response chain and avoids needlessly importing the same MCP tool list again.
- **A visible execution trace** — every routing, discovery, approval, result and answer event is surfaced in the interface.
- **Safe token handling** — access tokens come from `HOT_TAKE_PLUGIN_*` environment variables or live only in process memory; they are never written to `plugins.json`.

## Run it

Hot Take requires Node.js 24 or newer and has no npm dependencies.

```bash
cp .env.example .env
set -a && source .env && set +a
npm start
```

Open <http://localhost:3000>.

Without `OPENAI_API_KEY`, the app starts in clearly labelled **simulation mode**. `Roll 2d6+3 for me` still demonstrates skill routing and the approval interruption. Add an API key to make the Responses API discover and call the public Dice Lab MCP server for real.

Run the tests with:

```bash
npm test
```

## Architecture

```text
Browser
  → HTTP runtime
    → SkillRegistry selects skills/*/SKILL.md
    → PluginRegistry scopes data/plugins.json
    → OpenAIRuntime sends only those MCP definitions
      → Responses API imports tools/list
      → model requests a tool call
      → Hot Take pauses for approval
      → Responses API calls the MCP server
      → model returns the final answer
```

The preinstalled `tabletop` skill names `dice-lab`; therefore Dice Lab is invisible to ordinary questions. That is the distinction this demo is meant to make tangible: a skill tells the runtime **how and when**, while a plugin gives it **access**.

## Install a plugin

Open **Plugins** in the top-right corner. You can install:

1. A remote MCP server using its public HTTPS endpoint.
2. An OpenAI connector using one of the connector IDs offered in the form.

The plugin is intentionally inert until a skill includes its ID in `plugins: [...]`. Add a new directory under `skills/`, create its `SKILL.md`, and name the plugin in that front matter.

Use `allowedTools` to narrow a server's exposed capability surface. Keep approval set to `always` for anything that reads private data, writes, sends, purchases or deletes. Only trusted plugins can be configured to skip approval.

## Current demo boundaries

- Sessions and pasted connection tokens are in memory and disappear on restart.
- Plugin manifests persist in a local JSON file; production should use a database and encrypted credential vault.
- Skill routing is deterministic keyword scoring so the selection is easy to inspect. A production router can combine deterministic policy with model classification.
- OAuth client registration and refresh-token exchange belong in a real authentication service. This demo accepts a short-lived access token or reads one from an environment variable.

## Important security note

Remote MCP tool definitions and results are untrusted input. Connect only to servers whose operator you trust, expose the smallest possible `allowedTools` set, and keep approvals enabled for sensitive actions.
