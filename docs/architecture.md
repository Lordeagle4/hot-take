# Architecture

Hot Take separates reasoning from effects. A model may request an operation, but
the runtime owns tool resolution, authorisation, execution, and observation.

## Dependency direction

The core packages depend only on small contracts:

```text
application
    |
    v
agent.Runtime ---> provider.Model
    |            tool.Registry
    |            skill.Registry
    |            permission.Authorizer
    +----------> event.Sink
```

Concrete model clients, MCP transports, storage adapters, and application UIs
belong outside `agent`. This prevents any vendor or protocol from becoming a
framework primitive.

Provider continuation data is carried as opaque state. The runtime stores and
returns it without interpretation. This allows the OpenAI adapter to preserve a
Responses chain through `previous_response_id`, including reasoning items that
must not be reconstructed by the framework core.

## Runtime lifecycle

1. Validate the user input.
2. Select one skill deterministically.
3. Resolve the skill's required capabilities to registered tools.
4. Ask the model for the next turn.
5. Authorise every requested tool call.
6. Execute allowed calls and append their results to the transcript.
7. Repeat until the model returns a final response or the step limit is reached.

The step limit is mandatory. It protects applications from accidental infinite
tool loops and unbounded model cost.

## Skills and capabilities

A skill contains instructions, routing examples, and required capabilities. It
does not execute code. A capability is a stable semantic name such as
`clock.read`; one or more tools may provide it.

Skills depend on capabilities rather than providers. An application can replace
one calendar, email, or storage provider without rewriting the skill that uses
it.

## Permissions

Every tool call passes through `permission.Authorizer`. Applications may allow,
deny, or ask a human. The core ships with explicit allow and deny policies only;
interactive approval belongs in an adapter because the framework cannot assume
a terminal, web UI, or background worker.

## Error policy

Configuration and registration errors fail immediately. Runtime errors retain
the failed operation and relevant identifier. The runtime never turns a denied
tool call or provider failure into a fabricated answer.
