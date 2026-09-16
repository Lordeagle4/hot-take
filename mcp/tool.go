package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	frameworktool "github.com/Lordeagle4/hot-take/tool"
)

// RemoteTool adapts one advertised MCP tool to the framework tool contract.
type RemoteTool struct {
	client     *Client
	remoteName string
	definition frameworktool.Definition
}

// NewRemoteTool binds an advertised MCP definition to a collision-resistant
// local name and a set of vendor-neutral capabilities.
func NewRemoteTool(client *Client, namespace string, remote ToolDefinition, capabilities []string) (*RemoteTool, error) {
	if client == nil {
		return nil, fmt.Errorf("MCP client is required")
	}
	name, err := LocalToolName(namespace, remote.Name)
	if err != nil {
		return nil, err
	}
	if len(capabilities) == 0 {
		return nil, fmt.Errorf("MCP tool %q requires at least one capability", remote.Name)
	}
	validated := make([]string, 0, len(capabilities))
	seen := make(map[string]struct{}, len(capabilities))
	for _, capability := range capabilities {
		capability = strings.TrimSpace(capability)
		if capability == "" {
			return nil, fmt.Errorf("MCP tool %q has an empty capability", remote.Name)
		}
		if _, exists := seen[capability]; exists {
			continue
		}
		seen[capability] = struct{}{}
		validated = append(validated, capability)
	}

	return &RemoteTool{
		client:     client,
		remoteName: remote.Name,
		definition: frameworktool.Definition{
			Name:         name,
			Description:  remote.Description,
			InputSchema:  append(json.RawMessage(nil), remote.InputSchema...),
			Capabilities: validated,
			Strict:       false,
		},
	}, nil
}

// LocalToolName returns the deterministic name exposed to model providers.
// Dots in MCP names are replaced because some function-calling providers do
// not accept them; registration still rejects any resulting collision.
func LocalToolName(namespace string, remoteName string) (string, error) {
	if !validNamespace(namespace) {
		return "", fmt.Errorf("invalid MCP namespace %q", namespace)
	}
	if !validToolName(remoteName) {
		return "", fmt.Errorf("invalid MCP tool name %q", remoteName)
	}
	normalize := func(value string) string {
		value = strings.ReplaceAll(value, ".", "_")
		value = strings.ReplaceAll(value, "-", "_")
		return value
	}
	return normalize(namespace) + "__" + normalize(remoteName), nil
}

// Definition returns model-facing metadata for the bound MCP tool.
func (t *RemoteTool) Definition() frameworktool.Definition {
	definition := t.definition
	definition.InputSchema = append(json.RawMessage(nil), definition.InputSchema...)
	definition.Capabilities = append([]string(nil), definition.Capabilities...)
	return definition
}

// Execute invokes the remote MCP tool.
func (t *RemoteTool) Execute(ctx context.Context, arguments json.RawMessage) (json.RawMessage, error) {
	result, err := t.client.CallTool(ctx, t.remoteName, arguments)
	if err != nil {
		return nil, fmt.Errorf("execute remote MCP tool %q: %w", t.remoteName, err)
	}
	return result, nil
}

func validNamespace(value string) bool {
	if value == "" || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '-' || character == '_' {
			continue
		}
		return false
	}
	return true
}
