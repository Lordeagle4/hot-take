# Hot Take

Hot Take is a small, explicit framework for building tool-using AI agents in Go.
It provides the runtime and contracts; applications provide the model, skills,
tools, permission policy, and integrations.

The project is intentionally provider-neutral. OpenAI, another hosted model, or
a local model can implement the same `provider.Model` interface without changing
the agent runtime.

## Status

Hot Take **v0.1.0-alpha.1** is a developer alpha. Public APIs and configuration
formats may change. Live hosted-model verification and independent onboarding
are still outstanding; see the [release notes](docs/releases/v0.1.0-alpha.1.md)
for tested behaviour and limitations.

## Installation

Requires Go 1.24 or later:

```sh
go install github.com/Lordeagle4/hot-take/cmd/hot-take@v0.1.0-alpha.1
hot-take --version
hot-take demo "What time is it?"
```

See [installation and starter setup](docs/installation.md). The source-based
quick start below assumes you have cloned the repository.

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

Try the [complete MCP integration walkthrough](docs/integration.md) for a local
read-only plugin, interactive approval, rejection and failure handling.

Runs default to a two-minute deadline and 32 tool calls. Override these with
`run -timeout 90s -max-tool-calls 8`; Ctrl+C cancels an active run.

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
