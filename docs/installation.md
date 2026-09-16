# Install Hot Take alpha

Requires Go 1.24 or later. Git is required only to clone the starter projects.
This release is distributed as Go source, without prebuilt binaries.

## Install the CLI

```sh
go install github.com/Lordeagle4/hot-take/cmd/hot-take@v0.1.0-alpha.1
hot-take --version
hot-take demo "What time is it?"
```

The expected version is `0.1.0-alpha.1`. The demo needs no API key or network
access after installation. Go installs the executable into `GOBIN`, or otherwise
`$(go env GOPATH)/bin`. Add that directory to your PATH if `hot-take` is not found.
On Windows, the executable is `hot-take.exe`.

## Obtain and run the starter

The CLI does not yet generate projects or bundle the starter files. Clone the
same release to obtain them:

```sh
git clone --branch v0.1.0-alpha.1 --depth 1 https://github.com/Lordeagle4/hot-take.git
cd hot-take
```

Set `OPENAI_API_KEY` in your environment; do not put credentials in `agent.json`.
Choose a model available to your API project in `starter/base/agent.json`.

```sh
hot-take run -project ./starter/base "What time is it?"
```

Hosted-model runs incur provider charges. Follow the
[MCP walkthrough](integration.md) to connect the read-only catalogue fixture.

## Build from the tagged source

From the cloned repository:

```sh
go build -o ./bin/hot-take ./cmd/hot-take
./bin/hot-take --version
```

On Windows use `go build -o ./bin/hot-take.exe ./cmd/hot-take` and run
`.\bin\hot-take.exe --version` in PowerShell.

## Use the framework in a Go project

```sh
go get github.com/Lordeagle4/hot-take@v0.1.0-alpha.1
```

Start with [the architecture guide](architecture.md) and the exported Go types.
The CLI is the reference composition of the runtime, provider and tool registry.

## Upgrade policy

Pin the exact alpha version. Public Go APIs and JSON manifests may change in
subsequent alpha releases; compatibility is not guaranteed. Review release
notes before upgrading. There is no automated project migration command.
