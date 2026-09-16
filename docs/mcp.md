# MCP support

Hot Take implements the modern, stateless Model Context Protocol revision
`2026-07-28` over Streamable HTTP. It does not silently fall back to legacy
initialisation-based revisions.

## Implemented

- one HTTP POST per JSON-RPC request;
- protocol, method, name, and client metadata on every request;
- JSON and request-scoped Server-Sent Events responses;
- `tools/list` pagination with loop and page limits;
- `tools/call` with complete results and model-visible execution errors;
- `x-mcp-header` validation, nested argument extraction, and safe encoding;
- HTTPS enforcement outside loopback;
- bearer tokens supplied by environment variables;
- bounded response bodies and caller-controlled cancellation;
- namespaced, manifest-allowlisted tool registration.

Tool results with `resultType: "input_required"` currently return
`mcp.ErrInputRequired`. Multi round-trip input requests, subscriptions, OAuth
discovery, resources, prompts, and legacy MCP revisions are deliberately out of
scope for this slice.

## Security model

Server tool metadata is untrusted. Invalid schemas stop discovery; tools with
invalid `x-mcp-header` annotations are excluded as required by the transport
specification. Tool annotations never override local permission policy.

The project manifest stores only an environment variable name, never a token.
Every remote tool receives an exact `allow`, `deny`, or `ask` rule before it can
be executed. Tools advertised by the server but absent from the manifest are
not registered.
