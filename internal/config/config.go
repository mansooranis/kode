// Package config loads kode's TOML configuration, following Hunk's
// precedence: project-local .kode/config.toml, then user
// ~/.config/kode/config.toml, then built-in defaults.
package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Theme                 string `toml:"theme"`
	Mode                  string `toml:"mode"` // auto | split | stack
	VCS                   string `toml:"vcs"`  // auto | git | jj | sapling
	Watch                 bool   `toml:"watch"`
	ExcludeUntracked      bool   `toml:"exclude_untracked"`
	LineNumbers           bool   `toml:"line_numbers"`
	TabWidth              int    `toml:"tab_width"`
	WrapLines             bool   `toml:"wrap_lines"`
	MenuBar               bool   `toml:"menu_bar"`
	AgentNotes            bool   `toml:"agent_notes"`
	TransparentBackground bool   `toml:"transparent_background"`
	CheckUpdates          bool   `toml:"check_updates"`

	Keybindings map[string]string `toml:"keybindings"`
	Agent       AgentConfig       `toml:"agent"`
	MCP         MCPConfig         `toml:"mcp"`
	Annotations AnnotationsConfig `toml:"annotations"`
	Export      ExportConfig      `toml:"export"`
}

type AgentConfig struct {
	Enabled            bool                        `toml:"enabled"`
	Provider           string                      `toml:"provider"` // anthropic | openai | ...
	Model              string                      `toml:"model"`
	Effort             string                      `toml:"effort"` // low|medium|high|xhigh|max
	SkillsPath         string                      `toml:"skills_path"`
	AnnotationsEnabled bool                        `toml:"annotations_enabled"`
	DiagramsEnabled    bool                        `toml:"diagrams_enabled"`
	ProviderConfig     map[string]ProviderSettings `toml:"provider"`
}

type ProviderSettings struct {
	APIKeyEnv string `toml:"api_key_env"`
	BaseURL   string `toml:"base_url"`
}

// MCPConfig lists MCP servers kode's embedded agent connects OUT to as a
// client (see internal/agent/mcp), alongside its own native tools.
type MCPConfig struct {
	Servers []MCPServer `toml:"servers"`
}

type MCPServer struct {
	Name      string   `toml:"name"`
	Transport string   `toml:"transport"` // stdio | sse | http
	Command   string   `toml:"command"`
	Args      []string `toml:"args"`
	URL       string   `toml:"url"`
}

// AnnotationsConfig points at the JSON file annotations are persisted to and
// can be pushed into directly (by a human, script, or agent not connected
// live over MCP) — kode picks up new entries there on refresh ("r").
type AnnotationsConfig struct {
	FilePath string `toml:"file"`
}

type ExportConfig struct {
	DefaultFormat string `toml:"default_format"` // markdown | html
	OutputDir     string `toml:"output_dir"`
}

func Default() Config {
	return Config{
		Theme:        "dark",
		Mode:         "auto",
		VCS:          "auto",
		LineNumbers:  true,
		TabWidth:     4,
		MenuBar:      true,
		AgentNotes:   true,
		CheckUpdates: true,
		Agent: AgentConfig{
			Enabled:            true,
			Provider:           "anthropic",
			Model:              "claude-opus-5",
			Effort:             "medium",
			SkillsPath:         "~/.config/kode/skills",
			AnnotationsEnabled: true,
			DiagramsEnabled:    true,
		},
		Annotations: AnnotationsConfig{
			FilePath: ".kode/annotations.json",
		},
		Export: ExportConfig{
			DefaultFormat: "markdown",
			OutputDir:     "./kode-reports",
		},
	}
}

// Load resolves config in precedence order: project-local .kode/config.toml,
// then user ~/.config/kode/config.toml, then defaults. Later files override
// only the keys they set; anything unset keeps the prior value.
func Load() (Config, error) {
	cfg := Default()

	if home, err := os.UserHomeDir(); err == nil {
		userPath := filepath.Join(home, ".config", "kode", "config.toml")
		if err := mergeFile(&cfg, userPath); err != nil {
			return cfg, err
		}
	}

	if err := mergeFile(&cfg, filepath.Join(".kode", "config.toml")); err != nil {
		return cfg, err
	}

	return cfg, nil
}

func mergeFile(cfg *Config, path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	_, err := toml.DecodeFile(path, cfg)
	return err
}
