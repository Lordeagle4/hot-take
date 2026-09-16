# Reproducible MCP integration

This example connects the real OpenAI adapter, project loader, selected skill,
permission policy and MCP HTTP client to a read-only local catalogue server.
The server is a fixture implementing the two methods this example needs; it is
not a general MCP server or evidence of compatibility with third-party hosts.
See [MCP support](mcp.md) for the client's protocol boundary.

## Interactive run

From the repository root, with Go 1.24 or later:

```sh
go run ./examples/mcp-server
```

In another terminal, set `OPENAI_API_KEY` in your environment. Edit
`examples/mcp-agent/agent.json` to select a model available to your API project.
Then run:

```sh
go run ./cmd/hot-take run -project ./examples/mcp-agent -timeout 90s -max-tool-calls 2 "How many DEMO-001 are in stock?"
```

The CLI discovers `catalogue.lookup`, exposes only that configured tool to the
catalogue skill, and asks for approval before calling it. Enter `yes`. The tool
returns **Field Notebook, 12 units in stock**; the model should report that fact.
This run makes billable model requests. The fixture needs no credentials and
listens only on `127.0.0.1:8787`.

Repeat with `no` or an empty answer. The run must fail with `tool call denied`
without invoking the remote tool. Ctrl+C or the deadline also aborts approval.
A cancelled CLI run exits; it does not resume a partially completed operation.

To exercise transport failure, stop the example server and restart it with:

```sh
go run ./examples/mcp-server -fail-calls
```

Discovery still succeeds, but an approved tool call returns HTTP 503. The CLI
must report the failure and must not claim a successful lookup or retry it.

## Automated regression test

```sh
go test -race ./cmd/hot-take -run '^TestMCPIntegration$' -v
```

This uses the actual example project and MCP fixture with a local scripted
Responses endpoint. It verifies approved execution, zero calls after rejection,
server failure, exposed tool scope, call IDs, tool results and provider
continuation. It makes no external requests and runs in ordinary CI.

For a real hosted-model smoke test, set `OPENAI_API_KEY` and
`HOT_TAKE_LIVE_MODEL` in your environment, then run:

```sh
go test ./cmd/hot-take -run '^TestLiveMCPIntegration$' -v -count=1
```

This opt-in test makes billable API requests and automatically approves only the
local read-only fixture. It requires an actual tool call and a final answer
containing the known product and stock count. Ordinary CI skips it. A passing
scripted regression test must not be reported as a passing live-model test.
