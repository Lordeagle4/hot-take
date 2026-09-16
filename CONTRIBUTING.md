# Contributing to Hot Take

Thank you for improving Hot Take. The framework is deliberately small, so new
abstractions must earn their place.

## Standards

1. Keep packages focused on one responsibility.
2. Add documentation comments to every exported identifier.
3. Return errors with actionable context; do not silently recover from invalid
   state.
4. Accept dependencies through constructors so production code is testable.
5. Add tests for success, failure, cancellation, and policy boundaries affected
   by a change.
6. Avoid provider-specific types in the framework core.
7. Do not add a dependency when the standard library is sufficient.

## Before submitting

```bash
gofmt -w .
go vet ./...
go test -race ./...
```

Pull requests should explain the problem, the chosen boundary, alternatives
considered, and any compatibility impact.
