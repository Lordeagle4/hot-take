# Engineering rules

These rules apply to every contribution in this repository.

- Write production-quality code suitable for a public open-source project.
- Keep responsibilities narrow and dependencies explicit.
- Document every exported identifier and every non-obvious invariant.
- Prefer strict, concrete types. Do not encode domain state in unvalidated maps.
- Fail fast on invalid configuration, missing registrations, and malformed tool
  arguments.
- Never discard an error. Wrap it with operation-specific context.
- Keep provider, transport, and storage concerns outside the agent core.
- Add table-driven tests where multiple cases share behaviour.
- Run formatting, static analysis, race detection, tests, and builds before
  merging.
- Do not commit secrets, generated binaries, coverage files, or local state.
