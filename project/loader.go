// Package project loads declarative agent projects from disk.
package project

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Lordeagle4/hot-take/skill"
)

const maxProjectFileSize = 1 << 20

var (
	// ErrInvalid reports malformed project configuration.
	ErrInvalid = errors.New("invalid agent project")
	// ErrPathEscape reports a configured path outside its owning directory.
	ErrPathEscape = errors.New("project path escapes its root")
)

// Provider identifies the model provider configuration selected by a project.
type Provider struct {
	Name  string `json:"name"`
	Model string `json:"model"`
}

// PluginCapability maps a vendor-neutral capability to remote MCP tool names.
type PluginCapability struct {
	Name  string   `json:"name"`
	Tools []string `json:"tools"`
}

// Plugin is a validated external capability provider loaded from a manifest.
// TokenEnvironment names an environment variable; credentials are never read
// or retained by the project loader.
type Plugin struct {
	Name             string             `json:"name"`
	Transport        string             `json:"transport"`
	Endpoint         string             `json:"endpoint"`
	TokenEnvironment string             `json:"token_environment,omitempty"`
	Approval         string             `json:"approval"`
	Capabilities     []PluginCapability `json:"capabilities"`
}

// Definition is a validated, fully loaded agent project.
type Definition struct {
	Name         string
	Instructions string
	MaxSteps     int
	Provider     Provider
	Skills       *skill.Registry
	Plugins      []Plugin
}

// Loader reads agent.json, instructions, and skill definitions from a project.
type Loader struct{}

// Load reads and validates an agent project rooted at directory.
func (Loader) Load(directory string) (*Definition, error) {
	root, err := filepath.Abs(strings.TrimSpace(directory))
	if err != nil {
		return nil, fmt.Errorf("%w: resolve root: %v", ErrInvalid, err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("%w: resolve root symlinks: %v", ErrInvalid, err)
	}

	manifestPath, err := resolveWithin(root, "agent.json")
	if err != nil {
		return nil, err
	}
	var manifest manifest
	if err := readJSON(manifestPath, &manifest); err != nil {
		return nil, fmt.Errorf("%w: load agent.json: %v", ErrInvalid, err)
	}
	if err := validateManifest(manifest); err != nil {
		return nil, err
	}

	instructionsPath, err := resolveWithin(root, manifest.Instructions)
	if err != nil {
		return nil, fmt.Errorf("resolve instructions: %w", err)
	}
	instructions, err := readProjectFile(instructionsPath)
	if err != nil {
		return nil, fmt.Errorf("load instructions: %w", err)
	}
	if strings.TrimSpace(string(instructions)) == "" {
		return nil, fmt.Errorf("%w: instructions file is empty", ErrInvalid)
	}

	skillsPath, err := resolveWithin(root, manifest.SkillsDirectory)
	if err != nil {
		return nil, fmt.Errorf("resolve skills directory: %w", err)
	}
	skills, err := loadSkills(skillsPath)
	if err != nil {
		return nil, err
	}
	registry, err := skill.NewRegistry(manifest.DefaultSkill, skills)
	if err != nil {
		return nil, fmt.Errorf("%w: build skill registry: %v", ErrInvalid, err)
	}
	plugins, err := loadPlugins(root, manifest.PluginsDirectory)
	if err != nil {
		return nil, err
	}

	return &Definition{
		Name:         manifest.Name,
		Instructions: strings.TrimSpace(string(instructions)),
		MaxSteps:     manifest.MaxSteps,
		Provider:     manifest.Provider,
		Skills:       registry,
		Plugins:      plugins,
	}, nil
}

type manifest struct {
	Name            string   `json:"name"`
	Instructions    string   `json:"instructions"`
	DefaultSkill    string   `json:"default_skill"`
	SkillsDirectory string   `json:"skills_directory"`
	PluginsDirectory string  `json:"plugins_directory,omitempty"`
	MaxSteps        int      `json:"max_steps"`
	Provider        Provider `json:"provider"`
}

type skillManifest struct {
	Name                 string   `json:"name"`
	Description          string   `json:"description"`
	Instructions         string   `json:"instructions"`
	Examples             []string `json:"examples"`
	RequiredCapabilities []string `json:"required_capabilities"`
}

func validateManifest(candidate manifest) error {
	if strings.TrimSpace(candidate.Name) == "" {
		return fmt.Errorf("%w: agent name is required", ErrInvalid)
	}
	if strings.TrimSpace(candidate.Instructions) == "" || strings.TrimSpace(candidate.SkillsDirectory) == "" {
		return fmt.Errorf("%w: instructions and skills_directory are required", ErrInvalid)
	}
	if !validID(candidate.DefaultSkill) {
		return fmt.Errorf("%w: default_skill must be a lowercase identifier", ErrInvalid)
	}
	if candidate.MaxSteps < 1 {
		return fmt.Errorf("%w: max_steps must be positive", ErrInvalid)
	}
	if !validID(candidate.Provider.Name) || strings.TrimSpace(candidate.Provider.Model) == "" {
		return fmt.Errorf("%w: provider name and model are required", ErrInvalid)
	}

	return nil
}

func loadPlugins(root string, relativeDirectory string) ([]Plugin, error) {
	if strings.TrimSpace(relativeDirectory) == "" {
		return nil, nil
	}
	directory, err := resolveWithin(root, relativeDirectory)
	if err != nil {
		return nil, fmt.Errorf("resolve plugins directory: %w", err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("load plugins directory: %w", err)
	}
	plugins := make([]Plugin, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if !validID(entry.Name()) {
			return nil, fmt.Errorf("%w: plugin directory %q is not a lowercase identifier", ErrInvalid, entry.Name())
		}
		pluginDirectory, err := resolveWithin(directory, entry.Name())
		if err != nil {
			return nil, fmt.Errorf("resolve plugin %q: %w", entry.Name(), err)
		}
		manifestPath, err := resolveWithin(pluginDirectory, "plugin.json")
		if err != nil {
			return nil, fmt.Errorf("resolve plugin %q manifest: %w", entry.Name(), err)
		}
		var plugin Plugin
		if err := readJSON(manifestPath, &plugin); err != nil {
			return nil, fmt.Errorf("%w: load plugin %q manifest: %v", ErrInvalid, entry.Name(), err)
		}
		if plugin.Name != entry.Name() {
			return nil, fmt.Errorf("%w: plugin directory %q does not match name %q", ErrInvalid, entry.Name(), plugin.Name)
		}
		if err := validatePlugin(plugin); err != nil {
			return nil, err
		}
		if _, exists := seen[plugin.Name]; exists {
			return nil, fmt.Errorf("%w: duplicate plugin %q", ErrInvalid, plugin.Name)
		}
		seen[plugin.Name] = struct{}{}
		plugins = append(plugins, plugin)
	}
	return plugins, nil
}

func validatePlugin(plugin Plugin) error {
	if !validID(plugin.Name) {
		return fmt.Errorf("%w: plugin name %q is not a lowercase identifier", ErrInvalid, plugin.Name)
	}
	if plugin.Transport != "mcp_streamable_http" {
		return fmt.Errorf("%w: plugin %q has unsupported transport %q", ErrInvalid, plugin.Name, plugin.Transport)
	}
	if strings.TrimSpace(plugin.Endpoint) == "" {
		return fmt.Errorf("%w: plugin %q endpoint is required", ErrInvalid, plugin.Name)
	}
	if plugin.TokenEnvironment != "" && !validEnvironmentName(plugin.TokenEnvironment) {
		return fmt.Errorf("%w: plugin %q token_environment is invalid", ErrInvalid, plugin.Name)
	}
	if plugin.Approval != "allow" && plugin.Approval != "deny" && plugin.Approval != "ask" {
		return fmt.Errorf("%w: plugin %q approval must be allow, deny, or ask", ErrInvalid, plugin.Name)
	}
	if len(plugin.Capabilities) == 0 {
		return fmt.Errorf("%w: plugin %q has no capabilities", ErrInvalid, plugin.Name)
	}
	seenCapabilities := make(map[string]struct{}, len(plugin.Capabilities))
	for _, capability := range plugin.Capabilities {
		if !validCapability(capability.Name) {
			return fmt.Errorf("%w: plugin %q capability %q is invalid", ErrInvalid, plugin.Name, capability.Name)
		}
		if _, exists := seenCapabilities[capability.Name]; exists {
			return fmt.Errorf("%w: plugin %q repeats capability %q", ErrInvalid, plugin.Name, capability.Name)
		}
		seenCapabilities[capability.Name] = struct{}{}
		if len(capability.Tools) == 0 {
			return fmt.Errorf("%w: plugin %q capability %q has no tools", ErrInvalid, plugin.Name, capability.Name)
		}
		seenTools := make(map[string]struct{}, len(capability.Tools))
		for _, toolName := range capability.Tools {
			if !validMCPToolName(toolName) {
				return fmt.Errorf("%w: plugin %q has invalid MCP tool name %q", ErrInvalid, plugin.Name, toolName)
			}
			if _, exists := seenTools[toolName]; exists {
				return fmt.Errorf("%w: plugin %q capability %q repeats tool %q", ErrInvalid, plugin.Name, capability.Name, toolName)
			}
			seenTools[toolName] = struct{}{}
		}
	}
	return nil
}

func validEnvironmentName(value string) bool {
	if value == "" || (value[0] < 'A' || value[0] > 'Z') && value[0] != '_' {
		return false
	}
	for _, character := range value {
		if (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || character == '_' {
			continue
		}
		return false
	}
	return true
}

func validCapability(value string) bool {
	parts := strings.Split(value, ".")
	if len(parts) < 2 {
		return false
	}
	for _, part := range parts {
		if !validID(part) {
			return false
		}
	}
	return true
}

func validMCPToolName(value string) bool {
	if len(value) < 1 || len(value) > 128 {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '_' || character == '-' || character == '.' {
			continue
		}
		return false
	}
	return true
}

func loadSkills(directory string) ([]skill.Skill, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("load skills directory: %w", err)
	}
	loaded := make([]skill.Skill, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if !validID(entry.Name()) {
			return nil, fmt.Errorf("%w: skill directory %q is not a lowercase identifier", ErrInvalid, entry.Name())
		}
		skillDirectory, err := resolveWithin(directory, entry.Name())
		if err != nil {
			return nil, fmt.Errorf("resolve skill %q: %w", entry.Name(), err)
		}
		manifestPath, err := resolveWithin(skillDirectory, "skill.json")
		if err != nil {
			return nil, fmt.Errorf("resolve skill %q manifest: %w", entry.Name(), err)
		}
		var metadata skillManifest
		if err := readJSON(manifestPath, &metadata); err != nil {
			return nil, fmt.Errorf("%w: load skill %q manifest: %v", ErrInvalid, entry.Name(), err)
		}
		if metadata.Name != entry.Name() {
			return nil, fmt.Errorf("%w: skill directory %q does not match name %q", ErrInvalid, entry.Name(), metadata.Name)
		}
		instructionsPath, err := resolveWithin(skillDirectory, metadata.Instructions)
		if err != nil {
			return nil, fmt.Errorf("resolve skill %q instructions: %w", entry.Name(), err)
		}
		instructions, err := readProjectFile(instructionsPath)
		if err != nil {
			return nil, fmt.Errorf("load skill %q instructions: %w", entry.Name(), err)
		}
		loaded = append(loaded, skill.Skill{
			Name:                 metadata.Name,
			Description:          metadata.Description,
			Instructions:         strings.TrimSpace(string(instructions)),
			Examples:             metadata.Examples,
			RequiredCapabilities: metadata.RequiredCapabilities,
		})
	}
	if len(loaded) == 0 {
		return nil, fmt.Errorf("%w: no skills were found", ErrInvalid)
	}

	return loaded, nil
}

func readJSON(path string, destination any) error {
	payload, err := readProjectFile(path)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("decode JSON: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("decode JSON: multiple values are not allowed")
		}
		return fmt.Errorf("decode JSON trailing data: %w", err)
	}

	return nil
}

func readProjectFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	payload, readErr := io.ReadAll(io.LimitReader(file, maxProjectFileSize+1))
	closeErr := file.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if len(payload) > maxProjectFileSize {
		return nil, fmt.Errorf("file exceeds %d bytes", maxProjectFileSize)
	}

	return payload, nil
}

func resolveWithin(root string, relative string) (string, error) {
	if strings.TrimSpace(relative) == "" || filepath.IsAbs(relative) {
		return "", fmt.Errorf("%w: %q", ErrPathEscape, relative)
	}
	candidate := filepath.Join(root, filepath.Clean(relative))
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", err
	}
	relativeToRoot, err := filepath.Rel(root, resolved)
	if err != nil {
		return "", err
	}
	if relativeToRoot == ".." || strings.HasPrefix(relativeToRoot, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: %q", ErrPathEscape, relative)
	}

	return resolved, nil
}

func validID(value string) bool {
	if value == "" {
		return false
	}
	if value[0] < 'a' || value[0] > 'z' {
		return false
	}
	for index, character := range value {
		if (index > 0 && (character == '-' || character == '_')) || (character >= 'a' && character <= 'z') || (index > 0 && character >= '0' && character <= '9') {
			continue
		}
		return false
	}

	return true
}
