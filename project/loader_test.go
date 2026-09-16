package project_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Lordeagle4/hot-take/project"
)

func TestLoaderLoadsValidatedProject(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(t, root, "agent.json", `{
  "name": "Base Agent",
  "instructions": "instructions.md",
  "default_skill": "general",
  "skills_directory": "skills",
  "max_steps": 8,
  "provider": {"name": "openai", "model": "gpt-6-astra"}
}`)
	writeFixture(t, root, "instructions.md", "Be accurate and direct.\n")
	writeFixture(t, root, "skills/general/skill.json", `{
  "name": "general",
  "description": "General reasoning.",
  "instructions": "SKILL.md",
  "examples": [],
  "required_capabilities": []
}`)
	writeFixture(t, root, "skills/general/SKILL.md", "Answer the user directly.\n")

	definition, err := (project.Loader{}).Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if definition.Name != "Base Agent" || definition.Provider.Name != "openai" || definition.MaxSteps != 8 {
		t.Fatalf("Load() = %#v", definition)
	}
	selected, err := definition.Skills.Select("Explain emergence")
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if selected.Name != "general" {
		t.Fatalf("Select() = %q", selected.Name)
	}
}

func TestLoaderRejectsUnknownManifestField(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(t, root, "agent.json", `{"name":"Agent","unexpected":true}`)

	_, err := (project.Loader{}).Load(root)
	if !errors.Is(err, project.ErrInvalid) {
		t.Fatalf("Load() error = %v, want ErrInvalid", err)
	}
}

func TestLoaderRejectsEscapingInstructionsPath(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	root := filepath.Join(parent, "project")
	if err := os.Mkdir(root, 0o750); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	writeFixture(t, parent, "outside.md", "outside")
	writeFixture(t, root, "agent.json", `{
  "name": "Agent",
  "instructions": "../outside.md",
  "default_skill": "general",
  "skills_directory": "skills",
  "max_steps": 4,
  "provider": {"name": "openai", "model": "gpt-6-astra"}
}`)

	_, err := (project.Loader{}).Load(root)
	if !errors.Is(err, project.ErrPathEscape) {
		t.Fatalf("Load() error = %v, want ErrPathEscape", err)
	}
}

func TestLoaderLoadsPluginManifest(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(t, root, "agent.json", `{
  "name": "Plugin Agent",
  "instructions": "instructions.md",
  "default_skill": "general",
  "skills_directory": "skills",
  "plugins_directory": "plugins",
  "max_steps": 4,
  "provider": {"name": "openai", "model": "gpt-6-astra"}
}`)
	writeFixture(t, root, "instructions.md", "Use connected tools carefully.\n")
	writeFixture(t, root, "skills/general/skill.json", `{
  "name": "general",
  "description": "General work.",
  "instructions": "SKILL.md",
  "examples": [],
  "required_capabilities": ["repository.search"]
}`)
	writeFixture(t, root, "skills/general/SKILL.md", "Search when needed.\n")
	writeFixture(t, root, "plugins/github/plugin.json", `{
  "name": "github",
  "transport": "mcp_streamable_http",
  "endpoint": "https://example.com/mcp",
  "token_environment": "GITHUB_MCP_TOKEN",
  "approval": "ask",
  "capabilities": [{"name": "repository.search", "tools": ["repositories.search"]}]
}`)

	definition, err := (project.Loader{}).Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(definition.Plugins) != 1 || definition.Plugins[0].Name != "github" {
		t.Fatalf("Plugins = %#v", definition.Plugins)
	}
}

func TestLoaderRejectsPluginSecretValueField(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFixture(t, root, "agent.json", `{
  "name": "Plugin Agent",
  "instructions": "instructions.md",
  "default_skill": "general",
  "skills_directory": "skills",
  "plugins_directory": "plugins",
  "max_steps": 4,
  "provider": {"name": "openai", "model": "gpt-6-astra"}
}`)
	writeFixture(t, root, "instructions.md", "Be careful.\n")
	writeFixture(t, root, "skills/general/skill.json", `{
  "name": "general",
  "description": "General work.",
  "instructions": "SKILL.md",
  "examples": [],
  "required_capabilities": []
}`)
	writeFixture(t, root, "skills/general/SKILL.md", "Answer.\n")
	writeFixture(t, root, "plugins/github/plugin.json", `{
  "name": "github",
  "transport": "mcp_streamable_http",
  "endpoint": "https://example.com/mcp",
  "token": "must-not-be-accepted",
  "approval": "ask",
  "capabilities": [{"name": "repository.search", "tools": ["search"]}]
}`)

	_, err := (project.Loader{}).Load(root)
	if !errors.Is(err, project.ErrInvalid) {
		t.Fatalf("Load() error = %v, want ErrInvalid", err)
	}
}

func writeFixture(t *testing.T, root string, relative string, content string) {
	t.Helper()
	path := filepath.Join(root, relative)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", relative, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", relative, err)
	}
}
