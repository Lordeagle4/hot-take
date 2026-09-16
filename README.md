# Hot Take

Hot Take is a small, explicit framework for building tool-using AI agents in Go.
It provides the runtime and contracts; applications provide the model, skills,
tools, permission policy, and integrations.

The project is intentionally provider-neutral. OpenAI, another hosted model, or
a local model can implement the same `provider.Model` interface without changing
the agent runtime.

## Status

Hot Take is pre-alpha. The current slice establishes the public contracts, a
declarative project loader, an OpenAI Responses provider, a modern MCP
Streamable HTTP client, explicit approval policies, and a test-covered agent
loop. Expect breaking changes until the first tagged release.

## Design principles

- Skills contain task-specific judgement and instructions.
- Tools perform concrete operations.
- Capabilities describe what a tool can do without naming a vendor.
- Providers translate the framework's model contract to a model API.
- Permission policies govern every tool call before execution.
- Runtime events make selection and execution observable.

See [the architecture guide](docs/architecture.md) for the dependency rules and
request lifecycle.

## Quick start

Run the deterministic offline demonstration:

```bash
go run ./cmd/hot-take demo "What time is it?"
```

Run the declarative base agent with the OpenAI Responses API:

```bash
export OPENAI_API_KEY="your-project-api-key"
go run ./cmd/hot-take run -project ./starter/base "What time is it?"
```

The CLI reads credentials from the process environment and never from project
configuration. See [the configuration guide](docs/configuration.md) for the
project schema and directory layout.

Run the quality checks:

```bash
gofmt -w .
go vet ./...
go test -race ./...
```

## Repository layout

```text
agent/       orchestration loop and public runtime types
capability/  vendor-neutral capability registry
event/       runtime observability contracts
permission/  tool-call authorisation policies
provider/    model-provider contracts
providers/   concrete model-provider adapters
project/     strict declarative project loading
mcp/         modern MCP Streamable HTTP client and tool adapter
skill/       skill definitions and deterministic routing
starter/     runnable base-agent project
tool/        tool definitions, calls, and registry
tools/       first-party native tools
cmd/         command-line application
docs/        architecture and contributor documentation
```

## Contributing

Read [CONTRIBUTING.md](CONTRIBUTING.md) before opening a pull request. Every
change must be formatted, statically analysed, tested, and documented.

## Licence

Hot Take is available under the [MIT Licence](LICENSE).
